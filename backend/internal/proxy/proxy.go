// Package proxy is the built-in reverse proxy: an embedded Caddy server whose
// Caddyfile is generated from the domains in the database, plus whatever raw
// Caddyfile users add per domain or globally. It also keeps per-domain traffic
// counters and feeds the request log.
package proxy

import (
	"cmp"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	_ "github.com/caddyserver/caddy/v2/modules/standard"

	"moonshed/internal/config"
	"moonshed/internal/metrics"
	"moonshed/internal/requestlog"
	"moonshed/internal/store"
)

const (
	SettingEnabled        = "proxy_enabled"
	SettingACMEEmail      = "acme_email"
	SettingPanelHost      = "panel_domain"
	SettingPanelTLS       = "panel_tls"
	SettingTrustedProxies = "proxy_trusted_proxies" // space separated
	SettingGlobalOptions  = "proxy_global_options"
	SettingExtraSites     = "proxy_extra_sites"
)

type Proxy struct {
	store     *store.Store
	paths     config.Paths
	httpAddr  string
	httpsAddr string
	panel     http.Handler
	requests  *requestlog.Log
	traffic   sync.Map

	// certs counts certificate uploads and removals. A changed certificate keeps
	// its file path, so the config only looks different to Caddy through this.
	certs atomic.Int64

	mu         sync.Mutex
	running    bool
	lastErr    string
	loadedCert int64
}

type Status struct {
	Enabled   bool   `json:"enabled"`
	Listening bool   `json:"listening"`
	HTTPAddr  string `json:"http_addr"`
	HTTPSAddr string `json:"https_addr"`
	Error     string `json:"error,omitempty"`
}

func New(st *store.Store, paths config.Paths, cfg config.Config, panel http.Handler, requests *requestlog.Log) *Proxy {
	p := &Proxy{
		requests:  requests,
		store:     st,
		paths:     paths,
		httpAddr:  cfg.HTTPAddr,
		httpsAddr: cfg.HTTPSAddr,
		panel:     panel,
	}
	current.Store(p)
	return p
}

// Input reads everything the Caddyfile is generated from.
func (p *Proxy) Input() (*Input, error) {
	settings, err := p.store.Settings()
	if err != nil {
		return nil, err
	}
	domains, err := p.store.Domains("")
	if err != nil {
		return nil, err
	}
	apps, err := p.store.Apps()
	if err != nil {
		return nil, err
	}
	ports := map[string]int{}
	for _, a := range apps {
		ports[a.ID] = a.Port
	}
	return &Input{
		HTTPAddr:       p.httpAddr,
		HTTPSAddr:      p.httpsAddr,
		Storage:        filepath.Join(p.paths.Certs(), "acme"),
		Email:          settings[SettingACMEEmail],
		PanelHost:      normalizeHost(settings[SettingPanelHost]),
		PanelTLS:       cmp.Or(settings[SettingPanelTLS], store.TLSNone),
		Domains:        domains,
		Ports:          ports,
		CertFiles:      func(host string) (string, string) { return p.certFile(host), p.keyFile(host) },
		TrustedProxies: strings.Fields(settings[SettingTrustedProxies]),
		GlobalOptions:  settings[SettingGlobalOptions],
		ExtraSites:     settings[SettingExtraSites],
	}, nil
}

// Reload regenerates the config and applies it, or stops the proxy when it is turned off.
// Caddy swaps configs gracefully, but a swap closes idle keep-alive connections and can
// drop a connection that arrives at that instant, so an unchanged config is not reloaded.
// If the config only fails because of raw Caddyfile users wrote, the proxy runs
// without it and Status reports the error.
func (p *Proxy) Reload(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	enabled, err := p.store.Setting(SettingEnabled)
	if err != nil {
		return err
	}
	if enabled != "true" {
		p.stop()
		p.lastErr = ""
		return nil
	}
	in, err := p.Input()
	if err != nil {
		return err
	}
	certs := p.certs.Load()
	force := !p.running || certs != p.loadedCert
	err = load(in, force)
	if err != nil {
		safe := *in
		safe.SafeMode = true
		if load(&safe, force) == nil {
			p.running, p.loadedCert = true, certs
			p.lastErr = "Running without the advanced config, which failed: " + err.Error()
			slog.Error("proxy: advanced config failed, running without it", "err", err)
			return nil
		}
		p.lastErr = err.Error()
		return fmt.Errorf("proxy: %w", err)
	}
	p.running, p.loadedCert = true, certs
	p.lastErr = ""
	return nil
}

// load applies in. Caddy skips configs identical to the running one unless forced.
func load(in *Input, force bool) error {
	cfg, err := adapt(generate(in))
	if err != nil {
		return err
	}
	return caddy.Load(cfg, force)
}

// adapt turns Caddyfile text into Caddy's JSON config.
func adapt(text string) ([]byte, error) {
	adapter := caddyfile.Adapter{ServerType: httpcaddyfile.ServerType{}}
	cfg, _, err := adapter.Adapt([]byte(text), map[string]any{"filename": "Caddyfile"})
	return cfg, err
}

// Check builds the config with edit applied (a domain or setting not saved yet) and
// validates it without running it. It returns the generated Caddyfile either way.
func (p *Proxy) Check(edit func(*Input)) (string, error) {
	in, err := p.Input()
	if err != nil {
		return "", err
	}
	if edit != nil {
		edit(in)
	}
	text := generate(in)
	raw, err := adapt(text)
	if err != nil {
		return text, err
	}
	var cfg caddy.Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return text, err
	}
	return text, caddy.Validate(&cfg)
}

// Caddyfile returns the config generated from the current database state.
func (p *Proxy) Caddyfile() (string, error) {
	in, err := p.Input()
	if err != nil {
		return "", err
	}
	return generate(in), nil
}

func (p *Proxy) Status() Status {
	p.mu.Lock()
	defer p.mu.Unlock()
	enabled, _ := p.store.Setting(SettingEnabled)
	return Status{
		Enabled:   enabled == "true",
		Listening: p.running,
		HTTPAddr:  p.httpAddr,
		HTTPSAddr: p.httpsAddr,
		Error:     p.lastErr,
	}
}

func (p *Proxy) stop() {
	if !p.running {
		return
	}
	if err := caddy.Stop(); err != nil {
		slog.Error("proxy stop", "err", err)
	}
	p.running = false
}

func (p *Proxy) Shutdown() {
	p.mu.Lock()
	p.stop()
	p.mu.Unlock()
}

type counters struct {
	requests, s4xx, s5xx, latency atomic.Int64
}

func (p *Proxy) count(host string, status int, d time.Duration) {
	v, _ := p.traffic.LoadOrStore(host, &counters{})
	c := v.(*counters)
	c.requests.Add(1)
	c.latency.Add(int64(d))
	switch {
	case status >= 500:
		c.s5xx.Add(1)
	case status >= 400:
		c.s4xx.Add(1)
	}
}

// Drain returns traffic counted since the previous call and resets the counters.
func (p *Proxy) Drain() map[string]metrics.Traffic {
	out := map[string]metrics.Traffic{}
	p.traffic.Range(func(k, v any) bool {
		c := v.(*counters)
		out[k.(string)] = metrics.Traffic{
			Requests:  c.requests.Swap(0),
			Status4xx: c.s4xx.Swap(0),
			Status5xx: c.s5xx.Swap(0),
			Latency:   time.Duration(c.latency.Swap(0)),
		}
		return true
	})
	return out
}

func normalizeHost(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

// NormalizeHost is exported for validating domains entered in the UI.
func NormalizeHost(host string) string { return normalizeHost(host) }

func (p *Proxy) certFile(host string) string {
	return filepath.Join(p.paths.Certs(), "custom", host+".crt")
}

func (p *Proxy) keyFile(host string) string {
	return filepath.Join(p.paths.Certs(), "custom", host+".key")
}

// SaveCertificate validates and stores an uploaded certificate for host.
// Certificates are per host: every path on that host is served with it.
func (p *Proxy) SaveCertificate(host string, certPEM, keyPEM []byte) error {
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return fmt.Errorf("invalid certificate: %w", err)
	}
	if err := pair.Leaf.VerifyHostname(host); err != nil {
		return fmt.Errorf("certificate is not valid for %s (it covers %s)", host, strings.Join(names(pair.Leaf), ", "))
	}
	if time.Now().After(pair.Leaf.NotAfter) {
		return fmt.Errorf("certificate expired on %s", pair.Leaf.NotAfter.Format(time.DateOnly))
	}
	if err := os.MkdirAll(filepath.Dir(p.certFile(host)), 0o700); err != nil {
		return err
	}
	defer p.certs.Add(1)
	if err := os.WriteFile(p.certFile(host), certPEM, 0o644); err != nil {
		return err
	}
	return os.WriteFile(p.keyFile(host), keyPEM, 0o600)
}

func (p *Proxy) HasCertificate(host string) bool {
	_, err := os.Stat(p.certFile(host))
	return err == nil
}

// CertificateCovers reports whether the uploaded certificate for host is also valid for name.
func (p *Proxy) CertificateCovers(host, name string) bool {
	pair, err := tls.LoadX509KeyPair(p.certFile(host), p.keyFile(host))
	return err == nil && pair.Leaf != nil && pair.Leaf.VerifyHostname(name) == nil
}

func (p *Proxy) DeleteCertificate(host string) {
	os.Remove(p.certFile(host))
	os.Remove(p.keyFile(host))
	p.certs.Add(1)
}

type CertInfo struct {
	Names    []string  `json:"names"`
	Issuer   string    `json:"issuer"`
	NotAfter time.Time `json:"not_after"`
}

// Certificate describes the uploaded certificate for host, or nil if there is none.
func (p *Proxy) Certificate(host string) *CertInfo {
	pair, err := tls.LoadX509KeyPair(p.certFile(host), p.keyFile(host))
	if err != nil || pair.Leaf == nil {
		return nil
	}
	return &CertInfo{
		Names:    names(pair.Leaf),
		Issuer:   cmp.Or(pair.Leaf.Issuer.CommonName, pair.Leaf.Issuer.String()),
		NotAfter: pair.Leaf.NotAfter,
	}
}

func names(c *x509.Certificate) []string {
	if len(c.DNSNames) > 0 {
		return c.DNSNames
	}
	return []string{c.Subject.CommonName}
}

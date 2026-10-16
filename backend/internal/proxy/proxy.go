// Package proxy is the built-in reverse proxy: host/path routing to apps, HTTPS via
// Let's Encrypt or uploaded certificates, and per-domain traffic counters.
package proxy

import (
	"cmp"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/caddyserver/certmagic"

	"moonshed/internal/config"
	"moonshed/internal/metrics"
	"moonshed/internal/requestlog"
	"moonshed/internal/store"
)

const (
	SettingEnabled    = "proxy_enabled"
	SettingACMEEmail  = "acme_email"
	SettingPanelHost  = "panel_domain"
	SettingPanelTLS   = "panel_tls"
	acmeTLSALPNProto  = "acme-tls/1"
	readHeaderTimeout = 10 * time.Second
)

type Proxy struct {
	store     *store.Store
	paths     config.Paths
	httpAddr  string
	httpsAddr string
	panel     http.Handler
	requests  *requestlog.Log

	table   atomic.Pointer[table]
	custom  atomic.Pointer[map[string]*tls.Certificate]
	issuer  atomic.Pointer[certmagic.ACMEIssuer]
	magic   *certmagic.Config
	cache   *certmagic.Cache
	traffic sync.Map

	mu      sync.Mutex
	email   string
	managed map[string]bool
	servers []*http.Server
	lastErr string
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
		managed:   map[string]bool{},
	}
	p.table.Store(&table{})
	p.custom.Store(&map[string]*tls.Certificate{})
	certmagic.Default.Logger = logger
	p.cache = certmagic.NewCache(certmagic.CacheOptions{
		GetConfigForCert: func(certmagic.Certificate) (*certmagic.Config, error) { return p.magic, nil },
		Logger:           logger,
	})
	p.magic = certmagic.New(p.cache, certmagic.Config{
		Storage: &certmagic.FileStorage{Path: filepath.Join(paths.Certs(), "acme")},
		Logger:  logger,
	})
	p.setIssuer("")
	return p
}

func (p *Proxy) setIssuer(email string) {
	iss := certmagic.NewACMEIssuer(p.magic, certmagic.ACMEIssuer{
		CA:     certmagic.LetsEncryptProductionCA,
		Email:  email,
		Agreed: true,
		Logger: logger,
	})
	p.magic.Issuers = []certmagic.Issuer{iss}
	p.issuer.Store(iss)
}

// Reload rebuilds the routing table from the database and starts or stops the listeners.
func (p *Proxy) Reload(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	settings, err := p.store.Settings()
	if err != nil {
		return err
	}
	domains, err := p.store.Domains("")
	if err != nil {
		return err
	}
	apps, err := p.store.Apps()
	if err != nil {
		return err
	}
	ports := map[string]int{}
	for _, a := range apps {
		ports[a.ID] = a.Port
	}

	t := &table{
		hosts:     map[string][]*route{},
		panelHost: normalizeHost(settings[SettingPanelHost]),
		panelTLS:  cmp.Or(settings[SettingPanelTLS], store.TLSNone),
	}
	tlsHosts := map[string]string{}
	if t.panelHost != "" {
		tlsHosts[t.panelHost] = t.panelTLS
	}
	for _, d := range domains {
		port := ports[d.AppID]
		if port == 0 {
			continue
		}
		t.hosts[d.Host] = append(t.hosts[d.Host], &route{appID: d.AppID, path: d.Path, port: port, tls: d.TLS, proxy: reverseProxy(port)})
		if d.TLS != store.TLSNone {
			tlsHosts[d.Host] = d.TLS
		}
	}
	for _, routes := range t.hosts {
		slices.SortFunc(routes, func(a, b *route) int { return len(b.path) - len(a.path) })
	}
	p.table.Store(t)

	custom := map[string]*tls.Certificate{}
	var acme []string
	for host, mode := range tlsHosts {
		switch mode {
		case store.TLSCustom:
			cert, err := tls.LoadX509KeyPair(p.certFile(host), p.keyFile(host))
			if err != nil {
				slog.Warn("custom certificate", "host", host, "err", err)
				continue
			}
			custom[host] = &cert
		case store.TLSLetsEncrypt:
			acme = append(acme, host)
		}
	}
	p.custom.Store(&custom)

	enabled := settings[SettingEnabled] == "true"
	if !enabled {
		p.stopServers()
		return nil
	}
	if err := p.startServers(); err != nil {
		return err
	}
	if email := settings[SettingACMEEmail]; email != p.email {
		p.email = email
		p.setIssuer(email)
	}
	var fresh []string
	for _, host := range acme {
		if !p.managed[host] {
			fresh = append(fresh, host)
		}
	}
	if len(fresh) > 0 {
		if err := p.magic.ManageAsync(context.WithoutCancel(ctx), fresh); err != nil {
			return fmt.Errorf("let's encrypt: %w", err)
		}
		for _, host := range fresh {
			p.managed[host] = true
		}
	}
	return nil
}

func (p *Proxy) Status() Status {
	p.mu.Lock()
	defer p.mu.Unlock()
	enabled, _ := p.store.Setting(SettingEnabled)
	return Status{
		Enabled:   enabled == "true",
		Listening: len(p.servers) > 0,
		HTTPAddr:  p.httpAddr,
		HTTPSAddr: p.httpsAddr,
		Error:     p.lastErr,
	}
}

func (p *Proxy) startServers() error {
	if len(p.servers) > 0 {
		return nil
	}
	httpLn, err := net.Listen("tcp", p.httpAddr)
	if err != nil {
		p.lastErr = err.Error()
		return fmt.Errorf("proxy: %w", err)
	}
	httpsLn, err := net.Listen("tcp", p.httpsAddr)
	if err != nil {
		httpLn.Close()
		p.lastErr = err.Error()
		return fmt.Errorf("proxy: %w", err)
	}
	p.lastErr = ""

	httpSrv := &http.Server{Handler: http.HandlerFunc(p.serveHTTP), ReadHeaderTimeout: readHeaderTimeout}
	httpsSrv := &http.Server{
		Handler:           http.HandlerFunc(p.serve),
		ReadHeaderTimeout: readHeaderTimeout,
		TLSConfig: &tls.Config{
			GetCertificate: p.getCertificate,
			NextProtos:     []string{"h2", "http/1.1", acmeTLSALPNProto},
			MinVersion:     tls.VersionTLS12,
		},
	}
	go func() { logServeErr(httpSrv.Serve(httpLn)) }()
	go func() { logServeErr(httpsSrv.ServeTLS(httpsLn, "", "")) }()
	p.servers = []*http.Server{httpSrv, httpsSrv}
	slog.Info("proxy listening", "http", p.httpAddr, "https", p.httpsAddr)
	return nil
}

func logServeErr(err error) {
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("proxy server", "err", err)
	}
}

func (p *Proxy) stopServers() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, srv := range p.servers {
		srv.Shutdown(ctx)
	}
	p.servers = nil
}

func (p *Proxy) Shutdown() {
	p.mu.Lock()
	p.stopServers()
	p.mu.Unlock()
	p.cache.Stop()
}

func (p *Proxy) getCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if cert := (*p.custom.Load())[normalizeHost(hello.ServerName)]; cert != nil {
		return cert, nil
	}
	return p.magic.GetCertificate(hello)
}

func (p *Proxy) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if iss := p.issuer.Load(); iss != nil && iss.HandleHTTPChallenge(w, r) {
		return
	}
	p.serve(w, r)
}

func (p *Proxy) serve(w http.ResponseWriter, r *http.Request) {
	host := normalizeHost(r.Host)
	t := p.table.Load()
	if host != "" && host == t.panelHost {
		if r.TLS == nil && t.panelTLS != store.TLSNone {
			p.redirect(w, r, host)
			return
		}
		p.panel.ServeHTTP(w, r)
		return
	}
	rt := t.match(host, r.URL.Path)
	if rt == nil {
		http.Error(w, "No app is configured for this domain.", http.StatusNotFound)
		return
	}
	if r.TLS == nil && rt.tls != store.TLSNone {
		p.redirect(w, r, host)
		return
	}
	start := time.Now()
	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	rt.proxy.ServeHTTP(rec, r)
	took := time.Since(start)
	p.count(host, rec.status, took)
	if p.requests.Recording(rt.appID) {
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		p.requests.Record(requestlog.Entry{
			AppID:      rt.appID,
			Time:       start.UnixMilli(),
			Method:     r.Method,
			Host:       host,
			Path:       r.URL.Path,
			Query:      r.URL.RawQuery,
			Status:     rec.status,
			DurationMS: float64(took.Microseconds()) / 1000,
			Bytes:      rec.bytes,
			IP:         ip,
			UserAgent:  r.UserAgent(),
			Referer:    r.Referer(),
		})
	}
}

func (p *Proxy) redirect(w http.ResponseWriter, r *http.Request, host string) {
	if _, port, err := net.SplitHostPort(p.httpsAddr); err == nil && port != "443" {
		host = net.JoinHostPort(host, port)
	}
	http.Redirect(w, r, "https://"+host+r.URL.RequestURI(), http.StatusPermanentRedirect)
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

type table struct {
	hosts     map[string][]*route
	panelHost string
	panelTLS  string
}

type route struct {
	appID string
	path  string
	port  int
	tls   string
	proxy *httputil.ReverseProxy
}

func (t *table) match(host, path string) *route {
	for _, r := range t.hosts[host] {
		prefix := strings.TrimSuffix(r.path, "/")
		if prefix == "" || path == prefix || strings.HasPrefix(path, prefix+"/") {
			return r
		}
	}
	return nil
}

func reverseProxy(port int) *httputil.ReverseProxy {
	target := &url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(port))}
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.SetXForwarded()
			pr.Out.Host = pr.In.Host
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			slog.Debug("proxy upstream", "port", port, "err", err)
			http.Error(w, "The app is not responding.", http.StatusBadGateway)
		},
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.bytes += int64(n)
	return n, err
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

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
	if err := os.WriteFile(p.certFile(host), certPEM, 0o644); err != nil {
		return err
	}
	return os.WriteFile(p.keyFile(host), keyPEM, 0o600)
}

func (p *Proxy) HasCertificate(host string) bool {
	_, err := os.Stat(p.certFile(host))
	return err == nil
}

func (p *Proxy) DeleteCertificate(host string) {
	os.Remove(p.certFile(host))
	os.Remove(p.keyFile(host))
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

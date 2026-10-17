package proxy

import (
	"cmp"
	"fmt"
	"net"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"moonshed/internal/store"
)

const (
	noAppText      = "No app is configured for this domain."
	notRespondText = "The app is not responding."
)

// Input is everything the generated Caddyfile depends on.
type Input struct {
	HTTPAddr  string
	HTTPSAddr string
	Storage   string // ACME account and certificate storage
	Email     string

	PanelHost string
	PanelTLS  string

	Domains []*store.Domain
	Ports   map[string]int // app id → port; apps without a port are skipped

	// CertFiles returns the uploaded certificate and key for a host.
	CertFiles func(host string) (cert, key string)

	TrustedProxies []string // CIDRs, or "private_ranges"
	GlobalOptions  string   // raw lines inside the global options block
	ExtraSites     string   // raw site blocks appended to the file

	// SafeMode leaves out everything users typed as raw Caddyfile, so a bad
	// snippet can't keep the apps offline.
	SafeMode bool
}

type writer struct {
	strings.Builder
	depth int
}

func (c *writer) line(format string, args ...any) {
	c.WriteString(strings.Repeat("\t", c.depth))
	fmt.Fprintf(c, format, args...)
	c.WriteByte('\n')
}

func (c *writer) open(format string, args ...any) {
	c.line(format+" {", args...)
	c.depth++
}

func (c *writer) close() {
	c.depth--
	c.line("}")
}

// raw writes user text at the current depth.
func (c *writer) raw(text string) {
	for l := range strings.SplitSeq(strings.TrimRight(text, " \t\r\n"), "\n") {
		if l = strings.TrimRight(l, " \t\r"); l == "" {
			c.WriteByte('\n')
			continue
		}
		c.line("%s", l)
	}
}

// quote makes s a single Caddyfile token. Placeholders like {remote_host} still work.
func quote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

// generate renders the full Caddyfile for in. The output is deterministic.
func generate(in *Input) string {
	var c writer
	globalOptions(&c, in)

	hosts := map[string][]*store.Domain{}
	for _, d := range in.Domains {
		if in.Ports[d.AppID] != 0 {
			hosts[d.Host] = append(hosts[d.Host], d)
		}
	}
	names := make([]string, 0, len(hosts))
	for h := range hosts {
		names = append(names, h)
	}
	slices.Sort(names)

	for _, host := range names {
		c.WriteByte('\n')
		site(&c, in, host, hosts[host])
	}
	if in.PanelHost != "" {
		c.WriteByte('\n')
		c.open("%s", siteAddress(in.PanelHost, in.PanelTLS, true))
		tlsDirective(&c, in, in.PanelHost, in.PanelTLS)
		c.line("moonshed_panel")
		c.close()
		if in.PanelTLS != store.TLSNone {
			httpsRedirect(&c, in, in.PanelHost, in.PanelHost)
		}
	}

	c.WriteByte('\n')
	c.open("http://")
	c.line("respond %s 404", quote(noAppText))
	c.close()

	if !in.SafeMode && strings.TrimSpace(in.ExtraSites) != "" {
		c.WriteByte('\n')
		c.line("# Extra sites (Settings › Proxy)")
		c.raw(in.ExtraSites)
	}
	return c.String()
}

func globalOptions(c *writer, in *Input) {
	c.open("")
	c.line("admin off")
	c.line("persist_config off")
	c.line("grace_period 10s")
	httpHost, httpPort := splitAddr(in.HTTPAddr, "80")
	httpsHost, httpsPort := splitAddr(in.HTTPSAddr, "443")
	c.line("http_port %s", httpPort)
	c.line("https_port %s", httpsPort)
	var binds []string
	for _, h := range []string{httpHost, httpsHost} {
		if h != "" && !slices.Contains(binds, h) {
			binds = append(binds, h)
		}
	}
	if len(binds) > 0 {
		c.line("default_bind %s", strings.Join(binds, " "))
	}
	c.line("storage file_system %s", quote(filepath.ToSlash(in.Storage)))
	c.line("cert_issuer acme")
	if in.Email != "" {
		c.line("email %s", quote(in.Email))
	}
	c.open("log")
	c.line("output moonshed")
	c.line("format json")
	c.close()
	user := ""
	if !in.SafeMode {
		user = strings.TrimSpace(in.GlobalOptions)
	}
	// Caddy allows one servers block per listener, so trusted proxies join the
	// user's own servers block when there is one.
	if len(in.TrustedProxies) > 0 {
		trusted := "trusted_proxies static " + strings.Join(in.TrustedProxies, " ")
		if loc := serversBlock.FindStringIndex(user); loc != nil {
			user = user[:loc[1]] + "\n\t" + trusted + user[loc[1]:]
		} else {
			c.open("servers")
			c.line("%s", trusted)
			c.close()
		}
	}
	if user != "" {
		c.line("# Global options (Settings › Proxy)")
		c.raw(user)
	}
	c.close()
}

// serversBlock finds the opening of a servers block that applies to every listener.
var serversBlock = regexp.MustCompile(`(?m)^[ \t]*servers[ \t]*\{`)

// httpsRedirect answers plain HTTP for host with a permanent redirect to HTTPS on target.
// Caddy can do this itself, but only when its listener addresses match exactly, and
// spelling it out keeps the generated file honest about what happens.
func httpsRedirect(c *writer, in *Input, host, target string) {
	c.WriteByte('\n')
	c.open("http://%s", host)
	c.line("redir https://%s{uri} 308", withPort(target, in.HTTPSAddr, "443"))
	c.close()
}

// splitAddr returns the bind host and port of a listen address like ":80" or "10.0.0.1:80".
func splitAddr(addr, port string) (string, string) {
	h, p, err := net.SplitHostPort(addr)
	if err != nil {
		return "", port
	}
	return h, cmp.Or(p, port)
}

func siteAddress(host, tls string, forceHTTPS bool) string {
	switch {
	case tls == store.TLSNone:
		return "http://" + host
	case forceHTTPS:
		return host
	default:
		return host + ", http://" + host
	}
}

func tlsDirective(c *writer, in *Input, host, tls string) {
	if tls == store.TLSCustom {
		cert, key := in.CertFiles(host)
		c.line("tls %s %s", quote(filepath.ToSlash(cert)), quote(filepath.ToSlash(key)))
	}
}

// site renders one host. Every path on a host shares the TLS mode and the host-wide options.
func site(c *writer, in *Input, host string, rows []*store.Domain) {
	// Longest path first, so /api/v2 wins over /api and / comes last.
	rows = slices.Clone(rows)
	slices.SortStableFunc(rows, func(a, b *store.Domain) int { return len(b.Path) - len(a.Path) })
	first := rows[0]
	tls, opts := first.TLS, first.Options

	c.open("%s", siteAddress(host, tls, opts.ForceHTTPS))
	tlsDirective(c, in, host, tls)
	if opts.HSTS && tls != store.TLSNone {
		c.line("header Strict-Transport-Security %s", quote("max-age=31536000"))
	}
	for i, d := range rows {
		paths := ""
		if d.Path != "/" {
			paths = d.Path + " " + d.Path + "/*"
			c.line("@path%d path %s", i, paths)
		}
		if len(d.Options.AllowIPs) > 0 {
			c.open("@denied%d", i)
			if paths != "" {
				c.line("path %s", paths)
			}
			c.line("not client_ip %s", strings.Join(d.Options.AllowIPs, " "))
			c.close()
		}
	}
	// Inside route, handle blocks keep this order and only the first match runs.
	// A denied handle sits right before its path so it wins over basic_auth.
	c.open("route")
	for i, d := range rows {
		if len(d.Options.AllowIPs) > 0 {
			c.open("handle @denied%d", i)
			c.line("moonshed_track %s", d.AppID)
			c.line("respond %s 403", quote("Forbidden"))
			c.close()
		}
		if d.Path == "/" {
			c.open("handle")
		} else {
			c.open("handle @path%d", i)
		}
		handle(c, in, d)
		c.close()
	}
	c.close()
	c.line("respond %s 404", quote(noAppText))
	c.open("handle_errors 502 503 504")
	c.line("respond %s {err.status_code}", quote(notRespondText))
	c.close()
	c.close()

	if tls != store.TLSNone && opts.ForceHTTPS {
		httpsRedirect(c, in, host, host)
	}
	if opts.RedirectWWW && !strings.HasPrefix(host, "www.") {
		www := "www." + host
		c.WriteByte('\n')
		if tls == store.TLSNone {
			c.open("http://%s", www)
			c.line("redir http://%s{uri} 308", withPort(host, in.HTTPAddr, "80"))
			c.close()
			return
		}
		c.open("%s", www)
		tlsDirective(c, in, host, tls)
		c.line("redir https://%s{uri} 308", withPort(host, in.HTTPSAddr, "443"))
		c.close()
		httpsRedirect(c, in, www, host)
	}
}

// withPort adds the port of the listen address addr to host unless it is the standard one.
func withPort(host, addr, standard string) string {
	if _, port := splitAddr(addr, standard); port != standard {
		return net.JoinHostPort(host, port)
	}
	return host
}

func handle(c *writer, in *Input, d *store.Domain) {
	o := d.Options
	c.line("moonshed_track %s", d.AppID)
	if o.StripPrefix && d.Path != "/" {
		c.line("uri strip_prefix %s", d.Path)
	}
	if len(o.BasicAuth) > 0 {
		c.open("basic_auth")
		for _, u := range o.BasicAuth {
			c.line("%s %s", quote(u.User), quote(u.Hash))
		}
		c.close()
	}
	if o.MaxBody != "" {
		c.open("request_body")
		c.line("max_size %s", o.MaxBody)
		c.close()
	}
	for _, h := range o.RequestHeaders {
		c.line("request_header %s", headerArgs(h))
	}
	for _, h := range o.ResponseHeaders {
		c.line("header %s", headerArgs(h))
	}
	if o.Compress {
		c.line("encode zstd gzip")
	}
	if !in.SafeMode && strings.TrimSpace(d.Advanced) != "" {
		c.line("# Advanced")
		c.raw(d.Advanced)
	}
	c.line("reverse_proxy 127.0.0.1:%d", in.Ports[d.AppID])
}

func headerArgs(h store.HeaderRule) string {
	switch h.Op {
	case "add":
		return "+" + h.Name + " " + quote(h.Value)
	case "remove":
		return "-" + h.Name
	default:
		return h.Name + " " + quote(h.Value)
	}
}

// Balanced reports whether the braces in raw Caddyfile open and close in order,
// so a snippet can't close the block it is placed in.
func Balanced(text string) bool {
	depth := 0
	for _, r := range text {
		switch r {
		case '{':
			depth++
		case '}':
			if depth--; depth < 0 {
				return false
			}
		}
	}
	return depth == 0
}

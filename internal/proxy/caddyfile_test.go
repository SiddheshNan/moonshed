package proxy

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
	"golang.org/x/crypto/bcrypt"

	"github.com/SiddheshNan/moonshed/internal/requestlog"
	"github.com/SiddheshNan/moonshed/internal/store"
)

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// echo is an upstream that reports the path it received.
func echo(t *testing.T, name string) int {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Upstream", name)
		if r.Header.Get("Upgrade") == "websocket" {
			conn, buf, err := http.NewResponseController(w).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			defer conn.Close()
			buf.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
			buf.Flush()
			line, _ := buf.ReadString('\n')
			buf.WriteString(line)
			buf.Flush()
			return
		}
		if r.URL.Path == "/big" {
			w.Write([]byte(strings.Repeat("compress me ", 200)))
			return
		}
		fmt.Fprintf(w, "%s %s %s", name, r.URL.Path, r.Header.Get("X-Req"))
	}))
	t.Cleanup(srv.Close)
	port, _ := strconv.Atoi(srv.URL[strings.LastIndex(srv.URL, ":")+1:])
	return port
}

func baseInput(t *testing.T) *Input {
	dir := t.TempDir()
	return &Input{
		HTTPAddr:  "127.0.0.1:" + strconv.Itoa(freePort(t)),
		HTTPSAddr: "127.0.0.1:" + strconv.Itoa(freePort(t)),
		Storage:   filepath.Join(dir, "acme"),
		Ports:     map[string]int{},
		CertFiles: func(host string) (string, string) {
			return filepath.Join(dir, host+".crt"), filepath.Join(dir, host+".key")
		},
	}
}

func TestProxyRouting(t *testing.T) {
	in := baseInput(t)
	in.Ports["root"] = echo(t, "root")
	in.Ports["api"] = echo(t, "api")
	in.Ports["v2"] = echo(t, "v2")
	in.Ports["admin"] = echo(t, "admin")
	in.Ports["secret"] = echo(t, "secret")
	hash, _ := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.MinCost)
	in.Domains = []*store.Domain{
		{AppID: "root", Host: "a.test", Path: "/", TLS: store.TLSNone, Options: store.DomainOptions{
			Compress:        true,
			ResponseHeaders: []store.HeaderRule{{Op: "set", Name: "X-Frame-Options", Value: "DENY"}},
		}},
		{AppID: "api", Host: "a.test", Path: "/api", TLS: store.TLSNone, Options: store.DomainOptions{
			StripPrefix:    true,
			RequestHeaders: []store.HeaderRule{{Op: "set", Name: "X-Req", Value: "hello"}},
		}},
		{AppID: "v2", Host: "a.test", Path: "/api/v2", TLS: store.TLSNone},
		{AppID: "admin", Host: "a.test", Path: "/admin", TLS: store.TLSNone, Options: store.DomainOptions{
			BasicAuth: []store.BasicAuthUser{{User: "bob", Hash: string(hash)}},
		}},
		{AppID: "secret", Host: "a.test", Path: "/secret", TLS: store.TLSNone, Options: store.DomainOptions{
			AllowIPs:  []string{"10.0.0.0/8"},
			BasicAuth: []store.BasicAuthUser{{User: "bob", Hash: string(hash)}},
		}, Advanced: "header X-Advanced yes"},
		{AppID: "root", Host: "b.test", Path: "/", TLS: store.TLSNone, Advanced: "respond \"from advanced\" 200"},
		{AppID: "root", Host: "c.test", Path: "/", TLS: store.TLSCustom, Options: store.DomainOptions{ForceHTTPS: true, RedirectWWW: true}},
		{AppID: "root", Host: "d.test", Path: "/", TLS: store.TLSNone, Options: store.DomainOptions{RedirectWWW: true}},
	}
	writeSelfSigned(t, in, "c.test", "www.c.test")
	_, httpsPort := splitAddr(in.HTTPSAddr, "")
	_, httpPort := splitAddr(in.HTTPAddr, "")
	text := generate(in)
	cfg, err := adapt(text)
	if err != nil {
		t.Fatalf("adapt: %v\n%s", err, text)
	}
	if err := caddy.Load(cfg, true); err != nil {
		t.Fatalf("load: %v\n%s", err, text)
	}
	t.Cleanup(func() { caddy.Stop() })

	// Track reports to a live Proxy, so the recorder wrapping is exercised too.
	requests, err := requestlog.Open(filepath.Join(t.TempDir(), "requests.db"))
	if err != nil {
		t.Fatal(err)
	}
	current.Store(&Proxy{requests: requests})
	t.Cleanup(func() {
		current.Store(nil)
		requests.Close()
	})

	base := "http://" + in.HTTPAddr
	get := func(host, path string, setup func(*http.Request)) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest("GET", base+path, nil)
		req.Host = host
		if setup != nil {
			setup(req)
		}
		res, err := http.DefaultTransport.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		return res, string(body)
	}

	cases := []struct {
		host, path string
		setup      func(*http.Request)
		status     int
		body       string
	}{
		{"a.test", "/", nil, 200, "root / "},
		{"a.test", "/other", nil, 200, "root /other "},
		{"a.test", "/apix", nil, 200, "root /apix "}, // not a prefix match of /api
		{"a.test", "/api", nil, 200, "api / hello"},
		{"a.test", "/api/users", nil, 200, "api /users hello"},
		{"a.test", "/api/v2/x", nil, 200, "v2 /api/v2/x "},
		{"a.test", "/admin", nil, 401, ""},
		{"a.test", "/admin", func(r *http.Request) { r.SetBasicAuth("bob", "pw") }, 200, "admin /admin "},
		{"a.test", "/secret", func(r *http.Request) { r.SetBasicAuth("bob", "pw") }, 403, "Forbidden"},
		{"b.test", "/", nil, 200, "from advanced"},
		{"unknown.test", "/", nil, 404, noAppText},
		{"c.test", "/x?y=1", nil, 308, ""},
		{"www.c.test", "/x", nil, 308, ""},
		{"www.d.test", "/x", nil, 308, ""},
	}
	redirects := map[string]string{
		"c.test":     "https://c.test:" + httpsPort + "/x?y=1",
		"www.c.test": "https://c.test:" + httpsPort + "/x",
		"www.d.test": "http://d.test:" + httpPort + "/x",
	}
	for _, c := range cases {
		res, body := get(c.host, c.path, c.setup)
		if res.StatusCode != c.status || (c.body != "" && body != c.body) {
			t.Errorf("%s%s: got %d %q, want %d %q", c.host, c.path, res.StatusCode, body, c.status, c.body)
		}
		if want := redirects[c.host]; want != "" && res.Header.Get("Location") != want {
			t.Errorf("%s%s: redirected to %q, want %q", c.host, c.path, res.Header.Get("Location"), want)
		}
	}

	res, _ := get("a.test", "/big", func(r *http.Request) { r.Header.Set("Accept-Encoding", "gzip") })
	if res.Header.Get("Content-Encoding") != "gzip" {
		t.Errorf("compression: Content-Encoding %q", res.Header.Get("Content-Encoding"))
	}
	if res.Header.Get("X-Frame-Options") != "DENY" {
		t.Errorf("response header not set")
	}

	// Upgraded connections (WebSockets) pass through the tracking recorder.
	conn, err := net.Dial("tcp", in.HTTPAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	fmt.Fprintf(conn, "GET /ws HTTP/1.1\r\nHost: a.test\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n")
	br := bufio.NewReader(conn)
	if status, _ := br.ReadString('\n'); !strings.Contains(status, "101") {
		t.Fatalf("upgrade: %q", status)
	}
	for {
		if l, _ := br.ReadString('\n'); l == "\r\n" || l == "" {
			break
		}
	}
	fmt.Fprintf(conn, "ping\n")
	if got, _ := br.ReadString('\n'); got != "ping\n" {
		t.Errorf("upgraded echo: %q", got)
	}

	traffic := current.Load().Drain()
	if c := traffic["a.test"]; c.Requests == 0 || c.Status4xx == 0 {
		t.Errorf("traffic for a.test not counted: %+v", c)
	}

	// An app that is down gets the friendly 502.
	in.Ports["root"] = freePort(t)
	cfg, _ = adapt(generate(in))
	if err := caddy.Load(cfg, true); err != nil {
		t.Fatal(err)
	}
	if res, body := get("a.test", "/", nil); res.StatusCode != 502 || body != notRespondText {
		t.Errorf("down app: got %d %q", res.StatusCode, body)
	}
}

func TestSafeModeDropsUserConfig(t *testing.T) {
	in := baseInput(t)
	in.Ports["app"] = 3000
	in.Domains = []*store.Domain{{AppID: "app", Host: "a.test", Path: "/", TLS: store.TLSNone, Advanced: "not_a_directive"}}
	in.GlobalOptions = "bogus_option"
	if _, err := adapt(generate(in)); err == nil {
		t.Fatal("expected the bad snippet to fail")
	} else if !strings.Contains(err.Error(), "Caddyfile:") {
		t.Errorf("error should name the Caddyfile line: %v", err)
	}
	in.SafeMode = true
	if _, err := adapt(generate(in)); err != nil {
		t.Fatalf("safe mode: %v", err)
	}
}

func TestTLSModesValidate(t *testing.T) {
	in := baseInput(t)
	in.Ports["app"] = 3000
	in.Email = "ops@example.com"
	in.TrustedProxies = []string{"private_ranges", "173.245.48.0/20"}
	in.GlobalOptions = "servers {\n\ttimeouts {\n\t\tidle 5m\n\t}\n}"
	writeSelfSigned(t, in, "custom.example.com")
	in.Domains = []*store.Domain{
		{AppID: "app", Host: "le.example.com", Path: "/", TLS: store.TLSLetsEncrypt, Options: store.DomainOptions{ForceHTTPS: true, HSTS: true, RedirectWWW: true}},
		{AppID: "app", Host: "custom.example.com", Path: "/", TLS: store.TLSCustom, Options: store.DomainOptions{ForceHTTPS: true}},
		{AppID: "app", Host: "both.example.com", Path: "/", TLS: store.TLSLetsEncrypt},
	}
	in.PanelHost, in.PanelTLS = "panel.example.com", store.TLSLetsEncrypt
	text := generate(in)
	raw, err := adapt(text)
	if err != nil {
		t.Fatalf("adapt: %v\n%s", err, text)
	}
	var cfg caddy.Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	if err := caddy.Validate(&cfg); err != nil {
		t.Fatalf("validate: %v\n%s", err, text)
	}
	js := string(raw)
	for _, want := range []string{`"module":"acme"`, `"email":"ops@example.com"`, `"www.le.example.com"`, `"panel.example.com"`, `"moonshed_panel"`, `"173.245.48.0/20"`, `"load_files"`} {
		if !strings.Contains(js, want) {
			t.Errorf("adapted config is missing %s", want)
		}
	}
	if strings.Count(text, "servers {") != 1 {
		t.Errorf("trusted proxies should join the user's servers block:\n%s", text)
	}
	_, httpsPort := splitAddr(in.HTTPSAddr, "")
	for _, want := range []string{"http://le.example.com {", "redir https://le.example.com:" + httpsPort + "{uri} 308", "http://panel.example.com {"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "\nhttp://both.example.com {") {
		t.Errorf("force_https off must not redirect:\n%s", text)
	}
	if !strings.Contains(text, "both.example.com, http://both.example.com {") {
		t.Errorf("force_https off should serve plain HTTP too:\n%s", text)
	}
}

func writeSelfSigned(t *testing.T, in *Input, host string, extra ...string) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: host},
		DNSNames:     append([]string{host}, extra...),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, _ := x509.MarshalPKCS8PrivateKey(key)
	cert, keyFile := in.CertFiles(host)
	os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600)
	os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600)
}

func TestBalanced(t *testing.T) {
	for s, want := range map[string]bool{
		"header X {uri}":             true,
		"handle /x {\n respond 1\n}": true,
		"}\nexample.com {":           false,
		"handle {":                   false,
	} {
		if Balanced(s) != want {
			t.Errorf("Balanced(%q) = %v", s, !want)
		}
	}
}

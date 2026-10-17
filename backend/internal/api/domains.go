package api

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/netip"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"moonshed/internal/proxy"
	"moonshed/internal/store"
)

var (
	hostPattern = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	headerName  = regexp.MustCompile("^[A-Za-z0-9!#$%&'*+.^_`|~-]+$")
	byteSize    = regexp.MustCompile(`(?i)^\d+(\.\d+)?(b|kb|mb|gb|kib|mib|gib)?$`)
)

type domainView struct {
	*store.Domain
	Certificate *proxy.CertInfo `json:"certificate"`
}

func (s *Server) domainViews(appID string) ([]*domainView, error) {
	domains, err := s.Store.Domains(appID)
	if err != nil {
		return nil, err
	}
	views := make([]*domainView, 0, len(domains))
	for _, d := range domains {
		views = append(views, s.domainView(d))
	}
	return views, nil
}

// domainView never includes basic-auth password hashes.
func (s *Server) domainView(d *store.Domain) *domainView {
	c := *d
	c.Options.BasicAuth = make([]store.BasicAuthUser, len(d.Options.BasicAuth))
	for i, u := range d.Options.BasicAuth {
		c.Options.BasicAuth[i] = store.BasicAuthUser{User: u.User}
	}
	v := &domainView{Domain: &c}
	if d.TLS == store.TLSCustom {
		v.Certificate = s.Proxy.Certificate(d.Host)
	}
	return v
}

type domainInput struct {
	Host     string        `json:"host"`
	Path     string        `json:"path"`
	TLS      string        `json:"tls"`
	Options  *optionsInput `json:"options"`
	Advanced string        `json:"advanced"`
}

// optionsInput is store.DomainOptions with plain passwords instead of hashes.
// A user sent without a password keeps the hash already stored.
type optionsInput struct {
	ForceHTTPS      bool               `json:"force_https"`
	HSTS            bool               `json:"hsts"`
	RedirectWWW     bool               `json:"redirect_www"`
	Compress        bool               `json:"compress"`
	StripPrefix     bool               `json:"strip_prefix"`
	BasicAuth       []basicAuthInput   `json:"basic_auth"`
	AllowIPs        []string           `json:"allow_ips"`
	RequestHeaders  []store.HeaderRule `json:"request_headers"`
	ResponseHeaders []store.HeaderRule `json:"response_headers"`
	MaxBody         string             `json:"max_body"`
}

type basicAuthInput struct {
	User     string `json:"user"`
	Password string `json:"password"`
}

// domain validates in. existing is the stored domain being edited, or nil. When the
// request has no options (the quick add form), existing options and advanced config stay.
func (in domainInput) domain(existing *store.Domain) (*store.Domain, error) {
	d := &store.Domain{
		Host:     proxy.NormalizeHost(in.Host),
		Path:     "/" + strings.Trim(strings.TrimSpace(in.Path), "/"),
		TLS:      in.TLS,
		Options:  store.DomainOptions{ForceHTTPS: true},
		Advanced: strings.TrimSpace(in.Advanced),
	}
	if existing != nil {
		d.Options = existing.Options
		if in.Options == nil {
			d.Advanced = existing.Advanced
		}
	}
	if !hostPattern.MatchString(d.Host) {
		return nil, badRequest("invalid domain %q", in.Host)
	}
	switch d.TLS {
	case store.TLSNone, store.TLSLetsEncrypt, store.TLSCustom:
	default:
		return nil, badRequest("tls must be none, letsencrypt or custom")
	}
	if !proxy.Balanced(d.Advanced) {
		return nil, badRequest("advanced config has unbalanced braces")
	}
	if in.Options != nil {
		opts, err := in.Options.options(d.Options.BasicAuth)
		if err != nil {
			return nil, err
		}
		d.Options = *opts
	}
	return d, nil
}

func (o *optionsInput) options(previous []store.BasicAuthUser) (*store.DomainOptions, error) {
	out := &store.DomainOptions{
		ForceHTTPS:      o.ForceHTTPS,
		HSTS:            o.HSTS,
		RedirectWWW:     o.RedirectWWW,
		Compress:        o.Compress,
		StripPrefix:     o.StripPrefix,
		RequestHeaders:  o.RequestHeaders,
		ResponseHeaders: o.ResponseHeaders,
		MaxBody:         strings.ReplaceAll(strings.TrimSpace(o.MaxBody), " ", ""),
	}
	if out.MaxBody != "" && !byteSize.MatchString(out.MaxBody) {
		return nil, badRequest("max body size %q should look like 10MB", o.MaxBody)
	}
	for _, ip := range o.AllowIPs {
		if ip = strings.TrimSpace(ip); ip == "" {
			continue
		}
		if _, err := netip.ParsePrefix(ip); err != nil {
			if _, err := netip.ParseAddr(ip); err != nil {
				return nil, badRequest("%q is not an IP address or CIDR range", ip)
			}
		}
		out.AllowIPs = append(out.AllowIPs, ip)
	}
	for _, rules := range [][]store.HeaderRule{o.RequestHeaders, o.ResponseHeaders} {
		for _, h := range rules {
			if !headerName.MatchString(h.Name) {
				return nil, badRequest("invalid header name %q", h.Name)
			}
			if h.Op != "set" && h.Op != "add" && h.Op != "remove" {
				return nil, badRequest("header %s: op must be set, add or remove", h.Name)
			}
			if strings.ContainsAny(h.Value, "\r\n") {
				return nil, badRequest("header %s: value can't contain line breaks", h.Name)
			}
		}
	}
	hashes := map[string]string{}
	for _, u := range previous {
		hashes[u.User] = u.Hash
	}
	seen := map[string]bool{}
	for _, u := range o.BasicAuth {
		u.User = strings.TrimSpace(u.User)
		if u.User == "" || strings.ContainsAny(u.User, ": \t\"") {
			return nil, badRequest("invalid basic auth username %q", u.User)
		}
		if seen[u.User] {
			return nil, badRequest("basic auth user %s is listed twice", u.User)
		}
		seen[u.User] = true
		hash := hashes[u.User]
		if u.Password != "" {
			b, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
			if err != nil {
				return nil, badRequest("password for %s: %v", u.User, err)
			}
			hash = string(b)
		}
		if hash == "" {
			return nil, badRequest("set a password for %s", u.User)
		}
		out.BasicAuth = append(out.BasicAuth, store.BasicAuthUser{User: u.User, Hash: hash})
	}
	return out, nil
}

// checkDomain applies the rules that depend on other domains and settings, then
// validates the proxy config with d in it.
func (s *Server) checkDomain(d *store.Domain) error {
	if panel, _ := s.Store.Setting(proxy.SettingPanelHost); panel == d.Host {
		return badRequest("%s is used for the panel", d.Host)
	}
	if d.TLS == store.TLSCustom && !s.Proxy.HasCertificate(d.Host) {
		return badRequest("upload a certificate for %s", d.Host)
	}
	if d.Options.RedirectWWW && d.TLS == store.TLSCustom && !s.Proxy.CertificateCovers(d.Host, "www."+d.Host) {
		return badRequest("the certificate for %s doesn't cover www.%s, so www can't redirect", d.Host, d.Host)
	}
	siblings, err := s.siblings(d)
	if err != nil {
		return err
	}
	for _, o := range siblings {
		if o.TLS != d.TLS {
			return badRequest("%s%s uses %s; every path on a host needs the same HTTPS setting", o.Host, o.Path, o.TLS)
		}
	}
	if _, err := s.Proxy.Check(withDomain(d)); err != nil {
		return badRequest("proxy config: %v", err)
	}
	return nil
}

// siblings are the other domains on d's host.
func (s *Server) siblings(d *store.Domain) ([]*store.Domain, error) {
	all, err := s.Store.Domains("")
	if err != nil {
		return nil, err
	}
	var out []*store.Domain
	for _, o := range all {
		if o.Host == d.Host && o.ID != d.ID {
			out = append(out, o)
		}
	}
	return out, nil
}

// syncHost copies the host-wide options of d onto o and reports whether o changed.
func syncHost(o, d *store.Domain) bool {
	a, b := &o.Options, d.Options
	if a.ForceHTTPS == b.ForceHTTPS && a.HSTS == b.HSTS && a.RedirectWWW == b.RedirectWWW {
		return false
	}
	a.ForceHTTPS, a.HSTS, a.RedirectWWW = b.ForceHTTPS, b.HSTS, b.RedirectWWW
	return true
}

// withDomain puts the unsaved d into the proxy input the way saveDomain will store it.
func withDomain(d *store.Domain) func(*proxy.Input) {
	return func(in *proxy.Input) {
		replaced := false
		for i, o := range in.Domains {
			if d.ID != "" && o.ID == d.ID {
				in.Domains[i], replaced = d, true
			} else if o.Host == d.Host {
				syncHost(o, d)
			}
		}
		if !replaced {
			in.Domains = append(in.Domains, d)
		}
	}
}

func (s *Server) listDomains(w http.ResponseWriter, r *http.Request) error {
	app, err := s.appFromPath(r)
	if err != nil {
		return err
	}
	views, err := s.domainViews(app.ID)
	if err != nil {
		return err
	}
	return ok(w, views)
}

// createDomain takes JSON, or multipart when the domain comes with its own certificate
// (fields host, path, tls plus cert+key or pfx+password).
func (s *Server) createDomain(w http.ResponseWriter, r *http.Request) error {
	app, err := s.appFromPath(r)
	if err != nil {
		return err
	}
	if app.Port == 0 {
		return badRequest("set a port on the app before adding a domain")
	}

	var in domainInput
	multipart := isMultipart(r)
	if multipart {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			return badRequest("%v", err)
		}
		in = domainInput{Host: r.FormValue("host"), Path: r.FormValue("path"), TLS: r.FormValue("tls")}
	} else if err := decode(r, &in); err != nil {
		return err
	}
	d, err := in.domain(nil)
	if err != nil {
		return err
	}
	d.AppID = app.ID
	// A new path on a host that is already in use takes that host's settings.
	if in.Options == nil {
		if siblings, err := s.siblings(d); err == nil && len(siblings) > 0 {
			syncHost(d, siblings[0])
		}
	}
	if multipart && d.TLS == store.TLSCustom {
		if err := s.saveCertificate(r, d.Host); err != nil {
			return err
		}
	}
	if err := s.checkDomain(d); err != nil {
		return err
	}
	return s.saveDomain(w, r, d, nil, http.StatusCreated)
}

// editedDomain reads the domain in the path and applies the request body to it. The body
// is JSON, or multipart with the JSON in a "domain" field plus certificate files.
func (s *Server) editedDomain(r *http.Request) (*store.Domain, *store.Domain, error) {
	id, err := pathID(r, "id")
	if err != nil {
		return nil, nil, err
	}
	existing, err := s.Store.Domain(id)
	if err != nil {
		return nil, nil, err
	}
	var in domainInput
	if isMultipart(r) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			return nil, nil, badRequest("%v", err)
		}
		dec := json.NewDecoder(strings.NewReader(r.FormValue("domain")))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&in); err != nil {
			return nil, nil, badRequest("invalid domain field: %v", err)
		}
	} else if err := decode(r, &in); err != nil {
		return nil, nil, err
	}
	d, err := in.domain(existing)
	if err != nil {
		return nil, nil, err
	}
	d.ID, d.AppID = existing.ID, existing.AppID
	return d, existing, nil
}

func (s *Server) updateDomain(w http.ResponseWriter, r *http.Request) error {
	d, existing, err := s.editedDomain(r)
	if err != nil {
		return err
	}
	if isMultipart(r) && d.TLS == store.TLSCustom {
		if err := s.saveCertificate(r, d.Host); err != nil {
			return err
		}
	}
	if err := s.checkDomain(d); err != nil {
		return err
	}
	return s.saveDomain(w, r, d, existing, http.StatusOK)
}

// checkDomainDraft validates an edited domain without saving it and returns the
// Caddyfile it would produce.
func (s *Server) checkDomainDraft(w http.ResponseWriter, r *http.Request) error {
	d, _, err := s.editedDomain(r)
	if err != nil {
		return err
	}
	text, err := s.Proxy.Check(withDomain(d))
	return ok(w, checkResult(text, err))
}

func checkResult(caddyfile string, err error) map[string]any {
	res := map[string]any{"ok": err == nil, "caddyfile": caddyfile}
	if err != nil {
		res["error"] = err.Error()
	}
	return res
}

func (s *Server) saveDomain(w http.ResponseWriter, r *http.Request, d, previous *store.Domain, status int) error {
	if err := s.Store.SaveDomain(d); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return errorf(http.StatusConflict, "%s%s is already in use", d.Host, d.Path)
		}
		return err
	}
	siblings, err := s.siblings(d)
	if err != nil {
		return err
	}
	for _, o := range siblings {
		if syncHost(o, d) {
			if err := s.Store.SaveDomain(o); err != nil {
				return err
			}
		}
	}
	if previous != nil && previous.Host != d.Host {
		s.pruneCertificate(previous.Host)
	}
	if err := s.Proxy.Reload(r.Context()); err != nil {
		return err
	}
	action := "domain.create"
	if status == http.StatusOK {
		action = "domain.update"
	}
	s.auditDomain(r, action, d, previous)
	return writeJSON(w, status, s.domainView(d))
}

func (s *Server) deleteDomain(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	d, err := s.Store.Domain(id)
	if err != nil {
		return err
	}
	if err := s.Store.DeleteDomain(id); err != nil {
		return err
	}
	s.pruneCertificate(d.Host)
	if err := s.Proxy.Reload(r.Context()); err != nil {
		return err
	}
	s.auditDomain(r, "domain.delete", d, nil)
	return noContent(w)
}

// replaceDomainCertificate uploads a new certificate for a custom-TLS domain.
func (s *Server) replaceDomainCertificate(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	d, err := s.Store.Domain(id)
	if err != nil {
		return err
	}
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		return badRequest("%v", err)
	}
	if err := s.saveCertificate(r, d.Host); err != nil {
		return err
	}
	if err := s.Proxy.Reload(r.Context()); err != nil {
		return err
	}
	s.auditDomain(r, "domain.certificate", d, nil)
	return ok(w, s.domainView(d))
}

// uploadPanelCertificate stores the certificate for the panel domain, which may not be saved yet.
func (s *Server) uploadPanelCertificate(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		return badRequest("%v", err)
	}
	host := proxy.NormalizeHost(r.FormValue("host"))
	if !hostPattern.MatchString(host) {
		return badRequest("enter the panel domain first")
	}
	if err := s.saveCertificate(r, host); err != nil {
		return err
	}
	if err := s.Proxy.Reload(r.Context()); err != nil {
		return err
	}
	s.audit(r, "settings.panel_certificate", "", "", host, "")
	return ok(w, s.Proxy.Certificate(host))
}

// saveCertificate reads cert+key (PEM) or pfx+password from a parsed multipart form.
func (s *Server) saveCertificate(r *http.Request, host string) error {
	read := func(field string) []byte {
		f, _, err := r.FormFile(field)
		if err != nil {
			return nil
		}
		defer f.Close()
		data, _ := io.ReadAll(io.LimitReader(f, 1<<20))
		return data
	}
	certPEM, keyPEM := read("cert"), read("key")
	if pfx := read("pfx"); pfx != nil {
		var err error
		if certPEM, keyPEM, err = proxy.PFXToPEM(pfx, r.FormValue("password")); err != nil {
			return badRequest("pfx: %v", err)
		}
	}
	if certPEM == nil || keyPEM == nil {
		return badRequest("upload the certificate and key files, or a pfx file")
	}
	if err := s.Proxy.SaveCertificate(host, certPEM, keyPEM); err != nil {
		return badRequest("%v", err)
	}
	return nil
}

// pruneCertificate deletes an uploaded certificate once no domain or panel uses it.
func (s *Server) pruneCertificate(host string) {
	domains, err := s.Store.Domains("")
	if err != nil {
		return
	}
	for _, d := range domains {
		if d.Host == host && d.TLS == store.TLSCustom {
			return
		}
	}
	panelHost, _ := s.Store.Setting(proxy.SettingPanelHost)
	panelTLS, _ := s.Store.Setting(proxy.SettingPanelTLS)
	if panelHost == host && panelTLS == store.TLSCustom {
		return
	}
	s.Proxy.DeleteCertificate(host)
}

// auditDomain logs a domain change against the app that owns it. Changed options are
// named, never their values (passwords, header values).
func (s *Server) auditDomain(r *http.Request, action string, d, previous *store.Domain) {
	name := ""
	if app, err := s.Store.App(d.AppID); err == nil {
		name = app.Name
	}
	details := d.Host + d.Path + " (" + d.TLS + ")"
	if previous != nil {
		if changed := optionChanges(previous, d); len(changed) > 0 {
			details += ", changed " + strings.Join(changed, ", ")
		}
	}
	s.audit(r, action, "app", d.AppID, name, details)
}

func optionChanges(a, b *store.Domain) []string {
	x, y := a.Options, b.Options
	var out []string
	for _, c := range []struct {
		name    string
		differs bool
	}{
		{"force https", x.ForceHTTPS != y.ForceHTTPS},
		{"hsts", x.HSTS != y.HSTS},
		{"www redirect", x.RedirectWWW != y.RedirectWWW},
		{"compression", x.Compress != y.Compress},
		{"strip prefix", x.StripPrefix != y.StripPrefix},
		{"basic auth", !slices.Equal(x.BasicAuth, y.BasicAuth)},
		{"ip allow list", !slices.Equal(x.AllowIPs, y.AllowIPs)},
		{"request headers", !slices.Equal(x.RequestHeaders, y.RequestHeaders)},
		{"response headers", !slices.Equal(x.ResponseHeaders, y.ResponseHeaders)},
		{"max body", x.MaxBody != y.MaxBody},
		{"advanced config", a.Advanced != b.Advanced},
	} {
		if c.differs {
			out = append(out, c.name)
		}
	}
	return out
}

func isMultipart(r *http.Request) bool {
	ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return ct == "multipart/form-data"
}

package api

import (
	"io"
	"mime"
	"net/http"
	"regexp"
	"strings"

	"moonshed/internal/proxy"
	"moonshed/internal/store"
)

var hostPattern = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

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

func (s *Server) domainView(d *store.Domain) *domainView {
	v := &domainView{Domain: d}
	if d.TLS == store.TLSCustom {
		v.Certificate = s.Proxy.Certificate(d.Host)
	}
	return v
}

type domainInput struct {
	Host string `json:"host"`
	Path string `json:"path"`
	TLS  string `json:"tls"`
}

func (in domainInput) domain() (*store.Domain, error) {
	d := &store.Domain{
		Host: proxy.NormalizeHost(in.Host),
		Path: "/" + strings.Trim(strings.TrimSpace(in.Path), "/"),
		TLS:  in.TLS,
	}
	if !hostPattern.MatchString(d.Host) {
		return nil, badRequest("invalid domain %q", in.Host)
	}
	switch d.TLS {
	case store.TLSNone, store.TLSLetsEncrypt, store.TLSCustom:
	default:
		return nil, badRequest("tls must be none, letsencrypt or custom")
	}
	return d, nil
}

func (s *Server) checkDomain(d *store.Domain) error {
	if panel, _ := s.Store.Setting(proxy.SettingPanelHost); panel == d.Host {
		return badRequest("%s is used for the panel", d.Host)
	}
	if d.TLS == store.TLSCustom && !s.Proxy.HasCertificate(d.Host) {
		return badRequest("upload a certificate for %s", d.Host)
	}
	return nil
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
	d, err := in.domain()
	if err != nil {
		return err
	}
	if multipart && d.TLS == store.TLSCustom {
		if err := s.saveCertificate(r, d.Host); err != nil {
			return err
		}
	}
	if err := s.checkDomain(d); err != nil {
		return err
	}
	d.AppID = app.ID
	return s.saveDomain(w, r, d, "", http.StatusCreated)
}

func (s *Server) updateDomain(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	existing, err := s.Store.Domain(id)
	if err != nil {
		return err
	}
	var in domainInput
	if err := decode(r, &in); err != nil {
		return err
	}
	d, err := in.domain()
	if err != nil {
		return err
	}
	if err := s.checkDomain(d); err != nil {
		return err
	}
	d.ID, d.AppID = existing.ID, existing.AppID
	return s.saveDomain(w, r, d, existing.Host, http.StatusOK)
}

func (s *Server) saveDomain(w http.ResponseWriter, r *http.Request, d *store.Domain, previousHost string, status int) error {
	if err := s.Store.SaveDomain(d); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return errorf(http.StatusConflict, "%s%s is already in use", d.Host, d.Path)
		}
		return err
	}
	if previousHost != "" {
		s.pruneCertificate(previousHost)
	}
	if err := s.Proxy.Reload(r.Context()); err != nil {
		return err
	}
	action := "domain.create"
	if status == http.StatusOK {
		action = "domain.update"
	}
	s.auditDomain(r, action, d)
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
	s.auditDomain(r, "domain.delete", d)
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
	s.auditDomain(r, "domain.certificate", d)
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

// auditDomain logs a domain change against the app that owns it.
func (s *Server) auditDomain(r *http.Request, action string, d *store.Domain) {
	name := ""
	if app, err := s.Store.App(d.AppID); err == nil {
		name = app.Name
	}
	s.audit(r, action, "app", d.AppID, name, d.Host+d.Path+" ("+d.TLS+")")
}

func isMultipart(r *http.Request) bool {
	ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return ct == "multipart/form-data"
}

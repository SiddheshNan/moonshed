package api

import (
	"cmp"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"moonshed/internal/backup"
	"moonshed/internal/proxy"
	"moonshed/internal/store"
)

type settings struct {
	ProxyEnabled   bool     `json:"proxy_enabled"`
	ACMEEmail      string   `json:"acme_email"`
	PanelDomain    string   `json:"panel_domain"`
	PanelTLS       string   `json:"panel_tls"`
	TrustedProxies []string `json:"trusted_proxies"`
	GlobalOptions  string   `json:"global_options"`
	ExtraSites     string   `json:"extra_sites"`
	BackupCron     string   `json:"backup_cron"`
	BackupKeep     int      `json:"backup_keep"`
}

func (s *Server) loadSettings() (*settings, error) {
	m, err := s.Store.Settings()
	if err != nil {
		return nil, err
	}
	keep, _ := strconv.Atoi(m[backup.SettingKeep])
	return &settings{
		ProxyEnabled:   m[proxy.SettingEnabled] == "true",
		ACMEEmail:      m[proxy.SettingACMEEmail],
		PanelDomain:    m[proxy.SettingPanelHost],
		PanelTLS:       cmp.Or(m[proxy.SettingPanelTLS], store.TLSNone),
		TrustedProxies: strings.Fields(m[proxy.SettingTrustedProxies]),
		GlobalOptions:  m[proxy.SettingGlobalOptions],
		ExtraSites:     m[proxy.SettingExtraSites],
		BackupCron:     m[backup.SettingCron],
		BackupKeep:     cmp.Or(keep, 7),
	}, nil
}

// checkProxySettings validates the proxy part of in and the config it would produce.
func (s *Server) checkProxySettings(in *settings) (string, error) {
	var trusted []string
	for _, p := range in.TrustedProxies {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		if p != "private_ranges" {
			if _, err := netip.ParsePrefix(p); err != nil {
				if _, err := netip.ParseAddr(p); err != nil {
					return "", badRequest("trusted proxy %q is not an IP address or CIDR range", p)
				}
			}
		}
		trusted = append(trusted, p)
	}
	in.TrustedProxies = trusted
	in.GlobalOptions = strings.TrimSpace(in.GlobalOptions)
	in.ExtraSites = strings.TrimSpace(in.ExtraSites)
	if !proxy.Balanced(in.GlobalOptions) {
		return "", badRequest("global options have unbalanced braces")
	}
	if !proxy.Balanced(in.ExtraSites) {
		return "", badRequest("extra sites have unbalanced braces")
	}
	return s.Proxy.Check(func(pi *proxy.Input) {
		pi.Email = in.ACMEEmail
		pi.PanelHost, pi.PanelTLS = in.PanelDomain, in.PanelTLS
		pi.TrustedProxies = in.TrustedProxies
		pi.GlobalOptions, pi.ExtraSites = in.GlobalOptions, in.ExtraSites
	})
}

// checkSettingsDraft validates proxy settings without saving them.
func (s *Server) checkSettingsDraft(w http.ResponseWriter, r *http.Request) error {
	var in settings
	if err := decode(r, &in); err != nil {
		return err
	}
	in.PanelDomain = proxy.NormalizeHost(in.PanelDomain)
	in.PanelTLS = cmp.Or(in.PanelTLS, store.TLSNone)
	text, err := s.checkProxySettings(&in)
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		return err
	}
	return ok(w, checkResult(text, err))
}

func (s *Server) proxyCaddyfile(w http.ResponseWriter, r *http.Request) error {
	text, err := s.Proxy.Caddyfile()
	if err != nil {
		return err
	}
	return ok(w, map[string]string{"caddyfile": text})
}

func (s *Server) settingsResponse(st *settings) map[string]any {
	resp := map[string]any{"settings": st, "proxy": s.Proxy.Status(), "panel_addr": s.PanelAddr}
	if st.PanelDomain != "" && st.PanelTLS == store.TLSCustom {
		resp["panel_certificate"] = s.Proxy.Certificate(st.PanelDomain)
	}
	return resp
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) error {
	st, err := s.loadSettings()
	if err != nil {
		return err
	}
	return ok(w, s.settingsResponse(st))
}

func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) error {
	var in settings
	if err := decode(r, &in); err != nil {
		return err
	}
	in.PanelDomain = proxy.NormalizeHost(in.PanelDomain)
	in.PanelTLS = cmp.Or(in.PanelTLS, store.TLSNone)
	if in.PanelDomain != "" {
		panel, err := domainInput{Host: in.PanelDomain, TLS: in.PanelTLS}.domain(nil)
		if err != nil {
			return err
		}
		if panel.TLS == store.TLSCustom && !s.Proxy.HasCertificate(panel.Host) {
			return badRequest("upload a certificate for %s", panel.Host)
		}
		if domains, err := s.Store.Domains(""); err == nil {
			for _, d := range domains {
				if d.Host == panel.Host {
					return badRequest("%s is already used by an app", panel.Host)
				}
			}
		}
	}
	previous, err := s.loadSettings()
	if err != nil {
		return err
	}
	if err := backup.ValidateCron(in.BackupCron); err != nil {
		return badRequest("invalid backup schedule: %v", err)
	}
	if in.BackupKeep < 1 {
		return badRequest("backups to keep must be at least 1")
	}
	if _, err := s.checkProxySettings(&in); err != nil {
		var apiErr *apiError
		if errors.As(err, &apiErr) {
			return err
		}
		return badRequest("proxy config: %v", err)
	}
	values := map[string]string{
		proxy.SettingEnabled:        strconv.FormatBool(in.ProxyEnabled),
		proxy.SettingACMEEmail:      in.ACMEEmail,
		proxy.SettingPanelHost:      in.PanelDomain,
		proxy.SettingPanelTLS:       in.PanelTLS,
		proxy.SettingTrustedProxies: strings.Join(in.TrustedProxies, " "),
		proxy.SettingGlobalOptions:  in.GlobalOptions,
		proxy.SettingExtraSites:     in.ExtraSites,
		backup.SettingCron:          in.BackupCron,
		backup.SettingKeep:          strconv.Itoa(in.BackupKeep),
	}
	for k, v := range values {
		if err := s.Store.SetSetting(k, v); err != nil {
			return err
		}
	}
	if previous.PanelDomain != "" {
		s.pruneCertificate(previous.PanelDomain)
	}
	s.audit(r, "settings.update", "", "", "", settingsChanges(previous, &in))
	if err := s.Backups.Reload(); err != nil {
		return err
	}
	proxyErr := s.Proxy.Reload(r.Context())
	resp := s.settingsResponse(&in)
	if proxyErr != nil {
		resp["error"] = proxyErr.Error()
	}
	return ok(w, resp)
}

// settingsChanges names the settings that changed, for the audit log.
func settingsChanges(a, b *settings) string {
	var changed []string
	for _, c := range []struct {
		name    string
		differs bool
	}{
		{"proxy enabled", a.ProxyEnabled != b.ProxyEnabled},
		{"let's encrypt email", a.ACMEEmail != b.ACMEEmail},
		{"panel domain", a.PanelDomain != b.PanelDomain || a.PanelTLS != b.PanelTLS},
		{"trusted proxies", !slices.Equal(a.TrustedProxies, b.TrustedProxies)},
		{"proxy global options", a.GlobalOptions != b.GlobalOptions},
		{"proxy extra sites", a.ExtraSites != b.ExtraSites},
		{"platform backups", a.BackupCron != b.BackupCron || a.BackupKeep != b.BackupKeep},
	} {
		if c.differs {
			changed = append(changed, c.name)
		}
	}
	if len(changed) == 0 {
		return "no changes"
	}
	return "changed " + strings.Join(changed, ", ")
}

func (s *Server) listAppBackups(w http.ResponseWriter, r *http.Request) error {
	app, err := s.appFromPath(r)
	if err != nil {
		return err
	}
	return s.writeBackups(w, app.ID)
}

func (s *Server) listPlatformBackups(w http.ResponseWriter, r *http.Request) error {
	return s.writeBackups(w, "")
}

func (s *Server) writeBackups(w http.ResponseWriter, appID string) error {
	list, err := s.Store.Backups(appID)
	if err != nil {
		return err
	}
	if list == nil {
		list = []*store.Backup{}
	}
	return ok(w, list)
}

func (s *Server) createAppBackup(w http.ResponseWriter, r *http.Request) error {
	app, err := s.appFromPath(r)
	if err != nil {
		return err
	}
	b, err := s.Backups.BackupApp(app)
	if err != nil {
		return errorf(http.StatusConflict, "%v", err)
	}
	s.auditApp(r, "backup.create", app, "")
	return writeJSON(w, http.StatusAccepted, b)
}

func (s *Server) createPlatformBackup(w http.ResponseWriter, r *http.Request) error {
	b, err := s.Backups.BackupPlatform()
	if err != nil {
		return errorf(http.StatusConflict, "%v", err)
	}
	s.audit(r, "backup.create", "platform", "", "platform", "")
	return writeJSON(w, http.StatusCreated, b)
}

func (s *Server) backupFromPath(r *http.Request) (*store.Backup, error) {
	id, err := pathID(r, "id")
	if err != nil {
		return nil, err
	}
	return s.Store.Backup(id)
}

func (s *Server) restoreBackup(w http.ResponseWriter, r *http.Request) error {
	b, err := s.backupFromPath(r)
	if err != nil {
		return err
	}
	if b.AppID == "" {
		return badRequest("platform backups are restored by hand, see the docs")
	}
	app, err := s.Store.App(b.AppID)
	if err != nil {
		return err
	}
	if err := s.Backups.RestoreApp(app, b); err != nil {
		return errorf(http.StatusConflict, "%v", err)
	}
	s.auditApp(r, "backup.restore", app, filepath.Base(b.File))
	return noContent(w)
}

func (s *Server) downloadBackup(w http.ResponseWriter, r *http.Request) error {
	b, err := s.backupFromPath(r)
	if err != nil {
		return err
	}
	var appName string
	if b.AppID != "" {
		app, err := s.Store.App(b.AppID)
		if err != nil {
			return err
		}
		appName = app.Name
	}
	f, err := os.Open(b.File)
	if err != nil {
		return store.ErrNotFound
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, backup.FileName(appName, b)))
	http.ServeContent(w, r, filepath.Base(b.File), b.CreatedAt.Time, f)
	return nil
}

func (s *Server) deleteBackup(w http.ResponseWriter, r *http.Request) error {
	b, err := s.backupFromPath(r)
	if err != nil {
		return err
	}
	if err := s.Backups.Delete(b); err != nil {
		return errorf(http.StatusConflict, "%v", err)
	}
	s.audit(r, "backup.delete", "backup", b.ID, filepath.Base(b.File), "")
	return noContent(w)
}

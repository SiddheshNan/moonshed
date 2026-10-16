package api

import (
	"cmp"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"moonshed/internal/backup"
	"moonshed/internal/proxy"
	"moonshed/internal/store"
)

type settings struct {
	ProxyEnabled bool   `json:"proxy_enabled"`
	ACMEEmail    string `json:"acme_email"`
	PanelDomain  string `json:"panel_domain"`
	PanelTLS     string `json:"panel_tls"`
	BackupCron   string `json:"backup_cron"`
	BackupKeep   int    `json:"backup_keep"`
}

func (s *Server) loadSettings() (*settings, error) {
	m, err := s.Store.Settings()
	if err != nil {
		return nil, err
	}
	keep, _ := strconv.Atoi(m[backup.SettingKeep])
	return &settings{
		ProxyEnabled: m[proxy.SettingEnabled] == "true",
		ACMEEmail:    m[proxy.SettingACMEEmail],
		PanelDomain:  m[proxy.SettingPanelHost],
		PanelTLS:     cmp.Or(m[proxy.SettingPanelTLS], store.TLSNone),
		BackupCron:   m[backup.SettingCron],
		BackupKeep:   cmp.Or(keep, 7),
	}, nil
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
		panel, err := domainInput{Host: in.PanelDomain, TLS: in.PanelTLS}.domain()
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
	values := map[string]string{
		proxy.SettingEnabled:   strconv.FormatBool(in.ProxyEnabled),
		proxy.SettingACMEEmail: in.ACMEEmail,
		proxy.SettingPanelHost: in.PanelDomain,
		proxy.SettingPanelTLS:  in.PanelTLS,
		backup.SettingCron:     in.BackupCron,
		backup.SettingKeep:     strconv.Itoa(in.BackupKeep),
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

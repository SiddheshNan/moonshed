package store

import (
	"database/sql"
	"fmt"
)

const (
	RuntimeNative = "native"
	RuntimeDocker = "docker"

	SourceUpload = "upload"
	SourceGit    = "git"

	StateRunning = "running"
	StateStopped = "stopped"
)

type App struct {
	ID                   string `json:"id"`
	ProjectID            string `json:"project_id"`
	Name                 string `json:"name"`
	Runtime              string `json:"runtime"`
	Port                 int    `json:"port"`
	Source               string `json:"source"`
	GitURL               string `json:"git_url"`
	GitBranch            string `json:"git_branch"`
	GitSubdir            string `json:"git_subdir"`
	GitCredentialID      string `json:"git_credential_id"`
	GitHubInstallationID int64  `json:"github_installation_id"`
	Autodeploy           bool   `json:"autodeploy"`
	PushedCommit         string `json:"pushed_commit"`
	PushedMessage        string `json:"pushed_message"`
	WebhookSecret        string `json:"-"`
	RunAsUser            string `json:"run_as_user"`
	RunAsPassword        string `json:"-"`
	KeepBundles          int    `json:"keep_bundles"`
	LogRetentionDays     int    `json:"log_retention_days"`
	AutoRestart          bool   `json:"auto_restart"`
	BackupCron           string `json:"backup_cron"`
	BackupKeep           int    `json:"backup_keep"`
	RecordRequests       bool   `json:"record_requests"`
	CurrentReleaseID     string `json:"current_release_id"`
	DesiredState         string `json:"desired_state"`
	CreatedAt            Time   `json:"created_at"`
}

const appColumns = `id, COALESCE(project_id, ''), name, runtime, port, source, git_url, git_branch, git_subdir,
	COALESCE(git_credential_id, ''), COALESCE(github_installation_id, 0), autodeploy, pushed_commit, pushed_message,
	webhook_secret, run_as_user, run_as_password, keep_bundles, log_retention_days, auto_restart,
	backup_cron, backup_keep, record_requests, current_release_id, desired_state, created_at`

func (s *Store) scanApp(row interface{ Scan(...any) error }) (*App, error) {
	var a App
	err := row.Scan(&a.ID, &a.ProjectID, &a.Name, &a.Runtime, &a.Port, &a.Source, &a.GitURL, &a.GitBranch, &a.GitSubdir,
		&a.GitCredentialID, &a.GitHubInstallationID, &a.Autodeploy, &a.PushedCommit, &a.PushedMessage,
		&a.WebhookSecret, &a.RunAsUser, &a.RunAsPassword, &a.KeepBundles, &a.LogRetentionDays,
		&a.AutoRestart, &a.BackupCron, &a.BackupKeep, &a.RecordRequests, &a.CurrentReleaseID, &a.DesiredState, &a.CreatedAt)
	if err != nil {
		return nil, notFound(err)
	}
	if a.RunAsPassword, err = s.box.Decrypt(a.RunAsPassword); err != nil {
		return nil, fmt.Errorf("decrypt run-as password for %s: %w", a.Name, err)
	}
	return &a, nil
}

func (s *Store) Apps() ([]*App, error) {
	return s.queryApps("SELECT " + appColumns + " FROM apps ORDER BY name")
}

func (s *Store) ProjectApps(projectID string) ([]*App, error) {
	return s.queryApps("SELECT "+appColumns+" FROM apps WHERE project_id = ? ORDER BY name", projectID)
}

func (s *Store) queryApps(query string, args ...any) ([]*App, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var apps []*App
	for rows.Next() {
		a, err := s.scanApp(rows)
		if err != nil {
			return nil, err
		}
		apps = append(apps, a)
	}
	return apps, rows.Err()
}

func (s *Store) App(id string) (*App, error) {
	return s.scanApp(s.db.QueryRow("SELECT "+appColumns+" FROM apps WHERE id = ?", id))
}

func (s *Store) AppByName(name string) (*App, error) {
	return s.scanApp(s.db.QueryRow("SELECT "+appColumns+" FROM apps WHERE name = ?", name))
}

func nullInt(n int64) any {
	if n == 0 {
		return nil
	}
	return n
}

func (s *Store) CreateApp(a *App) error {
	runAsPassword, err := s.box.Encrypt(a.RunAsPassword)
	if err != nil {
		return err
	}
	a.ID, a.CreatedAt = NewID(), Now()
	_, err = s.db.Exec(`INSERT INTO apps (id, project_id, name, runtime, port, source, git_url, git_branch, git_subdir,
		git_credential_id, github_installation_id, autodeploy, webhook_secret, run_as_user, run_as_password,
		keep_bundles, log_retention_days, auto_restart, backup_cron, backup_keep, desired_state, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, nullID(a.ProjectID), a.Name, a.Runtime, a.Port, a.Source, a.GitURL, a.GitBranch, a.GitSubdir,
		nullID(a.GitCredentialID), nullInt(a.GitHubInstallationID), boolInt(a.Autodeploy), a.WebhookSecret, a.RunAsUser, runAsPassword,
		a.KeepBundles, a.LogRetentionDays, boolInt(a.AutoRestart), a.BackupCron, a.BackupKeep, a.DesiredState, a.CreatedAt)
	return err
}

func (s *Store) UpdateApp(a *App) error {
	runAsPassword, err := s.box.Encrypt(a.RunAsPassword)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE apps SET project_id = ?, runtime = ?, port = ?, source = ?, git_url = ?, git_branch = ?, git_subdir = ?,
		git_credential_id = ?, github_installation_id = ?, autodeploy = ?, run_as_user = ?, run_as_password = ?,
		keep_bundles = ?, log_retention_days = ?, auto_restart = ?, backup_cron = ?, backup_keep = ?
		WHERE id = ?`,
		nullID(a.ProjectID), a.Runtime, a.Port, a.Source, a.GitURL, a.GitBranch, a.GitSubdir,
		nullID(a.GitCredentialID), nullInt(a.GitHubInstallationID), boolInt(a.Autodeploy), a.RunAsUser, runAsPassword,
		a.KeepBundles, a.LogRetentionDays, boolInt(a.AutoRestart), a.BackupCron, a.BackupKeep, a.ID)
	return err
}

func (s *Store) SetRuntime(appID, runtime string) error {
	_, err := s.db.Exec("UPDATE apps SET runtime = ? WHERE id = ?", runtime, appID)
	return err
}

func (s *Store) SetAppPort(appID string, port int) error {
	_, err := s.db.Exec("UPDATE apps SET port = ? WHERE id = ?", port, appID)
	return err
}

// SetPushedCommit remembers the latest pushed commit, shown when autodeploy is off.
func (s *Store) SetPushedCommit(appID string, sha, message string) error {
	_, err := s.db.Exec("UPDATE apps SET pushed_commit = ?, pushed_message = ? WHERE id = ?", sha, message, appID)
	return err
}

func (s *Store) SetRecordRequests(appIDs []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("UPDATE apps SET record_requests = 0"); err != nil {
		return err
	}
	for _, id := range appIDs {
		if _, err := tx.Exec("UPDATE apps SET record_requests = 1 WHERE id = ?", id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SetDesiredState(appID string, state string) error {
	_, err := s.db.Exec("UPDATE apps SET desired_state = ? WHERE id = ?", state, appID)
	return err
}

func (s *Store) SetCurrentRelease(appID, releaseID string) error {
	_, err := s.db.Exec("UPDATE apps SET current_release_id = ? WHERE id = ?", releaseID, appID)
	return err
}

func (s *Store) DeleteApp(id string) error {
	_, err := s.db.Exec("DELETE FROM apps WHERE id = ?", id)
	return err
}

func (s *Store) Env(appID string) (map[string]string, error) {
	rows, err := s.db.Query("SELECT key, value FROM env_vars WHERE app_id = ? ORDER BY key", appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	env := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		if env[k], err = s.box.Decrypt(v); err != nil {
			return nil, fmt.Errorf("decrypt env %s: %w", k, err)
		}
	}
	return env, rows.Err()
}

func (s *Store) SetEnv(appID string, env map[string]string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM env_vars WHERE app_id = ?", appID); err != nil {
		return err
	}
	for k, v := range env {
		enc, err := s.box.Encrypt(v)
		if err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO env_vars (app_id, key, value) VALUES (?, ?, ?)", appID, k, enc); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type Domain struct {
	ID    string `json:"id"`
	AppID string `json:"app_id"`
	Host  string `json:"host"`
	Path  string `json:"path"`
	TLS   string `json:"tls"`
}

const (
	TLSNone        = "none"
	TLSLetsEncrypt = "letsencrypt"
	TLSCustom      = "custom"
)

func (s *Store) Domains(appID string) ([]*Domain, error) {
	query := "SELECT id, app_id, host, path, tls FROM domains"
	var args []any
	if appID != "" {
		query += " WHERE app_id = ?"
		args = append(args, appID)
	}
	rows, err := s.db.Query(query+" ORDER BY host, path", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Domain
	for rows.Next() {
		var d Domain
		if err := rows.Scan(&d.ID, &d.AppID, &d.Host, &d.Path, &d.TLS); err != nil {
			return nil, err
		}
		out = append(out, &d)
	}
	return out, rows.Err()
}

func (s *Store) Domain(id string) (*Domain, error) {
	var d Domain
	err := s.db.QueryRow("SELECT id, app_id, host, path, tls FROM domains WHERE id = ?", id).
		Scan(&d.ID, &d.AppID, &d.Host, &d.Path, &d.TLS)
	return &d, notFound(err)
}

func (s *Store) SaveDomain(d *Domain) error {
	if d.ID != "" {
		_, err := s.db.Exec("UPDATE domains SET host = ?, path = ?, tls = ? WHERE id = ?", d.Host, d.Path, d.TLS, d.ID)
		return err
	}
	d.ID = NewID()
	_, err := s.db.Exec("INSERT INTO domains (id, app_id, host, path, tls) VALUES (?, ?, ?, ?, ?)", d.ID, d.AppID, d.Host, d.Path, d.TLS)
	return err
}

func (s *Store) DeleteDomain(id string) error {
	_, err := s.db.Exec("DELETE FROM domains WHERE id = ?", id)
	return err
}

func (s *Store) Setting(key string) (string, error) {
	var v string
	err := s.db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

func (s *Store) Settings() (map[string]string, error) {
	rows, err := s.db.Query("SELECT key, value FROM settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

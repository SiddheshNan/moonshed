package api

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/SiddheshNan/moonshed/internal/backup"
	"github.com/SiddheshNan/moonshed/internal/engine"
	"github.com/SiddheshNan/moonshed/internal/metrics"
	"github.com/SiddheshNan/moonshed/internal/store"
)

var appNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

const maxBundleSize = 4 << 30

type appView struct {
	*store.App
	Status       engine.Status     `json:"status"`
	Stats        metrics.AppStats  `json:"stats"`
	Health       string            `json:"health"`
	HealthCheck  string            `json:"health_check"`
	Deployed     *store.Deployment `json:"deployed"`
	LastDeploy   *store.Deployment `json:"last_deployment"`
	HasRunAsPass bool              `json:"has_run_as_password"`
	Domains      []*domainView     `json:"domains"`
}

func (s *Server) view(app *store.App) (*appView, error) {
	domains, err := s.domainViews(app.ID)
	if err != nil {
		return nil, err
	}
	v := &appView{
		App:          app,
		Status:       s.Deploy.Status(app),
		Stats:        s.Metrics.App(app.ID),
		Health:       s.Monitor.Health(app.ID),
		HasRunAsPass: app.RunAsPassword != "",
		Domains:      domains,
	}
	last, err := s.Store.Deployments(app.ID, 1)
	if err != nil {
		return nil, err
	}
	if len(last) > 0 {
		v.LastDeploy = last[0]
	}
	if app.CurrentReleaseID != "" {
		if rel, err := s.Store.Release(app.CurrentReleaseID); err == nil && rel.Manifest != nil && rel.Manifest.Health != nil {
			if h := rel.Manifest.Health; h.HTTP != "" {
				v.HealthCheck = "HTTP " + h.HTTP
			} else {
				v.HealthCheck = "TCP"
			}
		}
		if d, err := s.Store.DeployedDeployment(app.CurrentReleaseID); err == nil {
			v.Deployed = d
		}
	}
	return v, nil
}

func (s *Server) listApps(w http.ResponseWriter, r *http.Request) error {
	apps, err := s.Store.Apps()
	if err != nil {
		return err
	}
	views := make([]*appView, 0, len(apps))
	for _, app := range apps {
		v, err := s.view(app)
		if err != nil {
			return err
		}
		views = append(views, v)
	}
	return ok(w, views)
}

func (s *Server) getApp(w http.ResponseWriter, r *http.Request) error {
	app, err := s.appFromPath(r)
	if err != nil {
		return err
	}
	v, err := s.view(app)
	if err != nil {
		return err
	}
	return ok(w, struct {
		*appView
		WebhookSecret string `json:"webhook_secret"`
	}{v, app.WebhookSecret})
}

type appInput struct {
	Name                 string  `json:"name"`
	ProjectID            string  `json:"project_id"`
	Runtime              string  `json:"runtime"`
	Port                 int     `json:"port"`
	Source               string  `json:"source"`
	GitURL               string  `json:"git_url"`
	GitBranch            string  `json:"git_branch"`
	GitSubdir            string  `json:"git_subdir"`
	GitCredentialID      string  `json:"git_credential_id"`
	GitHubInstallationID int64   `json:"github_installation_id"`
	Autodeploy           *bool   `json:"autodeploy"`
	RunAsUser            string  `json:"run_as_user"`
	RunAsPassword        *string `json:"run_as_password"`
	KeepBundles          int     `json:"keep_bundles"`
	LogRetentionDays     int     `json:"log_retention_days"`
	AutoRestart          bool    `json:"auto_restart"`
	BackupCron           string  `json:"backup_cron"`
	BackupKeep           int     `json:"backup_keep"`
}

// inputFrom fills an appInput with the app's current values, so an update request only
// has to contain the fields it changes.
func inputFrom(app *store.App) appInput {
	autodeploy := app.Autodeploy
	return appInput{
		ProjectID:            app.ProjectID,
		Runtime:              app.Runtime,
		Port:                 app.Port,
		Source:               app.Source,
		GitURL:               app.GitURL,
		GitBranch:            app.GitBranch,
		GitSubdir:            app.GitSubdir,
		GitCredentialID:      app.GitCredentialID,
		GitHubInstallationID: app.GitHubInstallationID,
		Autodeploy:           &autodeploy,
		RunAsUser:            app.RunAsUser,
		KeepBundles:          app.KeepBundles,
		LogRetentionDays:     app.LogRetentionDays,
		AutoRestart:          app.AutoRestart,
		BackupCron:           app.BackupCron,
		BackupKeep:           app.BackupKeep,
	}
}

// apply copies the editable fields onto app. A nil password keeps the current one.
func (in *appInput) apply(app *store.App) {
	app.Port = in.Port
	app.Source = cmp.Or(in.Source, store.SourceUpload)
	app.GitURL = strings.TrimSpace(in.GitURL)
	app.GitBranch = strings.TrimSpace(in.GitBranch)
	app.GitSubdir = strings.Trim(strings.TrimSpace(in.GitSubdir), "/")
	app.GitCredentialID = in.GitCredentialID
	app.GitHubInstallationID = in.GitHubInstallationID
	if app.GitHubInstallationID != 0 {
		app.GitCredentialID = ""
	}
	app.Autodeploy = in.Autodeploy == nil || *in.Autodeploy
	app.RunAsUser = strings.TrimSpace(in.RunAsUser)
	if in.RunAsPassword != nil {
		app.RunAsPassword = *in.RunAsPassword
	}
	app.KeepBundles = cmp.Or(in.KeepBundles, 5)
	app.LogRetentionDays = cmp.Or(in.LogRetentionDays, 14)
	app.AutoRestart = in.AutoRestart
	app.BackupCron = strings.TrimSpace(in.BackupCron)
	app.BackupKeep = cmp.Or(in.BackupKeep, 7)
}

func validRuntime(r string) bool {
	return r == "" || r == store.RuntimeNative || r == store.RuntimeDocker
}

func (s *Server) validateApp(app *store.App) error {
	if app.Port < 0 || app.Port > 65535 {
		return badRequest("port must be between 1 and 65535")
	}
	if app.Source != store.SourceUpload && app.Source != store.SourceGit {
		return badRequest("source must be upload or git")
	}
	if app.Source == store.SourceGit {
		if !strings.HasPrefix(app.GitURL, "https://") && !strings.HasPrefix(app.GitURL, "http://") {
			return badRequest("git URL must be an http(s) URL")
		}
		if app.GitHubInstallationID != 0 {
			if _, err := s.Store.GitHubInstallation(app.GitHubInstallationID); err != nil {
				return badRequest("github installation not found")
			}
		}
		if app.GitCredentialID != "" {
			if _, err := s.Store.GitCredential(app.GitCredentialID); err != nil {
				return badRequest("git credential not found")
			}
		}
	}
	if app.KeepBundles < 1 || app.LogRetentionDays < 1 || app.BackupKeep < 1 {
		return badRequest("retention values must be at least 1")
	}
	if err := backup.ValidateCron(app.BackupCron); err != nil {
		return badRequest("invalid backup schedule: %v", err)
	}
	if app.Port > 0 {
		apps, err := s.Store.Apps()
		if err != nil {
			return err
		}
		for _, other := range apps {
			if other.ID != app.ID && other.Port == app.Port {
				return badRequest("port %d is already used by %s", app.Port, other.Name)
			}
		}
	}
	return nil
}

func (s *Server) createApp(w http.ResponseWriter, r *http.Request) error {
	var in appInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if !appNamePattern.MatchString(in.Name) {
		return badRequest("name must be lowercase letters, digits and dashes (max 40)")
	}
	if !validRuntime(in.Runtime) {
		return badRequest("runtime must be native, docker, or empty to detect it from the bundle")
	}
	if err := s.checkProject(in.ProjectID); err != nil {
		return err
	}
	app := &store.App{
		Name:          in.Name,
		ProjectID:     in.ProjectID,
		Runtime:       in.Runtime,
		WebhookSecret: randomToken(),
		DesiredState:  store.StateStopped,
	}
	in.apply(app)
	if err := s.validateApp(app); err != nil {
		return err
	}
	if err := s.Store.CreateApp(app); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return errorf(http.StatusConflict, "an app named %s already exists", app.Name)
		}
		return err
	}
	if err := s.Paths.App(app.Name).Init(); err != nil {
		return err
	}
	s.Backups.Reload()
	details := "zip upload"
	if app.Source == store.SourceGit {
		details = app.GitURL
	}
	s.auditApp(r, "app.create", app, details)
	v, err := s.view(app)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusCreated, v)
}

func (s *Server) updateApp(w http.ResponseWriter, r *http.Request) error {
	app, err := s.appFromPath(r)
	if err != nil {
		return err
	}
	in := inputFrom(app)
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.Runtime != app.Runtime {
		if !validRuntime(in.Runtime) {
			return badRequest("runtime must be native, docker, or empty to detect it from the bundle")
		}
		if app.CurrentReleaseID != "" {
			return badRequest("the runtime can only be changed before the first deployment")
		}
	}
	if err := s.checkProject(in.ProjectID); err != nil {
		return err
	}
	before := *app
	app.ProjectID = in.ProjectID
	app.Runtime = in.Runtime
	in.apply(app)
	if err := s.validateApp(app); err != nil {
		return err
	}
	if err := s.Store.UpdateApp(app); err != nil {
		return err
	}
	s.Backups.Reload()
	if err := s.Proxy.Reload(r.Context()); err != nil {
		return err
	}
	s.auditApp(r, "app.update", app, changedFields(&before, app))
	v, err := s.view(app)
	if err != nil {
		return err
	}
	return ok(w, v)
}

// changedFields names the settings that differ, without values, for the audit log.
func changedFields(a, b *store.App) string {
	var changed []string
	check := func(name string, differs bool) {
		if differs {
			changed = append(changed, name)
		}
	}
	check("project", a.ProjectID != b.ProjectID)
	check("runtime", a.Runtime != b.Runtime)
	check("port", a.Port != b.Port)
	check("source", a.Source != b.Source || a.GitURL != b.GitURL || a.GitBranch != b.GitBranch || a.GitSubdir != b.GitSubdir)
	check("git auth", a.GitCredentialID != b.GitCredentialID || a.GitHubInstallationID != b.GitHubInstallationID)
	check("autodeploy", a.Autodeploy != b.Autodeploy)
	check("run as user", a.RunAsUser != b.RunAsUser || a.RunAsPassword != b.RunAsPassword)
	check("retention", a.KeepBundles != b.KeepBundles || a.LogRetentionDays != b.LogRetentionDays)
	check("auto restart", a.AutoRestart != b.AutoRestart)
	check("backups", a.BackupCron != b.BackupCron || a.BackupKeep != b.BackupKeep)
	if len(changed) == 0 {
		return "no changes"
	}
	return "changed " + strings.Join(changed, ", ")
}

func (s *Server) deleteApp(w http.ResponseWriter, r *http.Request) error {
	app, err := s.appFromPath(r)
	if err != nil {
		return err
	}
	if s.Deploy.Deploying(app.ID) {
		return errorf(http.StatusConflict, "kill the running deployment first")
	}
	if err := s.Deploy.Delete(app); err != nil {
		return err
	}
	s.MetricsDB.DeleteTarget(metrics.AppTarget(app.ID))
	s.Backups.Reload()
	s.configureRecording()
	if err := s.Proxy.Reload(r.Context()); err != nil {
		return err
	}
	s.auditApp(r, "app.delete", app, "")
	return noContent(w)
}

func (s *Server) appAction(w http.ResponseWriter, r *http.Request) error {
	app, err := s.appFromPath(r)
	if err != nil {
		return err
	}
	action := r.URL.Path[strings.LastIndexByte(r.URL.Path, '/')+1:]
	switch action {
	case "start":
		err = s.Deploy.Start(app)
	case "stop":
		err = s.Deploy.Stop(app)
	case "restart":
		err = s.Deploy.Restart(app)
	}
	if err != nil {
		var ae *apiError
		if errors.As(err, &ae) || errors.Is(err, store.ErrNotFound) {
			return err
		}
		return errorf(http.StatusConflict, "%v", err)
	}
	s.auditApp(r, "app."+action, app, "")
	return ok(w, map[string]any{"status": s.Deploy.Status(app)})
}

// deployApp accepts a zip as multipart field "bundle" or as a raw application/zip body.
// Without a body it deploys from the app's git repository.
func (s *Server) deployApp(w http.ResponseWriter, r *http.Request) error {
	app, err := s.appFromPath(r)
	if err != nil {
		return err
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBundleSize)
	ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	trigger := store.TriggerUpload
	if currentActor(r).token != "" {
		trigger = store.TriggerAPI
	}

	var d *store.Deployment
	switch ct {
	case "multipart/form-data":
		d, err = s.deployMultipart(r, app, trigger)
	case "application/zip", "application/octet-stream":
		d, err = s.Deploy.DeployUpload(app, r.Body, trigger)
	default:
		if trigger == store.TriggerUpload {
			trigger = store.TriggerManual
		}
		d, err = s.Deploy.DeployGit(app, trigger)
		if err != nil {
			return badRequest("%v", err)
		}
	}
	if err != nil {
		return err
	}
	s.auditApp(r, "app.deploy", app, triggerLabels[trigger])
	return writeJSON(w, http.StatusAccepted, d)
}

func (s *Server) deployMultipart(r *http.Request, app *store.App, trigger string) (*store.Deployment, error) {
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, badRequest("%v", err)
	}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			return nil, badRequest("missing bundle file")
		}
		if err != nil {
			return nil, badRequest("%v", err)
		}
		if part.FormName() == "bundle" {
			return s.Deploy.DeployUpload(app, part, trigger)
		}
	}
}

func (s *Server) listDeployments(w http.ResponseWriter, r *http.Request) error {
	app, err := s.appFromPath(r)
	if err != nil {
		return err
	}
	list, err := s.Store.Deployments(app.ID, 50)
	if err != nil {
		return err
	}
	return ok(w, list)
}

func (s *Server) getDeployment(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	d, err := s.Store.Deployment(id)
	if err != nil {
		return err
	}
	return ok(w, d)
}

func (s *Server) killDeployment(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	d, err := s.Store.Deployment(id)
	if err != nil {
		return err
	}
	if !s.Deploy.Kill(id) {
		return errorf(http.StatusConflict, "deployment is not running")
	}
	if app, err := s.Store.App(d.AppID); err == nil {
		s.auditApp(r, "app.kill", app, "")
	}
	return noContent(w)
}

func (s *Server) listBundles(w http.ResponseWriter, r *http.Request) error {
	app, err := s.appFromPath(r)
	if err != nil {
		return err
	}
	bundles, err := s.Store.Bundles(app.ID)
	if err != nil {
		return err
	}
	return ok(w, bundles)
}

func (s *Server) redeploy(w http.ResponseWriter, r *http.Request) error {
	app, err := s.appFromPath(r)
	if err != nil {
		return err
	}
	bundleID, err := pathID(r, "bundle")
	if err != nil {
		return err
	}
	d, err := s.Deploy.Redeploy(app, bundleID)
	if err != nil {
		return err
	}
	details := "zip upload"
	if b, err := s.Store.Bundle(bundleID); err == nil && b.GitCommit != "" {
		details = fmt.Sprintf("commit %.7s", b.GitCommit)
	}
	s.auditApp(r, "app.redeploy", app, details)
	return writeJSON(w, http.StatusAccepted, d)
}

var envKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func (s *Server) getEnv(w http.ResponseWriter, r *http.Request) error {
	app, err := s.appFromPath(r)
	if err != nil {
		return err
	}
	env, err := s.Store.Env(app.ID)
	if err != nil {
		return err
	}
	return ok(w, env)
}

func (s *Server) setEnv(w http.ResponseWriter, r *http.Request) error {
	app, err := s.appFromPath(r)
	if err != nil {
		return err
	}
	var env map[string]string
	if err := decode(r, &env); err != nil {
		return err
	}
	for k := range env {
		if !envKeyPattern.MatchString(k) {
			return badRequest("invalid variable name %q", k)
		}
	}
	old, err := s.Store.Env(app.ID)
	if err != nil {
		return err
	}
	if err := s.Store.SetEnv(app.ID, env); err != nil {
		return err
	}
	s.auditApp(r, "app.env", app, envChanges(old, env))
	return ok(w, env)
}

// envChanges lists which keys were added, removed or changed. Values are never logged.
func envChanges(old, new map[string]string) string {
	var added, removed, changed []string
	for k, v := range new {
		if ov, ok := old[k]; !ok {
			added = append(added, k)
		} else if ov != v {
			changed = append(changed, k)
		}
	}
	for k := range old {
		if _, ok := new[k]; !ok {
			removed = append(removed, k)
		}
	}
	var parts []string
	for _, p := range []struct {
		label string
		keys  []string
	}{{"added", added}, {"changed", changed}, {"removed", removed}} {
		if len(p.keys) > 0 {
			slices.Sort(p.keys)
			parts = append(parts, p.label+" "+strings.Join(p.keys, ", "))
		}
	}
	if len(parts) == 0 {
		return "no changes"
	}
	return strings.Join(parts, "; ")
}

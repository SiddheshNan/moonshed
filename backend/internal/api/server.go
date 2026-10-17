// Package api serves the JSON API and the web UI.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"moonshed/internal/backup"
	"moonshed/internal/config"
	"moonshed/internal/deploy"
	"moonshed/internal/engine"
	"moonshed/internal/githost"
	"moonshed/internal/health"
	"moonshed/internal/metrics"
	"moonshed/internal/proxy"
	"moonshed/internal/requestlog"
	"moonshed/internal/store"
)

type Deps struct {
	Version   string
	PanelAddr string
	Paths     config.Paths
	Store     *store.Store
	Deploy    *deploy.Manager
	Docker    *engine.Docker
	Metrics   *metrics.Collector
	MetricsDB *metrics.DB
	Monitor   *health.Monitor
	Backups   *backup.Service
	Proxy     *proxy.Proxy
	GitHub    *githost.GitHub
	Requests  *requestlog.Log
}

type Server struct {
	*Deps
	mux       *http.ServeMux
	limiter   *loginLimiter
	manifests manifestStates
}

func New(deps *Deps) *Server {
	s := &Server{Deps: deps, mux: http.NewServeMux(), limiter: newLoginLimiter()}
	s.routes()
	return s
}

// ServeHTTP turns a panic in any handler into a logged 500 instead of a dropped connection.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tw := &trackingWriter{ResponseWriter: w}
	defer func() {
		v := recover()
		if v == nil || v == http.ErrAbortHandler {
			if v != nil {
				panic(v)
			}
			return
		}
		slog.Error("api panic", "method", r.Method, "path", r.URL.Path, "panic", v, "stack", string(debug.Stack()))
		if tw.started {
			// Part of the response is out (e.g. a log stream); cut the connection so the
			// client doesn't take it for a complete response.
			panic(http.ErrAbortHandler)
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}()
	s.mux.ServeHTTP(tw, r)
}

// trackingWriter records whether the response has started. It passes Flush through
// because the log streams check for http.Flusher directly.
type trackingWriter struct {
	http.ResponseWriter
	started bool
}

func (t *trackingWriter) WriteHeader(code int) {
	t.started = true
	t.ResponseWriter.WriteHeader(code)
}

func (t *trackingWriter) Write(b []byte) (int, error) {
	t.started = true
	return t.ResponseWriter.Write(b)
}

func (t *trackingWriter) Flush() {
	if f, ok := t.ResponseWriter.(http.Flusher); ok {
		t.started = true
		f.Flush()
	}
}

func (t *trackingWriter) Unwrap() http.ResponseWriter { return t.ResponseWriter }

func (s *Server) routes() {
	s.public("GET /api/auth/status", s.authStatus)
	s.public("POST /api/auth/setup", s.setup)
	s.public("POST /api/auth/login", s.login)
	s.public("POST /api/hooks/github", s.githubAppWebhook)
	s.public("POST /api/hooks/apps/{id}", s.appWebhook)
	s.private("POST /api/auth/logout", s.logout)

	s.private("GET /api/users", s.listUsers)
	s.private("POST /api/users", s.createUser)
	s.private("PUT /api/users/{id}/password", s.changePassword)
	s.private("DELETE /api/users/{id}", s.deleteUser)
	s.private("GET /api/tokens", s.listTokens)
	s.private("POST /api/tokens", s.createToken)
	s.private("DELETE /api/tokens/{id}", s.deleteToken)

	s.private("GET /api/projects", s.listProjects)
	s.private("POST /api/projects", s.createProject)
	s.private("GET /api/projects/{id}", s.getProject)
	s.private("PUT /api/projects/{id}", s.updateProject)
	s.private("DELETE /api/projects/{id}", s.deleteProject)

	s.private("GET /api/apps", s.listApps)
	s.private("POST /api/apps", s.createApp)
	s.private("GET /api/apps/{id}", s.getApp)
	s.private("PUT /api/apps/{id}", s.updateApp)
	s.private("DELETE /api/apps/{id}", s.deleteApp)
	s.private("POST /api/apps/{id}/start", s.appAction)
	s.private("POST /api/apps/{id}/stop", s.appAction)
	s.private("POST /api/apps/{id}/restart", s.appAction)
	s.private("POST /api/apps/{id}/deploy", s.deployApp)
	s.private("GET /api/apps/{id}/deployments", s.listDeployments)
	s.private("GET /api/apps/{id}/bundles", s.listBundles)
	s.private("POST /api/apps/{id}/bundles/{bundle}/redeploy", s.redeploy)
	s.private("GET /api/apps/{id}/env", s.getEnv)
	s.private("PUT /api/apps/{id}/env", s.setEnv)
	s.private("GET /api/apps/{id}/domains", s.listDomains)
	s.private("POST /api/apps/{id}/domains", s.createDomain)
	s.private("GET /api/apps/{id}/logs", s.appLogs)
	s.private("GET /api/apps/{id}/logs/days", s.appLogDays)
	s.private("GET /api/apps/{id}/logs/stream", s.streamAppLogs)
	s.private("GET /api/apps/{id}/metrics", s.appMetrics)
	s.private("GET /api/apps/{id}/uptime", s.appUptime)
	s.private("GET /api/apps/{id}/backups", s.listAppBackups)
	s.private("POST /api/apps/{id}/backups", s.createAppBackup)

	s.private("GET /api/deployments", s.allDeployments)
	s.private("GET /api/deployments/{id}", s.getDeployment)
	s.private("POST /api/deployments/{id}/kill", s.killDeployment)
	s.private("GET /api/deployments/{id}/log", s.deploymentLog)

	s.private("PUT /api/domains/{id}", s.updateDomain)
	s.private("DELETE /api/domains/{id}", s.deleteDomain)
	s.private("PUT /api/domains/{id}/certificate", s.replaceDomainCertificate)
	s.private("POST /api/domains/{id}/check", s.checkDomainDraft)

	s.private("GET /api/platform/backups", s.listPlatformBackups)
	s.private("POST /api/platform/backups", s.createPlatformBackup)
	s.private("POST /api/backups/{id}/restore", s.restoreBackup)
	s.private("GET /api/backups/{id}/download", s.downloadBackup)
	s.private("DELETE /api/backups/{id}", s.deleteBackup)

	s.private("GET /api/server", s.serverInfo)
	s.private("GET /api/server/metrics", s.serverMetrics)
	s.private("GET /api/settings", s.getSettings)
	s.private("PUT /api/settings", s.updateSettings)
	s.private("POST /api/settings/panel-certificate", s.uploadPanelCertificate)
	s.private("POST /api/settings/proxy-check", s.checkSettingsDraft)
	s.private("GET /api/proxy/caddyfile", s.proxyCaddyfile)

	s.private("GET /api/audit", s.listAudit)

	s.private("GET /api/requests", s.listRequests)
	s.private("GET /api/requests/recording", s.recordingStatus)
	s.private("PUT /api/requests/recording", s.setRecording)

	s.private("GET /api/git", s.gitOverview)
	s.private("POST /api/git/credentials", s.saveCredential)
	s.private("PUT /api/git/credentials/{id}", s.saveCredential)
	s.private("DELETE /api/git/credentials/{id}", s.deleteCredential)
	s.private("POST /api/git/github/manifest", s.githubManifest)
	s.private("GET /api/git/github/callback", s.githubCallback)
	s.private("GET /api/git/github/setup", s.githubSetup)
	s.private("POST /api/git/github/{id}/sync", s.syncGitHub)
	s.private("DELETE /api/git/github/{id}", s.deleteGitHubApp)
	s.private("GET /api/git/installations/{id}/repos", s.githubRepos)
	s.private("GET /api/git/installations/{id}/branches", s.githubBranches)

	s.mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, errorf(http.StatusNotFound, "not found"))
	})
	s.mux.Handle("/", s.web())
}

type handlerFunc func(w http.ResponseWriter, r *http.Request) error

func (s *Server) public(pattern string, h handlerFunc) {
	s.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			writeError(w, err)
		}
	})
}

func (s *Server) private(pattern string, h handlerFunc) {
	s.public(pattern, func(w http.ResponseWriter, r *http.Request) error {
		a, err := s.authenticate(r)
		if err != nil {
			return err
		}
		return h(w, r.WithContext(context.WithValue(r.Context(), actorKey{}, a)))
	})
}

type apiError struct {
	status int
	msg    string
}

func (e *apiError) Error() string { return e.msg }

func errorf(status int, format string, args ...any) error {
	return &apiError{status: status, msg: fmt.Sprintf(format, args...)}
}

func badRequest(format string, args ...any) error {
	return errorf(http.StatusBadRequest, format, args...)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	msg := "internal server error"
	var ae *apiError
	switch {
	case errors.As(err, &ae):
		status, msg = ae.status, ae.msg
	case errors.Is(err, store.ErrNotFound):
		status, msg = http.StatusNotFound, "not found"
	case errors.Is(err, deploy.ErrBusy):
		status, msg = http.StatusConflict, err.Error()
	default:
		slog.Error("api", "err", err)
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(v)
}

func ok(w http.ResponseWriter, v any) error { return writeJSON(w, http.StatusOK, v) }

func noContent(w http.ResponseWriter) error {
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return badRequest("invalid request body: %v", err)
	}
	return nil
}

// pathID returns a UUID path parameter, or a 404 for anything that isn't one.
func pathID(r *http.Request, name string) (string, error) {
	id := r.PathValue(name)
	if _, err := uuid.Parse(id); err != nil {
		return "", errorf(http.StatusNotFound, "not found")
	}
	return id, nil
}

// pathInt returns a numeric path parameter, used for GitHub's own ids.
func pathInt(r *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil {
		return 0, errorf(http.StatusNotFound, "not found")
	}
	return id, nil
}

func (s *Server) appFromPath(r *http.Request) (*store.App, error) {
	id, err := pathID(r, "id")
	if err != nil {
		return nil, err
	}
	return s.Store.App(id)
}

// parseRange accepts 1h, 6h, 24h, 7d, 30d, 90d.
func parseRange(r *http.Request, def time.Duration) time.Duration {
	v := r.URL.Query().Get("range")
	if days, ok := strings.CutSuffix(v, "d"); ok {
		if n, err := strconv.Atoi(days); err == nil && n > 0 && n <= 90 {
			return time.Duration(n) * 24 * time.Hour
		}
	}
	if d, err := time.ParseDuration(v); err == nil && d > 0 && d <= 90*24*time.Hour {
		return d
	}
	return def
}

// web serves the built frontend from runtime/web, falling back to index.html for client routes.
func (s *Server) web() http.Handler {
	root := s.Paths.Web()
	files := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(root, filepath.FromSlash(filepath.Clean("/"+r.URL.Path)))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			if strings.HasPrefix(r.URL.Path, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			files.ServeHTTP(w, r)
			return
		}
		index := filepath.Join(root, "index.html")
		if _, err := os.Stat(index); err != nil {
			http.Error(w, "Web UI is not installed. Put the frontend build in "+root, http.StatusNotFound)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, index)
	})
}

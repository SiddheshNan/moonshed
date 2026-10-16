package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"moonshed/internal/store"
)

func (s *Server) gitOverview(w http.ResponseWriter, r *http.Request) error {
	creds, err := s.Store.GitCredentials()
	if err != nil {
		return err
	}
	apps, err := s.Store.GitHubApps()
	if err != nil {
		return err
	}
	installs, err := s.Store.GitHubInstallations()
	if err != nil {
		return err
	}
	return ok(w, map[string]any{
		"credentials":          creds,
		"github_apps":          apps,
		"github_installations": installs,
	})
}

type credentialInput struct {
	Name     string  `json:"name"`
	Host     string  `json:"host"`
	Username string  `json:"username"`
	Token    *string `json:"token"`
}

func (s *Server) saveCredential(w http.ResponseWriter, r *http.Request) error {
	c := &store.GitCredential{}
	status := http.StatusCreated
	if r.PathValue("id") != "" {
		id, err := pathID(r, "id")
		if err != nil {
			return err
		}
		if c, err = s.Store.GitCredential(id); err != nil {
			return err
		}
		status = http.StatusOK
	}
	var in credentialInput
	if err := decode(r, &in); err != nil {
		return err
	}
	c.Name = strings.TrimSpace(in.Name)
	c.Host = strings.ToLower(strings.TrimSpace(in.Host))
	c.Username = strings.TrimSpace(in.Username)
	if in.Token != nil {
		c.Token = strings.TrimSpace(*in.Token)
	}
	if c.Name == "" || c.Host == "" || c.Username == "" || c.Token == "" {
		return badRequest("name, host, username and token are required")
	}
	if err := s.Store.SaveGitCredential(c); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return errorf(http.StatusConflict, "a credential named %s already exists", c.Name)
		}
		return err
	}
	action := "git.credential.create"
	if status == http.StatusOK {
		action = "git.credential.update"
	}
	s.audit(r, action, "git_credential", c.ID, c.Name, c.Host)
	return writeJSON(w, status, c)
}

func (s *Server) deleteCredential(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	c, err := s.Store.GitCredential(id)
	if err != nil {
		return err
	}
	if err := s.Store.DeleteGitCredential(id); err != nil {
		return err
	}
	s.audit(r, "git.credential.delete", "git_credential", c.ID, c.Name, "")
	return noContent(w)
}

// manifestStates remembers manifest flows started from the panel, so the callback from
// GitHub can be matched to a real request.
type manifestStates struct {
	mu     sync.Mutex
	states map[string]time.Time
}

func (m *manifestStates) issue() string {
	b := make([]byte, 16)
	rand.Read(b)
	state := hex.EncodeToString(b)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.states == nil {
		m.states = map[string]time.Time{}
	}
	for k, exp := range m.states {
		if time.Now().After(exp) {
			delete(m.states, k)
		}
	}
	m.states[state] = time.Now().Add(time.Hour)
	return state
}

func (m *manifestStates) take(state string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	exp, ok := m.states[state]
	delete(m.states, state)
	return ok && time.Now().Before(exp)
}

// baseURL is how the browser reached the panel, which is also where GitHub sends webhooks.
func baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func reachableFromInternet(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		host = r.Host
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast())
	}
	return true
}

// githubManifest starts creating a GitHub App. The browser posts the manifest to GitHub,
// which then redirects back to githubCallback.
func (s *Server) githubManifest(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Organization string `json:"organization"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	base := baseURL(r)
	host, _, _ := strings.Cut(r.Host, ":")
	suffix := make([]byte, 3)
	rand.Read(suffix)
	name := fmt.Sprintf("moonshed-%s-%x", strings.ReplaceAll(host, ".", "-"), suffix)
	if len(name) > 34 {
		name = fmt.Sprintf("moonshed-%x", suffix)
	}
	manifest := map[string]any{
		"name":                name,
		"url":                 base,
		"hook_attributes":     map[string]any{"url": base + "/api/hooks/github", "active": true},
		"redirect_url":        base + "/api/git/github/callback",
		"setup_url":           base + "/api/git/github/setup",
		"setup_on_update":     true,
		"public":              false,
		"default_permissions": map[string]string{"contents": "read", "metadata": "read"},
		"default_events":      []string{"push"},
	}
	data, _ := json.Marshal(manifest)
	state := s.manifests.issue()
	action := "https://github.com/settings/apps/new?state=" + state
	if org := strings.TrimSpace(in.Organization); org != "" {
		action = "https://github.com/organizations/" + url.PathEscape(org) + "/settings/apps/new?state=" + state
	}
	resp := map[string]any{"action": action, "manifest": string(data), "webhook_url": base + "/api/hooks/github"}
	switch {
	case !reachableFromInternet(r):
		resp["warning"] = "GitHub can't reach " + r.Host + ", so pushes won't trigger deployments. Open the panel through its public domain before creating the app."
	case r.TLS == nil:
		resp["warning"] = "The panel is not on HTTPS, so webhook payloads will travel unencrypted. Use the panel's HTTPS domain if you can."
	}
	return ok(w, resp)
}

func redirectToGitSettings(w http.ResponseWriter, r *http.Request, problem string) error {
	target := "/settings/git"
	if problem != "" {
		target += "?error=" + url.QueryEscape(problem)
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
	return nil
}

// githubCallback receives the code GitHub issues after the app was created and exchanges it
// for the app's credentials, then sends the user on to install the app.
func (s *Server) githubCallback(w http.ResponseWriter, r *http.Request) error {
	if !s.manifests.take(r.URL.Query().Get("state")) {
		return redirectToGitSettings(w, r, "The GitHub setup link expired. Start again.")
	}
	app, err := s.GitHub.ConvertManifest(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		return redirectToGitSettings(w, r, err.Error())
	}
	if err := s.Store.CreateGitHubApp(app); err != nil {
		return redirectToGitSettings(w, r, err.Error())
	}
	s.audit(r, "git.github.create", "github_app", strconv.FormatInt(app.ID, 10), app.Name, app.Owner)
	http.Redirect(w, r, app.HTMLURL+"/installations/new", http.StatusSeeOther)
	return nil
}

// githubSetup is where GitHub sends the browser after the app is installed or its access changed.
func (s *Server) githubSetup(w http.ResponseWriter, r *http.Request) error {
	id, err := strconv.ParseInt(r.URL.Query().Get("installation_id"), 10, 64)
	if err != nil {
		return redirectToGitSettings(w, r, "")
	}
	apps, err := s.Store.GitHubApps()
	if err != nil {
		return err
	}
	for _, app := range apps {
		inst, err := s.GitHub.Installation(r.Context(), app, id)
		if err != nil {
			continue
		}
		if err := s.Store.SaveGitHubInstallation(inst); err != nil {
			return redirectToGitSettings(w, r, err.Error())
		}
		s.audit(r, "git.github.install", "github_installation", strconv.FormatInt(inst.ID, 10), inst.Account, app.Name)
		return redirectToGitSettings(w, r, "")
	}
	return redirectToGitSettings(w, r, "That installation doesn't belong to any GitHub App connected here.")
}

// syncGitHub refreshes the list of installations from GitHub.
func (s *Server) syncGitHub(w http.ResponseWriter, r *http.Request) error {
	id, err := pathInt(r, "id")
	if err != nil {
		return err
	}
	app, err := s.Store.GitHubApp(id)
	if err != nil {
		return err
	}
	installs, err := s.GitHub.Installations(r.Context(), app)
	if err != nil {
		return errorf(http.StatusBadGateway, "%v", err)
	}
	current, err := s.Store.GitHubInstallations()
	if err != nil {
		return err
	}
	seen := map[int64]bool{}
	for _, inst := range installs {
		seen[inst.ID] = true
		if err := s.Store.SaveGitHubInstallation(inst); err != nil {
			return err
		}
	}
	for _, inst := range current {
		if inst.GitHubAppID == app.ID && !seen[inst.ID] {
			s.Store.DeleteGitHubInstallation(inst.ID)
		}
	}
	return s.gitOverview(w, r)
}

func (s *Server) deleteGitHubApp(w http.ResponseWriter, r *http.Request) error {
	id, err := pathInt(r, "id")
	if err != nil {
		return err
	}
	app, err := s.Store.GitHubApp(id)
	if err != nil {
		return err
	}
	if err := s.Store.DeleteGitHubApp(id); err != nil {
		return err
	}
	s.audit(r, "git.github.delete", "github_app", strconv.FormatInt(app.ID, 10), app.Name, "")
	return noContent(w)
}

func (s *Server) installationApp(r *http.Request) (*store.GitHubApp, int64, error) {
	id, err := pathInt(r, "id")
	if err != nil {
		return nil, 0, err
	}
	inst, err := s.Store.GitHubInstallation(id)
	if err != nil {
		return nil, 0, err
	}
	app, err := s.Store.GitHubApp(inst.GitHubAppID)
	return app, inst.ID, err
}

func (s *Server) githubRepos(w http.ResponseWriter, r *http.Request) error {
	app, inst, err := s.installationApp(r)
	if err != nil {
		return err
	}
	repos, err := s.GitHub.Repositories(r.Context(), app, inst)
	if err != nil {
		return errorf(http.StatusBadGateway, "%v", err)
	}
	return ok(w, repos)
}

func (s *Server) githubBranches(w http.ResponseWriter, r *http.Request) error {
	app, inst, err := s.installationApp(r)
	if err != nil {
		return err
	}
	repo := r.URL.Query().Get("repo")
	if strings.Count(repo, "/") != 1 {
		return badRequest("repo must be owner/name")
	}
	branches, err := s.GitHub.Branches(r.Context(), app, inst, repo)
	if err != nil {
		return errorf(http.StatusBadGateway, "%v", err)
	}
	return ok(w, branches)
}

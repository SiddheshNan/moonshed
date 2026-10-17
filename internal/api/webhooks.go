package api

import (
	"cmp"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/SiddheshNan/moonshed/internal/store"
)

type push struct {
	Ref     string
	Commit  string
	Message string
	Repo    string // owner/name, lowercase
}

func (p push) branch() string { return strings.TrimPrefix(p.Ref, "refs/heads/") }

func validSignature(secret string, body []byte, header string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(want), []byte(header))
}

// repoPath turns a clone URL like https://github.com/Owner/Repo.git into owner/repo.
func repoPath(cloneURL string) string {
	u := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(cloneURL)), "/")
	u = strings.TrimSuffix(u, ".git")
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
	}
	if _, rest, ok := strings.Cut(u, "/"); ok {
		return rest
	}
	return u
}

func readBody(r *http.Request) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r.Body, 25<<20))
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

// githubAppWebhook handles every event for a GitHub App created here.
func (s *Server) githubAppWebhook(w http.ResponseWriter, r *http.Request) error {
	appID, _ := strconv.ParseInt(r.Header.Get("X-GitHub-Hook-Installation-Target-ID"), 10, 64)
	gh, err := s.Store.GitHubApp(appID)
	if err != nil {
		return errorf(http.StatusNotFound, "unknown github app")
	}
	body, err := readBody(r)
	if err != nil {
		return err
	}
	if !validSignature(gh.WebhookSecret, body, r.Header.Get("X-Hub-Signature-256")) {
		return errorf(http.StatusUnauthorized, "invalid signature")
	}

	var payload struct {
		Action       string `json:"action"`
		Ref          string `json:"ref"`
		After        string `json:"after"`
		Deleted      bool   `json:"deleted"`
		Installation struct {
			ID      int64 `json:"id"`
			Account struct {
				Login string `json:"login"`
				Type  string `json:"type"`
			} `json:"account"`
		} `json:"installation"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
		HeadCommit struct {
			Message string `json:"message"`
		} `json:"head_commit"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return badRequest("invalid payload")
	}

	switch r.Header.Get("X-GitHub-Event") {
	case "ping":
		return ok(w, map[string]string{"status": "pong"})
	case "installation":
		inst := payload.Installation
		if payload.Action == "deleted" {
			s.Store.DeleteGitHubInstallation(inst.ID)
		} else {
			s.Store.SaveGitHubInstallation(&store.GitHubInstallation{
				ID: inst.ID, GitHubAppID: gh.ID, Account: inst.Account.Login, AccountType: inst.Account.Type,
			})
		}
		return ok(w, map[string]string{"status": "ok"})
	case "push":
		if payload.Deleted {
			return ok(w, map[string]string{"status": "ignored", "reason": "branch deleted"})
		}
		p := push{Ref: payload.Ref, Commit: payload.After, Message: firstLine(payload.HeadCommit.Message), Repo: strings.ToLower(payload.Repository.FullName)}
		return s.handlePush(w, r, p, func(app *store.App) bool { return app.GitHubInstallationID == payload.Installation.ID })
	}
	return ok(w, map[string]string{"status": "ignored"})
}

// handlePush deploys every git app watching the pushed repo and branch, or records the
// commit for apps with autodeploy off.
func (s *Server) handlePush(w http.ResponseWriter, r *http.Request, p push, match func(*store.App) bool) error {
	apps, err := s.Store.Apps()
	if err != nil {
		return err
	}
	var deployed, pending []string
	for _, app := range apps {
		if app.Source != store.SourceGit || !match(app) || repoPath(app.GitURL) != p.Repo || cmp.Or(app.GitBranch, "main") != p.branch() {
			continue
		}
		if !s.applyPush(r, app, p, store.TriggerGitHubApp) {
			pending = append(pending, app.Name)
			continue
		}
		deployed = append(deployed, app.Name)
	}
	return writeJSON(w, http.StatusAccepted, map[string]any{"deployed": deployed, "pending": pending})
}

// applyPush deploys the app when autodeploy is on and reports whether it did.
func (s *Server) applyPush(r *http.Request, app *store.App, p push, trigger string) bool {
	if !app.Autodeploy {
		s.Store.SetPushedCommit(app.ID, p.Commit, p.Message)
		return false
	}
	if _, err := s.Deploy.DeployGit(app, trigger); err != nil {
		s.Deploy.AppLog(app.Name).Printf("webhook deploy failed: %v", err)
		return false
	}
	s.auditAs(r, store.ActorWebhook, triggerLabels[trigger], "app.deploy", "app", app.ID, app.Name,
		fmt.Sprintf("push to %s, %.7s", p.branch(), p.Commit))
	return true
}

// appWebhook is the per-app hook URL. It accepts GitHub and GitLab push webhooks signed
// with the app's secret, and a plain POST with ?token=<secret> from any CI, which always deploys.
func (s *Server) appWebhook(w http.ResponseWriter, r *http.Request) error {
	app, err := s.appFromPath(r)
	if err != nil {
		return err
	}
	body, err := readBody(r)
	if err != nil {
		return err
	}
	secret := []byte(app.WebhookSecret)

	switch {
	case r.Header.Get("X-Hub-Signature-256") != "":
		if !validSignature(app.WebhookSecret, body, r.Header.Get("X-Hub-Signature-256")) {
			return errorf(http.StatusUnauthorized, "invalid signature")
		}
		switch r.Header.Get("X-GitHub-Event") {
		case "ping":
			return ok(w, map[string]string{"status": "pong"})
		case "push":
		default:
			return ok(w, map[string]string{"status": "ignored"})
		}
		var payload struct {
			Ref        string `json:"ref"`
			After      string `json:"after"`
			Deleted    bool   `json:"deleted"`
			HeadCommit struct {
				Message string `json:"message"`
			} `json:"head_commit"`
		}
		if err := json.Unmarshal(body, &payload); err != nil || payload.Deleted {
			return ok(w, map[string]string{"status": "ignored"})
		}
		return s.appPush(w, r, app, push{Ref: payload.Ref, Commit: payload.After, Message: firstLine(payload.HeadCommit.Message)}, store.TriggerGitHubWebhook)

	case r.Header.Get("X-Gitlab-Token") != "":
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Gitlab-Token")), secret) != 1 {
			return errorf(http.StatusUnauthorized, "invalid token")
		}
		if r.Header.Get("X-Gitlab-Event") != "Push Hook" {
			return ok(w, map[string]string{"status": "ignored"})
		}
		var payload struct {
			Ref         string `json:"ref"`
			CheckoutSHA string `json:"checkout_sha"`
			Commits     []struct {
				ID      string `json:"id"`
				Message string `json:"message"`
			} `json:"commits"`
		}
		if err := json.Unmarshal(body, &payload); err != nil || payload.CheckoutSHA == "" {
			return ok(w, map[string]string{"status": "ignored"})
		}
		p := push{Ref: payload.Ref, Commit: payload.CheckoutSHA}
		for _, c := range payload.Commits {
			if c.ID == payload.CheckoutSHA {
				p.Message = firstLine(c.Message)
			}
		}
		return s.appPush(w, r, app, p, store.TriggerGitLabWebhook)

	case r.URL.Query().Get("token") != "":
		if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("token")), secret) != 1 {
			return errorf(http.StatusUnauthorized, "invalid token")
		}
		d, err := s.Deploy.DeployGit(app, store.TriggerDeployHook)
		if err != nil {
			return badRequest("%v", err)
		}
		s.auditAs(r, store.ActorWebhook, triggerLabels[store.TriggerDeployHook], "app.deploy", "app", app.ID, app.Name, "")
		return writeJSON(w, http.StatusAccepted, d)
	}
	return errorf(http.StatusUnauthorized, "missing signature or token")
}

func (s *Server) appPush(w http.ResponseWriter, r *http.Request, app *store.App, p push, trigger string) error {
	if app.Source != store.SourceGit {
		return ok(w, map[string]string{"status": "ignored", "reason": "app is not deployed from git"})
	}
	if p.branch() != cmp.Or(app.GitBranch, "main") {
		return ok(w, map[string]string{"status": "ignored", "reason": "different branch"})
	}
	if !s.applyPush(r, app, p, trigger) {
		return ok(w, map[string]string{"status": "pending", "reason": "autodeploy is off"})
	}
	return writeJSON(w, http.StatusAccepted, map[string]string{"status": "deploying"})
}

package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/SiddheshNan/moonshed/internal/store"
)

// audit records an action done by the user or API token behind the request.
func (s *Server) audit(r *http.Request, action, targetType, targetID, targetName, details string) {
	e := &store.AuditEntry{
		ActorType:  store.ActorSystem,
		ActorName:  "system",
		IP:         clientIP(r),
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		TargetName: targetName,
		Details:    details,
	}
	if a := currentActor(r); a.user != nil {
		e.ActorType, e.ActorID, e.ActorName = store.ActorUser, a.user.ID, a.user.Username
		if a.token != "" {
			e.ActorType = store.ActorToken
			e.Details = joinDetails(fmt.Sprintf("token %q", a.token), details)
		}
	}
	s.saveAudit(e)
}

// auditAs records an action by someone who isn't logged in, e.g. a webhook or a failed login.
func (s *Server) auditAs(r *http.Request, actorType, actorName, action, targetType, targetID, targetName, details string) {
	s.saveAudit(&store.AuditEntry{
		ActorType:  actorType,
		ActorName:  actorName,
		IP:         clientIP(r),
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		TargetName: targetName,
		Details:    details,
	})
}

// triggerLabels name deployment triggers in the audit log, the same way the UI does.
var triggerLabels = map[string]string{
	store.TriggerUpload:        "Zip upload",
	store.TriggerAPI:           "API token",
	store.TriggerManual:        "Manual",
	store.TriggerGitHubApp:     "GitHub push",
	store.TriggerGitHubWebhook: "GitHub webhook",
	store.TriggerGitLabWebhook: "GitLab webhook",
	store.TriggerDeployHook:    "Deploy hook",
	store.TriggerRedeploy:      "Redeploy",
	store.TriggerRestore:       "Backup restore",
}

func (s *Server) auditApp(r *http.Request, action string, app *store.App, details string) {
	s.audit(r, action, "app", app.ID, app.Name, details)
}

func (s *Server) saveAudit(e *store.AuditEntry) {
	if err := s.Store.AddAudit(e); err != nil {
		slog.Error("audit", "action", e.Action, "err", err)
	}
}

func joinDetails(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + ", " + b
}

func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	before, _ := strconv.ParseInt(q.Get("before"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	entries, err := s.Store.Audit(store.AuditFilter{
		Actor:      q.Get("actor"),
		Action:     q.Get("action"),
		TargetType: q.Get("target_type"),
		TargetID:   q.Get("target_id"),
		Before:     before,
		Limit:      limit,
	})
	if err != nil {
		return err
	}
	return ok(w, entries)
}

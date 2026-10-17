package api

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/SiddheshNan/moonshed/internal/requestlog"
	"github.com/SiddheshNan/moonshed/internal/store"
)

// configureRecording applies the apps marked for request recording to the request log.
func (s *Server) configureRecording() {
	apps, err := s.Store.Apps()
	if err != nil {
		slog.Error("requests: list apps", "err", err)
		return
	}
	var ids []string
	for _, app := range apps {
		if app.RecordRequests {
			ids = append(ids, app.ID)
		}
	}
	if err := s.Requests.Configure(ids); err != nil {
		slog.Error("requests: configure", "err", err)
	}
}

// ConfigureRecording is called once at startup.
func (s *Server) ConfigureRecording() { s.configureRecording() }

func (s *Server) recordingStatus(w http.ResponseWriter, r *http.Request) error {
	apps, err := s.Store.Apps()
	if err != nil {
		return err
	}
	ids := []string{}
	for _, app := range apps {
		if app.RecordRequests {
			ids = append(ids, app.ID)
		}
	}
	return ok(w, map[string]any{
		"active":  len(ids) > 0,
		"app_ids": ids,
		"counts":  s.Requests.Counts(),
		"cap":     requestlog.PerAppCap,
		"proxy":   s.Proxy.Status().Listening,
	})
}

// setRecording starts, changes or stops recording. An empty app list stops it and
// deletes everything that was recorded.
func (s *Server) setRecording(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		AppIDs []string `json:"app_ids"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	var names []string
	for _, id := range in.AppIDs {
		app, err := s.Store.App(id)
		if err != nil {
			return badRequest("app %s not found", id)
		}
		names = append(names, app.Name)
	}
	if err := s.Store.SetRecordRequests(in.AppIDs); err != nil {
		return err
	}
	s.configureRecording()
	if len(names) == 0 {
		s.audit(r, "requests.stop", "", "", "", "recorded requests discarded")
	} else {
		s.audit(r, "requests.record", "", "", "", strings.Join(names, ", "))
	}
	return s.recordingStatus(w, r)
}

func (s *Server) listRequests(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	before, _ := strconv.ParseInt(q.Get("before"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	entries, err := s.Requests.Query(requestlog.Filter{
		AppID:  q.Get("app"),
		Status: q.Get("status"),
		Method: q.Get("method"),
		Path:   q.Get("path"),
		Before: before,
		Limit:  limit,
	})
	if err != nil {
		return err
	}
	return ok(w, entries)
}

type deploymentRow struct {
	*store.Deployment
	AppName string `json:"app_name"`
}

// allDeployments lists deployments across every app for the Deployments page.
func (s *Server) allDeployments(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	list, err := s.Store.AllDeployments(q.Get("status"), q.Get("before"), limit)
	if err != nil {
		return err
	}
	apps, err := s.Store.Apps()
	if err != nil {
		return err
	}
	names := map[string]string{}
	for _, app := range apps {
		names[app.ID] = app.Name
	}
	rows := make([]deploymentRow, 0, len(list))
	for _, d := range list {
		rows = append(rows, deploymentRow{Deployment: d, AppName: names[d.AppID]})
	}
	return ok(w, rows)
}

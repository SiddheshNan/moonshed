package api

import (
	"net/http"
	"strings"

	"github.com/SiddheshNan/moonshed/internal/engine"
	"github.com/SiddheshNan/moonshed/internal/health"
	"github.com/SiddheshNan/moonshed/internal/store"
)

type projectSummary struct {
	*store.Project
	Apps      int `json:"apps"`
	Running   int `json:"running"`
	Unhealthy int `json:"unhealthy"`
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) error {
	projects, err := s.Store.Projects()
	if err != nil {
		return err
	}
	apps, err := s.Store.Apps()
	if err != nil {
		return err
	}
	byID := map[string]*projectSummary{}
	out := make([]*projectSummary, 0, len(projects))
	for _, p := range projects {
		sum := &projectSummary{Project: p}
		byID[p.ID] = sum
		out = append(out, sum)
	}
	for _, app := range apps {
		sum, ok := byID[app.ProjectID]
		if !ok {
			continue
		}
		sum.Apps++
		running := s.Deploy.Status(app).State == engine.StateRunning
		if running {
			sum.Running++
		}
		if s.needsAttention(app, running) {
			sum.Unhealthy++
		}
	}
	return ok(w, out)
}

// needsAttention matches the problems the UI shows for an app: down, crashed or
// stopped when it should run, or its last deployment failed.
func (s *Server) needsAttention(app *store.App, running bool) bool {
	if running && s.Monitor.Health(app.ID) == health.Down {
		return true
	}
	if !running && app.DesiredState == store.StateRunning {
		return true
	}
	last, err := s.Store.Deployments(app.ID, 1)
	return err == nil && len(last) > 0 && last[0].Status == store.DeployFailed
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	p, err := s.Store.Project(id)
	if err != nil {
		return err
	}
	apps, err := s.Store.ProjectApps(id)
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
	return ok(w, map[string]any{"project": p, "apps": views})
}

type projectInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (in *projectInput) validate() error {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	if in.Name == "" || len(in.Name) > 60 {
		return badRequest("project name is required (max 60 characters)")
	}
	return nil
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) error {
	var in projectInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	p := &store.Project{Name: in.Name, Description: in.Description}
	if err := s.saveProject(p); err != nil {
		return err
	}
	s.audit(r, "project.create", "project", p.ID, p.Name, "")
	return writeJSON(w, http.StatusCreated, p)
}

func (s *Server) updateProject(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	p, err := s.Store.Project(id)
	if err != nil {
		return err
	}
	var in projectInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	details := ""
	if p.Name != in.Name {
		details = "renamed from " + p.Name
	}
	p.Name, p.Description = in.Name, in.Description
	if err := s.saveProject(p); err != nil {
		return err
	}
	s.audit(r, "project.update", "project", p.ID, p.Name, details)
	return ok(w, p)
}

func (s *Server) saveProject(p *store.Project) error {
	if err := s.Store.SaveProject(p); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return errorf(http.StatusConflict, "a project named %s already exists", p.Name)
		}
		return err
	}
	return nil
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	p, err := s.Store.Project(id)
	if err != nil {
		return err
	}
	n, err := s.Store.ProjectAppCount(id)
	if err != nil {
		return err
	}
	if n > 0 {
		return errorf(http.StatusConflict, "move or delete the project's %d apps first", n)
	}
	if err := s.Store.DeleteProject(id); err != nil {
		return err
	}
	s.audit(r, "project.delete", "project", p.ID, p.Name, "")
	return noContent(w)
}

// checkProject makes sure an app's project is given and exists.
func (s *Server) checkProject(id string) error {
	if id == "" {
		return badRequest("project is required")
	}
	if _, err := s.Store.Project(id); err != nil {
		return badRequest("project not found")
	}
	return nil
}

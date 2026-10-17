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
		switch {
		case s.Deploy.Status(app).State == engine.StateRunning:
			sum.Running++
			if s.Monitor.Health(app.ID) == health.Down {
				sum.Unhealthy++
			}
		case app.DesiredState == store.StateRunning:
			sum.Unhealthy++
		}
	}
	return ok(w, out)
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
	projects, err := s.Store.Projects()
	if err != nil {
		return err
	}
	if len(projects) == 1 {
		return errorf(http.StatusConflict, "the last project can't be deleted")
	}
	if err := s.Store.DeleteProject(id); err != nil {
		return err
	}
	s.audit(r, "project.delete", "project", p.ID, p.Name, "")
	return noContent(w)
}

// projectFor returns the requested project, or the first one when none is given.
func (s *Server) projectFor(id string) (string, error) {
	if id != "" {
		if _, err := s.Store.Project(id); err != nil {
			return "", badRequest("project not found")
		}
		return id, nil
	}
	projects, err := s.Store.Projects()
	if err != nil {
		return "", err
	}
	if len(projects) == 0 {
		return "", badRequest("create a project first")
	}
	return projects[0].ID, nil
}

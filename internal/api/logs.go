package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/SiddheshNan/moonshed/internal/logs"
)

const deployLogLines = 5000

func (s *Server) appLogDays(w http.ResponseWriter, r *http.Request) error {
	app, err := s.appFromPath(r)
	if err != nil {
		return err
	}
	days := logs.Days(s.Paths.App(app.Name).Logs(), "app")
	if days == nil {
		days = []string{}
	}
	return ok(w, days)
}

// appLogs returns a full day of logs as plain text, for history and downloads.
func (s *Server) appLogs(w http.ResponseWriter, r *http.Request) error {
	app, err := s.appFromPath(r)
	if err != nil {
		return err
	}
	day := r.URL.Query().Get("date")
	if day == "" {
		day = time.Now().Format("2006-01-02")
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if r.URL.Query().Has("download") {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.log"`, app.Name, day))
	}
	if err := logs.ReadDay(w, s.Paths.App(app.Name).Logs(), "app", day); err != nil {
		return badRequest("%v", err)
	}
	return nil
}

func (s *Server) streamAppLogs(w http.ResponseWriter, r *http.Request) error {
	app, err := s.appFromPath(r)
	if err != nil {
		return err
	}
	recent, ch, cancel := s.Deploy.AppLog(app.Name).Subscribe()
	defer cancel()
	return streamLines(w, r, recent, ch)
}

// deploymentLog streams a running deployment's log, or replays a finished one and ends.
func (s *Server) deploymentLog(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	d, err := s.Store.Deployment(id)
	if err != nil {
		return err
	}
	if live := s.Deploy.DeployLog(id); live != nil {
		recent, ch, cancel := live.Subscribe()
		defer cancel()
		return streamLines(w, r, recent, ch)
	}
	app, err := s.Store.App(d.AppID)
	if err != nil {
		return err
	}
	lines, err := logs.TailFile(s.Paths.App(app.Name).DeployLog(id), deployLogLines)
	if err != nil {
		lines = []string{"log file is not available"}
	}
	return streamLines(w, r, lines, nil)
}

// streamLines writes server-sent events: the backlog, then new lines until the client
// leaves or the channel closes. A nil channel means the backlog is all there is.
func streamLines(w http.ResponseWriter, r *http.Request, backlog []string, ch <-chan string) error {
	flusher, okFlush := w.(http.Flusher)
	if !okFlush {
		return errorf(http.StatusInternalServerError, "streaming is not supported")
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	for _, line := range backlog {
		fmt.Fprintf(w, "data: %s\n\n", line)
	}
	if ch == nil {
		fmt.Fprint(w, "event: end\ndata: \n\n")
		flusher.Flush()
		return nil
	}
	flusher.Flush()

	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return nil
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
		case line, open := <-ch:
			if !open {
				fmt.Fprint(w, "event: end\ndata: \n\n")
				flusher.Flush()
				return nil
			}
			fmt.Fprintf(w, "data: %s\n\n", line)
		}
		flusher.Flush()
	}
}

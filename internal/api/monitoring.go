package api

import (
	"net/http"
	"time"

	"github.com/SiddheshNan/moonshed/internal/health"
	"github.com/SiddheshNan/moonshed/internal/metrics"
	"github.com/SiddheshNan/moonshed/internal/store"
)

var serverMetrics = []string{"cpu", "mem", "disk", "net_in", "net_out", "load"}

func (s *Server) serverInfo(w http.ResponseWriter, r *http.Request) error {
	dockerVersion, err := s.Docker.Version(r.Context())
	return ok(w, map[string]any{
		"version":        s.Version,
		"build":          s.Build,
		"stats":          s.Metrics.Server(),
		"docker":         err == nil,
		"docker_version": dockerVersion,
		"proxy":          s.Proxy.Status(),
		"panel_addr":     s.PanelAddr,
		"runtime_dir":    s.Paths.Root,
	})
}

func (s *Server) serverMetrics(w http.ResponseWriter, r *http.Request) error {
	series, err := s.MetricsDB.Series("server", serverMetrics, chartRange(r))
	if err != nil {
		return err
	}
	return ok(w, series)
}

func (s *Server) appMetrics(w http.ResponseWriter, r *http.Request) error {
	app, err := s.appFromPath(r)
	if err != nil {
		return err
	}
	span := chartRange(r)
	series, err := s.MetricsDB.Series(metrics.AppTarget(app.ID), []string{"cpu", "mem", "health_ms"}, span)
	if err != nil {
		return err
	}
	domains, err := s.Store.Domains(app.ID)
	if err != nil {
		return err
	}
	traffic := map[string]map[string][]metrics.Point{}
	for _, d := range domains {
		if _, seen := traffic[d.Host]; seen {
			continue
		}
		if traffic[d.Host], err = s.MetricsDB.Series(metrics.DomainTarget(d.Host), []string{"requests", "4xx", "5xx", "latency"}, span); err != nil {
			return err
		}
	}
	return ok(w, map[string]any{"app": series, "traffic": traffic})
}

func chartRange(r *http.Request) time.Duration {
	return min(parseRange(r, time.Hour), metrics.MaxRange)
}

// appUptime is built only from health checks that actually ran. Periods without
// checks (not deployed, stopped, platform down) are reported as no data, not as up.
func (s *Server) appUptime(w http.ResponseWriter, r *http.Request) error {
	app, err := s.appFromPath(r)
	if err != nil {
		return err
	}
	counts, err := s.Store.Uptime(app.ID, time.Now().Add(-health.UptimeRetention))
	if err != nil {
		return err
	}
	incidents, err := s.Store.Incidents(app.ID, time.Now().Add(-30*24*time.Hour))
	if err != nil {
		return err
	}

	end := time.Now().Truncate(time.Hour).Add(time.Hour)
	ratio := func(from, to time.Time) *float64 {
		var passed, total int
		for _, c := range counts {
			if c.Hour >= from.Unix() && c.Hour < to.Unix() {
				passed, total = passed+c.OK, total+c.Total
			}
		}
		if total == 0 {
			return nil
		}
		v := float64(passed) / float64(total) * 100
		return &v
	}

	span := min(parseRange(r, 24*time.Hour), health.UptimeRetention)
	size := time.Hour
	if span > 24*time.Hour {
		size = 24 * time.Hour
	}
	n := int(span / size)
	bars := make([]*float64, n)
	for i := range bars {
		start := end.Add(-time.Duration(n-i) * size)
		bars[i] = ratio(start, start.Add(size))
	}

	return ok(w, map[string]any{
		"uptime_24h": ratio(end.Add(-24*time.Hour), end),
		"uptime_7d":  ratio(end.Add(-7*24*time.Hour), end),
		"bars":       bars,
		"bars_start": end.Add(-time.Duration(n) * size),
		"bar_span":   size.Seconds(),
		"incidents":  append([]*store.Incident{}, incidents[:min(len(incidents), 50)]...),
		"health":     s.Monitor.Health(app.ID),
	})
}

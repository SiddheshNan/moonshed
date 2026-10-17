package health

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/SiddheshNan/moonshed/internal/engine"
	"github.com/SiddheshNan/moonshed/internal/logs"
	"github.com/SiddheshNan/moonshed/internal/manifest"
	"github.com/SiddheshNan/moonshed/internal/store"
)

const (
	tick            = 5 * time.Second
	processInterval = 30 * time.Second
	postDeployDelay = 5 * time.Second
)

// Apps is the slice of the deploy manager the monitor needs.
type Apps interface {
	Status(*store.App) engine.Status
	Restart(*store.App) error
	Deploying(appID string) bool
	AppLog(app string) *logs.Stream
}

type Recorder interface {
	Record(target, metric string, value float64)
}

// Monitor keeps running each app's health check after deploy and records up/down incidents.
type Monitor struct {
	store    *store.Store
	apps     Apps
	recorder Recorder
	target   func(appID string) string

	mu     sync.Mutex
	ctx    context.Context
	states map[string]*state
}

type state struct {
	release   string
	lastCheck time.Time
	failures  int
	down      bool
	checking  bool
	checked   bool
}

func NewMonitor(st *store.Store, apps Apps, recorder Recorder, target func(string) string) *Monitor {
	return &Monitor{store: st, apps: apps, recorder: recorder, target: target, ctx: context.Background(), states: map[string]*state{}}
}

func (m *Monitor) Run(ctx context.Context) {
	m.mu.Lock()
	m.ctx = ctx
	m.mu.Unlock()
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	prune := time.NewTicker(time.Hour)
	defer prune.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.tick(ctx)
		case <-prune.C:
			if err := m.store.PruneUptime(time.Now().Add(-UptimeRetention)); err != nil {
				slog.Error("health: prune uptime", "err", err)
			}
		}
	}
}

// CheckAfterDeploy runs one health check 5s after a successful deploy, so the app's
// status is known right away instead of after a full interval. Only for apps with a
// health check in app.yaml.
func (m *Monitor) CheckAfterDeploy(appID string) {
	time.AfterFunc(postDeployDelay, func() {
		app, err := m.store.App(appID)
		if err != nil {
			return
		}
		h, _, ok := m.checkConfig(app)
		if !ok || h == nil {
			return
		}
		if s := m.claim(app, true); s != nil {
			m.mu.Lock()
			ctx := m.ctx
			m.mu.Unlock()
			m.check(ctx, app, h, s)
		}
	})
}

// checkConfig returns the app's health check (nil means "process is alive") and how often
// it runs, or false when the app isn't something we monitor right now.
func (m *Monitor) checkConfig(app *store.App) (*manifest.Health, time.Duration, bool) {
	if app.DesiredState != store.StateRunning || app.CurrentReleaseID == "" || m.apps.Deploying(app.ID) {
		return nil, 0, false
	}
	rel, err := m.store.Release(app.CurrentReleaseID)
	if err != nil {
		return nil, 0, false
	}
	if rel.Manifest != nil && rel.Manifest.Health != nil {
		return rel.Manifest.Health, rel.Manifest.Health.Interval, true
	}
	return nil, processInterval, true
}

// claim marks a check as started and returns the app's state, or nil if one is already
// running. The state resets when a new release is deployed.
func (m *Monitor) claim(app *store.App, force bool) *state {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.states[app.ID]
	if !ok || s.release != app.CurrentReleaseID {
		s = &state{release: app.CurrentReleaseID, down: ok && s.down, checked: ok && s.checked}
		if ok {
			// A new release: the post-deploy check runs first, then the normal schedule.
			s.lastCheck = time.Now()
		}
		m.states[app.ID] = s
	}
	if s.checking {
		return nil
	}
	if !force {
		return s
	}
	s.checking, s.lastCheck = true, time.Now()
	return s
}

func (m *Monitor) tick(ctx context.Context) {
	apps, err := m.store.Apps()
	if err != nil {
		slog.Error("health: list apps", "err", err)
		return
	}
	active := map[string]bool{}
	for _, app := range apps {
		h, interval, ok := m.checkConfig(app)
		if !ok {
			continue
		}
		active[app.ID] = true
		s := m.claim(app, false)
		if s == nil {
			continue
		}
		m.mu.Lock()
		due := !s.checking && time.Since(s.lastCheck) >= interval
		if due {
			s.checking, s.lastCheck = true, time.Now()
		}
		m.mu.Unlock()
		if due {
			go m.check(ctx, app, h, s)
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for id, s := range m.states {
		if !active[id] && !s.checking {
			if s.down {
				m.store.CloseIncident(id)
			}
			delete(m.states, id)
		}
	}
}

func (m *Monitor) check(ctx context.Context, app *store.App, h *manifest.Health, s *state) {
	retries := 1
	var err error
	if h == nil {
		if st := m.apps.Status(app); st.State != engine.StateRunning {
			err = errors.New("process is not running")
		}
	} else {
		retries = h.Retries
		var latency time.Duration
		if latency, err = Probe(ctx, app.Port, h); err == nil {
			m.recorder.Record(m.target(app.ID), "health_ms", float64(latency.Microseconds())/1000)
		}
	}

	if err := m.store.AddUptime(app.ID, time.Now(), err == nil); err != nil {
		slog.Error("health: save check", "app", app.Name, "err", err)
	}
	m.mu.Lock()
	s.checking, s.checked = false, true
	wasDown := s.down
	if err == nil {
		s.failures, s.down = 0, false
	} else {
		s.failures++
		if s.failures >= retries {
			s.down = true
		}
	}
	nowDown := s.down
	m.mu.Unlock()

	log := m.apps.AppLog(app.Name)
	switch {
	case wasDown && !nowDown:
		log.Printf("health: app is up again")
		if err := m.store.CloseIncident(app.ID); err != nil {
			slog.Error("close incident", "app", app.Name, "err", err)
		}
	case !wasDown && nowDown:
		log.Printf("health: app is down: %v", err)
		if err := m.store.OpenIncident(app.ID, err.Error()); err != nil {
			slog.Error("open incident", "app", app.Name, "err", err)
		}
		if app.AutoRestart {
			log.Printf("health: auto-restarting")
			if err := m.apps.Restart(app); err != nil {
				log.Printf("health: restart failed: %v", err)
			}
		}
	}
}

const (
	Up      = "up"
	Down    = "down"
	Unknown = "unknown"
)

// Health is the app's current health: unknown until a check has actually run.
func (m *Monitor) Health(appID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.states[appID]
	switch {
	case !ok || !s.checked:
		return Unknown
	case s.down:
		return Down
	default:
		return Up
	}
}

const UptimeRetention = 7 * 24 * time.Hour

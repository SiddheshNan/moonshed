// Package deploy turns bundles into running releases and owns the app lifecycle.
package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/SiddheshNan/moonshed/internal/config"
	"github.com/SiddheshNan/moonshed/internal/engine"
	"github.com/SiddheshNan/moonshed/internal/githost"
	"github.com/SiddheshNan/moonshed/internal/logs"
	"github.com/SiddheshNan/moonshed/internal/store"
)

var ErrBusy = errors.New("a deployment for this app is in progress")

type Manager struct {
	paths  config.Paths
	store  *store.Store
	native *engine.Native
	docker *engine.Docker
	github *githost.GitHub
	queue  chan *job

	// OnDeployed is called after a deployment succeeds.
	OnDeployed func(appID string)

	mu       sync.Mutex
	jobs     map[string]*job
	appLocks map[string]*sync.Mutex
	appLogs  map[string]*logs.Stream
	wg       sync.WaitGroup
}

type job struct {
	deployment *store.Deployment
	log        *logs.Stream
	ctx        context.Context
	cancel     context.CancelFunc
}

func NewManager(paths config.Paths, st *store.Store, native *engine.Native, docker *engine.Docker, github *githost.GitHub) *Manager {
	return &Manager{
		paths:    paths,
		store:    st,
		native:   native,
		docker:   docker,
		github:   github,
		queue:    make(chan *job, 256),
		jobs:     map[string]*job{},
		appLocks: map[string]*sync.Mutex{},
		appLogs:  map[string]*logs.Stream{},
	}
}

// Run starts the build workers and brings up every app that should be running.
func (m *Manager) Run(ctx context.Context, workers int) error {
	if err := m.store.FailUnfinishedDeployments(); err != nil {
		return err
	}
	for range workers {
		m.wg.Go(func() {
			for {
				select {
				case <-ctx.Done():
					return
				case j := <-m.queue:
					m.run(j)
				}
			}
		})
	}
	apps, err := m.store.Apps()
	if err != nil {
		return err
	}
	for _, app := range apps {
		m.sweepReleases(app)
		if app.DesiredState != store.StateRunning || app.CurrentReleaseID == "" {
			continue
		}
		if err := m.boot(app); err != nil {
			slog.Error("start app", "app", app.Name, "err", err)
			m.AppLog(app.Name).Printf("start failed: %v", err)
		}
	}
	return nil
}

// sweepReleases removes release folders left behind by deployments interrupted by a restart.
func (m *Manager) sweepReleases(app *store.App) {
	entries, _ := os.ReadDir(m.paths.App(app.Name).Releases())
	for _, e := range entries {
		if e.Name() != app.CurrentReleaseID {
			os.RemoveAll(filepath.Join(m.paths.App(app.Name).Releases(), e.Name()))
		}
	}
	m.store.DeleteOtherReleases(app.ID, app.CurrentReleaseID)
}

func (m *Manager) boot(app *store.App) error {
	if app.Runtime == store.RuntimeDocker && m.docker.Status(app.Name).State == engine.StateRunning {
		m.docker.Watch(app.Name, m.AppLog(app.Name))
		return nil
	}
	return m.startRelease(app, app.CurrentReleaseID)
}

// Shutdown kills in-flight deployments and stops native apps. Docker containers keep running.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	for _, j := range m.jobs {
		j.cancel()
	}
	m.mu.Unlock()
	m.wg.Wait()
	m.native.StopAll()
	m.docker.Unwatch()
	m.mu.Lock()
	for _, s := range m.appLogs {
		s.Close()
	}
	m.mu.Unlock()
}

func (m *Manager) engineFor(app *store.App) engine.Engine {
	if app.Runtime == store.RuntimeDocker {
		return m.docker
	}
	return m.native
}

func (m *Manager) appLock(appID string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.appLocks[appID]
	if !ok {
		l = &sync.Mutex{}
		m.appLocks[appID] = l
	}
	return l
}

// AppLog is the long-lived runtime log for an app.
func (m *Manager) AppLog(app string) *logs.Stream {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.appLogs[app]
	if !ok {
		s = logs.NewStream(m.paths.App(app).Logs(), "app")
		m.appLogs[app] = s
	}
	return s
}

func (m *Manager) Status(app *store.App) engine.Status {
	return m.engineFor(app).Status(app.Name)
}

// DeployUpload stores an uploaded zip as a new bundle and queues a deployment for it.
func (m *Manager) DeployUpload(app *store.App, r io.Reader, trigger string) (*store.Deployment, error) {
	ap := m.paths.App(app.Name)
	if err := ap.Init(); err != nil {
		return nil, err
	}
	b := &store.Bundle{AppID: app.ID, Source: store.SourceUpload}
	if err := m.store.CreateBundle(b); err != nil {
		return nil, err
	}
	size, err := writeFile(ap.Bundle(b.ID), r)
	if err == nil {
		err = m.store.SetBundleSize(b.ID, size)
	}
	if err != nil {
		m.store.DeleteBundle(b.ID)
		os.Remove(ap.Bundle(b.ID))
		return nil, fmt.Errorf("save bundle: %w", err)
	}
	return m.enqueue(app, b.ID, trigger)
}

// DeployGit queues a deployment that clones the app's repository first.
func (m *Manager) DeployGit(app *store.App, trigger string) (*store.Deployment, error) {
	if app.Source != store.SourceGit || app.GitURL == "" {
		return nil, errors.New("app has no git repository configured")
	}
	if err := m.paths.App(app.Name).Init(); err != nil {
		return nil, err
	}
	return m.enqueue(app, "", trigger)
}

// Redeploy rebuilds a previously stored bundle, which is how rollback works.
func (m *Manager) Redeploy(app *store.App, bundleID string) (*store.Deployment, error) {
	b, err := m.store.Bundle(bundleID)
	if err != nil {
		return nil, err
	}
	if b.AppID != app.ID {
		return nil, store.ErrNotFound
	}
	return m.enqueue(app, b.ID, store.TriggerRedeploy)
}

func (m *Manager) enqueue(app *store.App, bundleID, trigger string) (*store.Deployment, error) {
	d := &store.Deployment{AppID: app.ID, BundleID: bundleID, Trigger: trigger}
	if err := m.store.CreateDeployment(d); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	j := &job{
		deployment: d,
		log:        logs.NewFile(m.paths.App(app.Name).DeployLog(d.ID)),
		ctx:        ctx,
		cancel:     cancel,
	}
	j.log.Printf("deployment queued (%s)", trigger)

	m.mu.Lock()
	m.jobs[d.ID] = j
	m.mu.Unlock()

	select {
	case m.queue <- j:
		return d, nil
	default:
		m.finishJob(j, store.DeployFailed, errors.New("build queue is full"))
		return nil, errors.New("build queue is full")
	}
}

// Kill cancels a queued or running deployment.
func (m *Manager) Kill(deploymentID string) bool {
	m.mu.Lock()
	j, ok := m.jobs[deploymentID]
	m.mu.Unlock()
	if ok {
		j.log.Printf("kill requested")
		j.cancel()
	}
	return ok
}

// Deploying reports whether a deployment for the app is queued or running.
func (m *Manager) Deploying(appID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		if j.deployment.AppID == appID {
			return true
		}
	}
	return false
}

// DeployLog returns the live stream for an active deployment, or nil when it has finished.
func (m *Manager) DeployLog(deploymentID string) *logs.Stream {
	m.mu.Lock()
	defer m.mu.Unlock()
	if j, ok := m.jobs[deploymentID]; ok {
		return j.log
	}
	return nil
}

func (m *Manager) Start(app *store.App) error {
	l := m.appLock(app.ID)
	if !l.TryLock() {
		return ErrBusy
	}
	defer l.Unlock()
	if app.CurrentReleaseID == "" {
		return errors.New("app has not been deployed yet")
	}
	if err := m.store.SetDesiredState(app.ID, store.StateRunning); err != nil {
		return err
	}
	if m.Status(app).State == engine.StateRunning {
		return nil
	}
	return m.startRelease(app, app.CurrentReleaseID)
}

func (m *Manager) Stop(app *store.App) error {
	l := m.appLock(app.ID)
	if !l.TryLock() {
		return ErrBusy
	}
	defer l.Unlock()
	if err := m.store.SetDesiredState(app.ID, store.StateStopped); err != nil {
		return err
	}
	m.AppLog(app.Name).Printf("stopping")
	return m.engineFor(app).Stop(app.Name)
}

// SwitchRuntime moves a deployed app between native and Docker. The running release can't
// be run by the other engine, so the app is stopped and the release dropped; the next
// deploy builds it for the new runtime.
func (m *Manager) SwitchRuntime(app *store.App, runtime string) error {
	l := m.appLock(app.ID)
	if !l.TryLock() {
		return ErrBusy
	}
	defer l.Unlock()
	if app.CurrentReleaseID != "" {
		if err := m.engineFor(app).Stop(app.Name); err != nil {
			return err
		}
		if rel, err := m.store.Release(app.CurrentReleaseID); err == nil {
			m.removeRelease(app, rel)
		}
		if err := m.store.SetCurrentRelease(app.ID, ""); err != nil {
			return err
		}
		if err := m.store.SetDesiredState(app.ID, store.StateStopped); err != nil {
			return err
		}
		m.AppLog(app.Name).Printf("runtime changed to %s; deploy to start it again", runtime)
	}
	app.Runtime, app.CurrentReleaseID, app.DesiredState = runtime, "", store.StateStopped
	return m.store.SetRuntime(app.ID, runtime)
}

func (m *Manager) Restart(app *store.App) error {
	l := m.appLock(app.ID)
	if !l.TryLock() {
		return ErrBusy
	}
	defer l.Unlock()
	if app.CurrentReleaseID == "" {
		return errors.New("app has not been deployed yet")
	}
	if err := m.store.SetDesiredState(app.ID, store.StateRunning); err != nil {
		return err
	}
	m.AppLog(app.Name).Printf("restarting")
	if err := m.engineFor(app).Stop(app.Name); err != nil {
		return err
	}
	return m.startRelease(app, app.CurrentReleaseID)
}

// Delete stops the app and removes everything the platform stored for it.
func (m *Manager) Delete(app *store.App) error {
	l := m.appLock(app.ID)
	if !l.TryLock() {
		return ErrBusy
	}
	defer l.Unlock()
	if err := m.engineFor(app).Stop(app.Name); err != nil {
		return err
	}
	if app.Runtime == store.RuntimeDocker && app.CurrentReleaseID != "" {
		if r, err := m.store.Release(app.CurrentReleaseID); err == nil && r.Image != "" {
			m.docker.RemoveImage(r.Image)
		}
	}
	if err := m.store.DeleteApp(app.ID); err != nil {
		return err
	}
	m.mu.Lock()
	if s, ok := m.appLogs[app.Name]; ok {
		s.Close()
		delete(m.appLogs, app.Name)
	}
	m.mu.Unlock()
	os.RemoveAll(m.paths.App(app.Name).Root)
	os.RemoveAll(m.paths.AppBackups(app.Name))
	return nil
}

func writeFile(path string, r io.Reader) (int64, error) {
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(f, r)
	if err != nil {
		f.Close()
		return n, err
	}
	return n, f.Close()
}

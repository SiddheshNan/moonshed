package deploy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/SiddheshNan/moonshed/internal/archive"
	"github.com/SiddheshNan/moonshed/internal/engine"
	"github.com/SiddheshNan/moonshed/internal/health"
	"github.com/SiddheshNan/moonshed/internal/logs"
	"github.com/SiddheshNan/moonshed/internal/manifest"
	"github.com/SiddheshNan/moonshed/internal/proc"
	"github.com/SiddheshNan/moonshed/internal/store"
)

func (m *Manager) run(j *job) {
	err := m.execute(j)
	switch {
	case err == nil:
		m.finishJob(j, store.DeploySuccess, nil)
		if m.OnDeployed != nil {
			m.OnDeployed(j.deployment.AppID)
		}
	case j.ctx.Err() != nil:
		m.finishJob(j, store.DeployKilled, errors.New("killed"))
	default:
		m.finishJob(j, store.DeployFailed, err)
	}
}

func (m *Manager) finishJob(j *job, status string, err error) {
	d := j.deployment
	d.Status, d.FinishedAt = status, store.Now()
	if err != nil {
		d.Error = err.Error()
		j.log.Printf("deployment %s: %v", status, err)
	} else {
		j.log.Printf("deployment succeeded")
	}
	if err := m.store.UpdateDeployment(d); err != nil {
		slog.Error("update deployment", "id", d.ID, "err", err)
	}
	m.mu.Lock()
	delete(m.jobs, d.ID)
	m.mu.Unlock()
	j.log.Close()
	j.cancel()
}

func (m *Manager) execute(j *job) error {
	if err := j.ctx.Err(); err != nil {
		return err
	}
	d := j.deployment
	lock := m.appLock(d.AppID)
	lock.Lock()
	defer lock.Unlock()

	app, err := m.store.App(d.AppID)
	if err != nil {
		return err
	}
	ap := m.paths.App(app.Name)
	d.Status, d.StartedAt = store.DeployBuilding, store.Now()
	if err := m.store.UpdateDeployment(d); err != nil {
		return err
	}

	if d.BundleID == "" {
		b, err := m.cloneBundle(j.ctx, app, d, j.log)
		if err != nil {
			return err
		}
		d.BundleID = b.ID
	} else if b, err := m.store.Bundle(d.BundleID); err == nil && d.GitCommit == "" {
		d.GitCommit = b.GitCommit
	}

	rel := &store.Release{AppID: app.ID, BundleID: d.BundleID}
	if err := m.store.CreateRelease(rel); err != nil {
		return err
	}
	d.ReleaseID = rel.ID
	if err := m.store.UpdateDeployment(d); err != nil {
		return err
	}
	dir := ap.Release(rel.ID)
	keep := false
	defer func() {
		if !keep {
			m.removeRelease(app, rel)
		}
	}()

	j.log.Printf("unpacking the bundle")
	if err := archive.Unzip(ap.Bundle(d.BundleID), dir); err != nil {
		return err
	}
	// The release keeps moonshed.yaml as found in the bundle; what runs is that file with
	// the app's UI settings on top.
	file, err := manifest.Load(dir)
	if errors.Is(err, manifest.ErrNotFound) {
		file, err = nil, nil
	}
	if err != nil {
		return err
	}
	rel.Manifest = file
	mf := manifest.Effective(file, app.Config)
	if err := m.resolveRuntime(app, dir, mf, j.log); err != nil {
		return err
	}
	if app.Runtime == store.RuntimeNative {
		if err := mf.ValidateNative(); err != nil {
			return err
		}
	}
	if err := m.applyManifestPort(app, file, j.log); err != nil {
		return err
	}
	if !mf.Check().ProcessOnly() && app.Port == 0 {
		return errors.New("the health check needs a port: set port in moonshed.yaml or on the app's Overview")
	}

	if err := m.preparePersist(app, dir, mf, j.log); err != nil {
		return fmt.Errorf("persist: %w", err)
	}
	if err := m.build(j, app, rel, mf); err != nil {
		return err
	}
	if err := m.store.UpdateRelease(rel); err != nil {
		return err
	}
	if err := j.ctx.Err(); err != nil {
		return err
	}

	d.Status = store.DeployStarting
	if err := m.store.UpdateDeployment(d); err != nil {
		return err
	}
	eng := m.engineFor(app)
	previous := app.CurrentReleaseID
	if previous != "" {
		j.log.Printf("stopping the previous release")
	}
	if err := eng.Stop(app.Name); err != nil {
		return err
	}
	j.log.Printf("starting the new release")
	err = m.startRelease(app, rel.ID)
	if err == nil {
		err = m.waitHealthy(j.ctx, app, mf.Check(), j.log)
	}
	if err != nil {
		eng.Stop(app.Name)
		if previous != "" && app.DesiredState == store.StateRunning {
			j.log.Printf("rolling back to the previous release")
			if err := m.startRelease(app, previous); err != nil {
				j.log.Printf("rollback failed: %v", err)
			}
		}
		return err
	}
	keep = true

	if err := m.store.SetCurrentRelease(app.ID, rel.ID); err != nil {
		return err
	}
	if err := m.store.SetDesiredState(app.ID, store.StateRunning); err != nil {
		return err
	}
	if d.GitCommit != "" && d.GitCommit == app.PushedCommit {
		m.store.SetPushedCommit(app.ID, "", "")
	}
	if app.Runtime == store.RuntimeNative {
		if post := mf.Post.Current(); post != "" {
			j.log.Printf("running post")
			if err := m.runScript(j.ctx, app, rel.ID, post, j.log); err != nil {
				j.log.Printf("warning: post failed: %v", err)
			}
		}
	}
	if previous != "" {
		if old, err := m.store.Release(previous); err == nil {
			m.removeRelease(app, old)
		}
	}
	m.pruneBundles(app, d.BundleID)
	return nil
}

// resolveRuntime decides native or Docker on the first deploy of an app created without
// one: a run command means native, otherwise a Dockerfile means Docker.
func (m *Manager) resolveRuntime(app *store.App, dir string, mf *manifest.Manifest, log *logs.Stream) error {
	if app.Runtime != "" {
		return nil
	}
	dockerfile := mf.Build.DockerfilePath()
	_, statErr := os.Stat(filepath.Join(dir, filepath.FromSlash(dockerfile)))
	hasDockerfile := statErr == nil
	switch {
	case mf.Run.Current() != "":
		app.Runtime = store.RuntimeNative
		if hasDockerfile {
			log.Printf("found a run command and %s; running natively (pick Docker in the app's Build settings to use the Dockerfile)", dockerfile)
		} else {
			log.Printf("found a run command, running natively")
		}
	case hasDockerfile:
		if _, err := m.docker.Version(context.Background()); err != nil {
			return fmt.Errorf("found %s, but Docker isn't installed or running on this server; install Docker, or set a run command to run natively", dockerfile)
		}
		app.Runtime = store.RuntimeDocker
		log.Printf("found %s, running in Docker", dockerfile)
	default:
		return fmt.Errorf("nothing to run: no run command and no %s; set one in %s or in the app's Build settings", dockerfile, manifest.FileName)
	}
	return m.store.SetRuntime(app.ID, app.Runtime)
}

// applyManifestPort uses moonshed.yaml's port when the app has none set in the UI.
// A port set in the UI always wins.
func (m *Manager) applyManifestPort(app *store.App, mf *manifest.Manifest, log *logs.Stream) error {
	if mf == nil || mf.Port == 0 || mf.Port == app.Port {
		return nil
	}
	if app.Port != 0 {
		log.Printf("%s asks for port %d, but the app is set to %d; using %d", manifest.FileName, mf.Port, app.Port, app.Port)
		return nil
	}
	apps, err := m.store.Apps()
	if err != nil {
		return err
	}
	for _, other := range apps {
		if other.ID != app.ID && other.Port == mf.Port {
			return fmt.Errorf("%s asks for port %d, which %s already uses", manifest.FileName, mf.Port, other.Name)
		}
	}
	if err := m.store.SetAppPort(app.ID, mf.Port); err != nil {
		return err
	}
	app.Port = mf.Port
	log.Printf("using port %d from %s", mf.Port, manifest.FileName)
	return nil
}

func (m *Manager) build(j *job, app *store.App, rel *store.Release, mf *manifest.Manifest) error {
	dir := m.paths.App(app.Name).Release(rel.ID)
	if app.Runtime == store.RuntimeDocker {
		if _, err := m.docker.Version(j.ctx); err != nil {
			return err
		}
		opts := engine.BuildOptions{
			Folder:     filepath.Join(dir, filepath.FromSlash(mf.Build.FolderPath())),
			Dockerfile: filepath.Join(dir, filepath.FromSlash(mf.Build.DockerfilePath())),
			Tag:        fmt.Sprintf("moonshed/%s:%s", app.Name, rel.ID),
		}
		if mf.Build != nil {
			opts.Stage, opts.Args = mf.Build.Stage, mf.Build.Args
		}
		if _, err := os.Stat(opts.Dockerfile); err != nil {
			return fmt.Errorf("%s not found in the bundle", mf.Build.DockerfilePath())
		}
		rel.Image = opts.Tag
		j.log.Printf("building image %s from %s", rel.Image, mf.Build.DockerfilePath())
		if err := m.docker.Build(j.ctx, opts, j.log); err != nil {
			return fmt.Errorf("docker build: %w", err)
		}
		return nil
	}

	if app.RunAsUser != "" {
		if err := m.grant(app, dir); err != nil {
			return err
		}
	}
	if pre := mf.Pre.Current(); pre != "" {
		j.log.Printf("running pre")
		if err := m.runScript(j.ctx, app, rel.ID, pre, j.log); err != nil {
			return fmt.Errorf("pre: %w", err)
		}
	}
	return nil
}

func (m *Manager) grant(app *store.App, releaseDir string) error {
	u, err := proc.LookupUser(app.RunAsUser, app.RunAsPassword)
	if err != nil {
		return fmt.Errorf("run as %s: %w", app.RunAsUser, err)
	}
	defer u.Close()
	ap := m.paths.App(app.Name)
	return u.Grant(releaseDir, ap.Shared(), ap.Cache())
}

func (m *Manager) runScript(ctx context.Context, app *store.App, releaseID string, script string, log *logs.Stream) error {
	env, err := m.env(app, releaseID)
	if err != nil {
		return err
	}
	opts := proc.Options{Dir: m.paths.App(app.Name).Release(releaseID), Env: env, Stdout: log, Stderr: log}
	if app.RunAsUser != "" {
		if opts.User, err = proc.LookupUser(app.RunAsUser, app.RunAsPassword); err != nil {
			return err
		}
	}
	return proc.Run(ctx, script, opts)
}

// waitHealthy decides whether a freshly started release is good enough to keep.
func (m *Manager) waitHealthy(ctx context.Context, app *store.App, h manifest.Health, log *logs.Stream) error {
	eng := m.engineFor(app)
	if h.ProcessOnly() {
		log.Printf("no health check configured, making sure the app stays up for 5s")
		if err := sleep(ctx, 5*time.Second); err != nil {
			return err
		}
		if st := eng.Status(app.Name); st.State != engine.StateRunning || st.Restarts > 0 {
			return errors.New("app exited during startup, check the app logs")
		}
		return nil
	}

	timeout := max(30*time.Second, h.Grace.D()+h.Interval.D()*time.Duration(h.Failures))
	deadline := time.Now().Add(timeout)
	log.Printf("waiting up to %s for the health check to pass", timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if _, lastErr = health.Probe(ctx, app.Port, h); lastErr == nil {
			log.Printf("health check passed")
			return nil
		}
		if err := sleep(ctx, 2*time.Second); err != nil {
			return err
		}
	}
	return fmt.Errorf("health check failed: %w", lastErr)
}

func sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

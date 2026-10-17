package deploy

import (
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/SiddheshNan/moonshed/internal/archive"
	"github.com/SiddheshNan/moonshed/internal/engine"
	"github.com/SiddheshNan/moonshed/internal/logs"
	"github.com/SiddheshNan/moonshed/internal/manifest"
	"github.com/SiddheshNan/moonshed/internal/store"
)

func (m *Manager) startRelease(app *store.App, releaseID string) error {
	rel, err := m.store.Release(releaseID)
	if err != nil {
		return fmt.Errorf("release %s: %w", releaseID, err)
	}
	env, err := m.env(app, rel.ID)
	if err != nil {
		return err
	}
	ap := m.paths.App(app.Name)
	spec := engine.Spec{
		App:     app.Name,
		Release: rel.ID,
		Dir:     ap.Release(rel.ID),
		Port:    app.Port,
		Env:     env,
		Log:     m.AppLog(app.Name),
	}
	if app.Runtime == store.RuntimeDocker {
		spec.Image = rel.Image
		if rel.Manifest != nil && len(rel.Manifest.Persist) > 0 {
			wd, err := m.docker.WorkDir(rel.Image)
			if err != nil {
				return err
			}
			spec.Mounts = engine.MountsFor(existing(ap.Shared(), rel.Manifest.Persist), ap.Shared(), wd)
		}
	} else {
		if rel.Manifest == nil || rel.Manifest.Run.Current() == "" {
			return errors.New("this release has no run script for this OS")
		}
		spec.Run = rel.Manifest.Run.Current()
		spec.RunAs, spec.RunAsPassword = app.RunAsUser, app.RunAsPassword
	}
	spec.Log.Printf("starting the app")
	return m.engineFor(app).Start(spec)
}

// env is what every script and the running app see, on top of the base environment.
func (m *Manager) env(app *store.App, releaseID string) ([]string, error) {
	vars, err := m.store.Env(app.ID)
	if err != nil {
		return nil, err
	}
	env := []string{"APP_NAME=" + app.Name, "RELEASE_ID=" + releaseID}
	if app.Port > 0 {
		env = append(env, "PORT="+strconv.Itoa(app.Port))
	}
	if app.Runtime == store.RuntimeNative {
		ap := m.paths.App(app.Name)
		env = append(env, "APP_DIR="+ap.Release(releaseID), "SHARED_DIR="+ap.Shared(), "CACHE_DIR="+ap.Cache())
	}
	for _, k := range slices.Sorted(maps.Keys(vars)) {
		env = append(env, k+"="+vars[k])
	}
	return env, nil
}

// preparePersist seeds shared/ from the bundle on first deploy and, for native apps,
// links each persisted path from the release into shared/.
func (m *Manager) preparePersist(app *store.App, dir string, mf *manifest.Manifest, log *logs.Stream) error {
	if mf == nil {
		return nil
	}
	shared := m.paths.App(app.Name).Shared()
	for _, p := range mf.Persist {
		isDir := strings.HasSuffix(p, "/")
		rel := filepath.FromSlash(strings.TrimSuffix(p, "/"))
		inRelease, inShared := filepath.Join(dir, rel), filepath.Join(shared, rel)

		if _, err := os.Lstat(inShared); errors.Is(err, os.ErrNotExist) {
			_, statErr := os.Lstat(inRelease)
			switch {
			case statErr == nil:
				log.Printf("persist: seeding %s from the bundle", p)
				if err := archive.CopyPath(inRelease, inShared); err != nil {
					return err
				}
			case isDir:
				if err := os.MkdirAll(inShared, 0o755); err != nil {
					return err
				}
			default:
				log.Printf("persist: %s is not in shared/ or the bundle yet, skipping", p)
				continue
			}
		} else if err != nil {
			return err
		}

		if app.Runtime == store.RuntimeDocker {
			continue
		}
		if err := os.RemoveAll(inRelease); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(inRelease), 0o755); err != nil {
			return err
		}
		if err := os.Symlink(inShared, inRelease); err != nil {
			return err
		}
	}
	return nil
}

func existing(shared string, persist []string) []string {
	var out []string
	for _, p := range persist {
		if _, err := os.Stat(filepath.Join(shared, filepath.FromSlash(strings.TrimSuffix(p, "/")))); err == nil {
			out = append(out, p)
		}
	}
	return out
}

func (m *Manager) removeRelease(app *store.App, rel *store.Release) {
	if err := os.RemoveAll(m.paths.App(app.Name).Release(rel.ID)); err != nil {
		slog.Warn("remove release", "app", app.Name, "release", rel.ID, "err", err)
	}
	if rel.Image != "" {
		m.docker.RemoveImage(rel.Image)
	}
	m.store.DeleteRelease(rel.ID)
}

// pruneBundles keeps the newest KeepBundles bundles plus the one currently deployed.
func (m *Manager) pruneBundles(app *store.App, current string) {
	bundles, err := m.store.Bundles(app.ID)
	if err != nil {
		return
	}
	keep := max(app.KeepBundles, 1)
	for i, b := range bundles {
		if i < keep || b.ID == current {
			continue
		}
		os.Remove(m.paths.App(app.Name).Bundle(b.ID))
		m.store.DeleteBundle(b.ID)
	}
}

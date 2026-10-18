package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/SiddheshNan/moonshed/internal/archive"
	"github.com/SiddheshNan/moonshed/internal/engine"
	"github.com/SiddheshNan/moonshed/internal/manifest"
	"github.com/SiddheshNan/moonshed/internal/store"
)

// An app backup unpacks to a folder named after the app that looks like the app itself:
// the deployed code, each persisted path in its real place, and a .env with its env vars.
// Moonshed's own files sit in a hidden .moonshed folder:
//
//	myapp/
//	  app.py, moonshed.yaml…     code
//	  data/                      persisted paths
//	  .env                       env vars
//	  .moonshed/logs/            app logs and the deploy log
//	  .moonshed/backup.json      Meta
const metaDir = ".moonshed"

// Meta describes an app backup.
type Meta struct {
	App       string            `json:"app"`
	CreatedAt time.Time         `json:"created_at"`
	Parts     store.BackupParts `json:"parts"`
	Release   string            `json:"release,omitempty"`
	Commit    string            `json:"commit,omitempty"`
	EnvFile   string            `json:"env_file,omitempty"`
	Data      []string          `json:"data,omitempty"` // persisted paths, relative to the app folder
}

// BackupApp starts an app backup with the chosen parts in the background.
func (s *Service) BackupApp(app *store.App, parts store.BackupParts) (*store.Backup, error) {
	if !parts.Any() {
		return nil, errors.New("pick at least one thing to back up")
	}
	if parts.Code && app.CurrentReleaseID == "" {
		return nil, errors.New("the app has no deployed code to back up")
	}
	if !s.claim(app.ID) {
		return nil, errors.New("a backup for this app is already running")
	}
	dir := s.paths.AppBackups(app.Name)
	b := &store.Backup{AppID: app.ID, Parts: parts, File: filepath.Join(dir, fmt.Sprintf("%s_%s.tar.gz", app.Name, timestamp()))}
	if err := s.start(b, dir); err != nil {
		s.release(app.ID)
		return nil, err
	}
	go func() {
		defer s.release(app.ID)
		s.finish(b, s.write(b.File, func(f *os.File) error { return s.writeApp(f, app, parts) }))
		s.prune(app.ID, app.BackupKeep)
	}()
	return b, nil
}

func (s *Service) writeApp(w io.Writer, app *store.App, parts store.BackupParts) error {
	ap := s.paths.App(app.Name)
	var rel *store.Release
	if app.CurrentReleaseID != "" {
		rel, _ = s.store.Release(app.CurrentReleaseID)
	}
	persist := s.persisted(app, rel)
	meta := Meta{App: app.Name, CreatedAt: time.Now().UTC(), Parts: parts, EnvFile: ".env"}
	tw := archive.NewTarGzWriter(w)
	root := app.Name

	if parts.Code {
		if rel == nil {
			return errors.New("the app has no deployed code to back up")
		}
		tmp, err := os.MkdirTemp(ap.Root, "backup-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		// The temp folder becomes the archive's top folder, so give it normal permissions.
		if err := os.Chmod(tmp, 0o755); err != nil {
			return err
		}
		if err := archive.Unzip(ap.Bundle(rel.BundleID), tmp); err != nil {
			return fmt.Errorf("code: %w", err)
		}
		// With data included, persisted paths come from shared/, not the bundle.
		skip := func(r string) bool { return parts.Data && under(r, persist) }
		if err := tw.AddPath(tmp, root, skip); err != nil {
			return fmt.Errorf("code: %w", err)
		}
		if _, err := os.Stat(filepath.Join(tmp, ".env")); err == nil {
			meta.EnvFile = ".env.moonshed"
		}
		meta.Release = rel.ID
		if d, err := s.store.DeployedDeployment(rel.ID); err == nil {
			meta.Commit = d.GitCommit
		}
	}

	if parts.Data {
		for _, p := range persist {
			p = strings.TrimSuffix(p, "/")
			src := filepath.Join(ap.Shared(), filepath.FromSlash(p))
			if _, err := os.Lstat(src); err != nil {
				continue
			}
			if err := tw.AddPath(src, path.Join(root, p), nil); err != nil {
				return fmt.Errorf("data: %w", err)
			}
			meta.Data = append(meta.Data, p)
		}
	}

	if parts.Env {
		env, err := s.store.Env(app.ID)
		if err != nil {
			return err
		}
		if err := tw.AddFile(path.Join(root, meta.EnvFile), formatEnv(env), 0o600); err != nil {
			return err
		}
	} else {
		meta.EnvFile = ""
	}

	if parts.Logs {
		if err := tw.AddPath(ap.Logs(), path.Join(root, metaDir, "logs"), nil); err != nil {
			return fmt.Errorf("logs: %w", err)
		}
		if rel != nil {
			if d, err := s.store.DeployedDeployment(rel.ID); err == nil {
				if err := tw.AddPath(ap.DeployLog(d.ID), path.Join(root, metaDir, "logs", "deploy.log"), nil); err != nil && !errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("logs: %w", err)
				}
			}
		}
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	if err := tw.AddFile(path.Join(root, metaDir, "backup.json"), data, 0o644); err != nil {
		return err
	}
	return tw.Close()
}

// persisted lists the app's persisted paths: from its running release, else its UI
// settings, else whatever is in shared/.
func (s *Service) persisted(app *store.App, rel *store.Release) []string {
	if rel != nil {
		return manifest.Effective(rel.Manifest, app.Config).Persist
	}
	if app.Config.Persist != nil {
		return *app.Config.Persist
	}
	entries, _ := os.ReadDir(s.paths.App(app.Name).Shared())
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// under reports whether the slash-separated path p is one of paths or inside one of them.
func under(p string, paths []string) bool {
	for _, q := range paths {
		q = strings.TrimSuffix(q, "/")
		if p == q || strings.HasPrefix(p, q+"/") {
			return true
		}
	}
	return false
}

// RestoreApp puts back the chosen parts of a backup: data replaces the persisted paths,
// env replaces the env vars, and code is redeployed as a new deployment. The app is
// stopped while its data is replaced and runs again afterwards if it was running.
func (s *Service) RestoreApp(app *store.App, b *store.Backup, parts store.BackupParts) error {
	if b.AppID != app.ID || b.Status != store.BackupSuccess {
		return errors.New("backup can't be restored")
	}
	if !parts.Code && !parts.Data && !parts.Env {
		return errors.New("pick what to restore")
	}
	if !s.claim(app.ID) {
		return errors.New("a backup for this app is running")
	}
	defer s.release(app.ID)

	ap := s.paths.App(app.Name)
	staging, err := os.MkdirTemp(ap.Root, "restore-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if err := untar(b.File, staging); err != nil {
		return err
	}
	root, meta, err := openBackup(staging)
	if err != nil {
		return err
	}
	for name, want := range map[string]bool{"code": parts.Code && !meta.Parts.Code, "data": parts.Data && !meta.Parts.Data, "env vars": parts.Env && !meta.Parts.Env} {
		if want {
			return fmt.Errorf("this backup has no %s", name)
		}
	}

	running := s.apps.Status(app).State == engine.StateRunning
	if parts.Data {
		if running {
			if err := s.apps.Stop(app); err != nil {
				return err
			}
		}
		if err := replaceShared(ap.Shared(), root, meta.Data, staging); err != nil {
			return fmt.Errorf("data: %w", err)
		}
	}
	if parts.Env {
		data, err := os.ReadFile(filepath.Join(root, meta.EnvFile))
		if err != nil {
			return fmt.Errorf("env vars: %w", err)
		}
		if err := s.store.SetEnv(app.ID, parseEnv(string(data))); err != nil {
			return err
		}
	}
	if parts.Code {
		return s.redeployCode(app, root, meta, staging)
	}
	switch {
	case parts.Data && running:
		return s.apps.Start(app)
	case parts.Env && running:
		return s.apps.Restart(app)
	}
	return nil
}

// redeployCode zips the backup's code, without its data, env file and .moonshed folder,
// and deploys it.
func (s *Service) redeployCode(app *store.App, root string, meta *Meta, staging string) error {
	zipPath := filepath.Join(staging, "code.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer f.Close()
	skip := func(r string) bool { return r == metaDir || r == meta.EnvFile || under(r, meta.Data) }
	if err := archive.ZipFunc(f, root, skip); err != nil {
		return fmt.Errorf("code: %w", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err = s.apps.DeployUpload(app, f, store.TriggerRestore)
	return err
}

// replaceShared swaps shared/ for the persisted paths found in the backup.
func replaceShared(shared, root string, data []string, staging string) error {
	next := filepath.Join(staging, "shared")
	if err := os.MkdirAll(next, 0o755); err != nil {
		return err
	}
	for _, p := range data {
		dst := filepath.Join(next, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.Rename(filepath.Join(root, filepath.FromSlash(p)), dst); err != nil {
			return err
		}
	}
	old := shared + ".old"
	os.RemoveAll(old)
	if err := os.Rename(shared, old); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(next, shared); err != nil {
		os.Rename(old, shared)
		return err
	}
	return os.RemoveAll(old)
}

// openBackup finds the app folder and its backup.json in an extracted backup. Backups
// from before backups had parts only held shared/.
func openBackup(staging string) (string, *Meta, error) {
	entries, err := os.ReadDir(staging)
	if err != nil {
		return "", nil, err
	}
	for _, e := range entries {
		root := filepath.Join(staging, e.Name())
		data, err := os.ReadFile(filepath.Join(root, metaDir, "backup.json"))
		if err != nil {
			continue
		}
		var meta Meta
		if err := json.Unmarshal(data, &meta); err != nil {
			return "", nil, fmt.Errorf("backup.json: %w", err)
		}
		return root, &meta, nil
	}
	root := filepath.Join(staging, "shared")
	old, err := os.ReadDir(root)
	if err != nil {
		return "", nil, errors.New("not a Moonshed backup")
	}
	meta := &Meta{Parts: store.BackupParts{Data: true}}
	for _, e := range old {
		meta.Data = append(meta.Data, e.Name())
	}
	return root, meta, nil
}

func untar(file, dest string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	return archive.UntarGz(f, dest)
}

// formatEnv writes env vars as a .env file, quoting values that need it.
func formatEnv(env map[string]string) []byte {
	var b strings.Builder
	for _, k := range slices.Sorted(maps.Keys(env)) {
		v := env[k]
		if strings.ContainsAny(v, " \t\n\r\"'#$\\=") {
			v = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "$", `\$`).Replace(v) + `"`
		}
		fmt.Fprintf(&b, "%s=%s\n", k, v)
	}
	return []byte(b.String())
}

// parseEnv reads a .env file written by formatEnv, or a hand-written one.
func parseEnv(text string) map[string]string {
	env := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(strings.TrimPrefix(line, "export "), "=")
		if !ok {
			continue
		}
		switch {
		case len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"':
			v = unquote(v[1 : len(v)-1])
		case len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'':
			v = v[1 : len(v)-1]
		}
		env[strings.TrimSpace(k)] = v
	}
	return env
}

func unquote(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i == len(s)-1 {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

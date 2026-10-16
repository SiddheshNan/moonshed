// Package backup archives each app's shared/ folder and the platform database.
package backup

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"moonshed/internal/archive"
	"moonshed/internal/config"
	"moonshed/internal/engine"
	"moonshed/internal/store"
)

const (
	SettingCron = "backup_cron"
	SettingKeep = "backup_keep"
)

// Apps is the slice of the deploy manager backups need for restores.
type Apps interface {
	Status(*store.App) engine.Status
	Start(*store.App) error
	Stop(*store.App) error
}

type Service struct {
	paths config.Paths
	store *store.Store
	apps  Apps
	cron  *cron.Cron

	mu      sync.Mutex
	entries []cron.EntryID
	running map[string]bool
}

func NewService(paths config.Paths, st *store.Store, apps Apps) *Service {
	return &Service{
		paths:   paths,
		store:   st,
		apps:    apps,
		cron:    cron.New(),
		running: map[string]bool{},
	}
}

func (s *Service) Start() error {
	if err := s.store.FailRunningBackups(); err != nil {
		return err
	}
	s.cron.Start()
	return s.Reload()
}

func (s *Service) Stop() { <-s.cron.Stop().Done() }

func ValidateCron(spec string) error {
	if spec == "" {
		return nil
	}
	_, err := cron.ParseStandard(spec)
	return err
}

// Reload re-reads every schedule from the database.
func (s *Service) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range s.entries {
		s.cron.Remove(id)
	}
	s.entries = nil

	apps, err := s.store.Apps()
	if err != nil {
		return err
	}
	for _, app := range apps {
		if app.BackupCron == "" {
			continue
		}
		appID := app.ID
		if err := s.schedule(app.BackupCron, func() { s.runScheduledApp(appID) }); err != nil {
			slog.Error("backup schedule", "app", app.Name, "err", err)
		}
	}
	spec, err := s.store.Setting(SettingCron)
	if err != nil {
		return err
	}
	if spec != "" {
		if err := s.schedule(spec, func() {
			_, err := s.BackupPlatform()
			s.logErr(err)
		}); err != nil {
			slog.Error("platform backup schedule", "err", err)
		}
	}
	return nil
}

func (s *Service) schedule(spec string, fn func()) error {
	id, err := s.cron.AddFunc(spec, fn)
	if err != nil {
		return err
	}
	s.entries = append(s.entries, id)
	return nil
}

func (s *Service) runScheduledApp(appID string) {
	app, err := s.store.App(appID)
	if err != nil {
		return
	}
	_, err = s.BackupApp(app)
	s.logErr(err)
}

func (s *Service) logErr(err error) {
	if err != nil {
		slog.Error("backup", "err", err)
	}
}

// BackupApp starts archiving the app's shared/ folder in the background.
func (s *Service) BackupApp(app *store.App) (*store.Backup, error) {
	if !s.claim(app.ID) {
		return nil, errors.New("a backup for this app is already running")
	}
	dir := s.paths.AppBackups(app.Name)
	b := &store.Backup{AppID: app.ID, File: filepath.Join(dir, timestamp()+".tar.gz")}
	if err := s.start(b, dir); err != nil {
		s.release(app.ID)
		return nil, err
	}
	go func() {
		defer s.release(app.ID)
		root := s.paths.App(app.Name).Root
		s.finish(b, s.write(b.File, func(f *os.File) error { return archive.TarGz(f, root, "shared") }))
		s.prune(app.ID, app.BackupKeep)
	}()
	return b, nil
}

// BackupPlatform snapshots the database (with VACUUM INTO, safe while running) and the
// secret key that decrypts it.
func (s *Service) BackupPlatform() (*store.Backup, error) {
	if !s.claim("") {
		return nil, errors.New("a platform backup is already running")
	}
	defer s.release("")
	dir := s.paths.PlatformBackups()
	b := &store.Backup{File: filepath.Join(dir, timestamp()+".tar.gz")}
	if err := s.start(b, dir); err != nil {
		return nil, err
	}
	err := s.write(b.File, func(f *os.File) error {
		tmp, err := os.MkdirTemp(s.paths.Data(), "backup-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		if _, err := s.store.DB().Exec("VACUUM INTO ?", filepath.Join(tmp, "moonshed.db")); err != nil {
			return err
		}
		if err := archive.CopyPath(s.paths.SecretKey(), filepath.Join(tmp, "secret.key")); err != nil {
			return err
		}
		return archive.TarGz(f, tmp, "moonshed.db", "secret.key")
	})
	s.finish(b, err)
	keep, _ := s.store.Setting(SettingKeep)
	n, convErr := strconv.Atoi(keep)
	if convErr != nil {
		n = 7
	}
	s.prune("", n)
	return b, err
}

// RestoreApp replaces the app's shared/ folder with the backup's contents.
// The app is stopped during the restore and started again if it was running.
func (s *Service) RestoreApp(app *store.App, b *store.Backup) error {
	if b.AppID != app.ID || b.Status != store.BackupSuccess {
		return errors.New("backup can't be restored")
	}
	if !s.claim(app.ID) {
		return errors.New("a backup for this app is running")
	}
	defer s.release(app.ID)

	wasRunning := s.apps.Status(app).State == engine.StateRunning
	if wasRunning {
		if err := s.apps.Stop(app); err != nil {
			return err
		}
	}
	shared := s.paths.App(app.Name).Shared()
	staging, err := os.MkdirTemp(s.paths.App(app.Name).Root, "restore-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)

	err = func() error {
		f, err := os.Open(b.File)
		if err != nil {
			return err
		}
		defer f.Close()
		if err := archive.UntarGz(f, staging); err != nil {
			return err
		}
		old := shared + ".old"
		os.RemoveAll(old)
		if err := os.Rename(shared, old); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.Rename(filepath.Join(staging, "shared"), shared); err != nil {
			os.Rename(old, shared)
			return err
		}
		return os.RemoveAll(old)
	}()
	if wasRunning {
		if startErr := s.apps.Start(app); err == nil {
			err = startErr
		}
	}
	return err
}

func (s *Service) Delete(b *store.Backup) error {
	if b.Status == store.BackupRunning {
		return errors.New("backup is still running")
	}
	if err := os.Remove(b.File); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return s.store.DeleteBackup(b.ID)
}

func (s *Service) start(b *store.Backup, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return s.store.CreateBackup(b)
}

func (s *Service) write(path string, fn func(*os.File) error) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := fn(f); err != nil {
		f.Close()
		os.Remove(path)
		return err
	}
	return f.Close()
}

func (s *Service) finish(b *store.Backup, err error) {
	b.Status = store.BackupSuccess
	if err != nil {
		b.Status, b.Error = store.BackupFailed, err.Error()
	} else if info, statErr := os.Stat(b.File); statErr == nil {
		b.Size = info.Size()
	}
	if err := s.store.UpdateBackup(b); err != nil {
		slog.Error("update backup", "id", b.ID, "err", err)
	}
}

// prune keeps the newest successful backups and drops older ones and old failures.
func (s *Service) prune(appID string, keep int) {
	if keep <= 0 {
		return
	}
	backups, err := s.store.Backups(appID)
	if err != nil {
		return
	}
	kept := 0
	for _, b := range backups {
		if b.Status == store.BackupRunning {
			continue
		}
		if b.Status == store.BackupSuccess && kept < keep {
			kept++
			continue
		}
		if time.Since(b.CreatedAt.Time) < 24*time.Hour && b.Status == store.BackupFailed {
			continue
		}
		s.Delete(b)
	}
}

func (s *Service) claim(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running[id] {
		return false
	}
	s.running[id] = true
	return true
}

func (s *Service) release(id string) {
	s.mu.Lock()
	delete(s.running, id)
	s.mu.Unlock()
}

func timestamp() string {
	return time.Now().Format("20060102-150405")
}

// FileName is the download name for a backup.
func FileName(appName string, b *store.Backup) string {
	if appName == "" {
		appName = "platform"
	}
	return fmt.Sprintf("%s-%s", appName, filepath.Base(b.File))
}

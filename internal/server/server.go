// Package server wires the store, deploy manager, proxy, monitoring and panel together.
package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/SiddheshNan/moonshed/internal/api"
	"github.com/SiddheshNan/moonshed/internal/backup"
	"github.com/SiddheshNan/moonshed/internal/config"
	"github.com/SiddheshNan/moonshed/internal/deploy"
	"github.com/SiddheshNan/moonshed/internal/engine"
	"github.com/SiddheshNan/moonshed/internal/githost"
	"github.com/SiddheshNan/moonshed/internal/health"
	"github.com/SiddheshNan/moonshed/internal/logger"
	"github.com/SiddheshNan/moonshed/internal/logs"
	"github.com/SiddheshNan/moonshed/internal/metrics"
	"github.com/SiddheshNan/moonshed/internal/proc"
	"github.com/SiddheshNan/moonshed/internal/proxy"
	"github.com/SiddheshNan/moonshed/internal/requestlog"
	"github.com/SiddheshNan/moonshed/internal/secret"
	"github.com/SiddheshNan/moonshed/internal/store"
	"github.com/SiddheshNan/moonshed/internal/version"
)

const auditRetention = 365 * 24 * time.Hour

type Server struct {
	cfg     config.Config
	paths   config.Paths
	store   *store.Store
	metrics *metrics.DB
	deps    *api.Deps
	panel   *api.Server
	closers []io.Closer
}

// New opens the databases and builds every component. Nothing runs until Run.
func New(cfg config.Config, paths config.Paths) (*Server, error) {
	if err := paths.Init(); err != nil {
		return nil, err
	}
	s := &Server{cfg: cfg, paths: paths}
	platformLog := logs.NewStream(paths.Logs(), "moonshed")
	s.closers = append(s.closers, platformLog)
	level, _ := logger.ParseLevel(cfg.LogLevel) // validated by config.Load
	logger.Init(level, platformLog)

	if err := s.open(); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func (s *Server) open() error {
	if err := proc.Init(); err != nil {
		return err
	}
	devUI, err := s.cfg.DevUI()
	if err != nil {
		return err
	}
	box, err := secret.LoadOrCreate(s.paths.SecretKey())
	if err != nil {
		return err
	}
	if s.store, err = store.Open(s.paths.DB(), box); err != nil {
		return err
	}
	s.closers = append(s.closers, s.store)
	if s.metrics, err = metrics.OpenDB(s.paths.MetricsDB()); err != nil {
		return err
	}
	s.closers = append(s.closers, s.metrics)
	requests, err := requestlog.Open(s.paths.RequestsDB())
	if err != nil {
		return err
	}
	s.closers = append(s.closers, requests)

	docker := engine.NewDocker()
	github := githost.NewGitHub()
	manager := deploy.NewManager(s.paths, s.store, engine.NewNative(), docker, github)
	deps := &api.Deps{
		Version:   version.Version,
		PanelAddr: s.cfg.PanelAddr,
		DevUI:     devUI,
		Paths:     s.paths,
		Store:     s.store,
		Deploy:    manager,
		Docker:    docker,
		MetricsDB: s.metrics,
		GitHub:    github,
		Requests:  requests,
	}
	s.panel = api.New(deps)
	s.panel.ConfigureRecording()
	deps.Proxy = proxy.New(s.store, s.paths, s.cfg, s.panel, requests)
	deps.Metrics = metrics.NewCollector(s.metrics, s.store, s.paths, manager.Status, docker, deps.Proxy.Drain)
	deps.Monitor = health.NewMonitor(s.store, manager, deps.Metrics, metrics.AppTarget)
	manager.OnDeployed = deps.Monitor.CheckAfterDeploy
	deps.Backups = backup.NewService(s.paths, s.store, manager)
	s.deps = deps
	return nil
}

// Run starts everything and blocks until ctx is cancelled or the panel fails. The order
// matters: apps boot first, then the proxy loads their routes, then monitoring, then the panel.
func (s *Server) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	d := s.deps
	slog.Info("starting", "version", version.Version, "env", s.cfg.Env, "pid", os.Getpid(), "runtime", s.paths.Root)
	if err := d.Deploy.Run(ctx, s.cfg.Builds); err != nil {
		return err
	}
	defer d.Deploy.Shutdown()
	if err := d.Backups.Start(); err != nil {
		return err
	}
	defer d.Backups.Stop()
	if err := d.Proxy.Reload(ctx); err != nil {
		slog.Error("proxy", "err", err)
	}
	defer d.Proxy.Shutdown()
	go d.Metrics.Run(ctx)
	go d.Monitor.Run(ctx)
	go d.Requests.Run(ctx)
	go s.prune(ctx)

	ln, err := net.Listen("tcp", s.cfg.PanelAddr)
	if err != nil {
		return err
	}
	// No read or write timeout: bundle uploads and live log streams legitimately run long.
	srv := &http.Server{
		Handler:           s.panel,
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
		ErrorLog:          logger.Std("panel"),
	}
	serveErr := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()
	if d.DevUI != nil {
		slog.Info("panel listening", "addr", s.cfg.PanelAddr, "ui", d.DevUI.String())
	} else {
		slog.Info("panel listening", "addr", s.cfg.PanelAddr)
	}

	var runErr error
	select {
	case <-ctx.Done():
		slog.Info("shutting down")
	case runErr = <-serveErr:
		slog.Error("panel server failed, shutting down", "err", runErr)
	}
	cancel()
	shutdownCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("panel shutdown", "err", err)
	}
	return runErr
}

// Close releases the databases and the platform log, newest first.
func (s *Server) Close() {
	for i := len(s.closers) - 1; i >= 0; i-- {
		s.closers[i].Close()
	}
}

// prune drops old log files and audit entries once an hour.
func (s *Server) prune(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		logs.Prune(s.paths.Logs(), "moonshed", 30)
		if err := s.store.PruneAudit(time.Now().Add(-auditRetention)); err != nil {
			slog.Error("prune audit log", "err", err)
		}
		if apps, err := s.store.Apps(); err == nil {
			for _, app := range apps {
				logs.Prune(s.paths.App(app.Name).Logs(), "app", app.LogRetentionDays)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

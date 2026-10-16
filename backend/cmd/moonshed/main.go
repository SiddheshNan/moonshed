package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/bcrypt"

	"moonshed/internal/api"
	"moonshed/internal/backup"
	"moonshed/internal/config"
	"moonshed/internal/deploy"
	"moonshed/internal/engine"
	"moonshed/internal/githost"
	"moonshed/internal/health"
	"moonshed/internal/logs"
	"moonshed/internal/metrics"
	"moonshed/internal/proc"
	"moonshed/internal/proxy"
	"moonshed/internal/requestlog"
	"moonshed/internal/secret"
	"moonshed/internal/service"
	"moonshed/internal/store"
)

var version = "dev"

const usage = `Usage: moonshed <command> [--runtime DIR]

Commands:
  serve                   run Moonshed
  install                 register as a systemd unit / Windows service and start it
  uninstall               stop and remove the service
  reset-password <user>   set a new random password for a user
  version                 print the version

--runtime defaults to the runtime/ folder next to the binary.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(2)
	}
	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	runtimeDir := fs.String("runtime", "", "runtime directory")
	args := parseAnywhere(fs, os.Args[2:])

	root, err := resolveRoot(*runtimeDir)
	if err != nil {
		fatal(err)
	}
	switch cmd {
	case "serve":
		err = serve(root)
	case "install":
		err = service.Install(root)
	case "uninstall":
		err = service.Uninstall()
	case "reset-password":
		err = resetPassword(root, firstArg(args))
	case "version":
		fmt.Println(version)
	default:
		fmt.Print(usage)
		os.Exit(2)
	}
	if err != nil {
		fatal(err)
	}
}

// parseAnywhere lets flags come before or after positional arguments,
// e.g. both "reset-password --runtime x admin" and "reset-password admin --runtime x".
func parseAnywhere(fs *flag.FlagSet, args []string) []string {
	var positional []string
	for len(args) > 0 {
		fs.Parse(args)
		args = fs.Args()
		if len(args) > 0 {
			positional = append(positional, args[0])
			args = args[1:]
		}
	}
	return positional
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func resolveRoot(dir string) (string, error) {
	if dir == "" {
		return config.DefaultRoot()
	}
	return filepath.Abs(dir)
}

func serve(root string) error {
	paths := config.Paths{Root: root}
	if err := paths.Init(); err != nil {
		return err
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return err
	}

	platformLog := logs.NewStream(paths.Logs(), "moonshed")
	defer platformLog.Close()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.MultiWriter(os.Stderr, platformLog), &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})))

	if err := proc.Init(); err != nil {
		return err
	}
	box, err := secret.LoadOrCreate(paths.SecretKey())
	if err != nil {
		return err
	}
	st, err := store.Open(paths.DB(), box)
	if err != nil {
		return err
	}
	defer st.Close()
	mdb, err := metrics.OpenDB(paths.MetricsDB())
	if err != nil {
		return err
	}
	defer mdb.Close()
	requests, err := requestlog.Open(paths.RequestsDB())
	if err != nil {
		return err
	}
	defer requests.Close()

	docker := engine.NewDocker()
	github := githost.NewGitHub()
	manager := deploy.NewManager(paths, st, engine.NewNative(), docker, github)
	deps := &api.Deps{
		Version:   version,
		PanelAddr: cfg.PanelAddr,
		Paths:     paths,
		Store:     st,
		Deploy:    manager,
		Docker:    docker,
		MetricsDB: mdb,
		GitHub:    github,
		Requests:  requests,
	}
	apiServer := api.New(deps)
	apiServer.ConfigureRecording()
	deps.Proxy = proxy.New(st, paths, cfg, apiServer, requests)
	deps.Metrics = metrics.NewCollector(mdb, st, paths, manager.Status, docker, deps.Proxy.Drain)
	deps.Monitor = health.NewMonitor(st, manager, deps.Metrics, metrics.AppTarget)
	manager.OnDeployed = deps.Monitor.CheckAfterDeploy
	deps.Backups = backup.NewService(paths, st, manager)

	return service.Run(func(ctx context.Context) error {
		slog.Info("starting", "version", version, "runtime", root)
		if err := manager.Run(ctx, cfg.Builds); err != nil {
			return err
		}
		defer manager.Shutdown()
		if err := deps.Backups.Start(); err != nil {
			return err
		}
		defer deps.Backups.Stop()
		if err := deps.Proxy.Reload(ctx); err != nil {
			slog.Error("proxy", "err", err)
		}
		defer deps.Proxy.Shutdown()
		go deps.Metrics.Run(ctx)
		go deps.Monitor.Run(ctx)
		go requests.Run(ctx)
		go prune(ctx, paths, st)

		ln, err := net.Listen("tcp", cfg.PanelAddr)
		if err != nil {
			return err
		}
		srv := &http.Server{Handler: apiServer, ReadHeaderTimeout: 10 * time.Second}
		go func() {
			if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Error("http server", "err", err)
			}
		}()
		slog.Info("panel listening", "addr", cfg.PanelAddr)

		<-ctx.Done()
		slog.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
		return nil
	})
}

const auditRetention = 365 * 24 * time.Hour

// prune drops old log files and audit entries once an hour.
func prune(ctx context.Context, paths config.Paths, st *store.Store) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		logs.Prune(paths.Logs(), "moonshed", 30)
		if err := st.PruneAudit(time.Now().Add(-auditRetention)); err != nil {
			slog.Error("prune audit log", "err", err)
		}
		if apps, err := st.Apps(); err == nil {
			for _, app := range apps {
				logs.Prune(paths.App(app.Name).Logs(), "app", app.LogRetentionDays)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func resetPassword(root, username string) error {
	if username == "" {
		return errors.New("usage: moonshed reset-password <user>")
	}
	paths := config.Paths{Root: root}
	box, err := secret.LoadOrCreate(paths.SecretKey())
	if err != nil {
		return err
	}
	st, err := store.Open(paths.DB(), box)
	if err != nil {
		return err
	}
	defer st.Close()
	u, err := st.UserByName(username)
	if err != nil {
		return fmt.Errorf("user %s: %w", username, err)
	}
	raw := make([]byte, 12)
	rand.Read(raw)
	password := base64.RawURLEncoding.EncodeToString(raw)
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := st.SetPassword(u.ID, string(hash)); err != nil {
		return err
	}
	fmt.Printf("New password for %s: %s\n", username, password)
	return nil
}

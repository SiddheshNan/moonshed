//go:build !windows

package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
)

const unitPath = "/etc/systemd/system/" + Name + ".service"

const unit = `[Unit]
Description=Moonshed
After=network-online.target docker.service
Wants=network-online.target

[Service]
ExecStart=%q serve -e %s --runtime %q
Restart=always
RestartSec=5
KillMode=mixed
TimeoutStopSec=60
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
`

// Run calls fn with a context that is cancelled on SIGINT or SIGTERM.
func Run(fn func(context.Context) error) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return fn(ctx)
}

func Install(env, runtimeDir string) error {
	if runtime.GOOS != "linux" {
		return errors.New("install is only supported on Linux (systemd) and Windows")
	}
	exe, err := executable()
	if err != nil {
		return err
	}
	if err := os.WriteFile(unitPath, fmt.Appendf(nil, unit, exe, env, runtimeDir), 0o644); err != nil {
		return err
	}
	if err := systemctl("daemon-reload"); err != nil {
		return err
	}
	return systemctl("enable", "--now", Name)
}

func Uninstall() error {
	if runtime.GOOS != "linux" {
		return errors.New("uninstall is only supported on Linux (systemd) and Windows")
	}
	systemctl("disable", "--now", Name)
	if err := os.Remove(unitPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return systemctl("daemon-reload")
}

func systemctl(args ...string) error {
	cmd := exec.Command("systemctl", args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("systemctl %v: %w", args, err)
	}
	return nil
}

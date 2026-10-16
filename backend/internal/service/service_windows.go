//go:build windows

package service

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// Run calls fn with a context that is cancelled when the service manager asks us to stop,
// or on Ctrl+C when running in a console.
func Run(fn func(context.Context) error) error {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return err
	}
	if !isService {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		return fn(ctx)
	}
	h := &handler{fn: fn}
	if err := svc.Run(Name, h); err != nil {
		return err
	}
	return h.err
}

type handler struct {
	fn  func(context.Context) error
	err error
}

func (h *handler) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		h.err = h.fn(ctx)
		close(done)
	}()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	for {
		select {
		case <-done:
			status <- svc.Status{State: svc.Stopped}
			if h.err != nil {
				return false, 1
			}
			return false, 0
		case req := <-requests:
			switch req.Cmd {
			case svc.Interrogate:
				status <- req.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending, WaitHint: 60000}
				cancel()
			}
		}
	}
}

func Install(runtimeDir string) error {
	exe, err := executable()
	if err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager (run as administrator): %w", err)
	}
	defer m.Disconnect()

	s, err := m.CreateService(Name, exe, mgr.Config{
		DisplayName: "Moonshed",
		Description: "Deploys and supervises apps.",
		StartType:   mgr.StartAutomatic,
	}, "serve", "--runtime", runtimeDir)
	if err != nil {
		return err
	}
	defer s.Close()
	err = s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
	}, 86400)
	if err != nil {
		return err
	}
	return s.Start()
}

func Uninstall() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager (run as administrator): %w", err)
	}
	defer m.Disconnect()
	s, err := m.OpenService(Name)
	if err != nil {
		return err
	}
	defer s.Close()
	if st, err := s.Control(svc.Stop); err == nil {
		for deadline := time.Now().Add(60 * time.Second); st.State != svc.Stopped && time.Now().Before(deadline); {
			time.Sleep(500 * time.Millisecond)
			if st, err = s.Query(); err != nil {
				break
			}
		}
	}
	return s.Delete()
}

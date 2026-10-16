package engine

import (
	"fmt"
	"sync"
	"time"

	"moonshed/internal/proc"
)

const maxBackoff = time.Minute

// Native supervises app processes directly, restarting them when they exit.
type Native struct {
	mu    sync.Mutex
	procs map[string]*supervised
}

func NewNative() *Native {
	return &Native{procs: map[string]*supervised{}}
}

func (n *Native) Start(spec Spec) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if _, ok := n.procs[spec.App]; ok {
		return fmt.Errorf("%s is already running", spec.App)
	}
	s := &supervised{spec: spec, stop: make(chan struct{}), done: make(chan struct{})}
	if err := s.launch(); err != nil {
		return err
	}
	n.procs[spec.App] = s
	go s.loop()
	return nil
}

func (n *Native) Stop(app string) error {
	n.mu.Lock()
	s, ok := n.procs[app]
	delete(n.procs, app)
	n.mu.Unlock()
	if !ok {
		return nil
	}
	close(s.stop)
	<-s.done
	return nil
}

// StopAll stops every app in parallel, used on platform shutdown.
func (n *Native) StopAll() {
	n.mu.Lock()
	apps := make([]string, 0, len(n.procs))
	for app := range n.procs {
		apps = append(apps, app)
	}
	n.mu.Unlock()
	var wg sync.WaitGroup
	for _, app := range apps {
		wg.Go(func() { n.Stop(app) })
	}
	wg.Wait()
}

func (n *Native) Status(app string) Status {
	n.mu.Lock()
	s, ok := n.procs[app]
	n.mu.Unlock()
	if !ok {
		return Status{State: StateStopped}
	}
	return s.status()
}

type supervised struct {
	spec Spec
	stop chan struct{}
	done chan struct{}

	mu       sync.Mutex
	proc     *proc.Process
	state    string
	since    time.Time
	restarts int
	exitCode int
}

func (s *supervised) launch() error {
	var user *proc.User
	if s.spec.RunAs != "" {
		u, err := proc.LookupUser(s.spec.RunAs, s.spec.RunAsPassword)
		if err != nil {
			return err
		}
		user = u
	}
	p, err := proc.Start(s.spec.Run, proc.Options{
		Dir:    s.spec.Dir,
		Env:    s.spec.Env,
		User:   user,
		Stdout: s.spec.Log,
		Stderr: s.spec.Log,
	})
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.proc, s.state, s.since = p, StateRunning, time.Now()
	s.mu.Unlock()
	return nil
}

func (s *supervised) loop() {
	defer close(s.done)
	backoff := time.Second
	for {
		if p := s.current(); p != nil {
			select {
			case <-p.Done():
			case <-s.stop:
				p.Stop(stopGrace)
				s.setState(StateStopped, p.ExitCode())
				return
			}
			if err := p.Err(); err != nil {
				s.spec.Log.Printf("process exited: %v", err)
			} else {
				s.spec.Log.Printf("process exited with code 0")
			}
			if time.Since(s.status().Since) > time.Minute {
				backoff = time.Second
			}
			s.setState(StateCrashed, p.ExitCode())
		}

		s.spec.Log.Printf("restarting in %s", backoff)
		select {
		case <-time.After(backoff):
		case <-s.stop:
			s.setState(StateStopped, s.status().ExitCode)
			return
		}
		backoff = min(backoff*2, maxBackoff)

		s.mu.Lock()
		s.restarts++
		s.mu.Unlock()
		if err := s.launch(); err != nil {
			s.spec.Log.Printf("start failed: %v", err)
			s.mu.Lock()
			s.proc = nil
			s.mu.Unlock()
		}
	}
}

func (s *supervised) current() *proc.Process {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.proc
}

func (s *supervised) setState(state string, exitCode int) {
	s.mu.Lock()
	s.state, s.exitCode = state, exitCode
	s.mu.Unlock()
}

func (s *supervised) status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Status{State: s.state, Release: s.spec.Release, Since: s.since, Restarts: s.restarts, ExitCode: s.exitCode}
	if s.proc != nil && s.state == StateRunning {
		st.PID = s.proc.Pid()
	}
	return st
}

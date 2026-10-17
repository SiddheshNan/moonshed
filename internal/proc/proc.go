// Package proc starts process trees that can be stopped as a unit,
// optionally as another OS user.
package proc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Options struct {
	Dir string
	// Env is appended to the platform's environment, or the run-as user's when User is set.
	Env    []string
	User   *User
	Stdout io.Writer
	Stderr io.Writer
}

// Process is a started process tree.
type Process struct {
	cmd  *exec.Cmd
	tree tree
	done chan struct{}
	err  error
}

// Start runs a manifest script. The script may be inline (one or many lines) or a path
// to a .sh/.ps1/.cmd file relative to opts.Dir. opts.User is owned by the process from
// here on and closed when it exits.
func Start(script string, opts Options) (*Process, error) {
	p, err := start(script, opts)
	if err != nil && opts.User != nil {
		opts.User.Close()
	}
	return p, err
}

func start(script string, opts Options) (*Process, error) {
	cmd, err := scriptCommand(script, opts.Dir)
	if err != nil {
		return nil, err
	}
	cmd.Dir = opts.Dir
	base := os.Environ()
	if opts.User != nil {
		base = opts.User.environ()
	}
	cmd.Env = append(base, opts.Env...)
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr

	p := &Process{cmd: cmd, done: make(chan struct{})}
	if err := p.tree.start(cmd, opts.User); err != nil {
		return nil, err
	}
	go func() {
		p.err = cmd.Wait()
		p.tree.cleanup()
		if opts.User != nil {
			opts.User.Close()
		}
		close(p.done)
	}()
	return p, nil
}

// Run starts a script and waits for it, killing the whole tree if ctx is cancelled.
func Run(ctx context.Context, script string, opts Options) error {
	p, err := Start(script, opts)
	if err != nil {
		return err
	}
	select {
	case <-p.Done():
		return p.Err()
	case <-ctx.Done():
		p.Kill()
		<-p.Done()
		return ctx.Err()
	}
}

func (p *Process) Pid() int              { return p.cmd.Process.Pid }
func (p *Process) Done() <-chan struct{} { return p.done }

// Err is valid once Done is closed.
func (p *Process) Err() error { return p.err }

func (p *Process) ExitCode() int {
	var exit *exec.ExitError
	if errors.As(p.err, &exit) {
		return exit.ExitCode()
	}
	if p.err != nil {
		return -1
	}
	return 0
}

// Stop asks the tree to exit and kills it if it's still alive after grace.
func (p *Process) Stop(grace time.Duration) {
	select {
	case <-p.done:
		return
	default:
	}
	if err := p.tree.interrupt(); err != nil {
		p.Kill()
		<-p.done
		return
	}
	select {
	case <-p.done:
	case <-time.After(grace):
		p.Kill()
		<-p.done
	}
}

func (p *Process) Kill() {
	select {
	case <-p.done:
	default:
		p.tree.kill()
	}
}

var scriptExts = []string{".sh", ".ps1", ".cmd", ".bat"}

func scriptFile(script, dir string) (string, bool, error) {
	if strings.ContainsAny(script, "\r\n") {
		return "", false, nil
	}
	ext := strings.ToLower(filepath.Ext(script))
	for _, e := range scriptExts {
		if ext == e {
			path := filepath.Join(dir, filepath.FromSlash(script))
			if _, err := os.Stat(path); err != nil {
				return "", false, fmt.Errorf("script file %s: %w", script, err)
			}
			return path, true, nil
		}
	}
	return "", false, nil
}

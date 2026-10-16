//go:build !windows

package proc

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
)

// Init prepares the platform process for supervising children. Nothing to do on unix.
func Init() error { return nil }

func scriptCommand(script, dir string) (*exec.Cmd, error) {
	file, ok, err := scriptFile(script, dir)
	if err != nil {
		return nil, err
	}
	if ok {
		if filepath.Ext(file) != ".sh" {
			return nil, fmt.Errorf("%s: only .sh scripts run on this OS", script)
		}
		return exec.Command("/bin/sh", "-e", file), nil
	}
	return exec.Command("/bin/sh", "-ec", script), nil
}

type tree struct {
	pgid int
}

func (t *tree) start(cmd *exec.Cmd, u *User) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if u != nil {
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: u.uid, Gid: u.gid, Groups: u.groups}
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	t.pgid = cmd.Process.Pid
	return nil
}

func (t *tree) interrupt() error { return syscall.Kill(-t.pgid, syscall.SIGTERM) }
func (t *tree) kill()            { syscall.Kill(-t.pgid, syscall.SIGKILL) }

// cleanup kills anything the leader left behind in its group.
func (t *tree) cleanup() { t.kill() }

type User struct {
	Name   string
	uid    uint32
	gid    uint32
	groups []uint32
	home   string
}

// LookupUser resolves a local account. The password is only used on Windows.
func LookupUser(name, _ string) (*User, error) {
	u, err := user.Lookup(name)
	if err != nil {
		return nil, err
	}
	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return nil, err
	}
	gid, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return nil, err
	}
	out := &User{Name: name, uid: uint32(uid), gid: uint32(gid), home: u.HomeDir}
	gids, err := u.GroupIds()
	if err != nil {
		return nil, err
	}
	for _, g := range gids {
		if n, err := strconv.ParseUint(g, 10, 32); err == nil {
			out.groups = append(out.groups, uint32(n))
		}
	}
	return out, nil
}

func (u *User) Close() {}

func (u *User) environ() []string {
	return append(os.Environ(), "HOME="+u.home, "USER="+u.Name, "LOGNAME="+u.Name)
}

// Grant makes the user the owner of each path, recursively.
func (u *User) Grant(paths ...string) error {
	for _, root := range paths {
		err := filepath.WalkDir(root, func(path string, _ fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			return os.Lchown(path, int(u.uid), int(u.gid))
		})
		if err != nil {
			return err
		}
	}
	return nil
}

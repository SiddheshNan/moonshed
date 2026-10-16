// Package service runs the platform under systemd or the Windows service manager.
package service

import (
	"os"
	"path/filepath"
)

const Name = "moonshed"

func executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

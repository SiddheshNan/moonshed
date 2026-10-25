// Package setup guards creating the first admin. Until one exists, setup needs a one-time
// token kept in data/setup-token, so only someone with access to the server can do it.
package setup

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/SiddheshNan/moonshed/internal/config"
	"github.com/SiddheshNan/moonshed/internal/store"
)

// Pending returns the setup token while there are no users, creating it on first use,
// or "" once setup is done.
func Pending(st *store.Store, p config.Paths) (string, error) {
	n, err := st.UserCount()
	if err != nil {
		return "", err
	}
	if n > 0 {
		return "", Done(p)
	}
	return token(p.SetupToken())
}

// token reads the token file, or creates it. O_EXCL keeps `install`, `serve` and
// `setup-link` from each writing a different token.
func token(path string) (string, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		b, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		if t := strings.TrimSpace(string(b)); t != "" {
			return t, nil
		}
		f, err = os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	defer f.Close()
	b := make([]byte, 18)
	rand.Read(b)
	t := base64.RawURLEncoding.EncodeToString(b)
	if _, err := f.WriteString(t); err != nil {
		return "", err
	}
	return t, f.Close()
}

func Valid(p config.Paths, token string) bool {
	b, err := os.ReadFile(p.SetupToken())
	want := strings.TrimSpace(string(b))
	return err == nil && want != "" && subtle.ConstantTimeCompare([]byte(want), []byte(token)) == 1
}

// Done removes the token once the first admin exists.
func Done(p config.Paths) error {
	if err := os.Remove(p.SetupToken()); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func Link(panelAddr, token string) string {
	return PanelURL(panelAddr) + "/setup?token=" + token
}

// PanelURL is where the panel can be opened. A panel on all interfaces is shown with
// this server's own IP, which may be a private one behind NAT.
func PanelURL(panelAddr string) string {
	host, port, _ := net.SplitHostPort(panelAddr)
	if ip := net.ParseIP(host); host == "" || ip != nil && ip.IsUnspecified() {
		host = hostIP()
	}
	return "http://" + net.JoinHostPort(host, port)
}

// hostIP is the address of the network card with the default route. Dialing UDP sends
// nothing; it only makes the OS pick the source address.
func hostIP() string {
	if c, err := net.Dial("udp4", "1.1.1.1:53"); err == nil {
		defer c.Close()
		return c.LocalAddr().(*net.UDPAddr).IP.String()
	}
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.IsGlobalUnicast() && n.IP.To4() != nil {
			return n.IP.String()
		}
	}
	return "localhost"
}

// Command is the exact command that prints the setup link on this server.
func Command(env string, p config.Paths) string {
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		exe = "moonshed"
	}
	cmd := quote(exe) + " setup-link -e " + env
	if strings.HasPrefix(cmd, `"`) {
		cmd = "& " + cmd // PowerShell only runs a quoted path with the call operator
	}
	if p.Root != filepath.Join(filepath.Dir(exe), "runtime") {
		cmd += " --runtime " + quote(p.Root)
	}
	if runtime.GOOS != "windows" && os.Geteuid() == 0 {
		cmd = "sudo " + cmd
	}
	return cmd
}

func quote(path string) string {
	if !strings.ContainsAny(path, " '\"") {
		return path
	}
	if runtime.GOOS == "windows" {
		return `"` + path + `"`
	}
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

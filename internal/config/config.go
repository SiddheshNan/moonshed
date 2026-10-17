package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/SiddheshNan/moonshed/internal/logger"
)

// Environments are the config files a runtime can hold, one per name in runtime/config/.
var Environments = []string{"development", "production"}

type Config struct {
	Env string `yaml:"-"`

	// PanelAddr always serves the panel, independent of the reverse proxy.
	// Bind it to 127.0.0.1:5050 to keep the panel off the network.
	PanelAddr string `yaml:"panel_addr"`
	HTTPAddr  string `yaml:"http_addr"`
	HTTPSAddr string `yaml:"https_addr"`
	Builds    int    `yaml:"concurrent_builds"`

	// UI is "/" to serve the built frontend from runtime/web, or the URL of a
	// frontend dev server (e.g. http://localhost:5173) to forward to it.
	UI string `yaml:"ui"`

	LogLevel string `yaml:"log_level"`
}

// Load reads runtime/config/<env>.yaml. Every key must be present; nothing is assumed.
func Load(env string, p Paths) (Config, error) {
	if !slices.Contains(Environments, env) {
		return Config{}, fmt.Errorf("invalid environment %q: pass -e %s", env, strings.Join(Environments, "|"))
	}
	file := p.ConfigFile(env)
	f, err := os.Open(file)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	defer f.Close()

	cfg := Config{Env: env}
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", file, err)
	}
	if err := cfg.validate(); err != nil {
		return Config{}, fmt.Errorf("%s: %w", file, err)
	}
	return cfg, nil
}

func (c Config) validate() error {
	for key, addr := range map[string]string{"panel_addr": c.PanelAddr, "http_addr": c.HTTPAddr, "https_addr": c.HTTPSAddr} {
		if _, _, err := net.SplitHostPort(addr); err != nil {
			return fmt.Errorf("%s must be host:port, e.g. \":5050\"", key)
		}
	}
	if c.Builds < 1 {
		return errors.New("concurrent_builds must be at least 1")
	}
	if _, err := logger.ParseLevel(c.LogLevel); err != nil {
		return fmt.Errorf("log_level: %w", err)
	}
	if c.UI == "" {
		return errors.New(`ui is required: "/" for the built frontend or a dev server URL`)
	}
	if _, err := c.DevUI(); err != nil {
		return err
	}
	return nil
}

// DevUI is the frontend dev server to forward to, or nil when the built UI is served.
func (c Config) DevUI() (*url.URL, error) {
	if c.UI == "/" {
		return nil, nil
	}
	u, err := url.Parse(c.UI)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf(`ui must be "/" or a URL like http://localhost:5173, got %q`, c.UI)
	}
	return u, nil
}

type Paths struct {
	Root string
}

// DefaultRoot is the runtime/ folder next to the executable, falling back to ./runtime
// (under `go run` the binary lives in a temporary directory).
func DefaultRoot() string {
	if exe, err := os.Executable(); err == nil {
		if exe, err = filepath.EvalSymlinks(exe); err == nil {
			dir := filepath.Join(filepath.Dir(exe), "runtime")
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				return dir
			}
		}
	}
	return "runtime"
}

func (p Paths) Init() error {
	for _, dir := range []string{p.Data(), p.Apps(), p.Certs(), p.Backups(), p.Logs(), p.Web()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (p Paths) Config() string     { return filepath.Join(p.Root, "config") }
func (p Paths) Data() string       { return filepath.Join(p.Root, "data") }
func (p Paths) DB() string         { return filepath.Join(p.Data(), "moonshed.db") }
func (p Paths) MetricsDB() string  { return filepath.Join(p.Data(), "metrics.db") }
func (p Paths) RequestsDB() string { return filepath.Join(p.Data(), "requests.db") }
func (p Paths) SecretKey() string  { return filepath.Join(p.Data(), "secret.key") }
func (p Paths) Apps() string       { return filepath.Join(p.Root, "apps") }
func (p Paths) Certs() string      { return filepath.Join(p.Root, "certs") }
func (p Paths) Backups() string    { return filepath.Join(p.Root, "backups") }
func (p Paths) Logs() string       { return filepath.Join(p.Root, "logs") }
func (p Paths) Web() string        { return filepath.Join(p.Root, "web") }

func (p Paths) ConfigFile(env string) string {
	return filepath.Join(p.Config(), env+".yaml")
}

func (p Paths) AppBackups(name string) string {
	return filepath.Join(p.Backups(), "apps", name)
}

func (p Paths) PlatformBackups() string {
	return filepath.Join(p.Backups(), "platform")
}

func (p Paths) App(name string) AppPaths {
	return AppPaths{Root: filepath.Join(p.Apps(), name)}
}

type AppPaths struct {
	Root string
}

func (a AppPaths) Bundles() string  { return filepath.Join(a.Root, "bundles") }
func (a AppPaths) Releases() string { return filepath.Join(a.Root, "releases") }
func (a AppPaths) Shared() string   { return filepath.Join(a.Root, "shared") }
func (a AppPaths) Cache() string    { return filepath.Join(a.Root, "cache") }
func (a AppPaths) Logs() string     { return filepath.Join(a.Root, "logs") }
func (a AppPaths) Deploys() string  { return filepath.Join(a.Root, "deploys") }

func (a AppPaths) Bundle(id string) string {
	return filepath.Join(a.Bundles(), id+".zip")
}

func (a AppPaths) Release(id string) string {
	return filepath.Join(a.Releases(), id)
}

func (a AppPaths) DeployLog(id string) string {
	return filepath.Join(a.Deploys(), id+".log")
}

func (a AppPaths) Init() error {
	for _, dir := range []string{a.Bundles(), a.Releases(), a.Shared(), a.Cache(), a.Logs(), a.Deploys()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	// PanelAddr always serves the panel, independent of the reverse proxy.
	// Bind it to 127.0.0.1:5050 to keep the panel off the network.
	PanelAddr string `yaml:"panel_addr"`
	HTTPAddr  string `yaml:"http_addr"`
	HTTPSAddr string `yaml:"https_addr"`
	Builds    int    `yaml:"concurrent_builds"`
}

func defaults() Config {
	return Config{
		PanelAddr: ":5050",
		HTTPAddr:  ":80",
		HTTPSAddr: ":443",
		Builds:    1,
	}
}

// Load reads config.yaml from the runtime dir, writing a default one on first run.
func Load(p Paths) (Config, error) {
	cfg := defaults()
	data, err := os.ReadFile(p.ConfigFile())
	if errors.Is(err, os.ErrNotExist) {
		out, _ := yaml.Marshal(cfg)
		return cfg, os.WriteFile(p.ConfigFile(), out, 0o644)
	}
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", p.ConfigFile(), err)
	}
	if cfg.Builds < 1 {
		cfg.Builds = 1
	}
	return cfg, nil
}

type Paths struct {
	Root string
}

// DefaultRoot is the runtime/ folder next to the executable.
func DefaultRoot() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(exe), "runtime"), nil
}

func (p Paths) Init() error {
	for _, dir := range []string{p.Data(), p.Apps(), p.Certs(), p.Backups(), p.Logs(), p.Web()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (p Paths) ConfigFile() string { return filepath.Join(p.Root, "config.yaml") }
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

// Package manifest reads moonshed.yaml and merges it with settings made in the UI.
package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const FileName = "moonshed.yaml"

var ErrNotFound = errors.New(FileName + " not found in bundle")

type Manifest struct {
	Pre              Script   `yaml:"pre" json:"pre"`
	Run              Script   `yaml:"run" json:"run"`
	Post             Script   `yaml:"post" json:"post"`
	Persist          []string `yaml:"persist" json:"persist"`
	Port             int      `yaml:"port" json:"port,omitempty"`
	Health           *Health  `yaml:"health" json:"health,omitempty"`
	RestartUnhealthy bool     `yaml:"restart_unhealthy" json:"restart_unhealthy,omitempty"`
	Build            *Build   `yaml:"build" json:"build,omitempty"`
}

// Health is an app's health check. With neither HTTP nor TCP set, it only checks that the
// process is running.
type Health struct {
	HTTP     string   `yaml:"http" json:"http,omitempty"`
	TCP      bool     `yaml:"tcp" json:"tcp,omitempty"`
	Interval Duration `yaml:"interval" json:"interval,omitempty"`
	Timeout  Duration `yaml:"timeout" json:"timeout,omitempty"`
	Failures int      `yaml:"failures" json:"failures,omitempty"`
	Grace    Duration `yaml:"grace" json:"grace,omitempty"`
}

// UnmarshalYAML accepts a path as shorthand for an HTTP check, e.g. `health: /healthz`.
func (h *Health) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		h.HTTP = n.Value
		return nil
	}
	type plain Health
	return n.Decode((*plain)(h))
}

func (h Health) ProcessOnly() bool { return h.HTTP == "" && !h.TCP }

// Build configures a Docker build. Paths are relative to the bundle root.
type Build struct {
	Dockerfile string            `yaml:"dockerfile" json:"dockerfile,omitempty"`
	Folder     string            `yaml:"folder" json:"folder,omitempty"`
	Stage      string            `yaml:"stage" json:"stage,omitempty"`
	Args       map[string]string `yaml:"args" json:"args,omitempty"`
}

// DockerfilePath is the Dockerfile to build: the configured one, or Dockerfile in the build folder.
func (b *Build) DockerfilePath() string {
	if b != nil && b.Dockerfile != "" {
		return b.Dockerfile
	}
	return path.Join(b.FolderPath(), "Dockerfile")
}

func (b *Build) FolderPath() string {
	if b != nil && b.Folder != "" {
		return b.Folder
	}
	return "."
}

// Duration reads "30s"-style values and writes them back the same way.
type Duration time.Duration

func (d Duration) D() time.Duration { return time.Duration(d) }

func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	v, err := time.ParseDuration(n.Value)
	if err != nil {
		return fmt.Errorf("%q is not a duration like 30s or 1m", n.Value)
	}
	*d = Duration(v)
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(time.Duration(d).String()) }

// UnmarshalJSON also reads plain nanoseconds, which older releases stored.
func (d *Duration) UnmarshalJSON(b []byte) error {
	var ns int64
	if json.Unmarshal(b, &ns) == nil {
		*d = Duration(ns)
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("%q is not a duration like 30s or 1m", s)
	}
	*d = Duration(v)
	return nil
}

// Script is either a single value for every OS or a per-OS mapping.
type Script struct {
	Linux   string `json:"linux,omitempty"`
	Windows string `json:"windows,omitempty"`
}

func (s *Script) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		s.Linux, s.Windows = n.Value, n.Value
		return nil
	}
	var m struct {
		Linux   string `yaml:"linux"`
		Windows string `yaml:"windows"`
	}
	if err := n.Decode(&m); err != nil {
		return err
	}
	s.Linux, s.Windows = m.Linux, m.Windows
	return nil
}

// Current returns the script for the OS we're running on. macOS uses the linux variant.
func (s Script) Current() string {
	if runtime.GOOS == "windows" {
		return strings.TrimSpace(s.Windows)
	}
	return strings.TrimSpace(s.Linux)
}

func Load(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

var unknownField = regexp.MustCompile(`field (\S+) not found in type \S+`)

func Parse(data []byte) (*Manifest, error) {
	var m Manifest
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil && !errors.Is(err, io.EOF) {
		msg := unknownField.ReplaceAllString(err.Error(), "unknown key \"$1\"")
		return nil, fmt.Errorf("invalid %s: %s", FileName, msg)
	}
	if m.Port < 0 || m.Port > 65535 {
		return nil, fmt.Errorf("%s: port must be between 1 and 65535", FileName)
	}
	for i, p := range m.Persist {
		clean, err := cleanPath(p, true)
		if err != nil {
			return nil, fmt.Errorf("%s: persist: %w", FileName, err)
		}
		m.Persist[i] = clean
	}
	if h := m.Health; h != nil {
		if h.ProcessOnly() {
			return nil, fmt.Errorf("%s: health: set http or tcp", FileName)
		}
		if err := h.Validate(); err != nil {
			return nil, fmt.Errorf("%s: health: %w", FileName, err)
		}
	}
	if b := m.Build; b != nil {
		if err := b.Validate(); err != nil {
			return nil, fmt.Errorf("%s: build: %w", FileName, err)
		}
	}
	return &m, nil
}

func (h *Health) Validate() error {
	if h.HTTP != "" && !strings.HasPrefix(h.HTTP, "/") {
		h.HTTP = "/" + h.HTTP
	}
	switch {
	case h.Interval != 0 && h.Interval.D() < time.Second:
		return errors.New("interval must be at least 1s")
	case h.Timeout < 0:
		return errors.New("timeout can't be negative")
	case h.Failures < 0:
		return errors.New("failures can't be negative")
	case h.Grace < 0:
		return errors.New("grace can't be negative")
	}
	return nil
}

func (b *Build) Validate() error {
	for _, p := range []*string{&b.Dockerfile, &b.Folder} {
		if *p == "" {
			continue
		}
		clean, err := cleanPath(*p, false)
		if err != nil {
			return err
		}
		*p = clean
	}
	return nil
}

// Overrides are the settings changed in the UI. A nil field falls back to moonshed.yaml,
// and then to the default.
type Overrides struct {
	Pre              *string   `json:"pre,omitempty"`
	Run              *string   `json:"run,omitempty"`
	Post             *string   `json:"post,omitempty"`
	Persist          *[]string `json:"persist,omitempty"`
	Health           *Health   `json:"health,omitempty"`
	RestartUnhealthy *bool     `json:"restart_unhealthy,omitempty"`
	Build            *Build    `json:"build,omitempty"`
}

// Validate cleans paths the way Parse does for the file.
func (o *Overrides) Validate() error {
	if o.Persist != nil {
		for i, p := range *o.Persist {
			clean, err := cleanPath(p, true)
			if err != nil {
				return fmt.Errorf("persist: %w", err)
			}
			(*o.Persist)[i] = clean
		}
	}
	if o.Health != nil {
		if err := o.Health.Validate(); err != nil {
			return fmt.Errorf("health: %w", err)
		}
	}
	if o.Build != nil {
		if err := o.Build.Validate(); err != nil {
			return fmt.Errorf("build: %w", err)
		}
	}
	return nil
}

// Effective merges the UI overrides onto the file (which may be nil). Overridden blocks
// like health and build replace the file's block as a whole.
func Effective(file *Manifest, o Overrides) *Manifest {
	var m Manifest
	if file != nil {
		m = *file
	}
	script := func(s *string) Script { return Script{Linux: *s, Windows: *s} }
	if o.Pre != nil {
		m.Pre = script(o.Pre)
	}
	if o.Run != nil {
		m.Run = script(o.Run)
	}
	if o.Post != nil {
		m.Post = script(o.Post)
	}
	if o.Persist != nil {
		m.Persist = *o.Persist
	}
	if o.Health != nil {
		m.Health = o.Health
	}
	if o.RestartUnhealthy != nil {
		m.RestartUnhealthy = *o.RestartUnhealthy
	}
	if o.Build != nil {
		m.Build = o.Build
	}
	return &m
}

const (
	DefaultInterval = 30 * time.Second
	DefaultTimeout  = 5 * time.Second
	DefaultFailures = 3
	DefaultGrace    = 10 * time.Second
)

// Check is the health check to run, with defaults filled in. Without one it only checks
// that the process is running, and a single failed check means down.
func (m *Manifest) Check() Health {
	var h Health
	if m != nil && m.Health != nil {
		h = *m.Health
	}
	if h.Interval <= 0 {
		h.Interval = Duration(DefaultInterval)
	}
	if h.Timeout <= 0 {
		h.Timeout = Duration(DefaultTimeout)
	}
	if h.Failures <= 0 {
		h.Failures = DefaultFailures
		if h.ProcessOnly() {
			h.Failures = 1
		}
	}
	if h.Grace <= 0 && !h.ProcessOnly() {
		h.Grace = Duration(DefaultGrace)
	}
	return h
}

// ValidateNative checks what a native (non-docker) app needs to run on this OS.
func (m *Manifest) ValidateNative() error {
	if m.Run.Current() == "" {
		return fmt.Errorf("no run command for %s: set one in %s or in the app's Build settings", runtime.GOOS, FileName)
	}
	return nil
}

// cleanPath keeps paths relative to the bundle. Persist paths remember a trailing slash,
// which marks a directory.
func cleanPath(p string, keepSlash bool) (string, error) {
	p = strings.ReplaceAll(strings.TrimSpace(p), "\\", "/")
	isDir := strings.HasSuffix(p, "/")
	clean := path.Clean(strings.TrimPrefix(p, "./"))
	if clean == "." && !keepSlash {
		return ".", nil
	}
	if clean == "." || path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") || filepath.VolumeName(clean) != "" {
		return "", fmt.Errorf("invalid path %q", p)
	}
	if isDir && keepSlash {
		clean += "/"
	}
	return clean, nil
}

package manifest

import (
	"bytes"
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

const FileName = "app.yaml"

var ErrNotFound = errors.New(FileName + " not found in bundle")

type Manifest struct {
	Pre     Script   `yaml:"pre" json:"pre"`
	Run     Script   `yaml:"run" json:"run"`
	Post    Script   `yaml:"post" json:"post"`
	Persist []string `yaml:"persist" json:"persist"`
	Port    int      `yaml:"port" json:"port,omitempty"`
	Health  *Health  `yaml:"health" json:"health,omitempty"`
}

type Health struct {
	HTTP     string        `yaml:"http" json:"http,omitempty"`
	TCP      bool          `yaml:"tcp" json:"tcp,omitempty"`
	Interval time.Duration `yaml:"interval" json:"interval"`
	Timeout  time.Duration `yaml:"timeout" json:"timeout"`
	Retries  int           `yaml:"retries" json:"retries"`
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
		clean, err := cleanPersistPath(p)
		if err != nil {
			return nil, err
		}
		m.Persist[i] = clean
	}
	if h := m.Health; h != nil {
		if h.HTTP != "" && !strings.HasPrefix(h.HTTP, "/") {
			h.HTTP = "/" + h.HTTP
		}
		if h.HTTP == "" && !h.TCP {
			return nil, errors.New("health: set either http or tcp")
		}
		if h.Interval <= 0 {
			h.Interval = 30 * time.Second
		}
		if h.Timeout <= 0 {
			h.Timeout = 5 * time.Second
		}
		if h.Retries <= 0 {
			h.Retries = 3
		}
	}
	return &m, nil
}

// ValidateNative checks what a native (non-docker) app needs to run on this OS.
func (m *Manifest) ValidateNative() error {
	if m.Run.Current() == "" {
		return fmt.Errorf("%s: no run script for %s", FileName, runtime.GOOS)
	}
	return nil
}

// cleanPersistPath keeps paths relative to the release and remembers whether they're directories.
func cleanPersistPath(p string) (string, error) {
	p = strings.ReplaceAll(strings.TrimSpace(p), "\\", "/")
	isDir := strings.HasSuffix(p, "/")
	clean := path.Clean(p)
	if clean == "." || path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") || filepath.VolumeName(clean) != "" {
		return "", fmt.Errorf("persist: invalid path %q", p)
	}
	if isDir {
		clean += "/"
	}
	return clean, nil
}

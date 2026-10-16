// Package engine runs app releases, either as supervised native processes or Docker containers.
package engine

import (
	"time"

	"moonshed/internal/logs"
)

const stopGrace = 10 * time.Second

const (
	StateRunning  = "running"
	StateStopped  = "stopped"
	StateCrashed  = "crashed"
	StateStarting = "starting"
)

type Spec struct {
	App     string
	Release string
	Dir     string
	Port    int
	Env     []string
	Log     *logs.Stream

	// Native
	Run           string
	RunAs         string
	RunAsPassword string

	// Docker
	Image  string
	Mounts []Mount
}

type Mount struct {
	Source string
	Target string
}

type Status struct {
	State    string    `json:"state"`
	PID      int       `json:"pid,omitempty"`
	Release  string    `json:"release_id,omitempty"`
	Since    time.Time `json:"since,omitzero"`
	Restarts int       `json:"restarts"`
	ExitCode int       `json:"exit_code"`
}

type Engine interface {
	Start(Spec) error
	Stop(app string) error
	Status(app string) Status
}

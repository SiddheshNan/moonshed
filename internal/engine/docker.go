package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"

	"github.com/SiddheshNan/moonshed/internal/logs"
)

var ErrNoDocker = errors.New("docker is not installed or not running")

const (
	labelApp     = "moonshed.app"
	labelRelease = "moonshed.release"
)

// Docker talks to the Docker Engine API (unix socket on Linux, named pipe on Windows).
// Containers are named moonshed-<app>.
type Docker struct {
	cli *client.Client

	mu       sync.Mutex
	watchers map[string]context.CancelFunc
	stats    map[string]ContainerStats
}

type ContainerStats struct {
	CPU    float64 // percent, 100 = one core
	Memory float64 // bytes
}

func NewDocker() *Docker {
	d := &Docker{watchers: map[string]context.CancelFunc{}, stats: map[string]ContainerStats{}}
	// Creating the client doesn't connect; a missing daemon shows up as an error on first use.
	d.cli, _ = client.New(client.FromEnv, client.WithAPIVersionNegotiation())
	return d
}

func containerName(app string) string { return "moonshed-" + app }

// Version returns the engine version, or ErrNoDocker.
func (d *Docker) Version(ctx context.Context) (string, error) {
	if d.cli == nil {
		return "", ErrNoDocker
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	v, err := d.cli.ServerVersion(ctx, client.ServerVersionOptions{})
	if err != nil {
		return "", ErrNoDocker
	}
	return v.Version, nil
}

func (d *Docker) RemoveImage(tag string) {
	d.cli.ImageRemove(context.Background(), tag, client.ImageRemoveOptions{})
}

// WorkDir is the image's WORKDIR, used to resolve persist paths inside the container.
func (d *Docker) WorkDir(image string) (string, error) {
	res, err := d.cli.ImageInspect(context.Background(), image)
	if err != nil {
		return "", err
	}
	if res.Config != nil && res.Config.WorkingDir != "" {
		return res.Config.WorkingDir, nil
	}
	return "/", nil
}

func (d *Docker) Start(spec Spec) error {
	ctx := context.Background()
	name := containerName(spec.App)
	d.remove(ctx, name)

	cfg := &container.Config{
		Image:  spec.Image,
		Env:    spec.Env,
		Labels: map[string]string{labelApp: spec.App, labelRelease: spec.Release},
	}
	host := &container.HostConfig{
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyUnlessStopped},
	}
	if spec.Port > 0 {
		port, err := network.ParsePort(fmt.Sprintf("%d/tcp", spec.Port))
		if err != nil {
			return err
		}
		cfg.ExposedPorts = network.PortSet{port: {}}
		host.PortBindings = network.PortMap{port: {{HostPort: strconv.Itoa(spec.Port)}}}
	}
	for _, m := range spec.Mounts {
		host.Mounts = append(host.Mounts, mount.Mount{Type: mount.TypeBind, Source: m.Source, Target: m.Target})
	}

	created, err := d.cli.ContainerCreate(ctx, client.ContainerCreateOptions{Name: name, Config: cfg, HostConfig: host})
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}
	if _, err := d.cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		d.remove(ctx, created.ID)
		return fmt.Errorf("start container: %w", err)
	}
	d.Watch(spec.App, spec.Log)
	return nil
}

func (d *Docker) Stop(app string) error {
	d.unwatch(app)
	ctx := context.Background()
	timeout := int(stopGrace.Seconds())
	d.cli.ContainerStop(ctx, containerName(app), client.ContainerStopOptions{Timeout: &timeout})
	return d.remove(ctx, containerName(app))
}

func (d *Docker) remove(ctx context.Context, id string) error {
	_, err := d.cli.ContainerRemove(ctx, id, client.ContainerRemoveOptions{Force: true})
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "no such container") {
		return fmt.Errorf("remove container: %w", err)
	}
	return nil
}

func (d *Docker) Status(app string) Status {
	if d.cli == nil {
		return Status{State: StateStopped}
	}
	res, err := d.cli.ContainerInspect(context.Background(), containerName(app), client.ContainerInspectOptions{})
	if err != nil || res.Container.State == nil {
		return Status{State: StateStopped}
	}
	c := res.Container
	st := Status{State: StateStopped, PID: c.State.Pid, Restarts: c.RestartCount, ExitCode: c.State.ExitCode}
	st.Since, _ = time.Parse(time.RFC3339Nano, c.State.StartedAt)
	if c.Config != nil {
		st.Release = c.Config.Labels[labelRelease]
	}
	switch {
	case c.State.Running && !c.State.Restarting:
		st.State = StateRunning
	case c.State.Restarting || c.State.ExitCode != 0:
		st.State = StateCrashed
	}
	return st
}

// Watch follows the container's logs into the app log and keeps its resource stats
// up to date, reconnecting when the container restarts. It runs until Stop.
func (d *Docker) Watch(app string, log *logs.Stream) {
	d.unwatch(app)
	ctx, cancel := context.WithCancel(context.Background())
	d.mu.Lock()
	d.watchers[app] = cancel
	d.mu.Unlock()
	go d.followLogs(ctx, app, log)
	go d.followStats(ctx, app)
}

func (d *Docker) unwatch(app string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if cancel, ok := d.watchers[app]; ok {
		cancel()
		delete(d.watchers, app)
	}
	delete(d.stats, app)
}

// Unwatch stops following every container, used on platform shutdown. Containers keep running.
func (d *Docker) Unwatch() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for app, cancel := range d.watchers {
		cancel()
		delete(d.watchers, app)
	}
}

func (d *Docker) followLogs(ctx context.Context, app string, log io.Writer) {
	since := time.Now()
	for ctx.Err() == nil {
		body, err := d.cli.ContainerLogs(ctx, containerName(app), client.ContainerLogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true,
			Since:      strconv.FormatInt(since.Unix(), 10),
		})
		if err == nil {
			stdcopy.StdCopy(log, log, body)
			body.Close()
		}
		since = time.Now()
		sleepCtx(ctx, 2*time.Second)
	}
}

func (d *Docker) followStats(ctx context.Context, app string) {
	for ctx.Err() == nil {
		res, err := d.cli.ContainerStats(ctx, containerName(app), client.ContainerStatsOptions{Stream: true})
		if err == nil {
			d.readStats(ctx, app, res.Body)
			res.Body.Close()
		}
		d.mu.Lock()
		delete(d.stats, app)
		d.mu.Unlock()
		sleepCtx(ctx, 2*time.Second)
	}
}

func (d *Docker) readStats(ctx context.Context, app string, body io.Reader) {
	dec := json.NewDecoder(body)
	for ctx.Err() == nil {
		var s container.StatsResponse
		if err := dec.Decode(&s); err != nil {
			return
		}
		d.mu.Lock()
		d.stats[app] = ContainerStats{CPU: cpuPercent(&s), Memory: memoryUsage(&s)}
		d.mu.Unlock()
	}
}

// Stats returns the latest resource usage of every watched container, keyed by app name.
func (d *Docker) Stats() map[string]ContainerStats {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make(map[string]ContainerStats, len(d.stats))
	for k, v := range d.stats {
		out[k] = v
	}
	return out
}

// cpuPercent uses the same formula as `docker stats`.
func cpuPercent(s *container.StatsResponse) float64 {
	if s.OSType == "windows" {
		intervals := uint64(s.Read.Sub(s.PreRead).Nanoseconds()/100) * uint64(max(s.NumProcs, 1))
		if intervals == 0 || s.CPUStats.CPUUsage.TotalUsage < s.PreCPUStats.CPUUsage.TotalUsage {
			return 0
		}
		return float64(s.CPUStats.CPUUsage.TotalUsage-s.PreCPUStats.CPUUsage.TotalUsage) / float64(intervals) * 100
	}
	cpuDelta := float64(s.CPUStats.CPUUsage.TotalUsage) - float64(s.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(s.CPUStats.SystemUsage) - float64(s.PreCPUStats.SystemUsage)
	if cpuDelta <= 0 || sysDelta <= 0 {
		return 0
	}
	cpus := float64(s.CPUStats.OnlineCPUs)
	if cpus == 0 {
		cpus = float64(len(s.CPUStats.CPUUsage.PercpuUsage))
	}
	return cpuDelta / sysDelta * cpus * 100
}

// memoryUsage excludes page cache, like `docker stats`.
func memoryUsage(s *container.StatsResponse) float64 {
	if s.OSType == "windows" {
		return float64(s.MemoryStats.PrivateWorkingSet)
	}
	usage := s.MemoryStats.Usage
	for _, key := range []string{"inactive_file", "total_inactive_file"} {
		if v, ok := s.MemoryStats.Stats[key]; ok && v < usage {
			return float64(usage - v)
		}
	}
	return float64(usage)
}

// MountsFor maps persist paths to bind mounts under the app's shared folder.
func MountsFor(persist []string, shared, workDir string) []Mount {
	var mounts []Mount
	for _, p := range persist {
		clean := strings.TrimSuffix(p, "/")
		mounts = append(mounts, Mount{
			Source: filepath.Join(shared, filepath.FromSlash(clean)),
			Target: path.Join(workDir, clean),
		})
	}
	return mounts
}

func sleepCtx(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

package metrics

import (
	"context"
	"log/slog"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	psnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"

	"moonshed/internal/archive"
	"moonshed/internal/config"
	"moonshed/internal/engine"
	"moonshed/internal/store"
)

const (
	interval     = 5 * time.Second
	diskInterval = 5 * time.Minute
)

type StatusFunc func(*store.App) engine.Status

// Traffic is what the proxy counted for one host since the last collection.
type Traffic struct {
	Requests  int64
	Status4xx int64
	Status5xx int64
	Latency   time.Duration
}

type ServerStats struct {
	Hostname  string  `json:"hostname"`
	OS        string  `json:"os"`
	Cores     int     `json:"cores"`
	Uptime    uint64  `json:"uptime"`
	CPU       float64 `json:"cpu"`
	MemUsed   float64 `json:"mem_used"`
	MemTotal  float64 `json:"mem_total"`
	DiskUsed  float64 `json:"disk_used"`
	DiskTotal float64 `json:"disk_total"`
	NetIn     float64 `json:"net_in"`
	NetOut    float64 `json:"net_out"`
	Load1     float64 `json:"load1"`
}

type AppStats struct {
	CPU       float64 `json:"cpu"`
	Memory    float64 `json:"memory"`
	Processes int     `json:"processes"`
	Disk      int64   `json:"disk"`
}

type Collector struct {
	db      *DB
	store   *store.Store
	paths   config.Paths
	status  StatusFunc
	docker  *engine.Docker
	traffic func() map[string]Traffic

	mu       sync.Mutex
	agg      *aggregator
	server   ServerStats
	apps     map[string]AppStats
	disk     map[string]int64
	diskAt   time.Time
	prevCPU  map[string]cpuMark
	prevNet  psnet.IOCountersStat
	prevTime time.Time
}

type cpuMark struct {
	pid     int
	seconds float64
	at      time.Time
}

func NewCollector(db *DB, st *store.Store, paths config.Paths, status StatusFunc, docker *engine.Docker, traffic func() map[string]Traffic) *Collector {
	return &Collector{
		db:      db,
		store:   st,
		paths:   paths,
		status:  status,
		docker:  docker,
		traffic: traffic,
		agg:     newAggregator(),
		apps:    map[string]AppStats{},
		disk:    map[string]int64{},
		prevCPU: map[string]cpuMark{},
	}
}

func (c *Collector) Run(ctx context.Context) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	lastPrune := time.Now()
	for {
		select {
		case <-ctx.Done():
			c.save(c.closeAll())
			return
		case now := <-ticker.C:
			c.collect(now)
			if now.Sub(lastPrune) >= time.Minute {
				if err := c.db.prune(now); err != nil {
					slog.Error("metrics: prune", "err", err)
				}
				lastPrune = now
			}
		}
	}
}

// Record adds a sample from elsewhere, e.g. health check latency.
func (c *Collector) Record(target, metric string, value float64) {
	c.mu.Lock()
	closed := c.agg.add(time.Now(), target, metric, value)
	c.mu.Unlock()
	c.save(closed)
}

func (c *Collector) Server() ServerStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.server
}

// App returns the latest stats for an app. CPU, memory and processes are only set while it runs.
func (c *Collector) App(id string) AppStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.apps[id]
}

func AppTarget(id string) string      { return "app:" + id }
func DomainTarget(host string) string { return "domain:" + host }

func (c *Collector) collect(now time.Time) {
	server := c.collectServer(now)
	type sample struct {
		target, metric string
		value          float64
	}
	samples := []sample{
		{"server", "cpu", server.CPU},
		{"server", "mem", server.MemUsed},
		{"server", "disk", server.DiskUsed},
		{"server", "net_in", server.NetIn},
		{"server", "net_out", server.NetOut},
		{"server", "load", server.Load1},
	}

	apps, err := c.store.Apps()
	if err != nil {
		slog.Error("metrics: list apps", "err", err)
		return
	}
	appStats, running := c.collectApps(apps, now)
	for id := range running {
		s, t := appStats[id], AppTarget(id)
		samples = append(samples, sample{t, "cpu", s.CPU}, sample{t, "mem", s.Memory}, sample{t, "procs", float64(s.Processes)})
	}
	for host, t := range c.traffic() {
		target := DomainTarget(host)
		samples = append(samples,
			sample{target, "requests", float64(t.Requests)},
			sample{target, "4xx", float64(t.Status4xx)},
			sample{target, "5xx", float64(t.Status5xx)},
			sample{target, "latency", float64(t.Latency.Microseconds()) / 1000})
	}

	c.mu.Lock()
	var closed []point
	for _, s := range samples {
		closed = append(closed, c.agg.add(now, s.target, s.metric, s.value)...)
	}
	closed = append(closed, c.agg.flush(now)...)
	c.server = server
	c.apps = appStats
	c.mu.Unlock()
	c.save(closed)
}

func (c *Collector) closeAll() []point {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.agg.flush(time.Now().Add(MaxRange))
}

func (c *Collector) save(points []point) {
	if err := c.db.write(points); err != nil {
		slog.Error("metrics: write", "err", err)
	}
}

func (c *Collector) collectServer(now time.Time) ServerStats {
	s := ServerStats{OS: runtime.GOOS, Cores: runtime.NumCPU()}
	s.Hostname, _ = os.Hostname()
	s.Uptime, _ = host.Uptime()
	if p, err := cpu.Percent(0, false); err == nil && len(p) > 0 {
		s.CPU = p[0]
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		s.MemUsed, s.MemTotal = float64(vm.Used), float64(vm.Total)
	}
	if du, err := disk.Usage(c.paths.Root); err == nil {
		s.DiskUsed, s.DiskTotal = float64(du.Used), float64(du.Total)
	}
	if l, err := load.Avg(); err == nil {
		s.Load1 = l.Load1
	}
	if io, err := psnet.IOCounters(false); err == nil && len(io) > 0 {
		if !c.prevTime.IsZero() {
			elapsed := now.Sub(c.prevTime).Seconds()
			s.NetIn = float64(io[0].BytesRecv-c.prevNet.BytesRecv) / elapsed
			s.NetOut = float64(io[0].BytesSent-c.prevNet.BytesSent) / elapsed
		}
		c.prevNet, c.prevTime = io[0], now
	}
	return s
}

// collectApps measures every running app. Apps that aren't running produce no samples.
func (c *Collector) collectApps(apps []*store.App, now time.Time) (map[string]AppStats, map[string]bool) {
	stats := map[string]AppStats{}
	running := map[string]bool{}
	var tree map[int32][]int32
	var dockerStats map[string]engine.ContainerStats

	refreshDisk := now.Sub(c.diskAt) >= diskInterval
	if refreshDisk {
		c.diskAt = now
	}
	for _, app := range apps {
		if _, measured := c.disk[app.ID]; refreshDisk || !measured {
			c.disk[app.ID] = archive.DirSize(c.paths.App(app.Name).Root)
		}
		s := AppStats{Disk: c.disk[app.ID]}
		st := c.status(app)
		if st.State != engine.StateRunning {
			delete(c.prevCPU, app.ID)
			stats[app.ID] = s
			continue
		}

		if app.Runtime == store.RuntimeDocker {
			if dockerStats == nil {
				dockerStats = c.docker.Stats()
			}
			ds, ok := dockerStats[app.Name]
			if !ok {
				stats[app.ID] = s
				continue
			}
			s.CPU = ds.CPU / float64(runtime.NumCPU())
			s.Memory, s.Processes = ds.Memory, 1
			stats[app.ID], running[app.ID] = s, true
			continue
		}

		if tree == nil {
			tree = processTree()
		}
		pids := descendants(int32(st.PID), tree)
		var cpuSeconds float64
		for _, pid := range pids {
			p := &process.Process{Pid: pid}
			if t, err := p.Times(); err == nil {
				cpuSeconds += t.User + t.System
			}
			if m, err := p.MemoryInfo(); err == nil {
				s.Memory += float64(m.RSS)
			}
		}
		s.Processes = len(pids)
		prev, ok := c.prevCPU[app.ID]
		c.prevCPU[app.ID] = cpuMark{pid: st.PID, seconds: cpuSeconds, at: now}
		if !ok || prev.pid != st.PID {
			// CPU needs two readings of the same process; skip this round rather than guess.
			stats[app.ID] = s
			continue
		}
		if elapsed := now.Sub(prev.at).Seconds(); elapsed > 0 {
			s.CPU = max(0, (cpuSeconds-prev.seconds)/elapsed/float64(runtime.NumCPU())*100)
		}
		stats[app.ID], running[app.ID] = s, true
	}
	return stats, running
}

func processTree() map[int32][]int32 {
	procs, err := process.Processes()
	if err != nil {
		return nil
	}
	tree := map[int32][]int32{}
	for _, p := range procs {
		if ppid, err := p.Ppid(); err == nil {
			tree[ppid] = append(tree[ppid], p.Pid)
		}
	}
	return tree
}

func descendants(root int32, tree map[int32][]int32) []int32 {
	out := []int32{root}
	seen := map[int32]bool{root: true}
	for i := 0; i < len(out); i++ {
		for _, child := range tree[out[i]] {
			if !seen[child] {
				seen[child] = true
				out = append(out, child)
			}
		}
	}
	return out
}

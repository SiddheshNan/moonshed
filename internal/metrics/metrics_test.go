package metrics

import (
	"path/filepath"
	"testing"
	"time"

	psnet "github.com/shirou/gopsutil/v4/net"
)

func TestAggregateAndRead(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "metrics.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	agg := newAggregator()
	start := time.Now().Truncate(time.Minute).Add(-2 * time.Minute)
	var closed []point
	// One minute of samples every 5s: cpu 10..120, 3 requests per sample, 30ms each.
	for i := range 12 {
		ts := start.Add(time.Duration(i) * 5 * time.Second)
		closed = append(closed, agg.add(ts, "app:1", "cpu", float64(10*(i+1)))...)
		closed = append(closed, agg.add(ts, "domain:x", "requests", 3)...)
		closed = append(closed, agg.add(ts, "domain:x", "latency", 90)...)
	}
	closed = append(closed, agg.flush(start.Add(time.Minute))...)
	if err := db.write(closed); err != nil {
		t.Fatal(err)
	}

	// 5m range reads the 5s tier: one point per sample.
	fine, err := db.Series("app:1", []string{"cpu"}, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(fine["cpu"]) != 12 {
		t.Fatalf("5s tier: got %d points, want 12", len(fine["cpu"]))
	}

	// 1h range reads the 1m tier: one bucket averaging the minute.
	coarse, err := db.Series("app:1", []string{"cpu"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if got := coarse["cpu"]; len(got) != 1 || got[0].V != 65 || *got[0].Max != 120 {
		t.Fatalf("1m tier: got %+v, want avg 65 max 120", got)
	}

	traffic, err := db.Series("domain:x", []string{"requests", "latency"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	// 36 requests in a 60s bucket is 0.6/s; 1080ms over 36 requests is 30ms each.
	if r := traffic["requests"]; len(r) != 1 || r[0].V != 0.6 {
		t.Fatalf("requests: got %+v, want 0.6/s", r)
	}
	if l := traffic["latency"]; len(l) != 1 || l[0].V != 30 {
		t.Fatalf("latency: got %+v, want 30ms", l)
	}

	empty, err := db.Series("app:2", []string{"cpu"}, time.Hour)
	if err != nil || len(empty["cpu"]) != 0 {
		t.Fatalf("unknown target should have no points, got %+v %v", empty, err)
	}
}

func TestNetDeltaIgnoresResetCounters(t *testing.T) {
	prev := map[string]psnet.IOCountersStat{
		"en0":  {Name: "en0", BytesRecv: 1000, BytesSent: 500},
		"awdl": {Name: "awdl", BytesRecv: 9000, BytesSent: 9000},
	}
	// After sleep/wake: en0 grew, awdl was reset to near zero, utun is new.
	cur := []psnet.IOCountersStat{
		{Name: "en0", BytesRecv: 1600, BytesSent: 700},
		{Name: "awdl", BytesRecv: 10, BytesSent: 20},
		{Name: "utun3", BytesRecv: 5000, BytesSent: 5000},
	}
	in, out := netDelta(prev, cur)
	if in != 600 || out != 200 {
		t.Fatalf("got in=%d out=%d, want 600 and 200", in, out)
	}
}

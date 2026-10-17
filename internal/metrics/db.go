package metrics

import (
	"database/sql"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/SiddheshNan/moonshed/internal/store"
)

var migrations = []string{
	`CREATE TABLE samples (target TEXT NOT NULL, metric TEXT NOT NULL, ts INTEGER NOT NULL, value REAL NOT NULL);`,
	`
DROP TABLE IF EXISTS samples;
DROP TABLE IF EXISTS samples_1m;
DROP TABLE IF EXISTS samples_1h;
DROP TABLE IF EXISTS rollups;

CREATE TABLE targets (
	id  INTEGER PRIMARY KEY,
	key TEXT NOT NULL UNIQUE
);

CREATE TABLE points (
	target INTEGER NOT NULL,
	metric INTEGER NOT NULL,
	tier   INTEGER NOT NULL,
	ts     INTEGER NOT NULL,
	sum    REAL NOT NULL,
	min    REAL NOT NULL,
	max    REAL NOT NULL,
	count  INTEGER NOT NULL,
	PRIMARY KEY (target, metric, tier, ts)
) WITHOUT ROWID;
`}

// A tier keeps buckets of one size for a fixed time, so storage never grows.
type tier struct {
	step time.Duration
	keep time.Duration
}

var tiers = []tier{
	{step: 5 * time.Second, keep: 15 * time.Minute},
	{step: time.Minute, keep: 12 * time.Hour},
	{step: 5 * time.Minute, keep: 48 * time.Hour},
	{step: 30 * time.Minute, keep: 7 * 24 * time.Hour},
}

// MaxRange is the longest range any chart can ask for.
var MaxRange = tiers[len(tiers)-1].keep

type kind int

const (
	gauge   kind = iota // averaged within a bucket, e.g. CPU %
	counter             // summed within a bucket and shown per second, e.g. requests
)

type metricDef struct {
	id   int
	kind kind
}

var metricDefs = map[string]metricDef{
	"cpu":       {1, gauge},
	"mem":       {2, gauge},
	"disk":      {3, gauge},
	"net_in":    {4, gauge},
	"net_out":   {5, gauge},
	"load":      {6, gauge},
	"procs":     {7, gauge},
	"health_ms": {8, gauge},
	"requests":  {9, counter},
	"4xx":       {10, counter},
	"5xx":       {11, counter},
	"latency":   {12, counter}, // total ms; read back as average per request
}

type DB struct {
	db *sql.DB

	mu      sync.Mutex
	targets map[string]int64
}

func OpenDB(path string) (*DB, error) {
	db, err := store.OpenDB(path)
	if err != nil {
		return nil, err
	}
	if err := store.Migrate(db, migrations); err != nil {
		db.Close()
		return nil, err
	}
	return &DB{db: db, targets: map[string]int64{}}, nil
}

func (d *DB) Close() error { return d.db.Close() }

func (d *DB) targetID(key string, create bool) (int64, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if id, ok := d.targets[key]; ok {
		return id, true, nil
	}
	var id int64
	err := d.db.QueryRow("SELECT id FROM targets WHERE key = ?", key).Scan(&id)
	if err == sql.ErrNoRows {
		if !create {
			return 0, false, nil
		}
		res, err := d.db.Exec("INSERT INTO targets (key) VALUES (?)", key)
		if err != nil {
			return 0, false, err
		}
		if id, err = res.LastInsertId(); err != nil {
			return 0, false, err
		}
	} else if err != nil {
		return 0, false, err
	}
	d.targets[key] = id
	return id, true, nil
}

// point is one closed bucket of one series in one tier.
type point struct {
	target string
	metric int
	tier   int
	start  int64
	b      bucket
}

func (d *DB) write(points []point) error {
	if len(points) == 0 {
		return nil
	}
	ids := map[string]int64{}
	for _, p := range points {
		if _, ok := ids[p.target]; !ok {
			id, _, err := d.targetID(p.target, true)
			if err != nil {
				return err
			}
			ids[p.target] = id
		}
	}
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare("INSERT OR REPLACE INTO points (target, metric, tier, ts, sum, min, max, count) VALUES (?, ?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, p := range points {
		if _, err := stmt.Exec(ids[p.target], p.metric, p.tier, p.start, p.b.sum, p.b.min, p.b.max, p.b.count); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// prune drops buckets that fell out of their tier's window.
func (d *DB) prune(now time.Time) error {
	for i, t := range tiers {
		if _, err := d.db.Exec("DELETE FROM points WHERE tier = ? AND ts < ?", i, now.Add(-t.keep).Unix()); err != nil {
			return err
		}
	}
	return nil
}

type Point struct {
	T   int64    `json:"t"`
	V   float64  `json:"v"`
	Max *float64 `json:"max,omitempty"`
}

// Series returns points for each metric of a target from the finest tier that covers the range.
// Gauges come back as the bucket average (with the bucket max), counters as a per-second rate.
func (d *DB) Series(target string, metrics []string, span time.Duration) (map[string][]Point, error) {
	out := map[string][]Point{}
	for _, m := range metrics {
		out[m] = []Point{}
	}
	id, ok, err := d.targetID(target, false)
	if err != nil || !ok {
		return out, err
	}

	ti := len(tiers) - 1
	for i, t := range tiers {
		if span <= t.keep {
			ti = i
			break
		}
	}
	step := tiers[ti].step.Seconds()
	from := time.Now().Add(-span).Unix()

	ids := []any{id, ti, from}
	names := map[int]string{}
	wanted := append([]string(nil), metrics...)
	if slices.Contains(metrics, "latency") && !slices.Contains(metrics, "requests") {
		wanted = append(wanted, "requests")
	}
	for _, m := range wanted {
		if def, ok := metricDefs[m]; ok {
			ids = append(ids, def.id)
			names[def.id] = m
		}
	}
	rows, err := d.db.Query(`SELECT metric, ts, sum, max, count FROM points
		WHERE target = ? AND tier = ? AND ts >= ? AND metric IN (`+placeholders(len(ids)-3)+`) ORDER BY ts`, ids...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	requests := map[int64]float64{}
	latency := []Point{}
	for rows.Next() {
		var metric int
		var ts, count int64
		var sum, maxV float64
		if err := rows.Scan(&metric, &ts, &sum, &maxV, &count); err != nil {
			return nil, err
		}
		name := names[metric]
		switch {
		case name == "latency":
			latency = append(latency, Point{T: ts, V: sum})
		case metricDefs[name].kind == counter:
			if name == "requests" {
				requests[ts] = sum
			}
			out[name] = append(out[name], Point{T: ts, V: sum / step})
		default:
			out[name] = append(out[name], Point{T: ts, V: sum / float64(count), Max: &maxV})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if _, asked := out["latency"]; asked {
		for _, p := range latency {
			if n := requests[p.T]; n > 0 {
				out["latency"] = append(out["latency"], Point{T: p.T, V: p.V / n})
			}
		}
	}
	if !slices.Contains(metrics, "requests") {
		delete(out, "requests")
	}
	return out, nil
}

func (d *DB) DeleteTarget(key string) error {
	id, ok, err := d.targetID(key, false)
	if err != nil || !ok {
		return err
	}
	if _, err := d.db.Exec("DELETE FROM points WHERE target = ?", id); err != nil {
		return err
	}
	if _, err := d.db.Exec("DELETE FROM targets WHERE id = ?", id); err != nil {
		return err
	}
	d.mu.Lock()
	delete(d.targets, key)
	d.mu.Unlock()
	return nil
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

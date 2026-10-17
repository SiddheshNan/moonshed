// Package requestlog records proxied requests while recording is switched on, for debugging.
// Nothing is kept once recording stops; while it runs each app keeps at most PerAppCap requests.
package requestlog

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SiddheshNan/moonshed/internal/store"
)

const (
	PerAppCap = 100_000
	trimSlack = 1_000
	batchSize = 500
)

var migrations = []string{`
CREATE TABLE requests (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	app_id      TEXT NOT NULL,
	ts          INTEGER NOT NULL,
	method      TEXT NOT NULL,
	host        TEXT NOT NULL,
	path        TEXT NOT NULL,
	query       TEXT NOT NULL,
	status      INTEGER NOT NULL,
	duration_ms REAL NOT NULL,
	bytes       INTEGER NOT NULL,
	ip          TEXT NOT NULL,
	user_agent  TEXT NOT NULL,
	referer     TEXT NOT NULL
);
CREATE INDEX requests_app ON requests(app_id, id);
`}

type Entry struct {
	ID         int64   `json:"id"`
	AppID      string  `json:"app_id"`
	Time       int64   `json:"time"` // unix milliseconds
	Method     string  `json:"method"`
	Host       string  `json:"host"`
	Path       string  `json:"path"`
	Query      string  `json:"query"`
	Status     int     `json:"status"`
	DurationMS float64 `json:"duration_ms"`
	Bytes      int64   `json:"bytes"`
	IP         string  `json:"ip"`
	UserAgent  string  `json:"user_agent"`
	Referer    string  `json:"referer"`
}

type Log struct {
	db        *sql.DB
	ch        chan Entry
	recording atomic.Pointer[map[string]bool]

	mu     sync.Mutex
	counts map[string]int
}

func Open(path string) (*Log, error) {
	db, err := store.OpenDB(path)
	if err != nil {
		return nil, err
	}
	if err := store.Migrate(db, migrations); err != nil {
		db.Close()
		return nil, err
	}
	l := &Log{db: db, ch: make(chan Entry, 4096), counts: map[string]int{}}
	l.recording.Store(&map[string]bool{})
	rows, err := db.Query("SELECT app_id, COUNT(*) FROM requests GROUP BY app_id")
	if err != nil {
		db.Close()
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var app string
		var n int
		if err := rows.Scan(&app, &n); err != nil {
			db.Close()
			return nil, err
		}
		l.counts[app] = n
	}
	return l, rows.Err()
}

func (l *Log) Close() error { return l.db.Close() }

// Configure sets which apps are recorded. Requests of apps that are no longer recorded
// are deleted, so turning recording off (no apps) clears everything.
func (l *Log) Configure(appIDs []string) error {
	set := make(map[string]bool, len(appIDs))
	for _, id := range appIDs {
		set[id] = true
	}
	l.recording.Store(&set)

	l.mu.Lock()
	defer l.mu.Unlock()
	var drop []string
	var args []any
	for app := range l.counts {
		if !set[app] {
			drop, args = append(drop, "?"), append(args, app)
			delete(l.counts, app)
		}
	}
	if len(drop) == 0 {
		return nil
	}
	_, err := l.db.Exec("DELETE FROM requests WHERE app_id IN ("+strings.Join(drop, ",")+")", args...)
	if err == nil && len(set) == 0 {
		_, err = l.db.Exec("VACUUM")
	}
	return err
}

// Recording reports whether requests for the app should be recorded. Safe on the hot path.
func (l *Log) Recording(appID string) bool {
	return (*l.recording.Load())[appID]
}

// Record queues an entry; it is dropped rather than slowing the proxy down if the queue is full.
func (l *Log) Record(e Entry) {
	select {
	case l.ch <- e:
	default:
	}
}

func (l *Log) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var batch []Entry
	for {
		select {
		case <-ctx.Done():
			l.write(batch)
			return
		case e := <-l.ch:
			batch = append(batch, e)
			if len(batch) >= batchSize {
				l.write(batch)
				batch = nil
			}
		case <-ticker.C:
			l.write(batch)
			batch = nil
		}
	}
}

func (l *Log) write(batch []Entry) {
	if len(batch) == 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	tx, err := l.db.Begin()
	if err != nil {
		slog.Error("requests: write", "err", err)
		return
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO requests (app_id, ts, method, host, path, query, status, duration_ms, bytes, ip, user_agent, referer)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		slog.Error("requests: write", "err", err)
		return
	}
	defer stmt.Close()
	touched := map[string]int{}
	for _, e := range batch {
		// Recording may have been switched off for this app while the entry was queued.
		if !l.Recording(e.AppID) {
			continue
		}
		if _, err := stmt.Exec(e.AppID, e.Time, e.Method, e.Host, e.Path, e.Query, e.Status, e.DurationMS, e.Bytes, e.IP, e.UserAgent, e.Referer); err != nil {
			slog.Error("requests: write", "err", err)
			return
		}
		touched[e.AppID]++
	}
	if err := tx.Commit(); err != nil {
		slog.Error("requests: write", "err", err)
		return
	}
	for app, n := range touched {
		l.counts[app] += n
		if l.counts[app] > PerAppCap+trimSlack {
			l.trim(app)
		}
	}
}

// trim keeps only the newest PerAppCap requests of an app.
func (l *Log) trim(app string) {
	_, err := l.db.Exec(`DELETE FROM requests WHERE app_id = ? AND id <=
		(SELECT id FROM requests WHERE app_id = ? ORDER BY id DESC LIMIT 1 OFFSET ?)`, app, app, PerAppCap)
	if err != nil {
		slog.Error("requests: trim", "app", app, "err", err)
		return
	}
	l.counts[app] = PerAppCap
}

func (l *Log) Counts() map[string]int {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make(map[string]int, len(l.counts))
	for k, v := range l.counts {
		out[k] = v
	}
	return out
}

type Filter struct {
	AppID  string
	Status string // "2xx", "3xx", "4xx", "5xx"
	Method string
	Path   string // substring
	Before int64  // entry id, for paging
	Limit  int
}

func (l *Log) Query(f Filter) ([]Entry, error) {
	where := []string{"1 = 1"}
	var args []any
	if f.AppID != "" {
		where, args = append(where, "app_id = ?"), append(args, f.AppID)
	}
	if len(f.Status) == 3 && strings.HasSuffix(f.Status, "xx") && f.Status[0] >= '1' && f.Status[0] <= '5' {
		lo := int(f.Status[0]-'0') * 100
		where, args = append(where, "status >= ? AND status < ?"), append(args, lo, lo+100)
	}
	if f.Method != "" {
		where, args = append(where, "method = ?"), append(args, strings.ToUpper(f.Method))
	}
	if f.Path != "" {
		where, args = append(where, "instr(path, ?) > 0"), append(args, f.Path)
	}
	if f.Before != 0 {
		where, args = append(where, "id < ?"), append(args, f.Before)
	}
	rows, err := l.db.Query(`SELECT id, app_id, ts, method, host, path, query, status, duration_ms, bytes, ip, user_agent, referer
		FROM requests WHERE `+strings.Join(where, " AND ")+" ORDER BY id DESC LIMIT ?", append(args, f.Limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.AppID, &e.Time, &e.Method, &e.Host, &e.Path, &e.Query, &e.Status,
			&e.DurationMS, &e.Bytes, &e.IP, &e.UserAgent, &e.Referer); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

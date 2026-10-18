package store

import (
	"encoding/json"
	"strings"
	"time"
)

type Incident struct {
	ID     string `json:"id"`
	AppID  string `json:"app_id"`
	DownAt Time   `json:"down_at"`
	UpAt   Time   `json:"up_at"`
	Reason string `json:"reason"`
}

// OpenIncident records that the app went down, unless an incident is already open.
func (s *Store) OpenIncident(appID, reason string) error {
	_, err := s.db.Exec(`INSERT INTO incidents (id, app_id, down_at, reason) SELECT ?, ?, ?, ?
		WHERE NOT EXISTS (SELECT 1 FROM incidents WHERE app_id = ? AND up_at IS NULL)`, NewID(), appID, Now(), reason, appID)
	return err
}

func (s *Store) HasOpenIncident(appID string) bool {
	var n int
	s.db.QueryRow("SELECT COUNT(*) FROM incidents WHERE app_id = ? AND up_at IS NULL", appID).Scan(&n)
	return n > 0
}

func (s *Store) CloseIncident(appID string) error {
	_, err := s.db.Exec("UPDATE incidents SET up_at = ? WHERE app_id = ? AND up_at IS NULL", Now(), appID)
	return err
}

func (s *Store) Incidents(appID string, since time.Time) ([]*Incident, error) {
	rows, err := s.db.Query(`SELECT id, app_id, down_at, up_at, reason FROM incidents
		WHERE app_id = ? AND (up_at IS NULL OR up_at >= ?) ORDER BY down_at DESC`, appID, since.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Incident
	for rows.Next() {
		var i Incident
		if err := rows.Scan(&i.ID, &i.AppID, &i.DownAt, &i.UpAt, &i.Reason); err != nil {
			return nil, err
		}
		out = append(out, &i)
	}
	return out, rows.Err()
}

// UptimeCount is how many health checks ran and passed for an app in one hour.
type UptimeCount struct {
	AppID string
	Hour  int64 // unix seconds at the start of the hour
	OK    int
	Total int
}

// AddUptime counts one health check in the app's row for the hour it ran in.
func (s *Store) AddUptime(appID string, at time.Time, passed bool) error {
	_, err := s.db.Exec(`INSERT INTO uptime (app_id, hour, ok, total) VALUES (?, ?, ?, 1)
		ON CONFLICT (app_id, hour) DO UPDATE SET ok = ok + excluded.ok, total = total + 1`,
		appID, at.Truncate(time.Hour).Unix(), boolInt(passed))
	if err != nil && strings.Contains(err.Error(), "FOREIGN KEY") {
		return nil // the app was deleted while its check was running
	}
	return err
}

// Uptime returns the hourly check counts for an app since the given time.
func (s *Store) Uptime(appID string, since time.Time) ([]UptimeCount, error) {
	rows, err := s.db.Query("SELECT hour, ok, total FROM uptime WHERE app_id = ? AND hour >= ? ORDER BY hour", appID, since.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UptimeCount
	for rows.Next() {
		c := UptimeCount{AppID: appID}
		if err := rows.Scan(&c.Hour, &c.OK, &c.Total); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) PruneUptime(before time.Time) error {
	_, err := s.db.Exec("DELETE FROM uptime WHERE hour < ?", before.Unix())
	return err
}

const (
	BackupRunning = "running"
	BackupSuccess = "success"
	BackupFailed  = "failed"
)

type Backup struct {
	ID        string      `json:"id"`
	AppID     string      `json:"app_id"`
	File      string      `json:"file"`
	Size      int64       `json:"size"`
	Status    string      `json:"status"`
	Error     string      `json:"error"`
	Parts     BackupParts `json:"parts"`
	CreatedAt Time        `json:"created_at"`
}

// BackupParts says what an app backup includes.
type BackupParts struct {
	Code bool `json:"code"`
	Data bool `json:"data"`
	Env  bool `json:"env"`
	Logs bool `json:"logs"`
}

func (p BackupParts) Any() bool { return p.Code || p.Data || p.Env || p.Logs }

func (s *Store) CreateBackup(b *Backup) error {
	b.ID, b.CreatedAt = NewID(), Now()
	b.Status = BackupRunning
	parts, err := json.Marshal(b.Parts)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("INSERT INTO backups (id, app_id, file, status, parts, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		b.ID, nullID(b.AppID), b.File, b.Status, string(parts), b.CreatedAt)
	return err
}

func (s *Store) UpdateBackup(b *Backup) error {
	_, err := s.db.Exec("UPDATE backups SET size = ?, status = ?, error = ? WHERE id = ?", b.Size, b.Status, b.Error, b.ID)
	return err
}

// Backups lists backups for an app, or platform backups when appID is empty.
func (s *Store) Backups(appID string) ([]*Backup, error) {
	where := "app_id IS NULL"
	var args []any
	if appID != "" {
		where = "app_id = ?"
		args = append(args, appID)
	}
	rows, err := s.db.Query("SELECT "+backupColumns+" FROM backups WHERE "+where+" ORDER BY id DESC", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Backup
	for rows.Next() {
		b, err := scanBackup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) Backup(id string) (*Backup, error) {
	return scanBackup(s.db.QueryRow("SELECT "+backupColumns+" FROM backups WHERE id = ?", id))
}

const backupColumns = "id, COALESCE(app_id, ''), file, size, status, error, parts, created_at"

func scanBackup(row interface{ Scan(...any) error }) (*Backup, error) {
	var b Backup
	var parts string
	if err := row.Scan(&b.ID, &b.AppID, &b.File, &b.Size, &b.Status, &b.Error, &parts, &b.CreatedAt); err != nil {
		return nil, notFound(err)
	}
	return &b, json.Unmarshal([]byte(parts), &b.Parts)
}

func (s *Store) DeleteBackup(id string) error {
	_, err := s.db.Exec("DELETE FROM backups WHERE id = ?", id)
	return err
}

func (s *Store) FailRunningBackups() error {
	_, err := s.db.Exec("UPDATE backups SET status = ?, error = 'interrupted by platform restart' WHERE status = ?", BackupFailed, BackupRunning)
	return err
}

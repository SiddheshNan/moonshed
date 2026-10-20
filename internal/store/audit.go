package store

import "strings"

const (
	ActorUser    = "user"
	ActorToken   = "token"
	ActorWebhook = "webhook"
	ActorSystem  = "system"
)

type AuditEntry struct {
	Seq        int64  `json:"seq"`
	Time       Time   `json:"time"`
	ActorType  string `json:"actor_type"`
	ActorID    string `json:"actor_id"`
	ActorName  string `json:"actor_name"`
	IP         string `json:"ip"`
	Action     string `json:"action"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	TargetName string `json:"target_name"`
	Details    string `json:"details"`
}

func (s *Store) AddAudit(e *AuditEntry) error {
	e.Time = Now()
	_, err := s.db.Exec(`INSERT INTO audit_log (ts, actor_type, actor_id, actor_name, ip, action, target_type, target_id, target_name, details)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Time, e.ActorType, e.ActorID, e.ActorName, e.IP, e.Action, e.TargetType, e.TargetID, e.TargetName, e.Details)
	return err
}

type AuditFilter struct {
	Actor      string
	Action     string // prefix, e.g. "app." matches every app action
	TargetType string
	TargetID   string
	Before     int64 // seq, for paging
	Limit      int
}

func (s *Store) Audit(f AuditFilter) ([]*AuditEntry, error) {
	var where []string
	var args []any
	if f.Actor != "" {
		where, args = append(where, "actor_name = ?"), append(args, f.Actor)
	}
	if f.Action != "" {
		where, args = append(where, "action LIKE ?"), append(args, f.Action+"%")
	}
	if f.TargetType != "" {
		where, args = append(where, "target_type = ?"), append(args, f.TargetType)
	}
	if f.TargetID != "" {
		where, args = append(where, "target_id = ?"), append(args, f.TargetID)
	}
	if f.Before != 0 {
		where, args = append(where, "seq < ?"), append(args, f.Before)
	}
	query := "SELECT seq, ts, actor_type, actor_id, actor_name, ip, action, target_type, target_id, target_name, details FROM audit_log"
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	rows, err := s.db.Query(query+" ORDER BY seq DESC LIMIT ?", append(args, f.Limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.Seq, &e.Time, &e.ActorType, &e.ActorID, &e.ActorName, &e.IP, &e.Action,
			&e.TargetType, &e.TargetID, &e.TargetName, &e.Details); err != nil {
			return nil, err
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}

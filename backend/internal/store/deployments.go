package store

import (
	"encoding/json"

	"moonshed/internal/manifest"
)

type Bundle struct {
	ID        string `json:"id"`
	AppID     string `json:"app_id"`
	Source    string `json:"source"`
	GitCommit string `json:"git_commit"`
	Size      int64  `json:"size"`
	CreatedAt Time   `json:"created_at"`
}

func (s *Store) CreateBundle(b *Bundle) error {
	b.ID, b.CreatedAt = NewID(), Now()
	_, err := s.db.Exec("INSERT INTO bundles (id, app_id, source, git_commit, size, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		b.ID, b.AppID, b.Source, b.GitCommit, b.Size, b.CreatedAt)
	return err
}

func (s *Store) SetBundleSize(id string, size int64) error {
	_, err := s.db.Exec("UPDATE bundles SET size = ? WHERE id = ?", size, id)
	return err
}

func (s *Store) Bundle(id string) (*Bundle, error) {
	var b Bundle
	err := s.db.QueryRow("SELECT id, app_id, source, git_commit, size, created_at FROM bundles WHERE id = ?", id).
		Scan(&b.ID, &b.AppID, &b.Source, &b.GitCommit, &b.Size, &b.CreatedAt)
	return &b, notFound(err)
}

func (s *Store) Bundles(appID string) ([]*Bundle, error) {
	rows, err := s.db.Query("SELECT id, app_id, source, git_commit, size, created_at FROM bundles WHERE app_id = ? ORDER BY id DESC", appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Bundle
	for rows.Next() {
		var b Bundle
		if err := rows.Scan(&b.ID, &b.AppID, &b.Source, &b.GitCommit, &b.Size, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &b)
	}
	return out, rows.Err()
}

func (s *Store) DeleteBundle(id string) error {
	_, err := s.db.Exec("DELETE FROM bundles WHERE id = ?", id)
	return err
}

type Release struct {
	ID        string             `json:"id"`
	AppID     string             `json:"app_id"`
	BundleID  string             `json:"bundle_id"`
	Manifest  *manifest.Manifest `json:"manifest"`
	Image     string             `json:"image"`
	CreatedAt Time               `json:"created_at"`
}

func (s *Store) CreateRelease(r *Release) error {
	r.ID, r.CreatedAt = NewID(), Now()
	_, err := s.db.Exec("INSERT INTO releases (id, app_id, bundle_id, created_at) VALUES (?, ?, ?, ?)", r.ID, r.AppID, r.BundleID, r.CreatedAt)
	return err
}

func (s *Store) UpdateRelease(r *Release) error {
	m, err := json.Marshal(r.Manifest)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("UPDATE releases SET manifest = ?, image = ? WHERE id = ?", string(m), r.Image, r.ID)
	return err
}

func (s *Store) Release(id string) (*Release, error) {
	var r Release
	var m string
	err := s.db.QueryRow("SELECT id, app_id, bundle_id, manifest, image, created_at FROM releases WHERE id = ?", id).
		Scan(&r.ID, &r.AppID, &r.BundleID, &m, &r.Image, &r.CreatedAt)
	if err != nil {
		return nil, notFound(err)
	}
	if m != "null" {
		r.Manifest = &manifest.Manifest{}
		if err := json.Unmarshal([]byte(m), r.Manifest); err != nil {
			return nil, err
		}
	}
	return &r, nil
}

// DeleteOtherReleases removes release rows left behind by interrupted deployments.
func (s *Store) DeleteOtherReleases(appID, keepID string) error {
	_, err := s.db.Exec("DELETE FROM releases WHERE app_id = ? AND id != ?", appID, keepID)
	return err
}

func (s *Store) DeleteRelease(id string) error {
	_, err := s.db.Exec("DELETE FROM releases WHERE id = ?", id)
	return err
}

const (
	DeployQueued   = "queued"
	DeployBuilding = "building"
	DeployStarting = "starting"
	DeploySuccess  = "success"
	DeployFailed   = "failed"
	DeployKilled   = "killed"

	TriggerUpload   = "upload"
	TriggerGit      = "git"
	TriggerWebhook  = "webhook"
	TriggerRedeploy = "redeploy"
	TriggerAPI      = "api"
)

type Deployment struct {
	ID         string `json:"id"`
	AppID      string `json:"app_id"`
	BundleID   string `json:"bundle_id"`
	ReleaseID  string `json:"release_id"`
	Trigger    string `json:"trigger"`
	GitCommit  string `json:"git_commit"`
	GitMessage string `json:"git_message"`
	Status     string `json:"status"`
	Error      string `json:"error"`
	CreatedAt  Time   `json:"created_at"`
	StartedAt  Time   `json:"started_at"`
	FinishedAt Time   `json:"finished_at"`
}

func (d *Deployment) Done() bool {
	return d.Status == DeploySuccess || d.Status == DeployFailed || d.Status == DeployKilled
}

const deploymentColumns = "id, app_id, bundle_id, release_id, trigger, git_commit, git_message, status, error, created_at, started_at, finished_at"

func scanDeployment(row interface{ Scan(...any) error }) (*Deployment, error) {
	var d Deployment
	err := row.Scan(&d.ID, &d.AppID, &d.BundleID, &d.ReleaseID, &d.Trigger, &d.GitCommit, &d.GitMessage, &d.Status, &d.Error, &d.CreatedAt, &d.StartedAt, &d.FinishedAt)
	return &d, notFound(err)
}

func (s *Store) CreateDeployment(d *Deployment) error {
	d.ID, d.CreatedAt = NewID(), Now()
	d.Status = DeployQueued
	_, err := s.db.Exec("INSERT INTO deployments (id, app_id, bundle_id, trigger, status, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		d.ID, d.AppID, d.BundleID, d.Trigger, d.Status, d.CreatedAt)
	return err
}

func (s *Store) UpdateDeployment(d *Deployment) error {
	_, err := s.db.Exec("UPDATE deployments SET bundle_id = ?, release_id = ?, git_commit = ?, git_message = ?, status = ?, error = ?, started_at = ?, finished_at = ? WHERE id = ?",
		d.BundleID, d.ReleaseID, d.GitCommit, d.GitMessage, d.Status, d.Error, d.StartedAt, d.FinishedAt, d.ID)
	return err
}

func (s *Store) Deployment(id string) (*Deployment, error) {
	return scanDeployment(s.db.QueryRow("SELECT "+deploymentColumns+" FROM deployments WHERE id = ?", id))
}

func (s *Store) Deployments(appID string, limit int) ([]*Deployment, error) {
	return s.queryDeployments("SELECT "+deploymentColumns+" FROM deployments WHERE app_id = ? ORDER BY id DESC LIMIT ?", appID, limit)
}

// AllDeployments lists deployments across apps, newest first, optionally only those with a status.
func (s *Store) AllDeployments(status, before string, limit int) ([]*Deployment, error) {
	query := "SELECT " + deploymentColumns + " FROM deployments WHERE 1 = 1"
	var args []any
	if status != "" {
		query, args = query+" AND status = ?", append(args, status)
	}
	if before != "" {
		query, args = query+" AND id < ?", append(args, before)
	}
	return s.queryDeployments(query+" ORDER BY id DESC LIMIT ?", append(args, limit)...)
}

func (s *Store) queryDeployments(query string, args ...any) ([]*Deployment, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Deployment
	for rows.Next() {
		d, err := scanDeployment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// FailUnfinishedDeployments marks deployments interrupted by a platform restart.
func (s *Store) FailUnfinishedDeployments() error {
	_, err := s.db.Exec(`UPDATE deployments SET status = ?, error = 'interrupted by platform restart', finished_at = ?
		WHERE status IN (?, ?, ?)`, DeployFailed, Now(), DeployQueued, DeployBuilding, DeployStarting)
	return err
}

// DeployedDeployment is the successful deployment that produced the given release.
func (s *Store) DeployedDeployment(releaseID string) (*Deployment, error) {
	return scanDeployment(s.db.QueryRow("SELECT "+deploymentColumns+" FROM deployments WHERE release_id = ? AND status = ? ORDER BY id DESC LIMIT 1",
		releaseID, DeploySuccess))
}

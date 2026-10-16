package store

type Project struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   Time   `json:"created_at"`
}

func (s *Store) Projects() ([]*Project, error) {
	rows, err := s.db.Query("SELECT id, name, description, created_at FROM projects ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

func (s *Store) Project(id string) (*Project, error) {
	var p Project
	err := s.db.QueryRow("SELECT id, name, description, created_at FROM projects WHERE id = ?", id).
		Scan(&p.ID, &p.Name, &p.Description, &p.CreatedAt)
	return &p, notFound(err)
}

func (s *Store) SaveProject(p *Project) error {
	if p.ID != "" {
		_, err := s.db.Exec("UPDATE projects SET name = ?, description = ? WHERE id = ?", p.Name, p.Description, p.ID)
		return err
	}
	p.ID, p.CreatedAt = NewID(), Now()
	_, err := s.db.Exec("INSERT INTO projects (id, name, description, created_at) VALUES (?, ?, ?, ?)", p.ID, p.Name, p.Description, p.CreatedAt)
	return err
}

func (s *Store) DeleteProject(id string) error {
	_, err := s.db.Exec("DELETE FROM projects WHERE id = ?", id)
	return err
}

func (s *Store) ProjectAppCount(id string) (int, error) {
	var n int
	err := s.db.QueryRow("SELECT COUNT(*) FROM apps WHERE project_id = ?", id).Scan(&n)
	return n, err
}

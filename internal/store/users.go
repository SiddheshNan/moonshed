package store

import (
	"time"
)

type User struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	CreatedAt    Time   `json:"created_at"`
}

func (s *Store) UserCount() (int, error) {
	var n int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&n)
	return n, err
}

func (s *Store) Users() ([]*User, error) {
	rows, err := s.db.Query("SELECT id, username, password_hash, created_at FROM users ORDER BY username")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &u)
	}
	return out, rows.Err()
}

func (s *Store) User(id string) (*User, error) {
	var u User
	err := s.db.QueryRow("SELECT id, username, password_hash, created_at FROM users WHERE id = ?", id).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt)
	return &u, notFound(err)
}

func (s *Store) UserByName(username string) (*User, error) {
	var u User
	err := s.db.QueryRow("SELECT id, username, password_hash, created_at FROM users WHERE username = ?", username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt)
	return &u, notFound(err)
}

func (s *Store) CreateUser(u *User) error {
	u.ID, u.CreatedAt = NewID(), Now()
	_, err := s.db.Exec("INSERT INTO users (id, username, password_hash, created_at) VALUES (?, ?, ?, ?)", u.ID, u.Username, u.PasswordHash, u.CreatedAt)
	return err
}

// CreateFirstUser adds u only while there are no users, in one statement so two
// concurrent setups can't both succeed. It reports whether u was added.
func (s *Store) CreateFirstUser(u *User) (bool, error) {
	u.ID, u.CreatedAt = NewID(), Now()
	res, err := s.db.Exec(`INSERT INTO users (id, username, password_hash, created_at)
		SELECT ?, ?, ?, ? WHERE NOT EXISTS (SELECT 1 FROM users)`, u.ID, u.Username, u.PasswordHash, u.CreatedAt)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func (s *Store) SetPassword(userID string, hash string) error {
	_, err := s.db.Exec("UPDATE users SET password_hash = ? WHERE id = ?", hash, userID)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("DELETE FROM sessions WHERE user_id = ?", userID)
	return err
}

func (s *Store) DeleteUser(id string) error {
	_, err := s.db.Exec("DELETE FROM users WHERE id = ?", id)
	return err
}

func (s *Store) CreateSession(tokenHash, userID string, expires time.Time) error {
	_, err := s.db.Exec("INSERT INTO sessions (token_hash, user_id, expires_at) VALUES (?, ?, ?)", tokenHash, userID, expires.Unix())
	return err
}

func (s *Store) SessionUser(tokenHash string) (*User, error) {
	var userID string
	err := s.db.QueryRow("SELECT user_id FROM sessions WHERE token_hash = ? AND expires_at > ?", tokenHash, time.Now().Unix()).Scan(&userID)
	if err != nil {
		return nil, notFound(err)
	}
	return s.User(userID)
}

func (s *Store) DeleteSession(tokenHash string) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE token_hash = ?", tokenHash)
	return err
}

func (s *Store) DeleteExpiredSessions() error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE expires_at <= ?", time.Now().Unix())
	return err
}

type APIToken struct {
	ID         string `json:"id"`
	UserID     string `json:"user_id"`
	Name       string `json:"name"`
	LastUsedAt Time   `json:"last_used_at"`
	CreatedAt  Time   `json:"created_at"`
}

func (s *Store) CreateAPIToken(t *APIToken, tokenHash string) error {
	t.ID, t.CreatedAt = NewID(), Now()
	_, err := s.db.Exec("INSERT INTO api_tokens (id, user_id, name, token_hash, created_at) VALUES (?, ?, ?, ?, ?)", t.ID, t.UserID, t.Name, tokenHash, t.CreatedAt)
	return err
}

func (s *Store) APITokens() ([]*APIToken, error) {
	rows, err := s.db.Query("SELECT id, user_id, name, last_used_at, created_at FROM api_tokens ORDER BY id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*APIToken
	for rows.Next() {
		var t APIToken
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.LastUsedAt, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &t)
	}
	return out, rows.Err()
}

// APITokenUser returns the token's owner and the token's name.
func (s *Store) APITokenUser(tokenHash string) (*User, string, error) {
	var id, userID string
	var name string
	err := s.db.QueryRow("SELECT id, user_id, name FROM api_tokens WHERE token_hash = ?", tokenHash).Scan(&id, &userID, &name)
	if err != nil {
		return nil, "", notFound(err)
	}
	if _, err := s.db.Exec("UPDATE api_tokens SET last_used_at = ? WHERE id = ?", Now(), id); err != nil {
		return nil, "", err
	}
	u, err := s.User(userID)
	return u, name, err
}

func (s *Store) DeleteAPIToken(id string) error {
	_, err := s.db.Exec("DELETE FROM api_tokens WHERE id = ?", id)
	return err
}

package store

import "fmt"

// GitCredential is an access token for cloning from any git host (GitLab, Bitbucket, Gitea, GitHub).
type GitCredential struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Host      string `json:"host"`
	Username  string `json:"username"`
	Token     string `json:"-"`
	CreatedAt Time   `json:"created_at"`
}

func (s *Store) GitCredentials() ([]*GitCredential, error) {
	rows, err := s.db.Query("SELECT id, name, host, username, token, created_at FROM git_credentials ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*GitCredential{}
	for rows.Next() {
		c, err := s.scanGitCredential(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GitCredential(id string) (*GitCredential, error) {
	return s.scanGitCredential(s.db.QueryRow("SELECT id, name, host, username, token, created_at FROM git_credentials WHERE id = ?", id))
}

func (s *Store) scanGitCredential(row interface{ Scan(...any) error }) (*GitCredential, error) {
	var c GitCredential
	if err := row.Scan(&c.ID, &c.Name, &c.Host, &c.Username, &c.Token, &c.CreatedAt); err != nil {
		return nil, notFound(err)
	}
	var err error
	if c.Token, err = s.box.Decrypt(c.Token); err != nil {
		return nil, fmt.Errorf("decrypt git credential %s: %w", c.Name, err)
	}
	return &c, nil
}

func (s *Store) SaveGitCredential(c *GitCredential) error {
	token, err := s.box.Encrypt(c.Token)
	if err != nil {
		return err
	}
	if c.ID != "" {
		_, err := s.db.Exec("UPDATE git_credentials SET name = ?, host = ?, username = ?, token = ? WHERE id = ?",
			c.Name, c.Host, c.Username, token, c.ID)
		return err
	}
	c.ID, c.CreatedAt = NewID(), Now()
	_, err = s.db.Exec("INSERT INTO git_credentials (id, name, host, username, token, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		c.ID, c.Name, c.Host, c.Username, token, c.CreatedAt)
	return err
}

func (s *Store) DeleteGitCredential(id string) error {
	_, err := s.db.Exec("DELETE FROM git_credentials WHERE id = ?", id)
	return err
}

// GitHubApp is a GitHub App created through the manifest flow. Its id is GitHub's app id.
type GitHubApp struct {
	ID            int64  `json:"id"`
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	Owner         string `json:"owner"`
	HTMLURL       string `json:"html_url"`
	ClientID      string `json:"-"`
	ClientSecret  string `json:"-"`
	PrivateKey    string `json:"-"`
	WebhookSecret string `json:"-"`
	CreatedAt     Time   `json:"created_at"`
}

const githubAppColumns = "id, slug, name, owner, html_url, client_id, client_secret, private_key, webhook_secret, created_at"

func (s *Store) scanGitHubApp(row interface{ Scan(...any) error }) (*GitHubApp, error) {
	var a GitHubApp
	if err := row.Scan(&a.ID, &a.Slug, &a.Name, &a.Owner, &a.HTMLURL, &a.ClientID, &a.ClientSecret, &a.PrivateKey, &a.WebhookSecret, &a.CreatedAt); err != nil {
		return nil, notFound(err)
	}
	for _, field := range []*string{&a.ClientSecret, &a.PrivateKey, &a.WebhookSecret} {
		plain, err := s.box.Decrypt(*field)
		if err != nil {
			return nil, fmt.Errorf("decrypt github app %s: %w", a.Slug, err)
		}
		*field = plain
	}
	return &a, nil
}

func (s *Store) GitHubApps() ([]*GitHubApp, error) {
	rows, err := s.db.Query("SELECT " + githubAppColumns + " FROM github_apps ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*GitHubApp{}
	for rows.Next() {
		a, err := s.scanGitHubApp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) GitHubApp(id int64) (*GitHubApp, error) {
	return s.scanGitHubApp(s.db.QueryRow("SELECT "+githubAppColumns+" FROM github_apps WHERE id = ?", id))
}

func (s *Store) CreateGitHubApp(a *GitHubApp) error {
	enc := make([]string, 3)
	for i, v := range []string{a.ClientSecret, a.PrivateKey, a.WebhookSecret} {
		var err error
		if enc[i], err = s.box.Encrypt(v); err != nil {
			return err
		}
	}
	a.CreatedAt = Now()
	_, err := s.db.Exec("INSERT INTO github_apps ("+githubAppColumns+") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		a.ID, a.Slug, a.Name, a.Owner, a.HTMLURL, a.ClientID, enc[0], enc[1], enc[2], a.CreatedAt)
	return err
}

func (s *Store) DeleteGitHubApp(id int64) error {
	_, err := s.db.Exec("DELETE FROM github_apps WHERE id = ?", id)
	return err
}

// GitHubInstallation is an install of one of our GitHub Apps on a user or organization.
// Its id is GitHub's installation id.
type GitHubInstallation struct {
	ID          int64  `json:"id"`
	GitHubAppID int64  `json:"github_app_id"`
	Account     string `json:"account"`
	AccountType string `json:"account_type"`
	CreatedAt   Time   `json:"created_at"`
}

func (s *Store) GitHubInstallations() ([]*GitHubInstallation, error) {
	rows, err := s.db.Query("SELECT id, github_app_id, account, account_type, created_at FROM github_installations ORDER BY account")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*GitHubInstallation{}
	for rows.Next() {
		var i GitHubInstallation
		if err := rows.Scan(&i.ID, &i.GitHubAppID, &i.Account, &i.AccountType, &i.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &i)
	}
	return out, rows.Err()
}

func (s *Store) GitHubInstallation(id int64) (*GitHubInstallation, error) {
	var i GitHubInstallation
	err := s.db.QueryRow("SELECT id, github_app_id, account, account_type, created_at FROM github_installations WHERE id = ?", id).
		Scan(&i.ID, &i.GitHubAppID, &i.Account, &i.AccountType, &i.CreatedAt)
	return &i, notFound(err)
}

func (s *Store) SaveGitHubInstallation(i *GitHubInstallation) error {
	i.CreatedAt = Now()
	_, err := s.db.Exec(`INSERT INTO github_installations (id, github_app_id, account, account_type, created_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET account = excluded.account, account_type = excluded.account_type`,
		i.ID, i.GitHubAppID, i.Account, i.AccountType, i.CreatedAt)
	return err
}

func (s *Store) DeleteGitHubInstallation(id int64) error {
	_, err := s.db.Exec("DELETE FROM github_installations WHERE id = ?", id)
	return err
}

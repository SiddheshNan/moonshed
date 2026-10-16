// Package githost talks to GitHub as a GitHub App: app JWTs, installation tokens,
// creating apps from a manifest and listing repositories.
package githost

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"moonshed/internal/store"
)

const apiURL = "https://api.github.com"

type GitHub struct {
	http *http.Client

	mu     sync.Mutex
	tokens map[int64]installationToken
}

type installationToken struct {
	token   string
	expires time.Time
}

func NewGitHub() *GitHub {
	return &GitHub{http: &http.Client{Timeout: 30 * time.Second}, tokens: map[int64]installationToken{}}
}

// appJWT authenticates as the app itself, valid for a few minutes.
func appJWT(app *store.GitHubApp) (string, error) {
	block, _ := pem.Decode([]byte(app.PrivateKey))
	if block == nil {
		return "", errors.New("github app private key is not PEM")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		parsed, err8 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err8 != nil {
			return "", fmt.Errorf("parse github app key: %w", err)
		}
		var ok bool
		if key, ok = parsed.(*rsa.PrivateKey); !ok {
			return "", errors.New("github app key is not RSA")
		}
	}
	now := time.Now()
	enc := base64.RawURLEncoding
	header := enc.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, _ := json.Marshal(map[string]any{
		"iat": now.Add(-time.Minute).Unix(),
		"exp": now.Add(9 * time.Minute).Unix(),
		"iss": strconv.FormatInt(app.ID, 10),
	})
	unsigned := header + "." + enc.EncodeToString(claims)
	sum := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(nil, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return unsigned + "." + enc.EncodeToString(sig), nil
}

func (g *GitHub) do(ctx context.Context, method, path, auth string, body, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, apiURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := g.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var e struct {
			Message string `json:"message"`
		}
		json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&e)
		return fmt.Errorf("github %s %s: %d %s", method, path, resp.StatusCode, e.Message)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (g *GitHub) asApp(ctx context.Context, app *store.GitHubApp, method, path string, out any) error {
	jwt, err := appJWT(app)
	if err != nil {
		return err
	}
	return g.do(ctx, method, path, "Bearer "+jwt, nil, out)
}

// ConvertManifest finishes the manifest flow: GitHub hands back the new app's credentials.
func (g *GitHub) ConvertManifest(ctx context.Context, code string) (*store.GitHubApp, error) {
	var res struct {
		ID            int64  `json:"id"`
		Slug          string `json:"slug"`
		Name          string `json:"name"`
		HTMLURL       string `json:"html_url"`
		ClientID      string `json:"client_id"`
		ClientSecret  string `json:"client_secret"`
		WebhookSecret string `json:"webhook_secret"`
		PEM           string `json:"pem"`
		Owner         struct {
			Login string `json:"login"`
		} `json:"owner"`
	}
	if err := g.do(ctx, http.MethodPost, "/app-manifests/"+code+"/conversions", "", nil, &res); err != nil {
		return nil, err
	}
	return &store.GitHubApp{
		ID:            res.ID,
		Slug:          res.Slug,
		Name:          res.Name,
		Owner:         res.Owner.Login,
		HTMLURL:       res.HTMLURL,
		ClientID:      res.ClientID,
		ClientSecret:  res.ClientSecret,
		PrivateKey:    res.PEM,
		WebhookSecret: res.WebhookSecret,
	}, nil
}

type installation struct {
	ID      int64 `json:"id"`
	Account struct {
		Login string `json:"login"`
		Type  string `json:"type"`
	} `json:"account"`
}

func (i installation) model(appID int64) *store.GitHubInstallation {
	return &store.GitHubInstallation{ID: i.ID, GitHubAppID: appID, Account: i.Account.Login, AccountType: i.Account.Type}
}

func (g *GitHub) Installation(ctx context.Context, app *store.GitHubApp, id int64) (*store.GitHubInstallation, error) {
	var inst installation
	if err := g.asApp(ctx, app, http.MethodGet, fmt.Sprintf("/app/installations/%d", id), &inst); err != nil {
		return nil, err
	}
	return inst.model(app.ID), nil
}

func (g *GitHub) Installations(ctx context.Context, app *store.GitHubApp) ([]*store.GitHubInstallation, error) {
	var list []installation
	if err := g.asApp(ctx, app, http.MethodGet, "/app/installations?per_page=100", &list); err != nil {
		return nil, err
	}
	out := make([]*store.GitHubInstallation, 0, len(list))
	for _, i := range list {
		out = append(out, i.model(app.ID))
	}
	return out, nil
}

// InstallationToken returns a short-lived token for cloning the installation's repositories.
func (g *GitHub) InstallationToken(ctx context.Context, app *store.GitHubApp, installationID int64) (string, error) {
	g.mu.Lock()
	cached, ok := g.tokens[installationID]
	g.mu.Unlock()
	if ok && time.Until(cached.expires) > 5*time.Minute {
		return cached.token, nil
	}
	jwt, err := appJWT(app)
	if err != nil {
		return "", err
	}
	var res struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := g.do(ctx, http.MethodPost, fmt.Sprintf("/app/installations/%d/access_tokens", installationID), "Bearer "+jwt, nil, &res); err != nil {
		return "", err
	}
	g.mu.Lock()
	g.tokens[installationID] = installationToken{token: res.Token, expires: res.ExpiresAt}
	g.mu.Unlock()
	return res.Token, nil
}

type Repo struct {
	FullName      string `json:"full_name"`
	CloneURL      string `json:"clone_url"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
}

// Repositories lists every repository the installation can access.
func (g *GitHub) Repositories(ctx context.Context, app *store.GitHubApp, installationID int64) ([]Repo, error) {
	token, err := g.InstallationToken(ctx, app, installationID)
	if err != nil {
		return nil, err
	}
	var all []Repo
	for page := 1; page <= 20; page++ {
		var res struct {
			TotalCount   int    `json:"total_count"`
			Repositories []Repo `json:"repositories"`
		}
		path := fmt.Sprintf("/installation/repositories?per_page=100&page=%d", page)
		if err := g.do(ctx, http.MethodGet, path, "token "+token, nil, &res); err != nil {
			return nil, err
		}
		all = append(all, res.Repositories...)
		if len(all) >= res.TotalCount || len(res.Repositories) == 0 {
			break
		}
	}
	return all, nil
}

func (g *GitHub) Branches(ctx context.Context, app *store.GitHubApp, installationID int64, fullName string) ([]string, error) {
	token, err := g.InstallationToken(ctx, app, installationID)
	if err != nil {
		return nil, err
	}
	var names []string
	for page := 1; page <= 10; page++ {
		var res []struct {
			Name string `json:"name"`
		}
		path := fmt.Sprintf("/repos/%s/branches?per_page=100&page=%d", fullName, page)
		if err := g.do(ctx, http.MethodGet, path, "token "+token, nil, &res); err != nil {
			return nil, err
		}
		for _, b := range res {
			names = append(names, b.Name)
		}
		if len(res) < 100 {
			break
		}
	}
	return names, nil
}

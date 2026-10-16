package deploy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"

	"moonshed/internal/archive"
	"moonshed/internal/logs"
	"moonshed/internal/store"
)

// gitAuth picks the credentials for cloning: the GitHub App installation, a saved
// credential, or none for public repositories.
func (m *Manager) gitAuth(ctx context.Context, app *store.App) (*githttp.BasicAuth, error) {
	switch {
	case app.GitHubInstallationID != 0:
		inst, err := m.store.GitHubInstallation(app.GitHubInstallationID)
		if err != nil {
			return nil, fmt.Errorf("github installation: %w", err)
		}
		gh, err := m.store.GitHubApp(inst.GitHubAppID)
		if err != nil {
			return nil, fmt.Errorf("github app: %w", err)
		}
		token, err := m.github.InstallationToken(ctx, gh, inst.ID)
		if err != nil {
			return nil, err
		}
		return &githttp.BasicAuth{Username: "x-access-token", Password: token}, nil
	case app.GitCredentialID != "":
		cred, err := m.store.GitCredential(app.GitCredentialID)
		if err != nil {
			return nil, fmt.Errorf("git credential: %w", err)
		}
		return &githttp.BasicAuth{Username: cred.Username, Password: cred.Token}, nil
	}
	return nil, nil
}

// cloneBundle does a shallow clone of the app's branch and stores it as a bundle zip.
// It also records the commit on the deployment.
func (m *Manager) cloneBundle(ctx context.Context, app *store.App, d *store.Deployment, log *logs.Stream) (*store.Bundle, error) {
	ap := m.paths.App(app.Name)
	tmp, err := os.MkdirTemp(ap.Root, "clone-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	branch := app.GitBranch
	if branch == "" {
		branch = "main"
	}
	log.Printf("cloning %s (%s)", app.GitURL, branch)
	opts := &git.CloneOptions{
		URL:           app.GitURL,
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		SingleBranch:  true,
		Depth:         1,
	}
	auth, err := m.gitAuth(ctx, app)
	if err != nil {
		return nil, err
	}
	if auth != nil {
		opts.Auth = auth
	}
	repo, err := git.PlainCloneContext(ctx, tmp, false, opts)
	if err != nil {
		return nil, fmt.Errorf("git clone: %w", err)
	}
	head, err := repo.Head()
	if err != nil {
		return nil, err
	}
	commit := head.Hash().String()
	d.GitCommit = commit
	if c, err := repo.CommitObject(head.Hash()); err == nil {
		d.GitMessage, _, _ = strings.Cut(strings.TrimSpace(c.Message), "\n")
	}
	log.Printf("checked out %s %s", commit[:12], d.GitMessage)

	src := tmp
	if app.GitSubdir != "" {
		src = filepath.Join(tmp, filepath.FromSlash(app.GitSubdir))
		if !strings.HasPrefix(src, tmp+string(filepath.Separator)) {
			return nil, fmt.Errorf("invalid git subfolder %q", app.GitSubdir)
		}
		if _, err := os.Stat(src); err != nil {
			return nil, fmt.Errorf("git subfolder %s: %w", app.GitSubdir, err)
		}
	}

	b := &store.Bundle{AppID: app.ID, Source: store.SourceGit, GitCommit: commit}
	if err := m.store.CreateBundle(b); err != nil {
		return nil, err
	}
	f, err := os.Create(ap.Bundle(b.ID))
	if err == nil {
		err = archive.Zip(f, src, ".git")
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}
	if err == nil {
		var info os.FileInfo
		if info, err = os.Stat(ap.Bundle(b.ID)); err == nil {
			b.Size = info.Size()
			err = m.store.SetBundleSize(b.ID, b.Size)
		}
	}
	if err != nil {
		os.Remove(ap.Bundle(b.ID))
		m.store.DeleteBundle(b.ID)
		return nil, fmt.Errorf("create bundle: %w", err)
	}
	return b, nil
}

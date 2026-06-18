package workspace

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/local/symphony/internal/domain"
	"github.com/local/symphony/internal/gitcmd"
	"github.com/local/symphony/internal/issueidentity"
)

// Create prepares a local workspace directory and clones the project repository.
func Create(ctx context.Context, issue domain.Issue, cfg domain.Config) (domain.Workspace, error) {
	proj, err := findProject(issue, cfg)
	if err != nil {
		return domain.Workspace{}, err
	}

	key := issueidentity.For(issue).WorkspaceKey()
	wsPath := filepath.Join(cfg.Workspace.Root, key)

	if err := os.MkdirAll(cfg.Workspace.Root, 0o755); err != nil {
		return domain.Workspace{}, fmt.Errorf("create workspace root: %w", err)
	}

	// Only clone if not already a git repo.
	if !hasGitDir(wsPath) {
		if err := os.MkdirAll(wsPath, 0o755); err != nil {
			return domain.Workspace{}, fmt.Errorf("create workspace dir: %w", err)
		}
		if empty, _ := isDirEmpty(wsPath); !empty {
			return domain.Workspace{}, fmt.Errorf("workspace %q is not empty and not a git repo", wsPath)
		}
		if err := cloneRepo(ctx, proj.RepoURL, cfg.Gitea.Token, wsPath); err != nil {
			_ = os.RemoveAll(wsPath)
			return domain.Workspace{}, err
		}
	}

	return domain.Workspace{Path: wsPath, IssueKey: key}, nil
}

// Clean removes the workspace directory.
func Clean(ctx context.Context, ws domain.Workspace) error {
	if strings.TrimSpace(ws.Path) == "" {
		return nil
	}
	return os.RemoveAll(ws.Path)
}

func findProject(issue domain.Issue, cfg domain.Config) (domain.ProjectConfig, error) {
	pid := strings.TrimSpace(issue.ProjectID)
	if pid == "" {
		return domain.ProjectConfig{}, fmt.Errorf("issue project id is required")
	}
	for _, p := range cfg.Gitea.Projects {
		if strings.EqualFold(strings.TrimSpace(p.ID), pid) {
			return p, nil
		}
	}
	return domain.ProjectConfig{}, fmt.Errorf("project %q not found in config", pid)
}

func hasGitDir(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && info.IsDir()
}

func isDirEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

func cloneRepo(ctx context.Context, repoURL, token, wsPath string) error {
	u, err := url.Parse(strings.TrimSpace(repoURL))
	if err != nil {
		return fmt.Errorf("parse repo url: %w", err)
	}
	u.User = nil
	cloneURL := u.String()

	opts := gitcmd.Options{Dir: filepath.Dir(wsPath), Token: token}
	if err := gitcmd.Run(ctx, opts, "clone", cloneURL, wsPath); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}
	return nil
}

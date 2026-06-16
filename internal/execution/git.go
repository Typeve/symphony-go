package execution

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"unicode"

	"github.com/local/symphony/internal/domain"
)

// BranchName returns a deterministic git branch name for an issue.
func BranchName(issue domain.Issue) string {
	prefix := strings.Trim(issue.ProjectID, "/")
	if prefix == "" {
		prefix = "symphony"
	} else {
		prefix = "symphony/" + slug(prefix)
	}
	number := extractNumber(issue)
	if number == "" {
		number = "0"
	}
	titleSlug := slug(issue.Title)
	if titleSlug == "" {
		titleSlug = "task"
	}
	if len(titleSlug) > 48 {
		titleSlug = strings.Trim(titleSlug[:48], "-")
	}
	return prefix + "/issue-" + number + "-" + titleSlug
}

// CreateBranch checks out a new branch in the workspace for the given issue.
func CreateBranch(workspace domain.Workspace, issue domain.Issue) error {
	branch := BranchName(issue)
	return runGit(workspace.Path, "checkout", "-B", branch)
}

// CommitAndPush stages all changes, commits, and pushes the branch to origin.
func CommitAndPush(ctx context.Context, workspace domain.Workspace, branch, token string) (domain.PublishResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	// Stage all changes.
	if err := runGit(workspace.Path, "add", "-A"); err != nil {
		return domain.PublishResult{}, err
	}

	// Commit.
	msg := fmt.Sprintf("symphony: automated changes for branch %s", branch)
	if err := runGitCtx(ctx, workspace.Path, "commit", "-m", msg, "--allow-empty"); err != nil {
		return domain.PublishResult{}, err
	}

	// Get commit hash.
	commit, err := gitOutput(ctx, workspace.Path, "rev-parse", "HEAD")
	if err != nil {
		return domain.PublishResult{}, err
	}
	commit = strings.TrimSpace(commit)

	// Push with optional token authentication.
	if token != "" {
		if err := pushWithToken(ctx, workspace.Path, branch, token); err != nil {
			return domain.PublishResult{}, err
		}
	} else {
		if err := runGitCtx(ctx, workspace.Path, "push", "origin", branch); err != nil {
			return domain.PublishResult{}, err
		}
	}

	return domain.PublishResult{Branch: branch, Commit: commit}, nil
}

// --- helpers ---

var numberPattern = regexp.MustCompile(`(\d+)`)

func extractNumber(issue domain.Issue) string {
	for _, v := range []string{issue.SourceID, issue.ID, issue.Identifier} {
		if m := numberPattern.FindString(strings.TrimSpace(v)); m != "" {
			return m
		}
	}
	return ""
}

func slug(value string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func runGit(dir string, args ...string) error {
	_, err := gitOutput(context.Background(), dir, args...)
	return err
}

func runGitCtx(ctx context.Context, dir string, args ...string) error {
	_, err := gitOutput(ctx, dir, args...)
	return err
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		text := strings.TrimSpace(out.String())
		if len(text) > 1024 {
			text = text[:1024] + "\n[truncated]"
		}
		return "", fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, text)
	}
	return out.String(), nil
}

func pushWithToken(ctx context.Context, dir, branch, token string) error {
	cred, cleanup, err := newAskpass(token)
	if err != nil {
		return err
	}
	defer cleanup()

	cmd := exec.CommandContext(ctx, "git", "push", "origin", branch)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), cred...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		text := strings.TrimRight(strings.ReplaceAll(out.String(), token, "[REDACTED]"), "\n")
		if len(text) > 1024 {
			text = text[:1024]
		}
		return fmt.Errorf("git push failed: %w: %s", err, text)
	}
	return nil
}

func newAskpass(token string) ([]string, func(), error) {
	dir, err := os.MkdirTemp("", "symphony-git-askpass-*")
	if err != nil {
		return nil, nil, fmt.Errorf("create askpass dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	script := "#!/bin/sh\ncase \"$1\" in *Username*) printf 'oauth2\\n' ;; *) printf '%s\\n' \"$SYMPHONY_GIT_TOKEN\" ;; esac\n"
	path := dir + "/askpass.sh"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("write askpass script: %w", err)
	}
	return []string{
		"GIT_ASKPASS=" + path,
		"GIT_TERMINAL_PROMPT=0",
		"SYMPHONY_GIT_TOKEN=" + token,
	}, cleanup, nil
}

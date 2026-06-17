package execution

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"unicode"

	"github.com/local/symphony/internal/domain"
	"github.com/local/symphony/internal/gitcmd"
)

// ErrNoChanges is returned when no allowed files remain staged for commit.
var ErrNoChanges = errors.New("no allowed changes to commit")

var defaultCommitExcludes = []string{
	".codex",
	".codex/**",
	".symphony/validation-verdict.json",
	".env*",
	"*.log",
}

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

	if err := stageAllowedChanges(ctx, workspace.Path); err != nil {
		return domain.PublishResult{}, err
	}

	// Commit.
	msg := fmt.Sprintf("symphony: automated changes for branch %s", branch)
	if err := runGitCtx(ctx, workspace.Path, "commit", "-m", msg); err != nil {
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

func stageAllowedChanges(ctx context.Context, dir string) error {
	if err := runGitCtx(ctx, dir, "add", "-A", "--", "."); err != nil {
		return err
	}
	for _, pattern := range defaultCommitExcludes {
		_ = runGitCtx(ctx, dir, "reset", "-q", "--", pattern)
	}
	changed, err := hasStagedChanges(ctx, dir)
	if err != nil {
		return err
	}
	if !changed {
		return ErrNoChanges
	}
	return nil
}

func hasStagedChanges(ctx context.Context, dir string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--quiet")
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return true, nil
		}
		text := strings.TrimSpace(out.String())
		if len(text) > 1024 {
			text = text[:1024] + "\n[truncated]"
		}
		return false, fmt.Errorf("git diff --cached --quiet failed: %w: %s", err, text)
	}
	return false, nil
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
	if err := gitcmd.Run(ctx, gitcmd.Options{Dir: dir, Token: token}, "push", "origin", branch); err != nil {
		return fmt.Errorf("git push failed: %w", err)
	}
	return nil
}

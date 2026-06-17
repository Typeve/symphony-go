package execution

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/local/symphony/internal/domain"
)

func TestStageAllowedChangesExcludesLocalArtifacts(t *testing.T) {
	dir := newTestRepo(t)

	writeFile(t, dir, "app.go", "package app\n")
	writeFile(t, dir, ".env", "SECRET=fixture\n")
	writeFile(t, dir, ".env.local", "SECRET=fixture\n")
	writeFile(t, dir, ".envrc", "export SECRET=fixture\n")
	writeFile(t, dir, ".codex/session.json", "{}\n")
	writeFile(t, dir, ".symphony/validation-verdict.json", "{}\n")
	writeFile(t, dir, "debug.log", "debug\n")

	if err := stageAllowedChanges(context.Background(), dir); err != nil {
		t.Fatalf("stageAllowedChanges returned error: %v", err)
	}

	got := gitOutputForTest(t, dir, "diff", "--cached", "--name-only")
	if strings.TrimSpace(got) != "app.go" {
		t.Fatalf("staged files = %q, want only app.go", got)
	}
}

func TestStageAllowedChangesReturnsErrNoChangesWhenOnlyExcludedFilesChanged(t *testing.T) {
	dir := newTestRepo(t)

	writeFile(t, dir, ".env", "SECRET=fixture\n")
	writeFile(t, dir, ".codex/session.json", "{}\n")
	writeFile(t, dir, "debug.log", "debug\n")

	err := stageAllowedChanges(context.Background(), dir)
	if !errors.Is(err, ErrNoChanges) {
		t.Fatalf("error = %v, want ErrNoChanges", err)
	}
}

func TestBranchNameIsDeterministic(t *testing.T) {
	branch := BranchName(domain.Issue{ProjectID: "project one", Identifier: "acme/app#42", Title: "Fix Login Error!"})
	if branch != "symphony/project-one/issue-42-fix-login-error" {
		t.Fatalf("branch = %q", branch)
	}
}

func newTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runTestGit(t, dir, "init")
	runTestGit(t, dir, "config", "user.email", "symphony@example.invalid")
	runTestGit(t, dir, "config", "user.name", "Symphony Test")
	writeFile(t, dir, "README.md", "baseline\n")
	runTestGit(t, dir, "add", "README.md")
	runTestGit(t, dir, "commit", "-m", "baseline")
	return dir
}

func runTestGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if _, err := gitOutput(context.Background(), dir, args...); err != nil {
		t.Fatal(err)
	}
}

func gitOutputForTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := gitOutput(context.Background(), dir, args...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

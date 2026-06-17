package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/local/symphony/internal/domain"
)

type testTracker struct{}

func (testTracker) FetchIssues(context.Context, domain.ProjectConfig) ([]domain.Issue, error) {
	return nil, nil
}
func (testTracker) MarkStatus(context.Context, domain.Issue, domain.Status) error { return nil }

func TestIsPendingDefaultsToOpenIssueWhenActiveStatesEmpty(t *testing.T) {
	var cfg domain.Config
	cfg.Gitea.Projects = []domain.ProjectConfig{{ID: "p"}}
	s := New(cfg, testTracker{})

	issue := domain.Issue{ProjectID: "p", ID: "1", State: "open"}
	if !s.isPending(issue) {
		t.Fatal("open issue with empty active_states is not pending")
	}
}

func TestIsPendingSkipsManagedStatusLabels(t *testing.T) {
	var cfg domain.Config
	cfg.Gitea.Projects = []domain.ProjectConfig{{ID: "p", ActiveStates: []string{"open"}}}
	s := New(cfg, testTracker{})

	for _, label := range []string{"symphony-running", "symphony-done", "symphony-failed"} {
		issue := domain.Issue{ProjectID: "p", ID: "1", State: "open", Labels: []string{label}}
		if s.isPending(issue) {
			t.Fatalf("issue with %s label is pending", label)
		}
	}
}

func TestRunCodexDoesNotExposeGiteaTokenToCommand(t *testing.T) {
	t.Setenv("GITEA_TOKEN", "fixture-token")
	t.Setenv("PATH", os.Getenv("PATH"))

	dir := t.TempDir()
	outPath := filepath.Join(dir, "env.txt")
	script := writeEnvCaptureCommand(t, dir, "codex", outPath)

	var cfg domain.Config
	cfg.Codex.Command = script
	cfg.Codex.Timeout = time.Minute
	s := New(cfg, testTracker{})
	issue := domain.Issue{ProjectID: "p", ID: "1", Title: "Do work"}
	ws := domain.Workspace{Path: dir, IssueKey: "p/1"}

	if err := s.runCodex(context.Background(), issue, ws); err != nil {
		t.Fatalf("runCodex returned error: %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "GITEA_TOKEN=") || strings.Contains(string(data), "fixture-token") {
		t.Fatalf("codex environment leaked token:\n%s", string(data))
	}
}

func TestRunCodexPassesConfiguredArgsAndPrompt(t *testing.T) {
	t.Setenv("GITEA_TOKEN", "fixture-token")
	t.Setenv("PATH", os.Getenv("PATH"))

	dir := t.TempDir()
	argvPath := filepath.Join(dir, "argv.txt")
	envPath := filepath.Join(dir, "env.txt")
	script := writeArgvEnvCaptureCommand(t, dir, "codex", argvPath, envPath)

	var cfg domain.Config
	cfg.Codex.Command = script + " app-server"
	cfg.Codex.Timeout = time.Minute
	s := New(cfg, testTracker{})
	issue := domain.Issue{ProjectID: "p", ID: "1", Title: "Do work"}
	ws := domain.Workspace{Path: dir, IssueKey: "p/1"}

	if err := s.runCodex(context.Background(), issue, ws); err != nil {
		t.Fatalf("runCodex returned error: %v", err)
	}

	argv, err := os.ReadFile(argvPath)
	if err != nil {
		t.Fatal(err)
	}
	argvText := string(argv)
	for _, want := range []string{"app-server", "--prompt"} {
		if !strings.Contains(argvText, want) {
			t.Fatalf("argv = %q, missing %q", argvText, want)
		}
	}

	envData, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(envData), "GITEA_TOKEN=") || strings.Contains(string(envData), "fixture-token") {
		t.Fatalf("codex environment leaked token:\n%s", string(envData))
	}
}

func TestBuildCodexPromptIncludesIssueDetails(t *testing.T) {
	s := New(domain.Config{}, testTracker{})
	description := "Details from the issue body"
	issue := domain.Issue{Title: "Do work", Description: &description}

	prompt := s.buildCodexPrompt(issue)

	for _, want := range []string{"Resolve the following issue", "Title: Do work", "Details from the issue body"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt = %q, missing %q", prompt, want)
		}
	}
}

func TestRunCodexReturnsBoundedOutputOnFailure(t *testing.T) {
	dir := t.TempDir()
	script := writeFailingOutputCommand(t, dir, "codex", strings.Repeat("x", 1600))

	var cfg domain.Config
	cfg.Codex.Command = script
	cfg.Codex.Timeout = time.Minute
	s := New(cfg, testTracker{})
	issue := domain.Issue{ProjectID: "p", ID: "1", Title: "Do work"}
	ws := domain.Workspace{Path: dir, IssueKey: "p/1"}

	err := s.runCodex(context.Background(), issue, ws)
	if err == nil {
		t.Fatal("runCodex returned nil error, want failure")
	}
	text := err.Error()
	if !strings.Contains(text, "...[truncated]") {
		t.Fatalf("error = %q, want truncated output marker", text)
	}
	if len(text) > 1200 {
		t.Fatalf("error length = %d, want bounded diagnostic", len(text))
	}
}

func TestShouldCleanWorkspaceAfterProcessing(t *testing.T) {
	ws := domain.Workspace{Path: "workspace"}
	if !shouldCleanWorkspace(false, ws) {
		t.Fatal("successful processing should clean workspace")
	}
	if shouldCleanWorkspace(true, ws) {
		t.Fatal("failed processing should preserve workspace")
	}
	if shouldCleanWorkspace(false, domain.Workspace{}) {
		t.Fatal("empty workspace path should not be cleaned")
	}
}

func writeEnvCaptureCommand(t *testing.T, dir, name, outPath string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		script := filepath.Join(dir, name+".cmd")
		content := "@echo off\r\nset > \"" + outPath + "\"\r\n"
		if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
		return script
	}

	script := filepath.Join(dir, name+".sh")
	content := "#!/bin/sh\nenv > " + shellQuote(outPath) + "\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return script
}

func writeArgvEnvCaptureCommand(t *testing.T, dir, name, argvPath, envPath string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		script := filepath.Join(dir, name+"-argv.cmd")
		content := "@echo off\r\n(\r\n  echo %1\r\n  echo %2\r\n) > \"" + argvPath + "\"\r\nset > \"" + envPath + "\"\r\n"
		if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
		return script
	}

	script := filepath.Join(dir, name+"-argv.sh")
	content := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + shellQuote(argvPath) + "\nenv > " + shellQuote(envPath) + "\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return script
}

func writeFailingOutputCommand(t *testing.T, dir, name, output string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		script := filepath.Join(dir, name+"-fail.cmd")
		content := "@echo off\r\necho " + output + "\r\nexit /b 1\r\n"
		if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
		return script
	}

	script := filepath.Join(dir, name+"-fail.sh")
	content := "#!/bin/sh\nprintf '%s' " + shellQuote(output) + " >&2\nexit 1\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return script
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

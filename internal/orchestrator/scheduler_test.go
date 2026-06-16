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

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

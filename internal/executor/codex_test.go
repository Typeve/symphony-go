package executor

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

func TestRunCodexPassesConfiguredArgsAndPrompt(t *testing.T) {
	t.Setenv("PATH", os.Getenv("PATH"))

	dir := t.TempDir()
	argvPath := filepath.Join(dir, "argv.txt")
	script := writeArgvCaptureCommand(t, dir, "codex", argvPath)

	var cfg domain.Config
	cfg.Codex.Command = script + " app-server"
	cfg.Codex.Model = "gpt-5.5"
	cfg.Codex.Timeout = time.Minute
	issue := domain.Issue{ProjectID: "p", ID: "1", Title: "Do work"}
	ws := domain.Workspace{Path: dir, IssueKey: "p/1"}

	if err := RunCodex(context.Background(), cfg, issue, ws); err != nil {
		t.Fatalf("RunCodex returned error: %v", err)
	}

	argv, err := os.ReadFile(argvPath)
	if err != nil {
		t.Fatal(err)
	}
	assertOrder(t, string(argv), "--model", "gpt-5.5", "app-server", "--prompt")
}

func TestBuildCodexPromptIncludesIssueDetails(t *testing.T) {
	description := "Details from the issue body"
	issue := domain.Issue{Title: "Do work", Description: &description}

	prompt := buildCodexPrompt(issue)

	for _, want := range []string{"Resolve the following issue", "Title: Do work", "Details from the issue body"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt = %q, missing %q", prompt, want)
		}
	}
}

func TestRunCodexWrapsFailureWithCodexContext(t *testing.T) {
	dir := t.TempDir()
	script := writeFailingOutputCommand(t, dir, "codex", "boom")

	var cfg domain.Config
	cfg.Codex.Command = script
	cfg.Codex.Timeout = time.Minute
	issue := domain.Issue{ProjectID: "p", ID: "1", Title: "Do work"}
	ws := domain.Workspace{Path: dir, IssueKey: "p/1"}

	err := RunCodex(context.Background(), cfg, issue, ws)
	if err == nil {
		t.Fatal("RunCodex returned nil error, want failure")
	}
	text := err.Error()
	if !strings.Contains(text, "codex:") || !strings.Contains(text, "boom") {
		t.Fatalf("error = %q, want codex context and command output", text)
	}
}

func writeArgvCaptureCommand(t *testing.T, dir, name, argvPath string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		script := filepath.Join(dir, name+"-argv.cmd")
		content := "@echo off\r\n(\r\n  echo %1\r\n  echo %2\r\n  echo %3\r\n  echo %4\r\n  echo %5\r\n  echo %6\r\n) > \"" + argvPath + "\"\r\n"
		if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
		return script
	}

	script := filepath.Join(dir, name+"-argv.sh")
	content := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + shellQuote(argvPath) + "\n"
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

func assertOrder(t *testing.T, text string, values ...string) {
	t.Helper()
	last := -1
	for _, value := range values {
		index := strings.Index(text, value)
		if index < 0 {
			t.Fatalf("argv = %q, missing %q", text, value)
		}
		if index <= last {
			t.Fatalf("argv = %q, want %q after previous values", text, value)
		}
		last = index
	}
}

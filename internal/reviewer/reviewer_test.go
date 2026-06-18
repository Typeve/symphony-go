package reviewer

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRunPassesConfiguredArgsAndPrompt(t *testing.T) {
	t.Setenv("PATH", os.Getenv("PATH"))

	dir := t.TempDir()
	argvPath := filepath.Join(dir, "argv.txt")
	script := writeArgvCaptureCommand(t, dir, "reviewer", argvPath)

	if err := Run(context.Background(), script+" --mode strict", time.Minute, dir); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	argv, err := os.ReadFile(argvPath)
	if err != nil {
		t.Fatal(err)
	}
	argvText := string(argv)
	for _, want := range []string{"--mode", "strict", "--prompt", "Review the changes"} {
		if !strings.Contains(argvText, want) {
			t.Fatalf("argv = %q, missing %q", argvText, want)
		}
	}
}

func TestRunWrapsFailureWithReviewerContext(t *testing.T) {
	dir := t.TempDir()
	script := writeFailingOutputCommand(t, dir, "reviewer", "bad review")

	err := Run(context.Background(), script, time.Minute, dir)
	if err == nil {
		t.Fatal("Run returned nil error, want failure")
	}
	text := err.Error()
	if !strings.Contains(text, "reviewer failed:") || !strings.Contains(text, "bad review") {
		t.Fatalf("error = %q, want reviewer context and command output", text)
	}
}

func writeArgvCaptureCommand(t *testing.T, dir, name, argvPath string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		script := filepath.Join(dir, name+"-argv.cmd")
		content := "@echo off\r\necho %* > \"" + argvPath + "\"\r\n"
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

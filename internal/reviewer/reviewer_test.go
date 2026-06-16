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

func TestRunDoesNotExposeGiteaTokenToReviewerCommand(t *testing.T) {
	t.Setenv("GITEA_TOKEN", "fixture-token")
	t.Setenv("PATH", os.Getenv("PATH"))

	dir := t.TempDir()
	outPath := filepath.Join(dir, "env.txt")
	script := writeEnvCaptureCommand(t, dir, "reviewer", outPath)

	if err := Run(context.Background(), script, time.Minute, dir); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "GITEA_TOKEN=") || strings.Contains(string(data), "fixture-token") {
		t.Fatalf("reviewer environment leaked token:\n%s", string(data))
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

package agentcmd

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRunDoesNotExposeGiteaTokenToCommand(t *testing.T) {
	t.Setenv("GITEA_TOKEN", "fixture-token")
	t.Setenv("PATH", os.Getenv("PATH"))

	dir := t.TempDir()
	outPath := filepath.Join(dir, "env.txt")
	script := writeEnvCaptureCommand(t, dir, "agent", outPath)

	if err := Run(context.Background(), Spec{
		Command:        script,
		DefaultCommand: "agent",
		Timeout:        time.Minute,
		Workspace:      dir,
		Prompt:         "Do work",
	}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "GITEA_TOKEN=") || strings.Contains(string(data), "fixture-token") {
		t.Fatalf("agent environment leaked token:\n%s", string(data))
	}
}

func TestRunPassesConfiguredArgsAndPrompt(t *testing.T) {
	t.Setenv("GITEA_TOKEN", "fixture-token")
	t.Setenv("PATH", os.Getenv("PATH"))

	dir := t.TempDir()
	argvPath := filepath.Join(dir, "argv.txt")
	envPath := filepath.Join(dir, "env.txt")
	script := writeArgvEnvCaptureCommand(t, dir, "agent", argvPath, envPath)

	if err := Run(context.Background(), Spec{
		Command:        script + " --mode strict",
		DefaultCommand: "agent",
		Timeout:        time.Minute,
		Workspace:      dir,
		Prompt:         "Review work",
	}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	argv, err := os.ReadFile(argvPath)
	if err != nil {
		t.Fatal(err)
	}
	argvText := string(argv)
	for _, want := range []string{"--mode", "strict", "--prompt", "Review work"} {
		if !strings.Contains(argvText, want) {
			t.Fatalf("argv = %q, missing %q", argvText, want)
		}
	}

	envData, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(envData), "GITEA_TOKEN=") || strings.Contains(string(envData), "fixture-token") {
		t.Fatalf("agent environment leaked token:\n%s", string(envData))
	}
}

func TestRunReturnsBoundedOutputOnFailure(t *testing.T) {
	dir := t.TempDir()
	script := writeFailingOutputCommand(t, dir, "agent", strings.Repeat("x", 1600))

	err := Run(context.Background(), Spec{
		Command:        script,
		DefaultCommand: "agent",
		Timeout:        time.Minute,
		Workspace:      dir,
		Prompt:         "Do work",
	})
	if err == nil {
		t.Fatal("Run returned nil error, want failure")
	}
	text := err.Error()
	if !strings.Contains(text, "...[truncated]") {
		t.Fatalf("error = %q, want truncated output marker", text)
	}
	if len(text) > 1200 {
		t.Fatalf("error length = %d, want bounded diagnostic", len(text))
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
		content := "@echo off\r\necho %* > \"" + argvPath + "\"\r\nset > \"" + envPath + "\"\r\n"
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

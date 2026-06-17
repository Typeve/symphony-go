package gitcmd

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestRunRedactsTokenFromFailureOutput(t *testing.T) {
	token := "fixture-token"

	err := Run(context.Background(), Options{Token: token}, "-c", "alias.fail=!echo fixture-token >&2; exit 1", "fail")
	if err == nil {
		t.Fatal("Run returned nil error, want failure")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("error leaked token: %v", err)
	}
	if !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("error = %q, want redaction marker", err.Error())
	}
}

func TestAskpassEnvCleansScript(t *testing.T) {
	env, cleanup, err := askpassEnv("fixture-token")
	if err != nil {
		t.Fatalf("askpassEnv returned error: %v", err)
	}

	askpass := envValue(env, "GIT_ASKPASS")
	if askpass == "" {
		t.Fatalf("env missing GIT_ASKPASS: %#v", env)
	}
	if envValue(env, "GIT_TERMINAL_PROMPT") != "0" {
		t.Fatalf("env missing GIT_TERMINAL_PROMPT=0: %#v", env)
	}
	if envValue(env, "SYMPHONY_GIT_TOKEN") != "fixture-token" {
		t.Fatalf("env missing SYMPHONY_GIT_TOKEN fixture value: %#v", env)
	}

	if _, err := os.Stat(askpass); err != nil {
		t.Fatalf("askpass script was not written: %v", err)
	}
	cleanup()
	if _, err := os.Stat(askpass); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("askpass script still exists or stat failed unexpectedly: %v", err)
	}
}

func TestRedactAndBoundBoundsFailureOutput(t *testing.T) {
	text := redactAndBound(strings.Repeat("x", 2000), nil)
	if len(text) > 1040 {
		t.Fatalf("bounded text length = %d, want close to max output", len(text))
	}
	if !strings.Contains(text, "[truncated]") {
		t.Fatalf("bounded text = %q, want truncated marker", text)
	}
}

func TestRunPreservesExitErrorContext(t *testing.T) {
	err := Run(context.Background(), Options{}, "definitely-not-a-real-git-command")
	if err == nil {
		t.Fatal("Run returned nil error, want failure")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error = %T, want wrapped exec.ExitError", err)
	}
}

func envValue(env []string, name string) string {
	prefix := name + "="
	for _, value := range env {
		if strings.HasPrefix(value, prefix) {
			return strings.TrimPrefix(value, prefix)
		}
	}
	return ""
}

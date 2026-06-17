# Plan 003: Unify Git Credential Runner Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Put credentialed Git command execution behind one deep Module so clone and push share the same Push Credential, askpass, redaction, and bounded diagnostic behavior.

**Architecture:** Create `internal/gitcmd` as a small Git command runner. It exposes a minimal Interface for running Git commands with or without a Push Credential, and keeps `GIT_ASKPASS`, `GIT_TERMINAL_PROMPT=0`, `SYMPHONY_GIT_TOKEN`, token redaction, and output truncation inside one Implementation. `workspace` continues to own workspace path creation; `execution` continues to own branch/commit/staging policy.

**Tech Stack:** Go 1.22, standard library only.

## Global Constraints

- Keep the simplified MVP: Gitea only, no HTTP API, no validator package, no retry/reconcile expansion.
- Do not pass Push Credentials to Codex or the Reviewer Command.
- Do not write tokens into remote URLs, logs, errors, tests, or docs.
- Do not change external config format or command behavior.
- Keep Git command output bounded to 1024 characters.
- Preserve current clone and push semantics.

---

## Current State

- `internal/workspace/workspace.go` has `cloneRepo` plus its own `newAskpass`.
- `internal/execution/git.go` has `pushWithToken` plus another `newAskpass`.
- Both askpass implementations write equivalent shell scripts and set equivalent environment variables.
- Both clone and push redact token values from command output before returning errors.

## Target File Structure

- Create `internal/gitcmd/gitcmd.go`: shared Git command runner and Push Credential handling.
- Create `internal/gitcmd/gitcmd_test.go`: redaction, askpass env, cleanup, and bounded-output tests.
- Modify `internal/workspace/workspace.go`: use `gitcmd.Run` for `git clone`.
- Modify `internal/execution/git.go`: use `gitcmd.Run` for `git push`.
- Keep existing staging, branch naming, and workspace key logic unchanged.

## Task 1: Add shared Git command runner

**Files:**
- Create: `internal/gitcmd/gitcmd_test.go`
- Create: `internal/gitcmd/gitcmd.go`

**Interfaces:**
- Produces:
  - `type Options struct { Dir string; Token string; Redact []string }`
  - `func Run(ctx context.Context, opts Options, args ...string) error`

- [x] **Step 1: Write failing tests**

Create `internal/gitcmd/gitcmd_test.go`:

```go
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
```

- [x] **Step 2: Verify red**

Run: `go test -count=1 ./internal/gitcmd`

Expected: FAIL because `Options` and `Run` are undefined.

- [x] **Step 3: Implement `internal/gitcmd/gitcmd.go`**

Create `internal/gitcmd/gitcmd.go`:

```go
package gitcmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const maxOutput = 1024

type Options struct {
	Dir    string
	Token  string
	Redact []string
}

func Run(ctx context.Context, opts Options, args ...string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = opts.Dir
	cmd.Env = os.Environ()

	cleanup := func() {}
	redactions := append([]string{}, opts.Redact...)
	if strings.TrimSpace(opts.Token) != "" {
		env, clean, err := askpassEnv(opts.Token)
		if err != nil {
			return err
		}
		cleanup = clean
		defer cleanup()
		cmd.Env = append(cmd.Env, env...)
		redactions = append(redactions, opts.Token)
	}

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		text := redactAndBound(out.String(), redactions)
		return fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, text)
	}
	return nil
}

func askpassEnv(token string) ([]string, func(), error) {
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

func redactAndBound(text string, redactions []string) string {
	text = strings.TrimRight(text, "\n")
	for _, value := range redactions {
		if value != "" {
			text = strings.ReplaceAll(text, value, "[REDACTED]")
		}
	}
	if len(text) > maxOutput {
		return text[:maxOutput] + "\n[truncated]"
	}
	return text
}
```

- [x] **Step 4: Verify green**

Run: `go test -count=1 ./internal/gitcmd`

Expected: PASS.

## Task 2: Migrate workspace clone to gitcmd

**Files:**
- Modify: `internal/workspace/workspace.go`

**Steps:**
- [x] Remove `bytes`, `os/exec`, and `newAskpass` from `workspace.go`.
- [x] Import `github.com/local/symphony/internal/gitcmd`.
- [x] In `cloneRepo`, replace manual command execution with:

```go
opts := gitcmd.Options{Dir: filepath.Dir(wsPath), Token: token}
if err := gitcmd.Run(ctx, opts, "clone", cloneURL, wsPath); err != nil {
	return fmt.Errorf("git clone failed: %w", err)
}
```

- [x] Run `go test -count=1 ./internal/workspace ./internal/gitcmd`.

Expected: PASS.

## Task 3: Migrate execution push to gitcmd

**Files:**
- Modify: `internal/execution/git.go`

**Steps:**
- [x] Remove `os` import and `newAskpass` from `git.go`.
- [x] Import `github.com/local/symphony/internal/gitcmd`.
- [x] Replace `pushWithToken` body with:

```go
func pushWithToken(ctx context.Context, dir, branch, token string) error {
	if err := gitcmd.Run(ctx, gitcmd.Options{Dir: dir, Token: token}, "push", "origin", branch); err != nil {
		return fmt.Errorf("git push failed: %w", err)
	}
	return nil
}
```

- [x] Run `go test -count=1 ./internal/execution ./internal/gitcmd`.

Expected: PASS.

## Task 4: Update docs and full verification

**Files:**
- Modify: `CONTEXT.md`
- Modify: `plans/README.md`

**Steps:**
- [x] Add a `Git Command Runner` term to `CONTEXT.md` near **Push Credential**.
- [x] Add Plan 003 to `plans/README.md` and mark `DONE` after verification.
- [x] Run:

```powershell
go test -count=1 ./...
go vet ./...
go build -o "$env:TEMP\\symphony-plan-003.exe" ./cmd/symphony
git diff --check
```

Expected: all commands exit 0.

## Done Criteria

- [x] `workspace.cloneRepo` no longer owns askpass creation or token redaction.
- [x] `execution.pushWithToken` no longer owns askpass creation or token redaction.
- [x] One Module owns `GIT_ASKPASS`, `SYMPHONY_GIT_TOKEN`, cleanup, redaction, and output bounding.
- [x] Existing clone/push behavior is preserved.
- [x] `go test -count=1 ./...`, `go vet ./...`, build, and `git diff --check` pass.

## STOP Conditions

- Any change would pass Push Credentials to Codex or Reviewer Command.
- Tests require real Gitea or network access.
- Git error wrapping loses `*exec.ExitError` where callers need it.
- Unifying the runner requires changing config format.

# Plan 002: Deepen Task Issue Execution Pipeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Executor instructions**: Read this plan fully before editing. Run the verification command listed in each task before moving on. If a STOP condition occurs, stop and report rather than improvising. When done, update the status row for this plan in `plans/README.md` unless a reviewer told you they maintain the index.

**Goal:** Move the per-Task Issue execution sequence behind one deep Module so Scheduler only polls, filters, and dispatches.

**Architecture:** Create `internal/executor` as the Task Issue execution Module. Its public Interface should be small: construct a runner, then process one Task Issue for one Managed Project. Workspace creation, Execution Branch creation, Agent Run, Review Gate, publish, Done Handoff, failure marking, and cleanup become the runner Implementation. Scheduler keeps the poll loop, concurrency slot, and pending filter only.

**Tech Stack:** Go 1.22, standard library, `log/slog`, existing `internal/tracker`, `internal/workspace`, `internal/execution`, `internal/reviewer`, and `internal/commandline`.

## Global Constraints

- Keep the simplified MVP from `AGENTS.md`: Gitea only, no Linear, no `httpapi`, no validator package, no reconcile loop, no per-state concurrency limits.
- Do not pass `GITEA_TOKEN` or other Push Credentials to Codex or the Reviewer Command.
- Do not add third-party dependencies.
- Do not change the external config format.
- Do not add PR creation, merge automation, issue closing, JSON-RPC streaming, or structured Verdict File parsing.
- Failed work after workspace creation must remain inspectable.
- This is an architecture refactor: behavior should remain the same except for clearer test seams and logs.

---

## Current State

- `internal/orchestrator/scheduler.go` owns both scheduling and execution. The same file polls projects, acquires concurrency slots, marks `symphony-running`, creates a workspace, creates an Execution Branch, runs Codex, runs reviewer, commits/pushes, marks `symphony-done`, marks failures, and cleans/preserves workspaces.
- `internal/orchestrator/scheduler_test.go` tests pending filtering, Codex command behavior, prompt construction, and cleanup policy through Scheduler internals.
- `internal/reviewer/reviewer.go`, `internal/workspace/workspace.go`, and `internal/execution/git.go` already contain focused operations that can sit behind a deeper Task Issue execution Module.
- `CONTEXT.md` already defines Task Issue, Execution Branch, Done Handoff, Agent Run, Reviewer Command, Review Gate, Push Credential, and Agent Environment. It does not yet name the new Task Issue Execution Module explicitly.

## Target File Structure

- Create `internal/executor/runner.go`: deep Module for one Task Issue execution pipeline.
- Create `internal/executor/codex.go`: Codex Agent Run command execution moved out of Scheduler.
- Create `internal/executor/runner_test.go`: tests for Done Handoff, failure marking, cleanup, and ordering through one runner Interface.
- Create `internal/executor/codex_test.go`: migrated Codex prompt, args, environment, and bounded-output tests.
- Modify `internal/orchestrator/scheduler.go`: remove execution Implementation; dispatch to an issue runner Interface.
- Modify `internal/orchestrator/scheduler_test.go`: keep scheduling tests; remove Codex execution tests; add dispatch test.
- Modify `CONTEXT.md`: add a short term for Task Issue Execution Module if the code uses that name in comments or docs.
- Modify `plans/README.md`: mark this plan `DONE` only after implementation and verification are complete.

## Commands You Will Need

| Purpose | Command | Expected On Success |
|---------|---------|---------------------|
| New executor tests | `go test -count=1 ./internal/executor` | exit 0 |
| Scheduler tests | `go test -count=1 ./internal/orchestrator` | exit 0 |
| Focused packages | `go test -count=1 ./internal/executor ./internal/orchestrator ./internal/reviewer ./internal/execution ./internal/workspace` | exit 0 |
| Full tests | `go test -count=1 ./...` | exit 0 |
| Vet | `go vet ./...` | exit 0 with no diagnostics |
| Build | `go build -o "$env:TEMP\\symphony-plan-002.exe" ./cmd/symphony` | exit 0 on PowerShell |
| Whitespace | `git diff --check` | exit 0 with no output |
| Status | `git status --short` | only expected plan/code/doc files changed |

## Git Workflow

- Suggested branch: `codex/deepen-task-issue-execution-pipeline`.
- Do not commit, push, or open a PR unless the operator explicitly asks.
- If commits are requested, use focused commits:
  - `refactor: add task issue executor`
  - `refactor: slim scheduler dispatch`
  - `docs: record task issue execution module`

## Task 1: Add the Task Issue executor Module

**Files:**
- Create: `internal/executor/runner.go`
- Create: `internal/executor/runner_test.go`

**Interfaces:**
- Consumes:
  - `tracker.Client`
  - `workspace.Create`
  - `workspace.Clean`
  - `execution.CreateBranch`
  - `execution.BranchName`
  - `execution.CommitAndPush`
  - `reviewer.Run`
- Produces:
  - `type Runner struct`
  - `func New(cfg domain.Config, tr tracker.Client) *Runner`
  - `func (r *Runner) Process(ctx context.Context, issue domain.Issue, project domain.ProjectConfig)`

- [ ] **Step 1: Write the failing runner success test**

Create `internal/executor/runner_test.go` with this test scaffold:

```go
package executor

import (
	"context"
	"reflect"
	"testing"

	"github.com/local/symphony/internal/domain"
)

type recordingTracker struct {
	statuses []domain.Status
}

func (r *recordingTracker) FetchIssues(context.Context, domain.ProjectConfig) ([]domain.Issue, error) {
	return nil, nil
}

func (r *recordingTracker) MarkStatus(_ context.Context, _ domain.Issue, status domain.Status) error {
	r.statuses = append(r.statuses, status)
	return nil
}

func TestRunnerCompletesDoneHandoffInOrder(t *testing.T) {
	var cfg domain.Config
	cfg.Gitea.Token = "push-token"
	tr := &recordingTracker{}
	r := New(cfg, tr)

	issue := domain.Issue{ProjectID: "p", ID: "12", Identifier: "acme/app#12", Title: "Fix login"}
	project := domain.ProjectConfig{ID: "p", RepoURL: "https://gitea.example.com/acme/app.git"}
	ws := domain.Workspace{Path: t.TempDir(), IssueKey: "p/issue-12-fix-login"}
	var calls []string
	cleaned := false

	r.createWorkspace = func(ctx context.Context, got domain.Issue, gotCfg domain.Config) (domain.Workspace, error) {
		calls = append(calls, "workspace")
		if !reflect.DeepEqual(got, issue) {
			t.Fatalf("createWorkspace issue = %#v, want %#v", got, issue)
		}
		return ws, nil
	}
	r.cleanWorkspace = func(ctx context.Context, got domain.Workspace) error {
		calls = append(calls, "clean")
		cleaned = true
		return nil
	}
	r.createBranch = func(got domain.Workspace, gotIssue domain.Issue) error {
		calls = append(calls, "branch")
		return nil
	}
	r.branchName = func(domain.Issue) string {
		return "symphony/p/issue-12-fix-login"
	}
	r.runCodex = func(ctx context.Context, gotCfg domain.Config, gotIssue domain.Issue, got domain.Workspace) error {
		calls = append(calls, "codex")
		return nil
	}
	r.runReviewer = func(ctx context.Context, command string, timeout time.Duration, path string) error {
		calls = append(calls, "reviewer")
		return nil
	}
	r.commitAndPush = func(ctx context.Context, got domain.Workspace, branch, token string) (domain.PublishResult, error) {
		calls = append(calls, "publish")
		if token != "push-token" {
			t.Fatalf("push token = %q, want push-token", token)
		}
		return domain.PublishResult{Branch: branch, Commit: "abc123"}, nil
	}

	r.Process(context.Background(), issue, project)

	if !reflect.DeepEqual(tr.statuses, []domain.Status{domain.StatusRunning, domain.StatusDone}) {
		t.Fatalf("statuses = %#v", tr.statuses)
	}
	wantCalls := []string{"workspace", "branch", "codex", "reviewer", "publish", "clean"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", calls, wantCalls)
	}
	if !cleaned {
		t.Fatal("successful run did not clean workspace")
	}
}
```

Also import `time` in the test because the `runReviewer` adapter receives a `time.Duration`.

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test -count=1 ./internal/executor`

Expected: FAIL because `internal/executor` does not exist or `New` is undefined.

- [ ] **Step 3: Implement `internal/executor/runner.go`**

Create `internal/executor/runner.go`:

```go
package executor

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/local/symphony/internal/domain"
	"github.com/local/symphony/internal/execution"
	"github.com/local/symphony/internal/reviewer"
	"github.com/local/symphony/internal/tracker"
	"github.com/local/symphony/internal/workspace"
)

// Runner processes one Task Issue through Agent Run, Review Gate, publish, and Done Handoff.
type Runner struct {
	Config  domain.Config
	Tracker tracker.Client
	Log     *slog.Logger

	createWorkspace func(context.Context, domain.Issue, domain.Config) (domain.Workspace, error)
	cleanWorkspace  func(context.Context, domain.Workspace) error
	createBranch    func(domain.Workspace, domain.Issue) error
	branchName      func(domain.Issue) string
	runCodex        func(context.Context, domain.Config, domain.Issue, domain.Workspace) error
	runReviewer     func(context.Context, string, time.Duration, string) error
	commitAndPush   func(context.Context, domain.Workspace, string, string) (domain.PublishResult, error)
}

// New creates a Runner. It panics if tracker is nil.
func New(cfg domain.Config, tr tracker.Client) *Runner {
	if tr == nil {
		panic("executor: tracker client is required")
	}
	return &Runner{
		Config:          cfg,
		Tracker:         tr,
		Log:             slog.Default(),
		createWorkspace: workspace.Create,
		cleanWorkspace:  workspace.Clean,
		createBranch:    execution.CreateBranch,
		branchName:      execution.BranchName,
		runCodex:        RunCodex,
		runReviewer:     reviewer.Run,
		commitAndPush:   execution.CommitAndPush,
	}
}

// Process runs the full Task Issue execution pipeline:
// mark running -> create workspace -> create branch -> Codex -> Review Gate -> publish -> mark done.
func (r *Runner) Process(ctx context.Context, issue domain.Issue, project domain.ProjectConfig) {
	log := r.logger().With(
		"project", project.ID,
		"issue", issue.Identifier,
		"title", issue.Title,
	)
	log.Info("processing issue")

	if err := r.Tracker.MarkStatus(ctx, issue, domain.StatusRunning); err != nil {
		log.Error("mark running failed", "error", err)
		return
	}

	failed := false
	var ws domain.Workspace
	fail := func(reason string) {
		failed = true
		if strings.TrimSpace(ws.Path) != "" {
			log.Error(reason, "workspace_path", ws.Path)
		} else {
			log.Error(reason)
		}
		_ = r.Tracker.MarkStatus(context.Background(), issue, domain.StatusFailed)
	}

	var err error
	ws, err = r.createWorkspace(ctx, issue, r.Config)
	if err != nil {
		fail(fmt.Sprintf("create workspace failed: %v", err))
		return
	}
	defer func() {
		if !shouldCleanWorkspace(failed, ws) {
			if failed && strings.TrimSpace(ws.Path) != "" {
				log.Info("preserving failed workspace", "workspace_path", ws.Path)
			}
			return
		}
		if cleanErr := r.cleanWorkspace(context.Background(), ws); cleanErr != nil {
			log.Error("clean workspace failed", "error", cleanErr)
		}
	}()

	if err := r.createBranch(ws, issue); err != nil {
		fail(fmt.Sprintf("create branch failed: %v", err))
		return
	}
	branch := r.branchName(issue)

	log.Info("running codex", "branch", branch)
	if err := r.runCodex(ctx, r.Config, issue, ws); err != nil {
		fail(fmt.Sprintf("codex failed: %v", err))
		return
	}

	log.Info("running reviewer")
	if err := r.runReviewer(ctx, r.Config.Reviewer.Command, r.Config.Reviewer.Timeout, ws.Path); err != nil {
		fail(fmt.Sprintf("reviewer failed: %v", err))
		return
	}

	log.Info("committing and pushing")
	result, err := r.commitAndPush(ctx, ws, branch, r.Config.Gitea.Token)
	if err != nil {
		fail(fmt.Sprintf("commit and push failed: %v", err))
		return
	}
	log.Info("pushed", "branch", result.Branch, "commit", result.Commit)

	if err := r.Tracker.MarkStatus(context.Background(), issue, domain.StatusDone); err != nil {
		log.Error("mark done failed", "error", err)
		return
	}
	log.Info("issue completed")
}

func (r *Runner) logger() *slog.Logger {
	if r.Log != nil {
		return r.Log
	}
	return slog.Default()
}

func shouldCleanWorkspace(failed bool, ws domain.Workspace) bool {
	return !failed && strings.TrimSpace(ws.Path) != ""
}
```

- [ ] **Step 4: Fix imports in the test**

Ensure `internal/executor/runner_test.go` imports `time`:

```go
import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/local/symphony/internal/domain"
)
```

- [ ] **Step 5: Run Task 1 verification**

Run: `go test -count=1 ./internal/executor`

Expected: PASS.

## Task 2: Move Codex Agent Run command execution into executor

**Files:**
- Create: `internal/executor/codex.go`
- Create: `internal/executor/codex_test.go`
- Modify later: `internal/orchestrator/scheduler_test.go` after migration

**Interfaces:**
- Produces:
  - `func RunCodex(ctx context.Context, cfg domain.Config, issue domain.Issue, ws domain.Workspace) error`
  - `func buildCodexPrompt(issue domain.Issue) string`
- Consumes:
  - `commandline.Split`
  - `agentenv.Filter`

- [ ] **Step 1: Write Codex tests in executor**

Create `internal/executor/codex_test.go` by moving the Codex-focused tests and helpers from `internal/orchestrator/scheduler_test.go`:

```go
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

func TestRunCodexDoesNotExposeGiteaTokenToCommand(t *testing.T) {
	t.Setenv("GITEA_TOKEN", "fixture-token")
	t.Setenv("PATH", os.Getenv("PATH"))

	dir := t.TempDir()
	outPath := filepath.Join(dir, "env.txt")
	script := writeEnvCaptureCommand(t, dir, "codex", outPath)

	var cfg domain.Config
	cfg.Codex.Command = script
	cfg.Codex.Timeout = time.Minute
	issue := domain.Issue{ProjectID: "p", ID: "1", Title: "Do work"}
	ws := domain.Workspace{Path: dir, IssueKey: "p/1"}

	if err := RunCodex(context.Background(), cfg, issue, ws); err != nil {
		t.Fatalf("RunCodex returned error: %v", err)
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
	issue := domain.Issue{ProjectID: "p", ID: "1", Title: "Do work"}
	ws := domain.Workspace{Path: dir, IssueKey: "p/1"}

	if err := RunCodex(context.Background(), cfg, issue, ws); err != nil {
		t.Fatalf("RunCodex returned error: %v", err)
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
	description := "Details from the issue body"
	issue := domain.Issue{Title: "Do work", Description: &description}

	prompt := buildCodexPrompt(issue)

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
	issue := domain.Issue{ProjectID: "p", ID: "1", Title: "Do work"}
	ws := domain.Workspace{Path: dir, IssueKey: "p/1"}

	err := RunCodex(context.Background(), cfg, issue, ws)
	if err == nil {
		t.Fatal("RunCodex returned nil error, want failure")
	}
	text := err.Error()
	if !strings.Contains(text, "...[truncated]") {
		t.Fatalf("error = %q, want truncated output marker", text)
	}
	if len(text) > 1200 {
		t.Fatalf("error length = %d, want bounded diagnostic", len(text))
	}
}
```

Append the helper functions from the current Scheduler test unchanged: `writeEnvCaptureCommand`, `writeArgvEnvCaptureCommand`, `writeFailingOutputCommand`, and `shellQuote`.

- [ ] **Step 2: Run executor tests and confirm the new Codex tests fail**

Run: `go test -count=1 ./internal/executor`

Expected: FAIL because `RunCodex` and `buildCodexPrompt` are not implemented.

- [ ] **Step 3: Implement `internal/executor/codex.go`**

Create `internal/executor/codex.go`:

```go
package executor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/local/symphony/internal/agentenv"
	"github.com/local/symphony/internal/commandline"
	"github.com/local/symphony/internal/domain"
)

// RunCodex executes the configured Codex command inside the workspace.
func RunCodex(ctx context.Context, cfg domain.Config, issue domain.Issue, ws domain.Workspace) error {
	cmdStr := strings.TrimSpace(cfg.Codex.Command)
	if cmdStr == "" {
		cmdStr = "codex"
	}

	timeout := cfg.Codex.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	prompt := buildCodexPrompt(issue)

	name, args, err := commandline.Split(cmdStr, "codex")
	if err != nil {
		return err
	}
	args = append(args, "--prompt", prompt)

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = ws.Path
	cmd.Env = agentenv.Filter(os.Environ())

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		text := strings.TrimSpace(out.String())
		if len(text) > 1024 {
			text = text[:1024] + "...[truncated]"
		}
		if text != "" {
			return fmt.Errorf("codex: %w: %s", err, text)
		}
		return fmt.Errorf("codex: %w", err)
	}
	return nil
}

func buildCodexPrompt(issue domain.Issue) string {
	var b strings.Builder
	b.WriteString("Resolve the following issue:\n\n")
	b.WriteString(fmt.Sprintf("Title: %s\n", issue.Title))
	if issue.Description != nil && *issue.Description != "" {
		b.WriteString(fmt.Sprintf("Description:\n%s\n", *issue.Description))
	}
	b.WriteString("\nImplement the necessary changes in this repository.")
	return b.String()
}
```

- [ ] **Step 4: Run Task 2 verification**

Run: `go test -count=1 ./internal/executor`

Expected: PASS.

## Task 3: Slim Scheduler to polling and dispatch

**Files:**
- Modify: `internal/orchestrator/scheduler.go`
- Modify: `internal/orchestrator/scheduler_test.go`

**Interfaces:**
- Consumes:
  - `executor.New(cfg, tracker)`
  - `Process(ctx, issue, project)`
- Produces:
  - Scheduler remains constructed with `orchestrator.New(cfg, tracker.Client) *Scheduler`.
  - Scheduler no longer exposes or owns Codex execution helpers.

- [ ] **Step 1: Write a Scheduler dispatch test**

Replace the simple `testTracker` in `internal/orchestrator/scheduler_test.go` with a recording tracker that can return issues:

```go
type testTracker struct {
	issues []domain.Issue
}

func (t testTracker) FetchIssues(context.Context, domain.ProjectConfig) ([]domain.Issue, error) {
	return t.issues, nil
}

func (testTracker) MarkStatus(context.Context, domain.Issue, domain.Status) error { return nil }
```

Add this runner test double:

```go
type recordingRunner struct {
	issues   []domain.Issue
	projects []domain.ProjectConfig
}

func (r *recordingRunner) Process(_ context.Context, issue domain.Issue, project domain.ProjectConfig) {
	r.issues = append(r.issues, issue)
	r.projects = append(r.projects, project)
}
```

Add the failing test:

```go
func TestPollDispatchesPendingIssueToRunner(t *testing.T) {
	var cfg domain.Config
	project := domain.ProjectConfig{ID: "p", ActiveStates: []string{"open"}}
	cfg.Gitea.Projects = []domain.ProjectConfig{project}
	issue := domain.Issue{ProjectID: "p", ID: "1", Identifier: "acme/app#1", Title: "Do work", State: "open"}
	runner := &recordingRunner{}
	s := New(cfg, testTracker{issues: []domain.Issue{issue}})
	s.runner = runner

	s.poll(context.Background())
	s.wg.Wait()

	if len(runner.issues) != 1 || runner.issues[0].ID != "1" {
		t.Fatalf("runner issues = %#v, want issue 1", runner.issues)
	}
	if len(runner.projects) != 1 || runner.projects[0].ID != "p" {
		t.Fatalf("runner projects = %#v, want project p", runner.projects)
	}
}
```

- [ ] **Step 2: Run Scheduler tests and confirm failure**

Run: `go test -count=1 ./internal/orchestrator`

Expected: FAIL because `Scheduler` has no `runner` field or matching runner Interface yet.

- [ ] **Step 3: Update Scheduler struct and constructor**

In `internal/orchestrator/scheduler.go`, remove imports that only support execution internals:

```go
import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/local/symphony/internal/domain"
	"github.com/local/symphony/internal/executor"
	"github.com/local/symphony/internal/tracker"
)
```

Add an unexported runner Interface:

```go
type issueRunner interface {
	Process(ctx context.Context, issue domain.Issue, project domain.ProjectConfig)
}
```

Add the field:

```go
type Scheduler struct {
	Config  domain.Config
	Tracker tracker.Client

	runner issueRunner
	sem    chan struct{}
	wg     sync.WaitGroup
}
```

Update `New`:

```go
return &Scheduler{
	Config:  cfg,
	Tracker: tr,
	runner:  executor.New(cfg, tr),
	sem:     make(chan struct{}, max),
}
```

- [ ] **Step 4: Change dispatch to call the runner**

In `poll`, replace the goroutine body:

```go
go func(is domain.Issue, project domain.ProjectConfig) {
	defer s.wg.Done()
	defer func() { <-s.sem }()
	s.runner.Process(ctx, is, project)
}(issue, proj)
```

- [ ] **Step 5: Delete execution internals from Scheduler**

Remove these functions from `internal/orchestrator/scheduler.go`:

```go
func (s *Scheduler) processIssue(...)
func (s *Scheduler) runCodex(...)
func (s *Scheduler) buildCodexPrompt(...)
func shouldCleanWorkspace(...)
```

Keep:

```go
func hasManagedStatusLabel(labels []string) bool
```

because `isPending` still needs the pending filter until Completion Marker policy is deepened in a later plan.

- [ ] **Step 6: Remove migrated Codex tests from Scheduler test**

Delete these tests and helpers from `internal/orchestrator/scheduler_test.go`:

```go
TestRunCodexDoesNotExposeGiteaTokenToCommand
TestRunCodexPassesConfiguredArgsAndPrompt
TestBuildCodexPromptIncludesIssueDetails
TestRunCodexReturnsBoundedOutputOnFailure
TestShouldCleanWorkspaceAfterProcessing
writeEnvCaptureCommand
writeArgvEnvCaptureCommand
writeFailingOutputCommand
shellQuote
```

Remove now-unused imports:

```go
os
path/filepath
runtime
time
```

Keep `strings` if `TestIsPendingSkipsManagedStatusLabels` still uses it indirectly; remove it if Go reports it unused.

- [ ] **Step 7: Run Task 3 verification**

Run: `go test -count=1 ./internal/orchestrator ./internal/executor`

Expected: PASS.

## Task 4: Add failure-path runner tests

**Files:**
- Modify: `internal/executor/runner_test.go`

**Interfaces:**
- Consumes:
  - `Runner.Process`
  - injected adapter functions on `Runner`
- Produces:
  - Evidence that Review Gate failure does not publish and preserves workspace.
  - Evidence that workspace creation failure does not clean an empty workspace path.

- [ ] **Step 1: Add reviewer-failure test**

Append to `internal/executor/runner_test.go`:

```go
func TestRunnerMarksFailedAndPreservesWorkspaceWhenReviewerFails(t *testing.T) {
	var cfg domain.Config
	tr := &recordingTracker{}
	r := New(cfg, tr)
	issue := domain.Issue{ProjectID: "p", ID: "12", Identifier: "acme/app#12", Title: "Fix login"}
	project := domain.ProjectConfig{ID: "p"}
	ws := domain.Workspace{Path: t.TempDir(), IssueKey: "p/issue-12-fix-login"}
	cleaned := false
	published := false

	r.createWorkspace = func(context.Context, domain.Issue, domain.Config) (domain.Workspace, error) {
		return ws, nil
	}
	r.cleanWorkspace = func(context.Context, domain.Workspace) error {
		cleaned = true
		return nil
	}
	r.createBranch = func(domain.Workspace, domain.Issue) error { return nil }
	r.branchName = func(domain.Issue) string { return "symphony/p/issue-12-fix-login" }
	r.runCodex = func(context.Context, domain.Config, domain.Issue, domain.Workspace) error { return nil }
	r.runReviewer = func(context.Context, string, time.Duration, string) error {
		return errors.New("review failed")
	}
	r.commitAndPush = func(context.Context, domain.Workspace, string, string) (domain.PublishResult, error) {
		published = true
		return domain.PublishResult{}, nil
	}

	r.Process(context.Background(), issue, project)

	if !reflect.DeepEqual(tr.statuses, []domain.Status{domain.StatusRunning, domain.StatusFailed}) {
		t.Fatalf("statuses = %#v", tr.statuses)
	}
	if cleaned {
		t.Fatal("failed run cleaned workspace; want preserved")
	}
	if published {
		t.Fatal("review failure published execution branch")
	}
}
```

Add `errors` to the test imports.

- [ ] **Step 2: Add workspace-create failure test**

Append:

```go
func TestRunnerMarksFailedWithoutCleaningWhenWorkspaceCreateFails(t *testing.T) {
	var cfg domain.Config
	tr := &recordingTracker{}
	r := New(cfg, tr)
	issue := domain.Issue{ProjectID: "p", ID: "12", Identifier: "acme/app#12", Title: "Fix login"}
	project := domain.ProjectConfig{ID: "p"}
	cleaned := false

	r.createWorkspace = func(context.Context, domain.Issue, domain.Config) (domain.Workspace, error) {
		return domain.Workspace{}, errors.New("clone failed")
	}
	r.cleanWorkspace = func(context.Context, domain.Workspace) error {
		cleaned = true
		return nil
	}

	r.Process(context.Background(), issue, project)

	if !reflect.DeepEqual(tr.statuses, []domain.Status{domain.StatusRunning, domain.StatusFailed}) {
		t.Fatalf("statuses = %#v", tr.statuses)
	}
	if cleaned {
		t.Fatal("workspace create failure should not clean empty workspace")
	}
}
```

- [ ] **Step 3: Run Task 4 verification**

Run: `go test -count=1 ./internal/executor`

Expected: PASS.

## Task 5: Document the new Module and run the full gate

**Files:**
- Modify: `CONTEXT.md`
- Modify: `plans/README.md`

**Interfaces:**
- Produces:
  - Domain term for the new Module if code comments use it.
  - Plan status update after implementation is complete.

- [ ] **Step 1: Add Task Issue Execution to `CONTEXT.md`**

Insert after **Task Issue**:

```markdown
**Task Issue Execution**:
The in-process module that runs one **Task Issue** from `symphony-running` through Agent Run, Review Gate, Execution Branch publish, and Done Handoff or failure marking.
_Avoid_: scheduler, worker, background job when referring to the per-issue execution sequence
```

- [ ] **Step 2: Update `plans/README.md` only after all code verification passes**

Change the table to include:

```markdown
| 002 | Deepen Task Issue execution pipeline | P1 | M | 001 | DONE |
```

Add a dependency note:

```markdown
- 002 should land after 001 because it preserves the hardened command, credential, failure, and commit behavior while moving per-issue execution behind one deeper Module.
```

- [ ] **Step 3: Run focused package verification**

Run:

```powershell
go test -count=1 ./internal/executor ./internal/orchestrator ./internal/reviewer ./internal/execution ./internal/workspace
```

Expected: exit 0.

- [ ] **Step 4: Run full verification**

Run:

```powershell
go test -count=1 ./...
go vet ./...
go build -o "$env:TEMP\\symphony-plan-002.exe" ./cmd/symphony
git diff --check
git status --short
```

Expected:

- `go test -count=1 ./...` exits 0.
- `go vet ./...` exits 0 with no diagnostics.
- `go build ...` exits 0.
- `git diff --check` exits 0 with no output.
- `git status --short` lists only the intended code, test, context, and plan index files.

## Review Focus

After implementation, run the available review loop required by `AGENTS.md`. Ask the reviewer to focus on:

- Scheduler no longer owns Agent Run, Review Gate, publish, or workspace cleanup Implementation.
- Runner Interface remains small; test adapter fields do not leak into production callers.
- Existing credential filtering and bounded diagnostics are preserved.
- Failure paths still mark `symphony-failed` and preserve workspaces after creation.
- The refactor does not add old validator, HTTP API, Linear, retry, blocked-by, or per-state concurrency concepts.

## Done Criteria

- [ ] `internal/executor` exists and owns one Task Issue execution sequence.
- [ ] Scheduler only polls, filters pending issues, controls concurrency, and dispatches to the runner.
- [ ] Codex Agent Run command execution no longer lives in Scheduler.
- [ ] Success path marks `running -> done`, publishes once, and cleans workspace.
- [ ] Reviewer failure marks `running -> failed`, does not publish, and preserves workspace.
- [ ] Workspace creation failure marks failed without attempting to clean an empty path.
- [ ] Existing Codex command parsing, Agent Environment filtering, and bounded failure output tests pass in `internal/executor`.
- [ ] `CONTEXT.md` names Task Issue Execution if the implementation uses that term.
- [ ] `go test -count=1 ./...` passes.
- [ ] `go vet ./...` passes.
- [ ] `go build -o "$env:TEMP\\symphony-plan-002.exe" ./cmd/symphony` passes.
- [ ] `git diff --check` passes.
- [ ] `plans/README.md` marks Plan 002 DONE only after the implementation gate passes.

## STOP Conditions

Stop and report if:

- The current code already diverges from the file roles described in this plan.
- Moving execution out of Scheduler requires changing config format or public CLI flags.
- Any implementation requires passing Push Credentials to Codex or the Reviewer Command.
- The refactor starts reintroducing validator package, HTTP API, Linear, multi-client tracker, retry, blocked-by, or per-state concurrency concepts.
- Tests require real Gitea, real Codex, real Claude, or network access.
- A failure path can no longer preserve the workspace path after workspace creation succeeds.
- A focused package test fails after two fix attempts.

## Self-Review

- Spec coverage: the plan covers the architecture-review recommendation to deepen Task Issue execution while preserving the simplified MVP constraints.
- Placeholder scan: no placeholder markers or unspecified implementation steps remain.
- Type consistency: `Runner.Process(ctx, issue, project)`, `RunCodex(ctx, cfg, issue, ws)`, and the runner adapter function signatures are consistent across tasks.

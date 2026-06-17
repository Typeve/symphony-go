# Plan 001: Harden MVP Execution Pipeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Executor instructions**: Follow this plan step by step. Run every verification command and confirm the expected result before moving to the next step. If anything in the "STOP conditions" section occurs, stop and report; do not improvise. When done, update the status row for this plan in `plans/README.md` unless a reviewer dispatched you and told you they maintain the index.
>
> **Drift check (run first)**: `git diff --stat d1d5d9b..HEAD -- internal/execution internal/orchestrator internal/reviewer internal/config internal/domain README.md WORKFLOW.example.md CONTEXT.md docs/operations/symphony-service.md plans/README.md`
>
> If any in-scope file changed since this plan was written, compare the "Current state" excerpts against the live code before proceeding; on a mismatch, treat it as a STOP condition.

**Goal:** Make the current simplified MVP execution path safe and diagnosable before adding new behavior.

**Architecture:** Keep the existing small pipeline: Gitea issue -> workspace clone -> execution branch -> Codex command -> reviewer command -> commit and push -> Gitea status label. Add narrow helpers around command-line parsing, commit staging, and failure handling instead of reintroducing the older validator/httpapi/retry architecture. Align docs to the simplified MVP so future agents do not implement stale contracts.

**Tech Stack:** Go 1.22, standard library, `gopkg.in/yaml.v3`, Gitea REST, `log/slog`, shell-out to Git/Codex/reviewer commands.

## Global Constraints

- Default communication and user-facing docs are Chinese unless an existing section is intentionally bilingual.
- Preserve the simplified MVP scope in `AGENTS.md`: Gitea only, no Linear, no `httpapi`, no validator Read-Only Guard package, no reconcile loop, no per-state concurrency limits.
- Keep `GITEA_TOKEN` out of Codex/reviewer environments, logs, issue comments, Git remote URLs, and plan/code examples.
- Do not add third-party dependencies for command parsing or glob matching unless the current standard library cannot satisfy a verified requirement.
- Do not implement PR creation, merge automation, issue closing, JSON-RPC streaming, or full External Validator/Verdict File workflow in this plan.

## Status

- **Priority**: P1
- **Effort**: M
- **Risk**: MED
- **Depends on**: none
- **Category**: security, correctness, dx, docs, tests
- **Planned at**: commit `d1d5d9b`, 2026-06-17

## Why This Matters

The repository is already small enough for the target MVP, but the core execution path still has unsafe edges. `git add -A` can stage local agent artifacts or sensitive files, configured commands containing arguments such as `codex app-server` do not run as intended, Codex output is discarded, and failed workspaces are removed before humans can inspect them. The docs also describe older Validator/Verdict/HTTP API behavior that the current code does not implement, which will mislead future agents.

Landing this plan makes the existing MVP trustworthy without expanding scope: failed runs leave evidence, command configuration works, sensitive artifacts are not committed, and docs match the code.

## Current State

Relevant files and roles:

- `internal/orchestrator/scheduler.go` - owns the issue pipeline and Codex invocation.
- `internal/execution/git.go` - creates execution branches, stages changes, commits, and pushes.
- `internal/reviewer/reviewer.go` - runs the reviewer command after Codex.
- `internal/config/config.go` and `internal/domain/domain.go` - load YAML into `domain.Config`.
- `internal/orchestrator/scheduler_test.go`, `internal/reviewer/reviewer_test.go`, `internal/tracker/gitea/client_test.go` - current test style and helper patterns.
- `README.md`, `WORKFLOW.example.md`, `CONTEXT.md`, `docs/operations/symphony-service.md` - public/operator contract.

Current pipeline in `internal/orchestrator/scheduler.go`:

```go
// internal/orchestrator/scheduler.go:153-154
// processIssue runs the full pipeline for a single issue:
// mark running -> create workspace -> create branch -> codex -> review -> commit & push -> mark done.
```

Failure handling currently marks failed but always schedules workspace cleanup:

```go
// internal/orchestrator/scheduler.go:169-184
fail := func(reason string) {
	log.Error(reason)
	_ = s.Tracker.MarkStatus(context.Background(), issue, domain.StatusFailed)
}

ws, err := workspace.Create(ctx, issue, s.Config)
if err != nil {
	fail(fmt.Sprintf("create workspace failed: %v", err))
	return
}
defer func() {
	if cleanErr := workspace.Clean(context.Background(), ws); cleanErr != nil {
		log.Error("clean workspace failed", "error", cleanErr)
	}
}()
```

Codex currently treats the whole configured command as a binary name and discards output:

```go
// internal/orchestrator/scheduler.go:224-249
cmdStr := strings.TrimSpace(s.Config.Codex.Command)
if cmdStr == "" {
	cmdStr = "codex"
}
prompt := s.buildCodexPrompt(issue)
cmd := exec.CommandContext(ctx, cmdStr, "--prompt", prompt)
cmd.Dir = ws.Path
cmd.Env = agentenv.Filter(os.Environ())
cmd.Stdout = nil
cmd.Stderr = nil
if err := cmd.Run(); err != nil {
	return fmt.Errorf("codex: %w", err)
}
```

Reviewer has the same command-string problem:

```go
// internal/reviewer/reviewer.go:18-43
command = strings.TrimSpace(command)
if command == "" {
	command = "claude"
}
reviewPrompt := "Review the changes in this repository for correctness, bugs, and code quality. Report your findings."
cmd := exec.CommandContext(ctx, command, "--prompt", reviewPrompt)
cmd.Dir = workspace
cmd.Env = agentenv.Filter(os.Environ())
if out, err := cmd.CombinedOutput(); err != nil {
	// returns truncated output only on failure
}
```

Commit currently stages everything and permits empty commits:

```go
// internal/execution/git.go:50-57
// Stage all changes.
if err := runGit(workspace.Path, "add", "-A"); err != nil {
	return domain.PublishResult{}, err
}
msg := fmt.Sprintf("symphony: automated changes for branch %s", branch)
if err := runGitCtx(ctx, workspace.Path, "commit", "-m", msg, "--allow-empty"); err != nil {
	return domain.PublishResult{}, err
}
```

The docs are split between simplified MVP and stale extended design:

```text
README.md:29-35
- Configuration is loaded from a YAML file, defaulting to `symphony.yaml`.
- Issue processing uses three managed states: `pending -> running -> done/failed`.
- Managed labels are limited to `symphony-running`, `symphony-done`, and `symphony-failed`.
- Codex and reviewer processes receive only a small environment allowlist.
- `GITEA_TOKEN` is used by Symphony for tracker and Git operations, but it is not passed to Codex or the reviewer process.
- Git clone and push use temporary `GIT_ASKPASS` credentials instead of writing tokens into remote URLs.
- No HTTP status API, PR creation, merge automation, dependency blocking, reconcile loop, or per-state concurrency limits are included in the current MVP.
```

```yaml
# WORKFLOW.example.md:21-24
commit:
  exclude:
    - ".codex/**"
    - ".symphony/validation-verdict.json"
```

```yaml
# WORKFLOW.example.md:50-65
codex:
  command: codex app-server
validator:
  kind: command
  command: claude-code --provider deepseek --workspace "$SYMPHONY_WORKSPACE" --output "$SYMPHONY_VERDICT_PATH"
```

Documented vocabulary still describes a validator publish gate:

```text
CONTEXT.md:56-57
The rule that an Execution Branch is pushed only after a passing Validation Verdict. Failed or blocked work remains in the local workspace for human inspection instead of being published as a remote branch.

CONTEXT.md:71-75
Validation must distinguish execution from judgment... Validator output must be a Verdict File.
```

Existing test conventions:

- Use package-local tests with `testing`, `httptest`, `t.TempDir`, helper scripts, and focused assertions.
- `internal/orchestrator/scheduler_test.go:46-70` verifies Codex does not receive `GITEA_TOKEN`.
- `internal/reviewer/reviewer_test.go:13-30` verifies reviewer does not receive `GITEA_TOKEN`.
- `internal/tracker/gitea/client_test.go:61-98` uses `httptest.NewServer` and exact request sequence assertions.

## Commands You Will Need

| Purpose | Command | Expected On Success |
|---------|---------|---------------------|
| Unit/integration tests | `go test -count=1 ./...` | exit 0; all packages pass |
| Vet | `go vet ./...` | exit 0; no output |
| Build | `go build -o "$env:TEMP\\symphony-plan-001.exe" ./cmd/symphony` on PowerShell, or `go build -o /tmp/symphony-plan-001 ./cmd/symphony` on POSIX | exit 0 |
| Whitespace check | `git diff --check` | exit 0; no output |
| Status check | `git status --short` | only expected in-scope files changed |

## Suggested Executor Toolkit

- Use `superpowers:test-driven-development` if available before writing implementation code.
- Use `review-feedback-adjudication` only if review feedback is provided after implementation.
- Use the project-local instructions in `AGENTS.md` as the source of truth when they conflict with stale docs.

## Scope

**In scope** (the only files you should modify):

- `internal/execution/git.go`
- `internal/execution/git_test.go` (create)
- `internal/orchestrator/scheduler.go`
- `internal/orchestrator/scheduler_test.go`
- `internal/reviewer/reviewer.go`
- `internal/reviewer/reviewer_test.go`
- `internal/config/config.go` only if config validation is added in Step 5
- `internal/config/config_test.go` (create only if Step 5 changes config loading)
- `internal/domain/domain.go` only if a minimal config struct field is required
- `README.md`
- `WORKFLOW.example.md`
- `CONTEXT.md`
- `docs/operations/symphony-service.md`
- `plans/README.md`

**Out of scope** (do NOT touch, even though they look related):

- Any Linear tracker implementation or multi-client abstraction.
- Any `internal/httpapi` package.
- Any validator package, Read-Only Guard, JSON Verdict File reader, rework loop, or `symphony-branch-ready` runtime state.
- PR creation, merge automation, issue closing, webhook support, or persistent database state.
- Any change that passes `GITEA_TOKEN` or other push credentials to Codex/reviewer.
- Any change that writes a token value into tests, docs, logs, comments, or plan text. Fixture names such as `fixture-token` are acceptable.

## Git Workflow

- Branch: `codex/harden-mvp-execution-pipeline`.
- Commit message style follows the current history: short conventional prefix, for example `fix: harden mvp execution pipeline`.
- Use focused commits if the operator wants commits: one for code/tests, one for docs alignment.
- Do not push or open a PR unless the operator explicitly instructs it.

## Steps

### Step 1: Add safe command-line parsing for configured commands

Add a small standard-library helper in `internal/reviewer/reviewer.go` first, then reuse it from scheduler. Keep it unexported unless tests need package-level access.

Required behavior:

- Empty command returns the caller-provided default binary and no extra args.
- `codex app-server` becomes binary `codex`, args `["app-server"]`.
- `claude-code --provider deepseek` becomes binary `claude-code`, args `["--provider", "deepseek"]`.
- Quoted substrings stay together: `tool --name "review bot"` becomes `["--name", "review bot"]`.
- Unterminated quotes return an error and do not execute a process.
- Do not invoke a shell. This is command splitting, not shell evaluation.

Suggested helper shape:

```go
func splitCommandLine(value, defaultCommand string) (string, []string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultCommand, nil, nil
	}
	// Implement a small scanner over runes:
	// - split on unicode.IsSpace outside quotes
	// - support single and double quotes
	// - support backslash escaping only inside double quotes and outside quotes
	// - return a clear error on unterminated quote
}
```

Update `reviewer.Run` to build commands like this:

```go
name, args, err := splitCommandLine(command, "claude")
if err != nil {
	return err
}
args = append(args, "--prompt", reviewPrompt)
cmd := exec.CommandContext(ctx, name, args...)
```

Update `Scheduler.runCodex` similarly:

```go
name, args, err := reviewer.SplitCommandLineForInternalUse(s.Config.Codex.Command, "codex")
if err != nil {
	return err
}
args = append(args, "--prompt", prompt)
cmd := exec.CommandContext(ctx, name, args...)
```

If you do not want to export from `reviewer`, create a tiny new package `internal/commandline` with:

```go
func Split(value, defaultCommand string) (string, []string, error)
```

If you create `internal/commandline`, it becomes in scope for this step and must include `internal/commandline/commandline_test.go`.

**Verify**: `go test -count=1 ./internal/reviewer ./internal/orchestrator` -> exit 0. At this point tests may not yet cover command args; Step 2 adds coverage.

### Step 2: Test command arguments, prompt passing, and token filtering

Extend the existing fake-command tests instead of using real Codex or Claude.

In `internal/reviewer/reviewer_test.go`, add a test that configures a reviewer command with one extra argument and captures argv.

POSIX helper script content:

```sh
#!/bin/sh
printf '%s\n' "$@" > "$ARGV_OUT"
env > "$ENV_OUT"
```

Windows `.cmd` helper content:

```bat
@echo off
echo %* > "%ARGV_OUT%"
set > "%ENV_OUT%"
```

The test should set `ARGV_OUT` and `ENV_OUT` using `t.Setenv`, then call:

```go
err := Run(context.Background(), script+" --mode strict", time.Minute, dir)
```

Assert:

- `err == nil`
- argv contains `--mode strict`
- argv contains `--prompt`
- env output does not contain `GITEA_TOKEN=` and does not contain `fixture-token`

In `internal/orchestrator/scheduler_test.go`, add a Codex equivalent:

```go
cfg.Codex.Command = script + " app-server"
```

Assert:

- argv contains `app-server`
- argv contains `--prompt`
- argv contains the issue title text only as part of the prompt argument
- env output does not contain `GITEA_TOKEN=` and does not contain `fixture-token`

Also add parser-focused tests if you created `internal/commandline`:

```go
func TestSplitCommandLineHandlesArgsAndQuotes(t *testing.T)
func TestSplitCommandLineRejectsUnterminatedQuote(t *testing.T)
func TestSplitCommandLineUsesDefaultForEmptyCommand(t *testing.T)
```

**Verify**:

- `go test -count=1 ./internal/reviewer ./internal/orchestrator` -> exit 0
- If `internal/commandline` exists: `go test -count=1 ./internal/commandline` -> exit 0

### Step 3: Prevent broad staging and empty execution commits

Refactor `internal/execution/git.go` so `CommitAndPush` stages only allowed changes and fails clearly when no allowed changes remain.

Add package-level defaults:

```go
var defaultCommitExcludes = []string{
	".codex",
	".codex/**",
	".symphony/validation-verdict.json",
	".env",
	".env.*",
	"*.log",
}

var ErrNoChanges = errors.New("no allowed changes to commit")
```

Implement a helper that can be tested without pushing:

```go
func stageAllowedChanges(ctx context.Context, dir string) error {
	if err := runGitCtx(ctx, dir, "add", "-A", "--", "."); err != nil {
		return err
	}
	for _, pattern := range defaultCommitExcludes {
		_ = runGitCtx(ctx, dir, "reset", "-q", "--", pattern)
	}
	changed, err := hasStagedChanges(ctx, dir)
	if err != nil {
		return err
	}
	if !changed {
		return ErrNoChanges
	}
	return nil
}

func hasStagedChanges(ctx context.Context, dir string) (bool, error) {
	err := runGitCtx(ctx, dir, "diff", "--cached", "--quiet")
	if err == nil {
		return false, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return true, nil
	}
	return false, err
}
```

If `runGitCtx` wraps the `*exec.ExitError` before callers can inspect it, add a new lower-level helper for this one check that preserves the exit code. Do not parse localized Git output to detect whether a diff exists.

Update `CommitAndPush`:

```go
if err := stageAllowedChanges(ctx, workspace.Path); err != nil {
	return domain.PublishResult{}, err
}
if err := runGitCtx(ctx, workspace.Path, "commit", "-m", msg); err != nil {
	return domain.PublishResult{}, err
}
```

Remove `--allow-empty`.

Create `internal/execution/git_test.go` with temp-repo tests:

- `TestStageAllowedChangesExcludesLocalArtifacts`: initialize a temp git repo, create a baseline commit, write `app.go`, `.env`, `.codex/session.json`, `.symphony/validation-verdict.json`, and `debug.log`; run `stageAllowedChanges`; assert `git diff --cached --name-only` returns only `app.go`.
- `TestStageAllowedChangesReturnsErrNoChangesWhenOnlyExcludedFilesChanged`: same setup but only excluded files changed; assert `errors.Is(err, ErrNoChanges)`.
- `TestBranchNameIsDeterministic`: optional if there is no current coverage; assert existing branch naming behavior remains unchanged.

Use these Git setup commands inside test helpers:

```go
runTestGit(t, dir, "init")
runTestGit(t, dir, "config", "user.email", "symphony@example.invalid")
runTestGit(t, dir, "config", "user.name", "Symphony Test")
```

**Verify**: `go test -count=1 ./internal/execution` -> exit 0.

### Step 4: Preserve failed workspace evidence and capture bounded command output

Update `internal/orchestrator/scheduler.go` and `internal/reviewer/reviewer.go` so failures leave enough evidence for a human without leaking secrets.

Required behavior:

- Successful issue processing still cleans the workspace.
- Failed processing keeps the workspace and logs `workspace_path`.
- Codex failure returns a bounded output excerpt.
- Reviewer failure already returns output; keep it bounded and make sure command-line parsing errors are also clear.
- Do not write full command output into Gitea comments in this plan.

Suggested scheduler shape:

```go
failed := false
fail := func(reason string) {
	failed = true
	log.Error(reason, "workspace_path", ws.Path)
	_ = s.Tracker.MarkStatus(context.Background(), issue, domain.StatusFailed)
}

defer func() {
	if failed {
		log.Info("preserving failed workspace", "workspace_path", ws.Path)
		return
	}
	if cleanErr := workspace.Clean(context.Background(), ws); cleanErr != nil {
		log.Error("clean workspace failed", "error", cleanErr)
	}
}()
```

Handle the pre-workspace case carefully: `ws.Path` is empty before `workspace.Create` succeeds. The log must not print a misleading path for workspace creation failures.

For Codex output, replace nil stdout/stderr with a bounded buffer:

```go
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
```

Add tests:

- In `internal/orchestrator/scheduler_test.go`, add `TestRunCodexReturnsBoundedOutputOnFailure`: fake command prints more than 1500 characters and exits non-zero; assert error includes `...[truncated]` and is shorter than 1200 characters.
- If practical without real clone/push, factor workspace cleanup decision into a small helper and test success vs failure behavior. If factoring would contort the code, rely on manual review plus final `go test`; do not build a large fake pipeline.

**Verify**: `go test -count=1 ./internal/orchestrator ./internal/reviewer` -> exit 0.

### Step 5: Add minimal config validation for required MVP fields

Currently `config.Load` unmarshals YAML and returns the struct without validating required fields. Add a small `Validate` function in `internal/config/config.go`.

Required checks:

- `gitea.endpoint` is required.
- `gitea.token` is required.
- At least one `gitea.projects` entry is required.
- Each project requires `id` and `repo_url`.
- `scheduler.max_concurrent <= 0` remains allowed because scheduler defaults it to 1.
- Empty `scheduler.poll_interval`, `codex.command`, `codex.timeout`, `reviewer.command`, `reviewer.timeout`, and `workspace.root` remain allowed because runtime code has defaults.

Suggested shape:

```go
func Validate(cfg domain.Config) error {
	if strings.TrimSpace(cfg.Gitea.Endpoint) == "" {
		return fmt.Errorf("gitea.endpoint is required")
	}
	if strings.TrimSpace(cfg.Gitea.Token) == "" {
		return fmt.Errorf("gitea.token is required")
	}
	if len(cfg.Gitea.Projects) == 0 {
		return fmt.Errorf("gitea.projects must contain at least one project")
	}
	for i, project := range cfg.Gitea.Projects {
		if strings.TrimSpace(project.ID) == "" {
			return fmt.Errorf("gitea.projects[%d].id is required", i)
		}
		if strings.TrimSpace(project.RepoURL) == "" {
			return fmt.Errorf("gitea.projects[%d].repo_url is required", i)
		}
	}
	return nil
}
```

Call it from `Load` after `yaml.Unmarshal`.

Create `internal/config/config_test.go`:

- `TestLoadExpandsEnvironmentAndValidatesRequiredFields`: write a temp YAML using `${GITEA_TOKEN_FOR_TEST}`, set env to `fixture-token`, call `Load`, assert token resolved.
- `TestLoadRejectsMissingGiteaEndpoint`: assert error contains `gitea.endpoint`.
- `TestLoadRejectsProjectWithoutRepoURL`: assert error contains `repo_url`.

Do not print real environment values in assertion failures.

**Verify**: `go test -count=1 ./internal/config` -> exit 0.

### Step 6: Align public docs to the simplified MVP contract

Update docs so they match the runtime after Steps 1-5.

`README.md`:

- Keep the existing simplified MVP bullets at lines 29-35.
- Add one bullet saying Symphony excludes common local agent artifacts from execution commits, including `.codex/**`, `.symphony/validation-verdict.json`, `.env*`, and `*.log`.
- Clarify that reviewer success is command exit code success in the simplified MVP; it is not a structured Verdict File.

`WORKFLOW.example.md`:

- Replace the old front matter shape with the same simplified `symphony.yaml` shape used in `README.md`.
- Keep a prompt section only if the runtime reads it. Since the runtime currently does not read `workflow_prompt`, remove `workflow_prompt`.
- Remove `validator`, `repair`, `labels.validating`, `labels.reworking`, `labels.branch_ready`, `commit.exclude`, and HTTP/API-era fields unless you implemented matching runtime config in this plan.
- Keep user-facing task instructions if useful, but label them as an optional prompt template not consumed by the current binary unless code was added to read it.

`CONTEXT.md`:

- Replace validator-specific terms with simplified terms:
  - `Reviewer Command`: command run after Codex inside the workspace.
  - `Review Gate`: reviewer command must exit 0 before commit/push.
  - `Execution Branch`: remains the handoff artifact.
  - `Push Credential` and `Agent Environment`: keep current credential boundary language.
- Remove or rewrite lines that say a Validation Verdict / Verdict File is required before publishing.

`docs/operations/symphony-service.md`:

- Update service invocation to use `-config /etc/symphony/symphony.yaml`, not `--port 8080 /etc/symphony/WORKFLOW.md`.
- Remove `/api/v1/state` and `/api/v1/refresh` status API instructions.
- Remove validator/Verdict File sections or clearly mark `examples/validator-go.sh` as a legacy helper not wired into the current simplified runtime.
- Replace `symphony-branch-ready`, `symphony-validating`, and `symphony-reworking` references with `symphony-running`, `symphony-done`, and `symphony-failed`.
- Document that failed workspaces are preserved after this plan lands.

**Verify**:

- `rg -n "branch_ready|symphony-branch-ready|symphony-validating|symphony-reworking|/api/v1/state|/api/v1/refresh|validator:|Validation Verdict|Verdict File|--port 8080" README.md WORKFLOW.example.md CONTEXT.md docs/operations/symphony-service.md`
- Expected: no matches except in a clearly labeled historical/legacy note. Prefer no matches.

### Step 7: Run full verification and review

Run the full local gate:

```powershell
go test -count=1 ./...
go vet ./...
go build -o "$env:TEMP\\symphony-plan-001.exe" ./cmd/symphony
git diff --check
git status --short
```

Expected:

- `go test -count=1 ./...` exits 0.
- `go vet ./...` exits 0 with no output.
- `go build ...` exits 0.
- `git diff --check` exits 0 with no output.
- `git status --short` lists only in-scope files plus `plans/README.md` status update if the executor is maintaining the index.

Because `AGENTS.md` requires review after code changes when a review skill exists, run the available code review flow after implementation. Ask the reviewer to focus on:

- command-line parsing without shell injection,
- token and artifact exclusion,
- no empty execution commits,
- failure workspace preservation,
- docs matching runtime behavior.

If review identifies valid issues, fix them and rerun the focused tests plus the full gate above.

## Test Plan

New tests to write:

- `internal/commandline/commandline_test.go` if a helper package is created:
  - empty command uses default,
  - command with args splits correctly,
  - quoted args remain grouped,
  - unterminated quotes fail.
- `internal/reviewer/reviewer_test.go`:
  - reviewer command accepts configured args and appends `--prompt`,
  - reviewer still filters `GITEA_TOKEN`.
- `internal/orchestrator/scheduler_test.go`:
  - Codex command accepts configured args and appends `--prompt`,
  - Codex still filters `GITEA_TOKEN`,
  - Codex failure includes bounded command output.
- `internal/execution/git_test.go`:
  - staging excludes `.env`, `.codex/**`, `.symphony/validation-verdict.json`, and `*.log`,
  - only excluded changes return `ErrNoChanges`.
- `internal/config/config_test.go`:
  - env expansion still works,
  - missing required Gitea config fails with clear field names.

Existing tests to model:

- `internal/orchestrator/scheduler_test.go:46-70` for fake command env capture.
- `internal/reviewer/reviewer_test.go:13-30` for reviewer env capture.
- `internal/tracker/gitea/client_test.go:61-98` for exact behavior assertions.

Full verification:

- `go test -count=1 ./...` -> all packages pass.
- `go vet ./...` -> exit 0.
- `go build -o "$env:TEMP\\symphony-plan-001.exe" ./cmd/symphony` -> exit 0 on PowerShell.
- `git diff --check` -> exit 0.

## Done Criteria

All must hold:

- [ ] Configured commands with arguments work for Codex and reviewer without invoking a shell.
- [ ] Codex and reviewer subprocess environments still exclude `GITEA_TOKEN`.
- [ ] `CommitAndPush` no longer stages common local agent/sensitive artifacts.
- [ ] `CommitAndPush` no longer creates empty commits after excluded files are removed from the index.
- [ ] Failed runs preserve the workspace path after workspace creation succeeds.
- [ ] Codex/reviewer failures expose bounded diagnostic output to logs/errors.
- [ ] Docs no longer describe unimplemented HTTP API, validator states, Verdict File gate, or `symphony-branch-ready` as current runtime behavior.
- [ ] `go test -count=1 ./...` exits 0.
- [ ] `go vet ./...` exits 0.
- [ ] `go build -o "$env:TEMP\\symphony-plan-001.exe" ./cmd/symphony` exits 0 on PowerShell, or the POSIX equivalent exits 0.
- [ ] `git diff --check` exits 0.
- [ ] `plans/README.md` status row for Plan 001 is updated when implementation is complete.

## STOP Conditions

Stop and report back without improvising if:

- The code at any cited "Current state" location no longer matches the excerpts after the drift check.
- Supporting command strings with arguments appears to require shell execution such as `sh -c` or `cmd /c`; that would change the security model.
- Excluding local artifacts would require parsing `.gitignore` or adding a glob dependency; keep this plan to a fixed default exclude set unless the operator expands scope.
- You discover that the operator wants the old External Validator / Verdict File / Publish Gate behavior restored instead of the simplified MVP reviewer gate.
- Preserving failed workspaces creates a path outside `workspace.root` or requires changing workspace ownership/cleanup semantics beyond success-vs-failure cleanup.
- Any test failure remains after two focused fix attempts.
- A fix requires touching files outside the in-scope list.

## Maintenance Notes

- The fixed default commit exclude list is intentionally small. If future users need project-specific exclusions, add a documented config field in a separate plan.
- The command-line parser is not a shell. Future docs must not promise shell features such as pipes, redirects, command substitution, or per-command env assignments.
- Failure workspace preservation means operators need a cleanup practice for old failed workspaces. Do not add automated cleanup in this plan.
- If a future milestone restores structured validators, it should be a separate design plan that updates `CONTEXT.md`, config schema, tests, and runtime together.

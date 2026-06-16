# Go Symphony Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go implementation of the Symphony core service that reads `WORKFLOW.md`, polls Linear, manages per-issue workspaces, runs Codex app-server sessions, and coordinates retries/reconciliation with observable logs.

**Architecture:** The implementation is a Go CLI daemon with small internal packages. The orchestrator is the single mutable state owner; tracker, workspace, prompt, and agent packages expose narrow interfaces so they can be tested without network or Codex dependencies.

**Tech Stack:** Go 1.22+, standard library, `gopkg.in/yaml.v3` for workflow front matter, optional `github.com/fsnotify/fsnotify` for file change watching, `go test` for unit tests.

---

## Current Environment Notes

- Current project root: `/root/work/symfony`.
- Repository was initialized during design approval.
- Go is not available on the current PATH at plan time; implementation must install or provide Go before running tests.
- Reference source is available at `/root/.opensrc/repos/github.com/openai/symphony/main`.
- Design document: `docs/plans/2026-05-23-go-symphony-design.md`.

## File Structure

- Create: `go.mod` — module definition and dependencies.
- Create: `go.sum` — dependency checksums after `go mod tidy`.
- Create: `.gitignore` — generated binary, coverage, temp workspace exclusions.
- Create: `cmd/symphony/main.go` — CLI entry point and signal handling.
- Create: `internal/domain/domain.go` — Issue, WorkflowDefinition, EffectiveConfig, Workspace, RunAttempt, LiveSession, RetryEntry, Snapshot types.
- Create: `internal/workflow/loader.go` — workflow path selection and YAML front matter parser.
- Create: `internal/workflow/loader_test.go` — loader tests.
- Create: `internal/config/resolver.go` — defaults, environment resolution, path expansion, validation.
- Create: `internal/config/resolver_test.go` — config tests.
- Create: `internal/prompt/render.go` — strict template rendering.
- Create: `internal/prompt/render_test.go` — prompt tests.
- Create: `internal/workspace/manager.go` — workspace path safety and hook execution.
- Create: `internal/workspace/manager_test.go` — workspace tests.
- Create: `internal/tracker/tracker.go` — tracker interface.
- Create: `internal/tracker/linear/client.go` — Linear GraphQL implementation.
- Create: `internal/tracker/linear/client_test.go` — query, pagination, normalization, error tests.
- Create: `internal/agent/runner.go` — Codex app-server subprocess runner and event parser.
- Create: `internal/agent/runner_test.go` — cwd, timeout, event, failure tests.
- Create: `internal/orchestrator/orchestrator.go` — state machine, tick, dispatch, retry, reconciliation.
- Create: `internal/orchestrator/orchestrator_test.go` — scheduler tests.
- Create: `internal/logging/logger.go` — structured logger with secret-safe fields.
- Create: `internal/logging/logger_test.go` — token redaction and context tests.
- Create: `internal/httpapi/server.go` — minimal optional state/refresh API.
- Create: `internal/httpapi/server_test.go` — state, refresh, issue detail, error response tests.
- Create: `WORKFLOW.example.md` — runnable example workflow for local validation.
- Create: `README.md` — operator-facing usage and safety posture.

---

### Task 1: Toolchain and Module Bootstrap

**Files:**
- Create: `.gitignore`
- Create: `go.mod`

- [ ] **Step 1: Verify Go is available**

Run:

```bash
go version
```

Expected: command prints Go 1.22 or newer. If it prints `command not found`, install Go 1.22+ and re-run the command before editing project files.

- [ ] **Step 2: Create module files**

Create `.gitignore`:

```gitignore
/bin/
/coverage.out
/tmp/
/symphony
*.test
```

Create `go.mod`:

```go
module github.com/local/symphony

go 1.22

require gopkg.in/yaml.v3 v3.0.1
```

- [ ] **Step 3: Run module resolution**

Run:

```bash
go mod tidy
```

Expected: command exits 0 and creates or updates `go.sum`.

- [ ] **Step 4: Run empty test suite**

Run:

```bash
go test ./...
```

Expected: exits 0 with no packages or with packages reporting no tests.

- [ ] **Step 5: Commit bootstrap**

```bash
git add .gitignore go.mod go.sum
git commit -m "chore: 初始化 Go 项目" -m "需求描述：为 Symphony Go 实现准备模块和基础忽略规则。" -m "实现思路：使用 Go module 管理依赖，先引入 YAML 解析能力，保持项目根目录干净。"
```

---

### Task 2: Domain Model

**Files:**
- Create: `internal/domain/domain.go`

- [ ] **Step 1: Write domain model file**

Create `internal/domain/domain.go`:

```go
package domain

import "time"

type BlockerRef struct {
	ID         string
	Identifier string
	State      string
}

type Issue struct {
	ID          string
	Identifier  string
	Title       string
	Description *string
	Priority    *int
	State       string
	BranchName  *string
	URL         *string
	Labels      []string
	BlockedBy   []BlockerRef
	CreatedAt   *time.Time
	UpdatedAt   *time.Time
}

type WorkflowDefinition struct {
	Path           string
	Config         map[string]any
	PromptTemplate string
	LoadedAt       time.Time
}

type HookConfig struct {
	AfterCreate  string
	BeforeRun    string
	AfterRun     string
	BeforeRemove string
	Timeout      time.Duration
}

type TrackerConfig struct {
	Kind           string
	Endpoint       string
	APIKey         string
	ProjectSlug    string
	ActiveStates   []string
	TerminalStates []string
}

type WorkspaceConfig struct {
	Root string
}

type AgentConfig struct {
	MaxConcurrentAgents        int
	MaxTurns                   int
	MaxRetryBackoff            time.Duration
	MaxConcurrentAgentsByState map[string]int
}

type CodexConfig struct {
	Command           string
	ApprovalPolicy    string
	ThreadSandbox     string
	TurnSandboxPolicy string
	TurnTimeout       time.Duration
	ReadTimeout       time.Duration
	StallTimeout      time.Duration
}

type EffectiveConfig struct {
	WorkflowPath string
	Tracker      TrackerConfig
	Polling      time.Duration
	Workspace    WorkspaceConfig
	Hooks        HookConfig
	Agent        AgentConfig
	Codex        CodexConfig
}

type Workspace struct {
	Path         string
	WorkspaceKey string
	CreatedNow   bool
}

type LiveSession struct {
	SessionID                 string
	ThreadID                  string
	TurnID                    string
	CodexAppServerPID         string
	LastCodexEvent            string
	LastCodexTimestamp        *time.Time
	LastCodexMessage          string
	CodexInputTokens          int64
	CodexOutputTokens         int64
	CodexTotalTokens          int64
	LastReportedInputTokens   int64
	LastReportedOutputTokens  int64
	LastReportedTotalTokens   int64
	TurnCount                 int
}

type RetryEntry struct {
	IssueID    string
	Identifier string
	Attempt    int
	DueAt      time.Time
	Error      string
}

type RunningEntry struct {
	Issue       Issue
	Session     LiveSession
	RetryAttempt int
	StartedAt   time.Time
	Cancel      func()
}

type CodexTotals struct {
	InputTokens    int64
	OutputTokens   int64
	TotalTokens    int64
	SecondsRunning float64
}

type Snapshot struct {
	GeneratedAt  time.Time
	Running      []RunningEntry
	Retrying     []RetryEntry
	CodexTotals  CodexTotals
	RateLimits   any
}
```

- [ ] **Step 2: Run package test discovery**

Run:

```bash
go test ./internal/domain
```

Expected: exits 0.

- [ ] **Step 3: Commit domain model**

```bash
git add internal/domain/domain.go
git commit -m "feat: 定义 Symphony 核心领域模型" -m "需求描述：为 workflow、tracker、workspace、agent 和 orchestrator 提供共享数据结构。" -m "实现思路：按规格定义 Issue、配置、运行会话、重试队列和快照模型，保持字段可测试且不绑定具体实现。"
```

---

### Task 3: Workflow Loader

**Files:**
- Create: `internal/workflow/loader.go`
- Create: `internal/workflow/loader_test.go`

- [ ] **Step 1: Write failing loader tests**

Create `internal/workflow/loader_test.go`:

```go
package workflow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadParsesFrontMatterAndPrompt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "WORKFLOW.md")
	body := "---\ntracker:\n  kind: linear\n---\n# Work\nUse issue {{ issue.identifier }}.\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	def, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if def.Path != path {
		t.Fatalf("Path = %q, want %q", def.Path, path)
	}
	tracker := def.Config["tracker"].(map[string]any)
	if tracker["kind"] != "linear" {
		t.Fatalf("tracker.kind = %#v, want linear", tracker["kind"])
	}
	if def.PromptTemplate != "# Work\nUse issue {{ issue.identifier }}." {
		t.Fatalf("PromptTemplate = %q", def.PromptTemplate)
	}
}

func TestLoadWithoutFrontMatterUsesWholeFileAsPrompt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "WORKFLOW.md")
	if err := os.WriteFile(path, []byte("  plain prompt  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	def, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(def.Config) != 0 {
		t.Fatalf("Config = %#v, want empty", def.Config)
	}
	if def.PromptTemplate != "plain prompt" {
		t.Fatalf("PromptTemplate = %q", def.PromptTemplate)
	}
}

func TestLoadMissingFileReturnsTypedError(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.md"))
	if err == nil || !IsMissingWorkflowFile(err) {
		t.Fatalf("err = %v, want missing workflow file", err)
	}
}

func TestLoadRejectsNonMapFrontMatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "WORKFLOW.md")
	if err := os.WriteFile(path, []byte("---\n- nope\n---\nprompt"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil || !IsFrontMatterNotMap(err) {
		t.Fatalf("err = %v, want front matter map error", err)
	}
}
```

- [ ] **Step 2: Run loader tests to verify failure**

Run:

```bash
go test ./internal/workflow
```

Expected: FAIL because `Load`, `IsMissingWorkflowFile`, and `IsFrontMatterNotMap` are undefined.

- [ ] **Step 3: Implement minimal loader**

Create `internal/workflow/loader.go`:

```go
package workflow

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/local/symphony/internal/domain"
	"gopkg.in/yaml.v3"
)

var (
	ErrMissingWorkflowFile  = errors.New("missing_workflow_file")
	ErrWorkflowParse        = errors.New("workflow_parse_error")
	ErrFrontMatterNotAMap   = errors.New("workflow_front_matter_not_a_map")
)

func IsMissingWorkflowFile(err error) bool { return errors.Is(err, ErrMissingWorkflowFile) }
func IsFrontMatterNotMap(err error) bool  { return errors.Is(err, ErrFrontMatterNotAMap) }

func Load(path string) (domain.WorkflowDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return domain.WorkflowDefinition{}, fmt.Errorf("%w: %s", ErrMissingWorkflowFile, path)
		}
		return domain.WorkflowDefinition{}, fmt.Errorf("%w: %v", ErrWorkflowParse, err)
	}

	text := string(data)
	config := map[string]any{}
	prompt := text
	if strings.HasPrefix(text, "---") {
		lines := strings.Split(text, "\n")
		end := -1
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				end = i
				break
			}
		}
		if end == -1 {
			return domain.WorkflowDefinition{}, fmt.Errorf("%w: unterminated front matter", ErrWorkflowParse)
		}
		raw := strings.Join(lines[1:end], "\n")
		if strings.TrimSpace(raw) != "" {
			var parsed any
			if err := yaml.Unmarshal([]byte(raw), &parsed); err != nil {
				return domain.WorkflowDefinition{}, fmt.Errorf("%w: %v", ErrWorkflowParse, err)
			}
			m, ok := parsed.(map[string]any)
			if !ok {
				return domain.WorkflowDefinition{}, ErrFrontMatterNotAMap
			}
			config = m
		}
		prompt = strings.Join(lines[end+1:], "\n")
	}

	return domain.WorkflowDefinition{
		Path:           path,
		Config:         config,
		PromptTemplate: strings.TrimSpace(prompt),
		LoadedAt:       time.Now().UTC(),
	}, nil
}
```

- [ ] **Step 4: Verify loader tests pass**

Run:

```bash
go test ./internal/workflow
```

Expected: PASS.

- [ ] **Step 5: Commit loader**

```bash
git add internal/workflow/loader.go internal/workflow/loader_test.go
git commit -m "feat: 加载 WORKFLOW 工作流文件" -m "需求描述：服务需要读取仓库内 WORKFLOW.md 并解析 front matter 与 prompt。" -m "实现思路：实现严格文件读取、YAML map 校验、prompt trim 和类型化错误。"
```

---

### Task 4: Config Resolver

**Files:**
- Create: `internal/config/resolver.go`
- Create: `internal/config/resolver_test.go`

- [ ] **Step 1: Write failing config tests**

Create tests covering:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/local/symphony/internal/domain"
)

func TestResolveAppliesDefaultsAndEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINEAR_TOKEN_FOR_TEST", "fixture-token")
	def := domain.WorkflowDefinition{
		Path: filepath.Join(dir, "WORKFLOW.md"),
		Config: map[string]any{
			"tracker": map[string]any{
				"kind": "linear", "api_key": "$LINEAR_TOKEN_FOR_TEST", "project_slug": "OPS",
			},
		},
	}

	cfg, err := Resolve(def)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if cfg.Tracker.APIKey != "fixture-token" {
		t.Fatalf("APIKey not resolved")
	}
	if cfg.Tracker.Endpoint != "https://api.linear.app/graphql" {
		t.Fatalf("Endpoint = %q", cfg.Tracker.Endpoint)
	}
	if cfg.Polling != 30*time.Second {
		t.Fatalf("Polling = %v", cfg.Polling)
	}
	if cfg.Codex.Command != "codex app-server" {
		t.Fatalf("Codex command = %q", cfg.Codex.Command)
	}
}

func TestResolveWorkspaceRootRelativeToWorkflow(t *testing.T) {
	dir := t.TempDir()
	def := domain.WorkflowDefinition{
		Path: filepath.Join(dir, "WORKFLOW.md"),
		Config: map[string]any{
			"tracker": map[string]any{"kind": "linear", "api_key": "x", "project_slug": "OPS"},
			"workspace": map[string]any{"root": "../workspaces"},
		},
	}
	cfg, err := Resolve(def)
	if err != nil { t.Fatal(err) }
	want, _ := filepath.Abs(filepath.Join(dir, "../workspaces"))
	if cfg.Workspace.Root != want { t.Fatalf("root = %q, want %q", cfg.Workspace.Root, want) }
}

func TestValidateDispatchRejectsMissingTrackerKey(t *testing.T) {
	cfg := domain.EffectiveConfig{Tracker: domain.TrackerConfig{Kind: "linear", ProjectSlug: "OPS"}, Codex: domain.CodexConfig{Command: "codex app-server"}}
	if err := ValidateDispatch(cfg); err == nil { t.Fatalf("expected validation error") }
}

func TestResolveNormalizesPerStateLimits(t *testing.T) {
	dir := t.TempDir()
	def := domain.WorkflowDefinition{Path: filepath.Join(dir, "WORKFLOW.md"), Config: map[string]any{
		"tracker": map[string]any{"kind": "linear", "api_key": "x", "project_slug": "OPS"},
		"agent": map[string]any{"max_concurrent_agents_by_state": map[string]any{"In Progress": 2, "Bad": 0}},
	}}
	cfg, err := Resolve(def)
	if err != nil { t.Fatal(err) }
	if cfg.Agent.MaxConcurrentAgentsByState["in progress"] != 2 { t.Fatalf("missing normalized state limit") }
	if _, ok := cfg.Agent.MaxConcurrentAgentsByState["bad"]; ok { t.Fatalf("invalid limit was kept") }
}

func TestMain(m *testing.M) { os.Exit(m.Run()) }
```

- [ ] **Step 2: Run config tests to verify failure**

Run:

```bash
go test ./internal/config
```

Expected: FAIL because resolver functions are undefined.

- [ ] **Step 3: Implement resolver**

Implement `Resolve(def domain.WorkflowDefinition) (domain.EffectiveConfig, error)` and `ValidateDispatch(cfg domain.EffectiveConfig) error` in `internal/config/resolver.go`. The implementation must include concrete helpers for string, int, duration milliseconds, list of strings, env reference, home expansion, and relative workspace resolution. Use defaults from the design document and Symphony spec.

- [ ] **Step 4: Verify config tests pass**

Run:

```bash
go test ./internal/config
```

Expected: PASS.

- [ ] **Step 5: Commit resolver**

```bash
git add internal/config/resolver.go internal/config/resolver_test.go
git commit -m "feat: 解析并校验 Symphony 配置" -m "需求描述：调度前需要得到带默认值、环境变量和路径解析的强类型配置。" -m "实现思路：集中处理 workflow 配置到 EffectiveConfig 的转换，并提供 dispatch preflight 校验。"
```

---

### Task 5: Strict Prompt Renderer

**Files:**
- Create: `internal/prompt/render.go`
- Create: `internal/prompt/render_test.go`

- [ ] **Step 1: Write failing prompt tests**

Create tests for rendering `issue.identifier`, labels iteration, attempt value, empty prompt fallback, and unknown variable failure. Use Go `text/template` with `Option("missingkey=error")`.

- [ ] **Step 2: Run prompt tests to verify failure**

Run:

```bash
go test ./internal/prompt
```

Expected: FAIL because renderer is undefined.

- [ ] **Step 3: Implement renderer**

Create `Render(templateText string, issue domain.Issue, attempt *int) (string, error)` that supplies `issue` and `attempt`, trims output, and returns typed errors for parse/render failures. Use a minimal default prompt only when `templateText` is empty.

- [ ] **Step 4: Verify prompt tests pass**

Run:

```bash
go test ./internal/prompt
```

Expected: PASS.

- [ ] **Step 5: Commit renderer**

```bash
git add internal/prompt/render.go internal/prompt/render_test.go
git commit -m "feat: 严格渲染 issue prompt" -m "需求描述：每个 agent run 需要基于 issue 和 attempt 生成可靠 prompt。" -m "实现思路：使用 strict template，未知变量直接失败，空模板使用最小默认提示。"
```

---

### Task 6: Workspace Manager and Hooks

**Files:**
- Create: `internal/workspace/manager.go`
- Create: `internal/workspace/manager_test.go`

- [ ] **Step 1: Write failing workspace tests**

Tests must cover identifier sanitization, directory creation, reuse, non-directory collision failure, root containment, after_create only on creation, before_run failure, after_run ignored failure, before_remove ignored failure, and hook timeout.

- [ ] **Step 2: Run workspace tests to verify failure**

Run:

```bash
go test ./internal/workspace
```

Expected: FAIL because manager is undefined.

- [ ] **Step 3: Implement workspace manager**

Create a `Manager` with:

```go
type Manager struct { Config domain.EffectiveConfig }
func (m Manager) Create(ctx context.Context, identifier string) (domain.Workspace, error)
func (m Manager) RunBeforeRun(ctx context.Context, ws domain.Workspace) error
func (m Manager) RunAfterRun(ctx context.Context, ws domain.Workspace)
func (m Manager) Remove(ctx context.Context, identifier string) error
func SanitizeIdentifier(identifier string) string
func EnsureInsideRoot(root, candidate string) error
```

Hook execution uses `sh -lc <script>` with `cmd.Dir = workspace.Path` and `context.WithTimeout` using `hooks.timeout_ms`.

- [ ] **Step 4: Verify workspace tests pass**

Run:

```bash
go test ./internal/workspace
```

Expected: PASS.

- [ ] **Step 5: Commit workspace manager**

```bash
git add internal/workspace/manager.go internal/workspace/manager_test.go
git commit -m "feat: 管理安全隔离工作区" -m "需求描述：每个 issue 必须在独立且受路径边界保护的工作区中执行。" -m "实现思路：实现 identifier 清洗、root containment 校验、目录复用和 hook 生命周期。"
```

---

### Task 7: Linear Tracker Client

**Files:**
- Create: `internal/tracker/tracker.go`
- Create: `internal/tracker/linear/client.go`
- Create: `internal/tracker/linear/client_test.go`

- [ ] **Step 1: Define tracker interface and failing tests**

Create tracker interface:

```go
package tracker

import (
	"context"
	"github.com/local/symphony/internal/domain"
)

type Client interface {
	FetchCandidateIssues(ctx context.Context) ([]domain.Issue, error)
	FetchIssuesByStates(ctx context.Context, states []string) ([]domain.Issue, error)
	FetchIssueStatesByIDs(ctx context.Context, ids []string) ([]domain.Issue, error)
}
```

Tests use `httptest.Server` to assert GraphQL body contains project `slugId`, active states, pagination variables, and `[ID!]` for state refresh.

- [ ] **Step 2: Run tracker tests to verify failure**

Run:

```bash
go test ./internal/tracker/... 
```

Expected: FAIL because Linear client is undefined.

- [ ] **Step 3: Implement Linear client**

Implement `linear.New(cfg domain.TrackerConfig, httpClient *http.Client) *Client`. Normalize labels to lowercase, blockers from inverse relations of type `blocks`, integer priority, ISO timestamps, and GraphQL errors as explicit errors.

- [ ] **Step 4: Verify tracker tests pass**

Run:

```bash
go test ./internal/tracker/...
```

Expected: PASS.

- [ ] **Step 5: Commit tracker**

```bash
git add internal/tracker/tracker.go internal/tracker/linear/client.go internal/tracker/linear/client_test.go
git commit -m "feat: 接入 Linear issue 读取" -m "需求描述：调度器需要从 Linear 获取候选任务、终态任务和运行中任务状态。" -m "实现思路：封装 GraphQL 查询、分页、错误映射和归一化逻辑，隐藏 tracker 细节。"
```

---

### Task 8: Orchestrator State Machine

**Files:**
- Create: `internal/orchestrator/orchestrator.go`
- Create: `internal/orchestrator/orchestrator_test.go`

- [ ] **Step 1: Write failing orchestrator tests**

Tests must cover sort order, missing required issue fields, active state filtering, terminal filtering, running/claimed exclusion, global limit, per-state limit, blocker rule for issue state `Todo`, normal exit continuation retry, abnormal exit exponential retry, cap by `max_retry_backoff_ms`, no running reconciliation no-op, active refresh update, non-active stop without cleanup, terminal stop with cleanup, stall detection disabled when timeout is non-positive.

- [ ] **Step 2: Run orchestrator tests to verify failure**

Run:

```bash
go test ./internal/orchestrator
```

Expected: FAIL because orchestrator is undefined.

- [ ] **Step 3: Implement orchestrator core**

Implement pure functions first: `SortForDispatch`, `ShouldDispatch`, `AvailableSlots`, `ScheduleRetry`, `RetryDelay`, `ApplyWorkerExit`, `ReconcileSnapshot`. Then wire a runtime `Orchestrator` type around these functions with injected tracker and worker launcher.

- [ ] **Step 4: Verify orchestrator tests pass**

Run:

```bash
go test ./internal/orchestrator
```

Expected: PASS.

- [ ] **Step 5: Commit orchestrator**

```bash
git add internal/orchestrator/orchestrator.go internal/orchestrator/orchestrator_test.go
git commit -m "feat: 实现调度状态机" -m "需求描述：服务需要单一权威状态处理 dispatch、retry、reconcile 和 stall。" -m "实现思路：先实现可单测的纯状态转换，再封装运行时 orchestrator。"
```

---

### Task 9: Agent Runner

**Files:**
- Create: `internal/agent/runner.go`
- Create: `internal/agent/runner_test.go`

- [ ] **Step 1: Write failing agent tests**

Tests must use small shell scripts instead of real Codex. Cover `cmd.Dir` equals workspace path, out-of-root workspace rejection, `bash -lc` invocation, read timeout, turn timeout, subprocess exit error, JSON line event parsing for session identity, token totals, rate-limit payload, and user-input-required failure.

- [ ] **Step 2: Run agent tests to verify failure**

Run:

```bash
go test ./internal/agent
```

Expected: FAIL because runner is undefined.

- [ ] **Step 3: Implement agent runner**

Create:

```go
type Event struct {
	IssueID string
	Event string
	SessionID string
	At time.Time
	InputTokens int64
	OutputTokens int64
	TotalTokens int64
	RateLimits any
	Message string
}

type Runner struct { Config domain.EffectiveConfig }
func (r Runner) Run(ctx context.Context, issue domain.Issue, ws domain.Workspace, prompt string, attempt *int, onEvent func(Event)) error
```

Use `exec.CommandContext(ctx, "bash", "-lc", cfg.Codex.Command)`, assign `cmd.Dir = ws.Path`, read stdout as JSON lines, read stderr separately, map failures to explicit errors, and never wait indefinitely for input.

- [ ] **Step 4: Verify agent tests pass**

Run:

```bash
go test ./internal/agent
```

Expected: PASS.

- [ ] **Step 5: Commit agent runner**

```bash
git add internal/agent/runner.go internal/agent/runner_test.go
git commit -m "feat: 运行 Codex app-server 子进程" -m "需求描述：每个 issue run 需要在安全工作区中启动并管理 Codex 会话。" -m "实现思路：通过 context、cwd 校验、stdout JSON line 解析和超时控制封装 agent runner。"
```

---

### Task 10: Logging and Snapshot API

**Files:**
- Create: `internal/logging/logger.go`
- Create: `internal/logging/logger_test.go`
- Create: `internal/httpapi/server.go`
- Create: `internal/httpapi/server_test.go`

- [ ] **Step 1: Write failing logging/API tests**

Logging tests must assert issue logs include `issue_id` and `issue_identifier`, session logs include `session_id`, and token values are not printed when field name indicates a secret. HTTP API tests must assert `/api/v1/state`, `/api/v1/refresh`, issue detail success, unknown issue 404 envelope, and unsupported method 405.

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/logging ./internal/httpapi
```

Expected: FAIL because packages are undefined.

- [ ] **Step 3: Implement logging and API**

Implement JSON logger using `encoding/json` to stdout/stderr. Implement HTTP server with `net/http`; it receives a snapshot provider and refresh trigger interface from orchestrator. Bind loopback by default when enabled.

- [ ] **Step 4: Verify tests pass**

Run:

```bash
go test ./internal/logging ./internal/httpapi
```

Expected: PASS.

- [ ] **Step 5: Commit observability**

```bash
git add internal/logging/logger.go internal/logging/logger_test.go internal/httpapi/server.go internal/httpapi/server_test.go
git commit -m "feat: 提供结构化日志和状态接口" -m "需求描述：操作人员需要看到调度、失败、重试和运行状态。" -m "实现思路：输出结构化日志，并提供最小状态/刷新 HTTP API 作为可观测入口。"
```

---

### Task 11: CLI Host Lifecycle

**Files:**
- Create: `cmd/symphony/main.go`
- Create: `WORKFLOW.example.md`
- Create: `README.md`

- [ ] **Step 1: Write CLI behavior tests or script checks**

If the CLI remains thin, use `go test ./cmd/symphony` for path parsing helpers and shell checks for host behavior. Cover explicit workflow path, default `./WORKFLOW.md`, missing explicit file, startup validation failure, normal signal shutdown.

- [ ] **Step 2: Run CLI tests to verify failure**

Run:

```bash
go test ./cmd/symphony
```

Expected: FAIL because CLI is undefined.

- [ ] **Step 3: Implement CLI**

`main.go` parses positional workflow path and optional `--port`. It loads workflow, resolves config, validates startup, creates tracker/workspace/prompt/agent/orchestrator dependencies, runs startup terminal cleanup, then handles SIGINT/SIGTERM with context cancellation.

- [ ] **Step 4: Add example workflow**

Create `WORKFLOW.example.md` with safe sample config using `$LINEAR_API_KEY`, a local workspace root under `/tmp`, and a prompt that instructs the agent to work from the issue title, description, labels, and blockers without exposing internal service state to end users.

- [ ] **Step 5: Add README**

Document install/build/run commands, config fields, safety posture, secret handling, and verification commands. Mention that tracker writes are expected to be performed by the agent workflow rather than orchestrator business logic.

- [ ] **Step 6: Verify CLI build**

Run:

```bash
go test ./...
go build ./cmd/symphony
```

Expected: both commands exit 0.

- [ ] **Step 7: Commit CLI**

```bash
git add cmd/symphony/main.go WORKFLOW.example.md README.md
git commit -m "feat: 提供 Symphony CLI 入口" -m "需求描述：服务需要可启动的主程序和面向操作人员的使用说明。" -m "实现思路：组合已测试的内部组件，提供 workflow 路径、可选端口和信号退出能力。"
```

---

### Task 12: Full Verification and Conformance Review

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Run complete automated checks**

Run:

```bash
go test ./...
go build ./cmd/symphony
```

Expected: both commands exit 0.

- [ ] **Step 2: Run targeted behavior checks**

Run with a temporary workflow that uses a fake local codex command and a test Linear server where applicable. Confirm logs show validation failures, dispatch attempts, retries, and graceful shutdown without printing secrets.

- [ ] **Step 3: Review spec checklist**

Compare implemented tests against these required groups: workflow/config, workspace safety, tracker reads, orchestrator dispatch/retry/reconcile, agent subprocess handling, logs, CLI lifecycle. Add any missing test before claiming completion.

- [ ] **Step 4: Commit final docs update**

```bash
git add README.md
git commit -m "docs: 补充 Symphony 验证说明" -m "需求描述：交付前需要明确如何验证核心能力与运行安全。" -m "实现思路：记录自动化检查、配置示例、日志检查和 Core Conformance 对照方法。"
```

---

## Self-Review

### Spec Coverage

- Workflow path selection, front matter parsing, typed errors: Task 3.
- Typed config, defaults, env indirection, path resolution, validation: Task 4.
- Strict prompt rendering: Task 5.
- Workspace layout, hooks, safety invariants: Task 6.
- Linear candidate, terminal, state refresh reads: Task 7.
- Dispatch, sorting, blockers, concurrency, retry, reconciliation, stall: Task 8.
- Codex subprocess launch, cwd safety, timeouts, event extraction: Task 9.
- Structured logs and baseline snapshot API: Task 10.
- CLI lifecycle and operator docs: Task 11.
- End-to-end verification: Task 12.

### Intentional Extension Boundaries

- SSH worker extension is outside the first implementation pass.
- Full browser dashboard is outside the first implementation pass; JSON API is included.
- Persistent retry/session storage is outside the first implementation pass.
- Non-Linear tracker adapters are outside the first implementation pass.

### Placeholder Scan

This plan uses concrete file paths, commands, commit messages, interfaces, and test expectations. Each package has an explicit failing-test step and pass-verification step before commit.

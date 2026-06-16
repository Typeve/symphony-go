# Gitea Completion Marker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prevent duplicate dispatch of completed open Gitea issues by marking completion with a Gitea label/comment and skipping marked issues.

**Architecture:** Add a typed completion config, an optional tracker completion marker capability, a Gitea implementation that ensures label + issue comment, and runtime logic that marks completed issues without scheduling continuation retries. Runtime keeps local completed/claimed protection even when marker write fails, exposing the failure through diagnostics instead of rerunning Codex.

**Tech Stack:** Go 1.23, standard `net/http`, existing `domain`, `config`, `tracker/gitea`, and `orchestrator` packages.

---

## File Structure

- Modify `internal/domain/domain.go`: add `CompletionConfig` and `EffectiveConfig.Completion`.
- Modify `internal/config/resolver.go`: parse/default `completion.enabled`, `completion.label`, `completion.comment`; validate unsupported completion marker configuration.
- Modify `internal/config/resolver_test.go`: cover Gitea defaults, Linear default disabled, explicit completion values.
- Modify `internal/tracker/tracker.go`: add optional `CompletionMarker` interface.
- Modify `internal/tracker/gitea/client.go`: filter completion label and implement `MarkIssueCompleted`.
- Modify `internal/tracker/gitea/client_test.go`: cover filtering, marker API calls, and token-safe errors.
- Modify `internal/orchestrator/orchestrator.go`: normal worker exits complete without continuation retry; add diagnostic helper for completion marker failures if needed.
- Modify `internal/orchestrator/runtime.go`: call completion marker after successful worker run; record marker failures without rerunning Codex.
- Modify `internal/orchestrator/runtime_test.go`: cover no retry, no redispatch, and marker failure diagnostics.
- Modify `README.md`, `WORKFLOW.example.md`, `WORKFLOW.md`: document and enable completion marker behavior.

---

### Task 1: Completion Config

**Files:**
- Modify: `internal/domain/domain.go`
- Modify: `internal/config/resolver.go`
- Test: `internal/config/resolver_test.go`

- [ ] **Step 1: Write failing config tests**

Add tests to `internal/config/resolver_test.go`:

```go
func TestResolveGiteaCompletionDefaults(t *testing.T) {
	cfg, err := Resolve(validDefinition(filepath.Join(t.TempDir(), "WORKFLOW.md"), map[string]any{
		"tracker": map[string]any{
			"kind":         "gitea",
			"endpoint":     "http://gitea.local:3000",
			"api_key":      "token-value",
			"project_slug": "owner/repo",
		},
	}))
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if !cfg.Completion.Enabled {
		t.Fatal("Completion.Enabled = false, want true for gitea")
	}
	if cfg.Completion.Label != "symphony-completed" {
		t.Fatalf("Completion.Label = %q", cfg.Completion.Label)
	}
	if cfg.Completion.Comment == "" || strings.Contains(cfg.Completion.Comment, "TODO") {
		t.Fatalf("Completion.Comment = %q, want friendly default", cfg.Completion.Comment)
	}
}

func TestResolveLinearCompletionDefaultsDisabled(t *testing.T) {
	cfg, err := Resolve(validDefinition(filepath.Join(t.TempDir(), "WORKFLOW.md"), nil))
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if cfg.Completion.Enabled {
		t.Fatal("Completion.Enabled = true, want default false for linear")
	}
}

func TestResolveCompletionOverrides(t *testing.T) {
	cfg, err := Resolve(validDefinition(filepath.Join(t.TempDir(), "WORKFLOW.md"), map[string]any{
		"tracker": map[string]any{
			"kind":         "gitea",
			"endpoint":     "http://gitea.local:3000",
			"api_key":      "token-value",
			"project_slug": "owner/repo",
		},
		"completion": map[string]any{
			"enabled": false,
			"label":   "done-by-bot",
			"comment": "任务已完成处理。",
		},
	}))
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if cfg.Completion.Enabled {
		t.Fatal("Completion.Enabled = true, want explicit false")
	}
	if cfg.Completion.Label != "done-by-bot" || cfg.Completion.Comment != "任务已完成处理。" {
		t.Fatalf("Completion config = %#v", cfg.Completion)
	}
}
```

- [ ] **Step 2: Run failing config tests**

Run:

```bash
go test ./internal/config -run 'TestResolveGiteaCompletionDefaults|TestResolveLinearCompletionDefaultsDisabled|TestResolveCompletionOverrides'
```

Expected: FAIL because `EffectiveConfig.Completion` does not exist.

- [ ] **Step 3: Implement config**

Add to `internal/domain/domain.go`:

```go
type CompletionConfig struct {
	Enabled bool
	Label   string
	Comment string
}
```

Add `Completion CompletionConfig` to `EffectiveConfig`.

In `internal/config/resolver.go`, allow top-level `completion`; set defaults after tracker kind is known:

```go
const (
	defaultCompletionLabel   = "symphony-completed"
	defaultCompletionComment = "任务已完成处理，后续不会重复派发。"
)
```

Parse keys `enabled`, `label`, `comment` using `boolValue` and `stringValue`. If tracker kind is gitea and `completion.enabled` is absent, default enabled to true. Default label/comment when blank.

- [ ] **Step 4: Verify config tests pass**

Run:

```bash
go test ./internal/config -run 'TestResolveGiteaCompletionDefaults|TestResolveLinearCompletionDefaultsDisabled|TestResolveCompletionOverrides'
```

Expected: PASS.

---

### Task 2: Gitea Candidate Filtering and Marker API

**Files:**
- Modify: `internal/tracker/tracker.go`
- Modify: `internal/tracker/gitea/client.go`
- Test: `internal/tracker/gitea/client_test.go`

- [ ] **Step 1: Write failing Gitea tests**

Add to `internal/tracker/gitea/client_test.go`:

```go
func TestFetchCandidateIssuesSkipsCompletionLabel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode([]map[string]any{
			{"number": 7, "title": "Done", "state": "open", "labels": []map[string]any{{"name": "symphony-completed"}}},
			{"number": 8, "title": "Ready", "state": "open", "labels": []map[string]any{{"name": "backend"}}},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := New(domain.TrackerConfig{Kind: "gitea", Endpoint: server.URL, APIKey: "token-value", ProjectSlug: "owner/repo", ActiveStates: []string{"open"}}, server.Client())
	client.completion = domain.CompletionConfig{Enabled: true, Label: "symphony-completed", Comment: "done"}
	issues, err := client.FetchCandidateIssues(context.Background())
	if err != nil {
		t.Fatalf("FetchCandidateIssues returned error: %v", err)
	}
	if len(issues) != 1 || issues[0].ID != "8" {
		t.Fatalf("issues = %#v, want only unmarked issue 8", issues)
	}
}

func TestMarkIssueCompletedEnsuresLabelAddsLabelAndComments(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		if got := r.Header.Get("Authorization"); got != "token token-value" {
			t.Fatalf("Authorization = %q", got)
		}
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v1/repos/owner/repo/labels":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":1,"name":"symphony-completed"}`))
		case "POST /api/v1/repos/owner/repo/issues/7/labels":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode labels body: %v", err)
			}
			labels := body["labels"].([]any)
			if len(labels) != 1 || labels[0] != "symphony-completed" {
				t.Fatalf("labels body = %#v", body)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		case "POST /api/v1/repos/owner/repo/issues/7/comments":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode comment body: %v", err)
			}
			if body["body"] != "任务已完成处理。" {
				t.Fatalf("comment body = %#v", body)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":2}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := New(domain.TrackerConfig{Kind: "gitea", Endpoint: server.URL, APIKey: "token-value", ProjectSlug: "owner/repo"}, server.Client())
	client.completion = domain.CompletionConfig{Enabled: true, Label: "symphony-completed", Comment: "任务已完成处理。"}
	if err := client.MarkIssueCompleted(context.Background(), domain.Issue{ID: "7", Identifier: "owner/repo#7"}); err != nil {
		t.Fatalf("MarkIssueCompleted returned error: %v", err)
	}
	want := []string{
		"POST /api/v1/repos/owner/repo/labels",
		"POST /api/v1/repos/owner/repo/issues/7/labels",
		"POST /api/v1/repos/owner/repo/issues/7/comments",
	}
	if !reflect.DeepEqual(requests, want) {
		t.Fatalf("requests = %#v, want %#v", requests, want)
	}
}
```

- [ ] **Step 2: Run failing Gitea tests**

Run:

```bash
go test ./internal/tracker/gitea -run 'TestFetchCandidateIssuesSkipsCompletionLabel|TestMarkIssueCompletedEnsuresLabelAddsLabelAndComments'
```

Expected: FAIL because `completion` and `MarkIssueCompleted` are missing.

- [ ] **Step 3: Implement Gitea marker**

Add to `internal/tracker/tracker.go`:

```go
type CompletionMarker interface {
	MarkIssueCompleted(ctx context.Context, issue domain.Issue) error
}
```

In Gitea client add `completion domain.CompletionConfig`, `WithCompletion` or a constructor-set default, filter `normalizeIssues` output by label, and implement:

```go
func (c *Client) MarkIssueCompleted(ctx context.Context, issue domain.Issue) error
```

Use POST endpoints:

- `/api/v1/repos/{owner}/{repo}/labels`
- `/api/v1/repos/{owner}/{repo}/issues/{number}/labels`
- `/api/v1/repos/{owner}/{repo}/issues/{number}/comments`

Treat label already existing (`409` or `422`) as non-fatal. Other non-2xx responses return explicit status errors.

- [ ] **Step 4: Verify Gitea tests pass**

Run:

```bash
go test ./internal/tracker/gitea
```

Expected: PASS.

---

### Task 3: Runtime Completion Handling

**Files:**
- Modify: `internal/orchestrator/orchestrator.go`
- Modify: `internal/orchestrator/runtime.go`
- Test: `internal/orchestrator/orchestrator_test.go`
- Test: `internal/orchestrator/runtime_test.go`

- [ ] **Step 1: Write failing orchestrator tests**

Update `TestApplyWorkerExitUpdatesStateAndSchedulesRetry` or add a new test:

```go
func TestApplyWorkerExitNormalCompletionDoesNotScheduleContinuationRetry(t *testing.T) {
	cfg := testConfig()
	now := time.Date(2026, 5, 24, 20, 0, 0, 0, time.UTC)
	state := State{Running: map[string]domain.RunningEntry{"issue-1": {Issue: issue("issue-1", "OPS-1", "Todo")}}}

	next := ApplyWorkerExit(state, "issue-1", true, "", now, cfg)
	if _, ok := next.Completed["issue-1"]; !ok {
		t.Fatal("normal exit should record completed issue")
	}
	if len(next.RetryAttempts) != 0 {
		t.Fatalf("RetryAttempts = %#v, want no continuation retry", next.RetryAttempts)
	}
}
```

Add runtime tests:

```go
func TestRuntimeMarksSuccessfulGiteaIssueAndDoesNotRedispatchOpenIssue(t *testing.T) {
	// use fake tracker implementing MarkIssueCompleted, candidate remains open
	// dispatch once, wait for worker, tick again, assert mark called once and agent not called twice.
}

func TestRuntimeCompletionMarkerFailureRecordsDiagnosticWithoutRerunningCodex(t *testing.T) {
	// fake marker returns error, candidate remains open
	// assert diagnostics contain marker error and second tick does not start another agent run.
}
```

- [ ] **Step 2: Run failing orchestrator tests**

Run:

```bash
go test ./internal/orchestrator -run 'TestApplyWorkerExitNormalCompletionDoesNotScheduleContinuationRetry|TestRuntimeMarksSuccessfulGiteaIssueAndDoesNotRedispatchOpenIssue|TestRuntimeCompletionMarkerFailureRecordsDiagnosticWithoutRerunningCodex'
```

Expected: FAIL because normal exits still schedule retry and runtime does not call marker.

- [ ] **Step 3: Implement runtime behavior**

Change `ApplyWorkerExit` normal branch to record completed and not call `ScheduleRetry`.

Add runtime marker call after successful worker execution. `workerResult` should include `completionErr error`; `applyWorkerResult` should record diagnostics for completion marker failures while still treating the worker as completed locally.

- [ ] **Step 4: Verify orchestrator tests pass**

Run:

```bash
go test ./internal/orchestrator
```

Expected: PASS.

---

### Task 4: Wire Config and Documentation

**Files:**
- Modify: `cmd/symphony/main.go`
- Modify: `README.md`
- Modify: `WORKFLOW.example.md`
- Modify: `WORKFLOW.md`
- Test: `cmd/symphony/main_test.go`

- [ ] **Step 1: Write failing CLI validation test if needed**

Add a test that explicitly enabling completion for unsupported tracker fails validation if runtime cannot mark completion. If validation remains config-only, keep this in `internal/config` instead.

- [ ] **Step 2: Wire Gitea client completion config**

When constructing the Gitea tracker client, pass `effectiveConfig.Completion` into the Gitea client.

- [ ] **Step 3: Update docs and workflow**

Document:

```yaml
completion:
  enabled: true
  label: symphony-completed
  comment: 任务已完成处理，后续不会重复派发。
```

- [ ] **Step 4: Verify all tests and build**

Run:

```bash
go test -count=1 ./...
go build ./cmd/symphony
git diff --check
```

Expected: all commands exit 0.

---

## Self-Review

- Spec coverage: duplicate dispatch prevention, Gitea label/comment marker, candidate filtering, marker failure diagnostics, docs/config are covered.
- Placeholder scan: no TODO/TBD placeholders are used.
- Type consistency: `domain.CompletionConfig`, `tracker.CompletionMarker`, and `MarkIssueCompleted` names are consistent.

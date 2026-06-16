# Gitea Tracker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** Add tracker.kind = gitea so Symphony can poll Gitea issues from an owner/repo repository and feed them into the existing orchestration pipeline.

**Architecture:** Reuse the existing tracker.Client interface. Extend config validation to accept gitea, add internal/tracker/gitea as a REST client, and update the CLI factory to instantiate Linear or Gitea based on tracker.kind. Keep PR creation and issue comments in workflow/Codex instructions, not in the orchestrator.

**Tech Stack:** Go standard library net/http, encoding/json, net/url, httptest; existing domain/config/tracker/orchestrator packages.

---

## File Structure

- Modify: internal/domain/domain.go — add optional tracker repository helper fields only if needed; default plan keeps existing TrackerConfig fields.
- Modify: internal/config/resolver.go — allow gitea kind and validate endpoint/api_key/project_slug for both trackers.
- Modify: internal/config/resolver_test.go — add Gitea config tests.
- Create: internal/tracker/gitea/client.go — implement REST client and payload normalization.
- Create: internal/tracker/gitea/client_test.go — cover request construction, mapping, pagination, PR skipping, and errors.
- Modify: cmd/symphony/main.go — add tracker factory for linear/gitea.
- Modify: cmd/symphony/main_test.go — verify Gitea tracker startup wiring does not reject supported kind.
- Modify: README.md — document Gitea configuration.
- Modify: WORKFLOW.example.md — add commented Gitea example.

---

### Task 1: Config accepts and validates Gitea tracker

**Files:**
- Modify: internal/config/resolver_test.go
- Modify: internal/config/resolver.go

- [ ] **Step 1: Write failing config tests**

Add these tests to internal/config/resolver_test.go near existing tracker validation tests:

~~~go
func TestResolveAcceptsGiteaTracker(t *testing.T) {
	def := validDefinition(t)
	def.Config["tracker"] = map[string]any{
		"kind":            "gitea",
		"endpoint":        "http://gitea.local:3000",
		"api_key":         "token-value",
		"project_slug":    "owner/repo",
		"active_states":   []any{"open"},
		"terminal_states": []any{"closed"},
	}

	cfg, err := Resolve(def)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if cfg.Tracker.Kind != "gitea" || cfg.Tracker.Endpoint != "http://gitea.local:3000" || cfg.Tracker.ProjectSlug != "owner/repo" {
		t.Fatalf("tracker config = %#v, want gitea endpoint and owner/repo", cfg.Tracker)
	}
	if got := cfg.Tracker.ActiveStates; len(got) != 1 || got[0] != "open" {
		t.Fatalf("active states = %#v, want open", got)
	}
	if got := cfg.Tracker.TerminalStates; len(got) != 1 || got[0] != "closed" {
		t.Fatalf("terminal states = %#v, want closed", got)
	}
}

func TestValidateDispatchRequiresGiteaOwnerRepoSlug(t *testing.T) {
	cfg := validDispatchConfig()
	cfg.Tracker.Kind = "gitea"
	cfg.Tracker.Endpoint = "http://gitea.local:3000"
	cfg.Tracker.APIKey = "token-value"
	cfg.Tracker.ProjectSlug = "owner-only"

	err := ValidateDispatch(cfg)
	if err == nil || !strings.Contains(err.Error(), "tracker.project_slug") || !strings.Contains(err.Error(), "owner/repo") {
		t.Fatalf("ValidateDispatch error = %v, want owner/repo project_slug error", err)
	}
}

func TestValidateDispatchRequiresGiteaEndpoint(t *testing.T) {
	cfg := validDispatchConfig()
	cfg.Tracker.Kind = "gitea"
	cfg.Tracker.Endpoint = ""
	cfg.Tracker.APIKey = "token-value"
	cfg.Tracker.ProjectSlug = "owner/repo"

	err := ValidateDispatch(cfg)
	if err == nil || !strings.Contains(err.Error(), "tracker.endpoint") {
		t.Fatalf("ValidateDispatch error = %v, want endpoint error", err)
	}
}
~~~

If validDefinition or validDispatchConfig helpers differ, adapt to existing helper names but keep the assertions exactly about kind, endpoint, api_key and owner/repo.

- [ ] **Step 2: Run tests and verify red**

Run:

~~~bash
go test -count=1 ./internal/config
~~~

Expected: FAIL because ValidateDispatch rejects tracker.kind "gitea" or does not enforce the new Gitea constraints.

- [ ] **Step 3: Implement config support**

In internal/config/resolver.go:

1. Keep existing Linear default endpoint behavior only for kind linear.
2. Change ValidateDispatch kind validation to allow linear and gitea.
3. Require endpoint/api_key/project_slug for both trackers.
4. For gitea only, require project_slug to split into exactly two non-empty path segments.

Implementation sketch:

~~~go
func ValidateDispatch(cfg domain.EffectiveConfig) error {
	kind := strings.ToLower(strings.TrimSpace(cfg.Tracker.Kind))
	if kind == "" {
		return fmt.Errorf("tracker.kind is required")
	}
	if kind != "linear" && kind != "gitea" {
		return fmt.Errorf("tracker.kind %q is not supported", cfg.Tracker.Kind)
	}
	if strings.TrimSpace(cfg.Tracker.APIKey) == "" {
		return fmt.Errorf("tracker.api_key is required")
	}
	if strings.TrimSpace(cfg.Tracker.Endpoint) == "" {
		return fmt.Errorf("tracker.endpoint is required")
	}
	if strings.TrimSpace(cfg.Tracker.ProjectSlug) == "" {
		return fmt.Errorf("tracker.project_slug is required")
	}
	if kind == "gitea" && !validOwnerRepo(cfg.Tracker.ProjectSlug) {
		return fmt.Errorf("tracker.project_slug must be owner/repo for gitea")
	}
	// keep existing validation for states, concurrency, codex and workspace
}

func validOwnerRepo(value string) bool {
	parts := strings.Split(strings.TrimSpace(value), "/")
	return len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != ""
}
~~~

Do not remove existing validations after the tracker block.

- [ ] **Step 4: Run tests and verify green**

Run:

~~~bash
go test -count=1 ./internal/config
~~~

Expected: PASS.

- [ ] **Step 5: Commit**

~~~bash
git add internal/config/resolver.go internal/config/resolver_test.go
git commit -m "feat: 支持 Gitea tracker 配置" -m "需求描述：允许 tracker.kind=gitea 并校验 Gitea 必需配置。" -m "实现思路：复用 tracker.project_slug 表示 owner/repo，保留 Linear 默认端点逻辑并扩展启动校验。"
~~~

---

### Task 2: Gitea client lists candidate issues

**Files:**
- Create: internal/tracker/gitea/client_test.go
- Create: internal/tracker/gitea/client.go

- [ ] **Step 1: Write failing list/mapping tests**

Create internal/tracker/gitea/client_test.go with tests like:

~~~go
package gitea

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/local/symphony/internal/domain"
)

func TestFetchCandidateIssuesListsOpenIssuesAndSkipsPullRequests(t *testing.T) {
	created := "2026-05-24T01:02:03Z"
	updated := "2026-05-24T02:03:04Z"
	var gotPath, gotAuth, gotState, gotPage, gotLimit string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotState = r.URL.Query().Get("state")
		gotPage = r.URL.Query().Get("page")
		gotLimit = r.URL.Query().Get("limit")
		json.NewEncoder(w).Encode([]map[string]any{
			{
				"number": 7,
				"title": "Build NAS flow",
				"body": "Use Gitea issues",
				"state": "open",
				"html_url": "http://gitea.local/owner/repo/issues/7",
				"created_at": created,
				"updated_at": updated,
				"labels": []map[string]any{{"name": "Backend"}},
			},
			{
				"number": 8,
				"title": "PR should be ignored",
				"state": "open",
				"pull_request": map[string]any{"url": "http://example/pr"},
			},
		})
	}))
	defer server.Close()

	client := New(domain.TrackerConfig{Kind: "gitea", Endpoint: server.URL, APIKey: "token-value", ProjectSlug: "owner/repo", ActiveStates: []string{"open"}}, server.Client())
	issues, err := client.FetchCandidateIssues(context.Background())
	if err != nil {
		t.Fatalf("FetchCandidateIssues returned error: %v", err)
	}
	if gotPath != "/api/v1/repos/owner/repo/issues" || gotState != "open" || gotPage != "1" || gotLimit != "50" {
		t.Fatalf("request path/state/page/limit = %s/%s/%s/%s", gotPath, gotState, gotPage, gotLimit)
	}
	if gotAuth != "token token-value" {
		t.Fatalf("Authorization = %q, want token auth", gotAuth)
	}
	if len(issues) != 1 {
		t.Fatalf("issues = %#v, want one non-PR issue", issues)
	}
	issue := issues[0]
	if issue.ID != "7" || issue.Identifier != "owner/repo#7" || issue.Title != "Build NAS flow" || issue.State != "open" {
		t.Fatalf("issue = %#v, want mapped Gitea issue", issue)
	}
	if issue.Description == nil || *issue.Description != "Use Gitea issues" {
		t.Fatalf("description = %#v", issue.Description)
	}
	if len(issue.Labels) != 1 || issue.Labels[0] != "backend" {
		t.Fatalf("labels = %#v, want lower-case backend", issue.Labels)
	}
	if issue.URL == nil || !strings.Contains(*issue.URL, "/issues/7") {
		t.Fatalf("url = %#v, want html_url", issue.URL)
	}
	wantCreated, _ := time.Parse(time.RFC3339, created)
	if issue.CreatedAt == nil || !issue.CreatedAt.Equal(wantCreated) {
		t.Fatalf("created_at = %#v, want %v", issue.CreatedAt, wantCreated)
	}
}
~~~

- [ ] **Step 2: Run test and verify red**

Run:

~~~bash
go test -count=1 ./internal/tracker/gitea
~~~

Expected: FAIL because package/client does not exist or New is undefined.

- [ ] **Step 3: Implement minimal Gitea client**

Create internal/tracker/gitea/client.go:

~~~go
package gitea

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/local/symphony/internal/domain"
)

const defaultPageSize = 50

type Client struct {
	cfg        domain.TrackerConfig
	httpClient *http.Client
	owner      string
	repo       string
}

func New(cfg domain.TrackerConfig, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	owner, repo := splitOwnerRepo(cfg.ProjectSlug)
	return &Client{cfg: cfg, httpClient: httpClient, owner: owner, repo: repo}
}

func (c *Client) FetchCandidateIssues(ctx context.Context) ([]domain.Issue, error) {
	return c.FetchIssuesByStates(ctx, c.cfg.ActiveStates)
}

func (c *Client) FetchIssuesByStates(ctx context.Context, states []string) ([]domain.Issue, error) {
	if len(states) == 0 {
		return []domain.Issue{}, nil
	}
	var out []domain.Issue
	seen := map[string]struct{}{}
	for _, state := range normalizeStates(states) {
		issues, err := c.fetchIssuesByState(ctx, state)
		if err != nil {
			return nil, err
		}
		for _, issue := range issues {
			if _, ok := seen[issue.ID]; ok {
				continue
			}
			seen[issue.ID] = struct{}{}
			out = append(out, issue)
		}
	}
	return out, nil
}

func (c *Client) fetchIssuesByState(ctx context.Context, state string) ([]domain.Issue, error) {
	page := 1
	var out []domain.Issue
	for {
		var raw []giteaIssue
		if err := c.getJSON(ctx, c.issuesURL(state, page), &raw); err != nil {
			return nil, err
		}
		mapped, err := c.normalizeIssues(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, mapped...)
		if len(raw) < defaultPageSize {
			return out, nil
		}
		page++
	}
}

func (c *Client) issuesURL(state string, page int) string {
	base := strings.TrimRight(c.cfg.Endpoint, "/")
	u, _ := url.Parse(base)
	u.Path = path.Join(u.Path, "/api/v1/repos", c.owner, c.repo, "issues")
	q := u.Query()
	q.Set("state", state)
	q.Set("page", strconv.Itoa(page))
	q.Set("limit", strconv.Itoa(defaultPageSize))
	u.RawQuery = q.Encode()
	return u.String()
}

func (c *Client) getJSON(ctx context.Context, url string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("gitea request error: create request: %w", err)
	}
	req.Header.Set("Authorization", "token "+c.cfg.APIKey)
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gitea request error: send request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("gitea status error: unexpected HTTP status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("gitea payload error: decode JSON: %w", err)
	}
	return nil
}

type giteaIssue struct {
	Number      int           `json:"number"`
	Title       string        `json:"title"`
	Body        *string       `json:"body"`
	State       string        `json:"state"`
	HTMLURL     *string       `json:"html_url"`
	CreatedAt   *string       `json:"created_at"`
	UpdatedAt   *string       `json:"updated_at"`
	Labels      []giteaLabel  `json:"labels"`
	PullRequest any           `json:"pull_request"`
}

type giteaLabel struct { Name string `json:"name"` }

func (c *Client) normalizeIssues(raw []giteaIssue) ([]domain.Issue, error) {
	out := make([]domain.Issue, 0, len(raw))
	for index, issue := range raw {
		if issue.PullRequest != nil {
			continue
		}
		mapped, err := c.normalizeIssue(issue)
		if err != nil {
			return nil, fmt.Errorf("normalize issue at index %d: %w", index, err)
		}
		out = append(out, mapped)
	}
	return out, nil
}

func (c *Client) normalizeIssue(raw giteaIssue) (domain.Issue, error) {
	if raw.Number <= 0 { return domain.Issue{}, fmt.Errorf("missing number") }
	if strings.TrimSpace(raw.Title) == "" { return domain.Issue{}, fmt.Errorf("issue %d missing title", raw.Number) }
	if strings.TrimSpace(raw.State) == "" { return domain.Issue{}, fmt.Errorf("issue %d missing state", raw.Number) }
	created, err := parseOptionalTime(raw.Number, "created_at", raw.CreatedAt)
	if err != nil { return domain.Issue{}, err }
	updated, err := parseOptionalTime(raw.Number, "updated_at", raw.UpdatedAt)
	if err != nil { return domain.Issue{}, err }
	id := strconv.Itoa(raw.Number)
	return domain.Issue{ID: id, Identifier: c.owner + "/" + c.repo + "#" + id, Title: raw.Title, Description: raw.Body, State: raw.State, URL: raw.HTMLURL, Labels: normalizeLabels(raw.Labels), CreatedAt: created, UpdatedAt: updated}, nil
}

func parseOptionalTime(issueNumber int, field string, value *string) (*time.Time, error) {
	if value == nil || strings.TrimSpace(*value) == "" { return nil, nil }
	parsed, err := time.Parse(time.RFC3339, *value)
	if err != nil { return nil, fmt.Errorf("issue %d invalid %s: %w", issueNumber, field, err) }
	return &parsed, nil
}

func normalizeLabels(labels []giteaLabel) []string {
	if len(labels) == 0 { return nil }
	out := make([]string, 0, len(labels))
	for _, label := range labels {
		name := strings.ToLower(strings.TrimSpace(label.Name))
		if name != "" { out = append(out, name) }
	}
	return out
}

func normalizeStates(states []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, state := range states {
		normalized := strings.ToLower(strings.TrimSpace(state))
		if normalized == "" { continue }
		if normalized != "open" && normalized != "closed" && normalized != "all" { continue }
		if _, ok := seen[normalized]; ok { continue }
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func splitOwnerRepo(slug string) (string, string) {
	parts := strings.Split(strings.TrimSpace(slug), "/")
	if len(parts) != 2 { return "", "" }
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}
~~~

- [ ] **Step 4: Run tests and verify green**

Run:

~~~bash
go test -count=1 ./internal/tracker/gitea
~~~

Expected: PASS.

- [ ] **Step 5: Commit**

~~~bash
git add internal/tracker/gitea/client.go internal/tracker/gitea/client_test.go
git commit -m "feat: 读取 Gitea 候选任务" -m "需求描述：Gitea tracker 需要从仓库 issues 拉取可调度任务。" -m "实现思路：通过 REST issues API 分页读取 open/closed/all 状态，跳过 PR 并映射为领域 Issue。"
~~~

---

### Task 3: Gitea client refreshes issue states and handles errors

**Files:**
- Modify: internal/tracker/gitea/client_test.go
- Modify: internal/tracker/gitea/client.go

- [ ] **Step 1: Write failing refresh and error tests**

Add tests:

~~~go
func TestFetchIssueStatesByIDsFetchesEachIssueNumber(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		json.NewEncoder(w).Encode(map[string]any{"number": 42, "title": "Refresh me", "state": "closed"})
	}))
	defer server.Close()

	client := New(domain.TrackerConfig{Kind: "gitea", Endpoint: server.URL, APIKey: "token-value", ProjectSlug: "owner/repo"}, server.Client())
	issues, err := client.FetchIssueStatesByIDs(context.Background(), []string{"42"})
	if err != nil { t.Fatalf("FetchIssueStatesByIDs returned error: %v", err) }
	if len(paths) != 1 || paths[0] != "/api/v1/repos/owner/repo/issues/42" {
		t.Fatalf("paths = %#v, want single issue path", paths)
	}
	if len(issues) != 1 || issues[0].ID != "42" || issues[0].State != "closed" {
		t.Fatalf("issues = %#v, want refreshed closed issue", issues)
	}
}

func TestFetchIssueStatesByIDsRejectsNonNumericID(t *testing.T) {
	client := New(domain.TrackerConfig{Kind: "gitea", Endpoint: "http://gitea.local", APIKey: "token-value", ProjectSlug: "owner/repo"}, nil)
	errIssues, err := client.FetchIssueStatesByIDs(context.Background(), []string{"owner/repo#42"})
	if err == nil || !strings.Contains(err.Error(), "issue id") {
		t.Fatalf("issues=%#v err=%v, want issue id error", errIssues, err)
	}
}

func TestFetchCandidateIssuesReturnsStatusAndPayloadErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer server.Close()

	client := New(domain.TrackerConfig{Kind: "gitea", Endpoint: server.URL, APIKey: "token-value", ProjectSlug: "owner/repo", ActiveStates: []string{"open"}}, server.Client())
	_, err := client.FetchCandidateIssues(context.Background())
	if err == nil || !strings.Contains(err.Error(), "status") || strings.Contains(err.Error(), "token-value") {
		t.Fatalf("error = %v, want status error without token", err)
	}
}
~~~

- [ ] **Step 2: Run tests and verify red**

Run:

~~~bash
go test -count=1 ./internal/tracker/gitea
~~~

Expected: FAIL because FetchIssueStatesByIDs is not implemented or error behavior is incomplete.

- [ ] **Step 3: Implement refresh**

Add to client.go:

~~~go
func (c *Client) FetchIssueStatesByIDs(ctx context.Context, ids []string) ([]domain.Issue, error) {
	if len(ids) == 0 { return []domain.Issue{}, nil }
	out := make([]domain.Issue, 0, len(ids))
	for _, id := range ids {
		number, err := strconv.Atoi(strings.TrimSpace(id))
		if err != nil || number <= 0 {
			return nil, fmt.Errorf("gitea payload error: issue id %q must be a positive issue number", id)
		}
		var raw giteaIssue
		if err := c.getJSON(ctx, c.issueURL(number), &raw); err != nil {
			return nil, err
		}
		if raw.PullRequest != nil { continue }
		mapped, err := c.normalizeIssue(raw)
		if err != nil { return nil, err }
		out = append(out, mapped)
	}
	return out, nil
}

func (c *Client) issueURL(number int) string {
	base := strings.TrimRight(c.cfg.Endpoint, "/")
	u, _ := url.Parse(base)
	u.Path = path.Join(u.Path, "/api/v1/repos", c.owner, c.repo, "issues", strconv.Itoa(number))
	return u.String()
}
~~~

Ensure non-2xx errors never include request headers or token.

- [ ] **Step 4: Run tests and verify green**

Run:

~~~bash
go test -count=1 ./internal/tracker/gitea
~~~

Expected: PASS.

- [ ] **Step 5: Commit**

~~~bash
git add internal/tracker/gitea/client.go internal/tracker/gitea/client_test.go
git commit -m "feat: 刷新 Gitea 任务状态" -m "需求描述：调度器需要按运行中任务 ID 重新读取 Gitea issue 状态。" -m "实现思路：按 issue number 调用单条 issue API，并对非法 ID、HTTP 状态和 payload 错误显式失败。"
~~~

---

### Task 4: Wire Gitea into CLI and docs

**Files:**
- Modify: cmd/symphony/main.go
- Modify: cmd/symphony/main_test.go
- Modify: README.md
- Modify: WORKFLOW.example.md

- [ ] **Step 1: Write failing CLI wiring test**

Add a test in cmd/symphony/main_test.go that writes a workflow with tracker.kind gitea and starts/stops by signal without contacting Gitea because the first automatic poll happens after interval:

~~~go
func TestRunAcceptsGiteaTracker(t *testing.T) {
	dir := t.TempDir()
	workflowPath := filepath.Join(dir, "WORKFLOW.md")
	content := `---
tracker:
  kind: gitea
  endpoint: "http://gitea.local:3000"
  api_key: $GITEA_TOKEN_FOR_CLI_TEST
  project_slug: "owner/repo"
  active_states: ["open"]
  terminal_states: ["closed"]
workspace:
  root: "` + filepath.ToSlash(filepath.Join(dir, "workspaces")) + `"
codex:
  command: "printf '{}\n'"
---
请根据 Gitea 任务完成工作。
`
	if err := os.WriteFile(workflowPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	t.Setenv("GITEA_TOKEN_FOR_CLI_TEST", "gitea-token")

	signals := make(chan os.Signal, 1)
	signals <- os.Interrupt
	var stdout, stderr bytes.Buffer
	err := run(context.Background(), []string{workflowPath}, &stdout, &stderr, runOptions{signals: signals})
	if err != nil {
		t.Fatalf("run returned error for Gitea tracker: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %s, want no error output", stderr.String())
	}
}
~~~

- [ ] **Step 2: Run tests and verify red**

Run:

~~~bash
go test -count=1 ./cmd/symphony
~~~

Expected: FAIL because newHost still always creates Linear or config validation rejects gitea.

- [ ] **Step 3: Implement tracker factory**

In cmd/symphony/main.go imports add:

~~~go
"github.com/local/symphony/internal/tracker/gitea"
~~~

Replace direct Linear creation with helper:

~~~go
trackerClient, err := newTrackerClient(effectiveConfig, http.DefaultClient)
if err != nil { return nil, err }
~~~

Add helper:

~~~go
func newTrackerClient(cfg domain.EffectiveConfig, httpClient *http.Client) (tracker.Client, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Tracker.Kind)) {
	case "linear":
		return linear.New(cfg.Tracker, httpClient), nil
	case "gitea":
		return gitea.New(cfg.Tracker, httpClient), nil
	default:
		return nil, fmt.Errorf("tracker.kind %q is not supported", cfg.Tracker.Kind)
	}
}
~~~

- [ ] **Step 4: Update docs**

README.md:

- Change intro from Linear-only to issue tracker flow with Linear/Gitea.
- Update config table: tracker.kind supports linear and gitea.
- Add Gitea snippet with endpoint/api_key/project_slug owner/repo.
- Preserve user-friendly wording; do not mention TODO or internal implementation status.

WORKFLOW.example.md:

- Keep Linear default example.
- Add commented Gitea alternative block showing tracker.kind gitea and owner/repo.

- [ ] **Step 5: Run tests and verify green**

Run:

~~~bash
go test -count=1 ./cmd/symphony ./internal/config ./internal/tracker/gitea
~~~

Expected: PASS.

- [ ] **Step 6: Commit**

~~~bash
git add cmd/symphony/main.go cmd/symphony/main_test.go README.md WORKFLOW.example.md
git commit -m "feat: 接入 Gitea tracker 启动链路" -m "需求描述：CLI 需要根据 tracker.kind 启动 Linear 或 Gitea 调度来源。" -m "实现思路：新增 tracker factory，并补充 Gitea 配置文档与启动接线测试。"
~~~

---

### Task 5: Full verification and review

**Files:**
- No planned edits unless verification reveals a defect.

- [ ] **Step 1: Run full verification**

Run:

~~~bash
go test -count=1 ./...
go test -race -count=1 ./internal/tracker/gitea ./internal/orchestrator
go build ./cmd/symphony
go vet ./...
go mod tidy -diff
git diff --check
gofmt -l $(git ls-files '*.go')
~~~

Expected: all commands exit 0; gofmt command prints nothing.

- [ ] **Step 2: Fix any failures with TDD**

If a command fails, use systematic-debugging first. Add or adjust a failing test that captures the defect before changing production code.

- [ ] **Step 3: Request code review**

Use requesting-code-review with base SHA from before Task 1 and current HEAD. Ask reviewer to focus on Gitea REST semantics, config validation, token redaction, and integration with existing runtime behavior.

- [ ] **Step 4: Address review feedback**

Fix Critical and Important items before merge. Re-run full verification after fixes.

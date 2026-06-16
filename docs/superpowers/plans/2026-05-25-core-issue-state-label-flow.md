# Core Issue State Label Flow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the MVP core flow dispatch only open, unclaimed Gitea issues and stop redispatching running, done, or failed issues.

**Architecture:** Gitea native issue state remains limited to `open`, `closed`, and `all`. Symphony processing state is represented by mutually exclusive labels: `symphony-running`, `symphony-done`, and `symphony-failed`. The scheduler treats open issues without those labels as pending.

**Tech Stack:** Go, standard `net/http/httptest`, Gitea REST API, `go test`.

---

### Task 1: Gitea fetch filtering

**Files:**
- Modify: `internal/tracker/gitea/client.go`
- Create: `internal/tracker/gitea/client_test.go`

- [ ] Write a failing test proving `FetchIssues` skips issues carrying `symphony-running`, `symphony-done`, or `symphony-failed`.
- [ ] Write a failing test proving invalid `active_states` such as `Todo` returns a clear error instead of silently yielding no work.
- [ ] Implement state validation and managed-label filtering.
- [ ] Run `go test ./internal/tracker/gitea` and confirm pass.

### Task 2: Gitea status label writeback

**Files:**
- Modify: `internal/tracker/gitea/client.go`
- Modify: `internal/tracker/gitea/client_test.go`

- [ ] Write a failing test proving `MarkStatus(done)` removes `symphony-running`, then adds `symphony-done`, then comments.
- [ ] Implement DELETE issue-label call for existing managed labels before adding the target status label.
- [ ] Run `go test ./internal/tracker/gitea` and confirm pass.

### Task 3: Scheduler pending semantics

**Files:**
- Modify: `internal/orchestrator/scheduler.go`
- Create: `internal/orchestrator/scheduler_test.go`

- [ ] Write a failing test proving empty `active_states` defaults to open issues being pending.
- [ ] Write a failing test proving managed status labels are not pending.
- [ ] Implement minimal scheduler checks.
- [ ] Run `go test ./internal/orchestrator` and confirm pass.

### Task 4: Remove obsolete PRD docs and verify

**Files:**
- Delete: `docs/prd/*`

- [ ] Remove obsolete PRD files.
- [ ] Run `go build ./...`.
- [ ] Run `go test ./...`.
- [ ] Run `go vet ./...`.

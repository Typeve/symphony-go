# Symphony

[openai/symphony](https://github.com/openai/symphony)

Languages: [English](README.md) | [简体中文](README.zh-CN.md)

Symphony is a small Go scheduler for issue-driven coding work. It polls Gitea issues, creates isolated workspaces, runs Codex to implement changes, asks a reviewer command such as Claude to review the result, commits the workspace, pushes an execution branch, and writes status back to the original issue.

It is intentionally scoped as an MVP. Symphony coordinates the workflow; it does not write business code itself, create pull requests, merge branches, or close issues.

## What It Does

```text
Gitea issue
-> scheduler filters pending work
-> workspace is cloned from the configured repo
-> execution branch is created
-> Codex runs in the workspace
-> reviewer command runs in the workspace
-> Symphony commits and pushes the branch
-> Gitea issue receives a final status label and comment
```

## MVP Scope

- Gitea is the only tracker.
- Configuration is loaded from a YAML file, defaulting to `symphony.yaml`.
- Issue processing uses three managed states: `pending -> running -> done/failed`.
- Managed labels are limited to `symphony-running`, `symphony-done`, and `symphony-failed`.
- Projects can require a trigger label with `task_label` so ordinary open issues are not dispatched.
- Codex and reviewer processes receive only a small environment allowlist.
- `GITEA_TOKEN` is used by Symphony for tracker and Git operations, but it is not passed to Codex or the reviewer process.
- Git clone and push use temporary `GIT_ASKPASS` credentials instead of writing tokens into remote URLs.
- Execution commits exclude common local artifacts such as `.codex/**`, `.symphony/validation-verdict.json`, `.env*`, and `*.log`.
- Reviewer success is the reviewer command exiting with status 0; the current MVP does not parse a structured verdict file.
- No HTTP status API, PR creation, merge automation, dependency blocking, reconcile loop, or per-state concurrency limits are included in the current MVP.

## Build And Test

```bash
go test -count=1 ./...
go build -o bin/symphony ./cmd/symphony
```

## Configuration

Create `symphony.yaml`:

```yaml
gitea:
  endpoint: "https://gitea.example.com"
  token: "${GITEA_TOKEN}"
  projects:
    - id: "my-project"
      repo_url: "https://gitea.example.com/owner/repo.git"
      active_states: ["open"]
      task_label: "symphony-task"

scheduler:
  poll_interval: 30s
  max_concurrent: 3

codex:
  command: "codex"
  model: "gpt-5.5"
  timeout: 30m

reviewer:
  command: "claude"
  timeout: 15m

workspace:
  root: "/tmp/symphony-workspaces"
```

Then run:

```bash
export GITEA_TOKEN="gitea_token_xxx"
./bin/symphony -config symphony.yaml
```

Use `-once` to poll once, wait for any dispatched work, and exit:

```bash
./bin/symphony -config symphony.yaml -once
```

If `-config` is omitted, Symphony reads `symphony.yaml` from the current directory.

## Issue Handling

By default, Symphony treats Gitea `open` issues as pending unless they already have a managed Symphony label. If `task_label` is set for a project, only issues with that label are pending. When an issue is processed:

- `symphony-running` is added when work starts.
- `symphony-done` is added after Codex, review, commit, and push succeed; the done comment includes the pushed execution branch and commit.
- `symphony-failed` is added when a pipeline stage fails; when available, the failure comment includes the failed stage reason and preserved workspace path.

Execution branches use deterministic names such as:

```text
symphony/my-project/issue-123-fix-login-error
```

## Safety Notes

- Do not commit `symphony.yaml` if it contains real endpoints or tokens.
- Keep `.env`, logs, private keys, and local workspaces outside the public repository.
- Review pushed execution branches manually before merging them.
- Run `go test -count=1 ./...` before publishing changes to this scheduler.

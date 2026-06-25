# Symphony 工作流

本文描述当前简化 MVP 的运行契约。Symphony 不再把 `WORKFLOW.md` 当作运行时配置或 prompt 模板读取；二进制默认读取 `symphony.yaml`，也可通过 `-config` 指定配置文件。

## 执行流程

```text
Gitea open issue
-> 过滤缺少 task_label 或已有管理状态 label 的 issue
-> 创建本地 workspace 并 clone 配置的仓库
-> 创建 deterministic execution branch
-> 运行 Codex 命令
-> 运行 reviewer 命令
-> reviewer 退出码为 0 后提交并推送 execution branch
-> 写回 Gitea 状态 label 和 comment
```

管理状态 label 只有：

- `symphony-running`
- `symphony-done`
- `symphony-failed`

带有以上任一 label 的 open issue 会被跳过，避免重复派发。

如果项目配置了 `task_label`，只有带该 label 的 issue 会被派发；为空时保持兼容行为，所有未带管理状态 label 的 active issue 都可进入队列。

## 配置

推荐配置文件名是 `symphony.yaml`：

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
  max_concurrent: 2

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

`gitea.token` 可以使用环境变量展开。不要把真实 token 提交到仓库。

## 成功与失败语义

- `symphony-running`：Symphony 已接管该 issue。
- `symphony-done`：Codex、reviewer、commit、push 均已成功，execution branch 已可供人工 review。
- `symphony-failed`：某个阶段失败，需要人工检查日志、issue comment 和保留的 workspace。

`symphony-done` 不表示 PR 已创建、分支已合并或 issue 已关闭。

## 安全边界

- `GITEA_TOKEN` 只给 Symphony 用于 tracker 写回和 Git push，不传给 Codex 或 reviewer。
- Git clone 和 push 通过临时 `GIT_ASKPASS` 提供凭据，不把 token 写进 remote URL。
- execution commit 会排除 `.codex/**`、`.symphony/validation-verdict.json`、`.env*` 和 `*.log`。
- reviewer 通过退出码控制 publish gate：0 表示通过，非 0 表示失败。

## 人工恢复

失败后先查看 issue comment 和服务日志，再进入保留的 workspace 检查现场。确认可以重跑时，人工移除对应管理状态 label，等待下一轮轮询重新派发。

## 发布前验证

```bash
go test -count=1 ./...
go vet ./...
go build ./cmd/symphony
git diff --check
```

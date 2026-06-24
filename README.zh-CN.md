# Symphony

[openai/symphony](https://github.com/openai/symphony)

Languages: [English](README.md) | [简体中文](README.zh-CN.md)

Symphony 是一个面向 Gitea issue 的小型 Go 调度器。它轮询 Gitea issue，创建隔离 workspace，调用 Codex 实现代码，再调用 Claude 等 reviewer 命令进行评审，随后提交并推送执行分支，最后把状态写回原 issue。

Symphony 的定位是 MVP 调度器。它负责编排流程，不替 Codex 写业务代码，也不会自动创建 PR、自动合并分支或自动关闭 issue。

## 功能流程

```text
Gitea issue
-> 调度器过滤待处理任务
-> 从配置仓库 clone workspace
-> 创建 execution branch
-> Codex 在 workspace 中运行
-> reviewer 命令在 workspace 中运行
-> Symphony commit 并 push 分支
-> Gitea issue 写入最终状态 label 和 comment
```

## MVP 范围

- 只保留 Gitea tracker。
- 配置从 YAML 文件读取，默认是 `symphony.yaml`。
- issue 处理状态保持精简：`pending -> running -> done/failed`。
- 管理状态 label 只包括 `symphony-running`、`symphony-done` 和 `symphony-failed`。
- Codex 和 reviewer 子进程只继承一个很小的环境变量白名单。
- `GITEA_TOKEN` 只供 Symphony 主进程做 tracker 和 Git 操作，不传给 Codex 或 reviewer。
- Git clone / push 使用临时 `GIT_ASKPASS`，不会把 token 写进 remote URL。
- execution commit 会排除常见本地产物，例如 `.codex/**`、`.symphony/validation-verdict.json`、`.env*` 和 `*.log`。
- reviewer 命令以退出码 0 表示通过；当前 MVP 不解析结构化 verdict 文件。
- 当前 MVP 不包含 HTTP 状态接口、自动 PR、自动 merge、依赖阻塞检查、reconcile loop 或按状态拆分并发限制。

## 构建与测试

```bash
go test -count=1 ./...
go build -o bin/symphony ./cmd/symphony
```

## 配置

创建 `symphony.yaml`：

```yaml
gitea:
  endpoint: "https://gitea.example.com"
  token: "${GITEA_TOKEN}"
  projects:
    - id: "my-project"
      repo_url: "https://gitea.example.com/owner/repo.git"
      active_states: ["open"]

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

运行：

```bash
export GITEA_TOKEN="gitea_token_xxx"
./bin/symphony -config symphony.yaml
```

使用 `-once` 可以只轮询一次，等待已派发任务结束后退出：

```bash
./bin/symphony -config symphony.yaml -once
```

如果不传 `-config`，Symphony 会读取当前目录下的 `symphony.yaml`。

## Issue 处理

默认情况下，Symphony 会把 Gitea `open` issue 视为待处理任务，但已带有 Symphony 管理状态 label 的 issue 会被跳过。处理过程中：

- 开始处理时添加 `symphony-running`。
- Codex、review、commit、push 全部成功后添加 `symphony-done`；完成 comment 会包含已推送的 execution branch 和 commit。
- 任一阶段失败时添加 `symphony-failed`；如果已有失败上下文，失败 comment 会包含失败阶段原因和保留的 workspace 路径。

执行分支名是确定性的，例如：

```text
symphony/my-project/issue-123-fix-login-error
```

## 安全提示

- 如果 `symphony.yaml` 包含真实 endpoint 或 token，不要提交它。
- `.env`、日志、私钥和本地 workspaces 应保留在公开仓库之外。
- execution branch 推送后仍应人工审核，再决定是否合并。
- 修改调度器本身后，发布前运行 `go test -count=1 ./...`。

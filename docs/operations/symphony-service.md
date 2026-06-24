# Symphony 运维手册

## 目标

本文说明如何把当前简化 MVP 版 Symphony 作为本机长期运行服务部署，并完成启动、停止、日志查看和故障恢复。

## 环境变量

建议把 Gitea token 放在 root-only 环境文件中：

```bash
sudo install -d -m 0750 /etc/symphony
sudo tee /etc/symphony/symphony.env >/dev/null <<'ENV'
GITEA_TOKEN=请替换为实际 token
ENV
sudo chmod 0640 /etc/symphony/symphony.env
```

不要把 token 写入 `symphony.yaml`、issue comment、prompt 文件或仓库代码。

## 推荐目录

```text
/opt/symphony/bin/symphony          # 可执行文件
/etc/symphony/symphony.yaml         # 配置文件
/etc/symphony/symphony.env          # 环境变量
/var/lib/symphony/workspaces/       # workspace root
```

`workspace.root` 必须独立于正在运行的 Symphony 源码目录。例如不要配置为 `/opt/symphony/src` 或其子目录。

Symphony 会在 execution commit 中排除常见本地产物：`.codex/**`、`.symphony/validation-verdict.json`、`.env*` 和 `*.log`。这些排除项是当前 MVP 的固定安全边界，不需要在配置文件中声明。

## 配置示例

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
  max_concurrent: 2

codex:
  command: "codex"
  model: "gpt-5.5"
  timeout: 30m

reviewer:
  command: "claude"
  timeout: 15m

workspace:
  root: "/var/lib/symphony/workspaces"
```

`codex.command` 和 `reviewer.command` 可以包含简单参数，例如 `"codex app-server"` 或 `"claude --mode strict"`。它们不会经过 shell 执行，因此不要依赖管道、重定向、命令替换或内联环境变量赋值。

## systemd service 示例

```ini
[Unit]
Description=Symphony Codex Workflow Host
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=/etc/symphony
EnvironmentFile=/etc/symphony/symphony.env
ExecStart=/opt/symphony/bin/symphony -config /etc/symphony/symphony.yaml
Restart=on-failure
RestartSec=10
TimeoutStopSec=30
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ReadWritePaths=/var/lib/symphony/workspaces /etc/symphony

[Install]
WantedBy=multi-user.target
```

安装和启动：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now symphony.service
sudo systemctl status symphony.service
```

停止和重启：

```bash
sudo systemctl stop symphony.service
sudo systemctl restart symphony.service
```

## 日志查看

```bash
journalctl -u symphony.service -f
journalctl -u symphony.service --since "1 hour ago"
```

日志中不应出现 token 原文。如发现敏感信息，请立即轮换 token，并检查配置、issue 内容和仓库文件。

## Issue 状态

当前 MVP 只使用三个 Gitea 管理状态 label：

- `symphony-running`：任务已被 Symphony 接管，正在处理。
- `symphony-done`：Codex、reviewer、commit 和 push 均已成功；完成 comment 会写入已推送的 execution branch 和 commit。
- `symphony-failed`：某个阶段失败，需要人工检查；失败 comment 会尽量写入失败阶段原因和保留的 workspace 路径。

带有以上任一 label 的 open issue 会在后续轮询中跳过，避免重复消耗 Codex 额度。若确认某个失败任务可以重新处理，请先人工检查失败 workspace 和 issue comment，再移除对应管理 label 后重新派发。

## 故障恢复

### 自动处理失败

查看 issue comment 和 `symphony-failed` label。失败 comment 和日志会包含失败阶段、简短原因，并在 workspace 创建成功后记录 `workspace_path`。

失败 workspace 会保留在：

```text
{workspace.root}/{project_id}/issue-{number}-{slug}/
```

人工检查后，可移除 `symphony-failed`，修正 issue 或代码，再等待下一轮轮询。

### reviewer 失败

reviewer 命令退出码非 0 时，Symphony 不会提交或推送 execution branch。请查看日志中的 reviewer 输出摘要和保留 workspace，修复后重新派发。

### Codex 失败

Codex 命令退出码非 0 时，Symphony 会记录截断后的命令输出摘要，并保留 workspace。不要只根据 issue label 判断根因，优先查看服务日志和 workspace 内容。

### push 冲突

若远端已有同名 branch，Git push 失败会使任务进入 `symphony-failed`。请人工确认分支归属，必要时重命名或删除冲突分支后重新派发。

### 服务重启

服务重启后，Gitea 上已有 `symphony-running`、`symphony-done` 或 `symphony-failed` 的 open issue 不会重复派发。若确认某个中间态任务可以重新处理，请先人工检查 workspace 和 issue comment，再移除对应 label。

## 发布前验证

```bash
go test -count=1 ./...
go vet ./...
go build ./cmd/symphony
git diff --check
```

部署后可先跑一次轮询 smoke：

```bash
/opt/symphony/bin/symphony -config /etc/symphony/symphony.yaml -once
```

部署前建议使用测试仓库创建一个低风险 open issue，确认能生成 execution branch，并在 issue 中看到最终状态 comment。

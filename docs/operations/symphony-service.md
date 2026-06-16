# Symphony 运维手册

## 目标

本文说明如何把 Symphony 作为本机长期运行服务部署，并完成启动、停止、日志查看、状态检查和故障恢复。

## 环境变量

建议把 Gitea token 放在 root-only 环境文件中：

```bash
sudo install -d -m 0750 /etc/symphony
sudo tee /etc/symphony/symphony.env >/dev/null <<'ENV'
GITEA_TOKEN=请替换为实际 token
ENV
sudo chmod 0640 /etc/symphony/symphony.env
```

不要把 token 写入 `WORKFLOW.md`、issue comment、prompt 文件或仓库代码。

## 推荐目录

```text
/opt/symphony/bin/symphony          # 可执行文件
/etc/symphony/WORKFLOW.md          # 工作流配置
/etc/symphony/symphony.env         # 环境变量
/var/lib/symphony/workspaces/       # workspace root
```

`workspace.root` 必须独立于正在运行的 Symphony 源码目录。例如不要配置为 `/root/work/symfony` 或其子目录。

建议在 `commit.exclude` 中排除验证产物和本地会话文件，例如 `.symphony/validation-verdict.json` 与 `.codex/**`。Symphony 会始终提交 `.symphony/execution.json`，用于后续确认分支归属。

## Go 项目验证器示例

仓库提供了 `examples/validator-go.sh`，可作为 Go 项目的 External Validator 示例。它会在 `SYMPHONY_WORKSPACE` 中执行：

```bash
go test -mod=readonly -count=1 ./...
```

然后把 verdict 写入 `SYMPHONY_VERDICT_PATH`。测试失败会写出 `fail` verdict；缺少 Go 工具链会写出 `blocked` verdict。示例配置：

```yaml
validator:
  kind: command
  command: /opt/symphony/examples/validator-go.sh
  timeout_ms: 1800000
  verdict_path: .symphony/validation-verdict.json
  env_allowlist:
    - PATH
    - HOME
    - SHELL
```

启用 Gitea 项目时必须配置 External Validator。缺少验证器时，Symphony 会在启动校验阶段停止，避免未经独立验证就交接任务。

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
ExecStart=/opt/symphony/bin/symphony --port 8080 /etc/symphony/WORKFLOW.md
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

日志中不应出现 token 原文。如发现敏感信息，请立即轮换 token，并检查 workflow、prompt、hooks 和仓库文件。

## 状态 API

状态 API 默认只监听本机地址。

```bash
curl -s http://127.0.0.1:8080/api/v1/state
curl -s -X POST http://127.0.0.1:8080/api/v1/refresh
```

`/api/v1/state` 可用于确认：

- 每个项目是否 active 或 suspended。
- 每个项目当前 running、retrying、连续失败次数、最近失败阶段和错误类型。
- `last_poll_at` 和 `next_poll_at` 是否符合预期轮询节奏。
- 是否存在被过滤任务以及对应 filter_reason，例如缺少 `ai-ready`、命中排除 label、assignee/milestone 不匹配或标题仍是 Draft/WIP。
- 当前是否有 queued、running、validating、reworking、retrying、branch_ready 或 failed 任务。
- 是否有重试队列。
- 是否存在显式诊断信息。
- Codex token 使用量是否持续增长。

## 故障恢复

### 日志排查

Symphony 输出 JSON 日志。任务状态变化会包含 `project_id`、`repo`、`issue_number`、`issue_id`、`issue_identifier`、`workspace_path`、`branch`、`state_from`、`state_to`，失败时还会包含 `error_stage` 和 `error_category`。External Validator 返回 verdict 时会记录 `validator_status`、`validator_summary` 以及 findings、commands、risks 数量，便于定位验证阶段卡点。敏感字段和常见密钥片段会被脱敏。

### 任务已处理但 issue 仍是 open

这是预期行为。MVP 不自动关闭 issue。成功处理后会设置 `symphony-branch-ready`，后续轮询会跳过该 issue。

### 任务处于处理中间态

Symphony 会在 Gitea issue 上切换 `symphony-running`、`symphony-validating` 和 `symphony-reworking`，并写入简短评论说明当前处于处理、验证或返工阶段。这些 label 表示任务已被接管，后续轮询会跳过它们，避免服务重启后重复消耗 Codex 额度。成功或失败写回时，中间态 label 会被清理，并保留 `symphony-branch-ready` 或 `symphony-failed` 作为人工接管标记。

在 `/api/v1/state` 的 `skipped` 列表中，可以看到因 `symphony-running`、`symphony-validating`、`symphony-reworking`、`symphony-branch-ready` 或 `symphony-failed` 被跳过的 issue 和 `filter_reason`。这用于区分“当前没有任务”和“任务已被 Symphony 接管或交接”。

### 项目被暂停

如果 `/api/v1/state` 中某个项目显示 `suspended`，表示连续失败次数达到 `scheduler.project_failure_threshold`。请先查看诊断、`last_error_stage`、`last_error_category` 和对应 issue comment，处理根因后重启服务或调整任务 label 再恢复派发。其他项目不受影响。

### 自动处理失败

查看 issue comment 和 `symphony-failed` label。失败评论会包含失败阶段、错误类型、简短原因、保留 workspace 路径；如果失败来自最终验证结果，还会包含验证意见摘要。workspace 会保留在：

```text
{workspace.root}/{project_id}/issue-{number}-{slug}/
```

人工检查后，可移除 `symphony-failed`，修正 issue 或代码，再重新添加 `ai-ready`。

### push 冲突

若远端已有同名 branch 且 metadata 不匹配，Symphony 不会覆盖远端分支。请人工确认分支归属，必要时重命名或删除冲突分支后重新派发。

### validator 失败或阻塞

`fail` 会按配置触发有限返工；达到上限后会写回失败。`blocked` 不会自动返工，需要人工处理 blocker 或更新 issue。

### 服务重启

服务重启后，Gitea 上已有 `symphony-running`、`symphony-validating`、`symphony-reworking`、`symphony-branch-ready` 或 `symphony-failed` 的 open issue 不会重复派发。若确认某个中间态任务可以重新处理，请先人工检查 workspace 和 issue comment，再移除对应中间态 label 后重新派发。

## 发布前验证

```bash
go test -count=1 ./...
go build ./cmd/symphony
git diff --check
```

部署前建议使用测试仓库创建一个带 `ai-ready` 的低风险 issue，确认能生成 execution branch，并在 issue 中看到 branch、commit 和验证摘要。

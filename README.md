# Go Symphony

Go Symphony 是一个面向 Gitea issue 的本地主机调度器。它负责多项目轮询、过滤、workspace 管理、Codex 调用、External Validator 编排、中文提交、execution branch 推送和 issue 写回。Codex 负责实现代码；Symphony 不替 Codex 写业务代码，也不会自动创建 PR、自动合并或自动关闭 issue。

当前 MVP 工作流：

```text
Gitea issue + ai-ready label
→ Symphony 多项目轮询与过滤
→ 项目隔离 workspace 自动 clone 仓库
→ Codex 在 workspace 中实现
→ External Validator 只读验证
→ Symphony commit + push execution branch
→ Symphony 写回 branch / commit / verdict，并设置运行、验证、分支就绪或失败 label
```

## 构建与运行

```bash
# 运行验证
go test -count=1 ./...

# 构建 CLI
go build -o bin/symphony ./cmd/symphony

# 准备配置
cp WORKFLOW.example.md WORKFLOW.md
export GITEA_TOKEN="gitea_token_xxx"

# 使用默认 ./WORKFLOW.md 启动
./bin/symphony

# 使用显式工作流路径启动
./bin/symphony ./WORKFLOW.md

# 同时开启本地状态接口
./bin/symphony --port 8080 ./WORKFLOW.md
```

`--port` 仅绑定 `127.0.0.1`。端口为 `0` 时由系统分配可用端口，启动日志会记录实际地址。

## 工作流配置

工作流文件由 YAML front matter 和 Markdown 提示词组成。默认路径是当前目录的 `WORKFLOW.md`，也可以在命令末尾传入显式路径。

关键字段：

| 字段 | 说明 |
| --- | --- |
| `projects` | 必填。一个或多个 Gitea managed projects。旧的根级 `tracker` 配置不再支持。 |
| `projects[].id` | 项目 ID，用于 workspace 隔离、分支命名和错误定位。 |
| `projects[].tracker.kind` | MVP 使用 `gitea`。 |
| `projects[].tracker.endpoint` | Gitea 根地址，例如 `https://gitea.example.com`。 |
| `projects[].tracker.api_key` | Gitea token，建议写成环境变量引用，例如 `$GITEA_TOKEN`。 |
| `projects[].tracker.owner` / `repo` | Gitea 仓库坐标，也可用 `project_slug: owner/repo`。 |
| `projects[].filters.labels_include` | 默认建议包含 `ai-ready`，只有准备好的任务会被派发。 |
| `projects[].filters.labels_exclude` | 默认建议排除 `blocked`、`human-only`、`security`、`credential`、`infra` 和 Symphony 状态 label。 |
| `projects[].filters.assignees_include` | 可选。只派发指派给指定 Gitea 用户的 issue。 |
| `projects[].filters.milestones_include` | 可选。只派发属于指定 milestone 标题或 ID 的 issue。 |
| `projects[].filters.exclude_draft` | 可选。为 `true` 时跳过标题以 Draft / WIP 开头的 issue。 |
| `projects[].branch.prefix` | Execution Branch 前缀；未配置时使用 `symphony/{project_id}`。 |
| `commit.exclude` / `projects[].commit.exclude` | 可选。提交时排除生成物或验证产物；`.symphony/execution.json` 会始终保留用于分支归属校验。 |
| `projects[].workflow_prompt` | 可选。项目仓库内的提示词文件路径，例如 `.symphony/WORKFLOW.md`。 |
| `labels.*` | 可选。配置 `running`、`validating`、`reworking`、`branch_ready`、`failed` 五类 Gitea 状态 label；默认使用 `symphony-*`。 |
| `workspace.root` | 本地 workspace 根目录。实际路径为 `{root}/{project_id}/issue-{number}-{slug}/`。 |
| `codex.env_allowlist` | Codex 子进程可继承的环境变量白名单；敏感变量名会被拒绝。 |
| `validator.*` | External Validator 命令配置；启用 Gitea 项目时必须配置，验证结论必须写入 JSON verdict file。 |
| `repair.max_attempts` | 验证失败后的有限返工次数，默认 1 次。 |

Go 项目可直接参考 `examples/validator-go.sh` 作为 External Validator 示例。它会执行 `go test -mod=readonly -count=1 ./...`，并写出符合 Symphony schema 的 verdict file。

提示词支持以下模板变量：

- `.project.id`
- `.project.tracker.kind`
- `.project.tracker.endpoint`
- `.project.tracker.project_slug`
- `.issue.identifier`、`.issue.title`、`.issue.description`、`.issue.url`、`.issue.labels`、`.issue.blocked_by`
- `.workspace.path`、`.workspace.key`
- `.execution_branch`
- `.attempt`

Gitea token 不会进入模板上下文。

## Issue 标记

Symphony 使用 Gitea label 防止重复派发：

| Label | 含义 |
| --- | --- |
| `ai-ready` | 任务已准备好，可以自动处理。 |
| `symphony-running` | Symphony 正在处理该任务。 |
| `symphony-validating` | External Validator 正在验证处理结果。 |
| `symphony-reworking` | Codex 正在根据验证意见进行有限返工。 |
| `symphony-branch-ready` | execution branch 已推送，等待人工审核。 |
| `symphony-failed` | 自动处理未能完成，等待人工检查。 |
| `blocked` / `human-only` / `security` / `credential` / `infra` | 默认建议排除，避免敏感或未准备好的任务被处理。 |

处理中间态 label 会随状态迁移自动切换；成功或失败后会清理中间态 label，并保留 `symphony-branch-ready` 或 `symphony-failed` 防止重复派发。进入处理、验证或返工时，issue comment 会记录关键状态变化。成功时，issue comment 会包含 branch、commit、验证摘要、风险提示和人工下一步建议。失败时，issue comment 会包含失败阶段、错误类型、简短原因、保留 workspace 路径和验证意见摘要，方便人工接管。

## 本地状态接口

传入 `--port` 后，CLI 会开启本地 HTTP API：

- `GET /api/v1/state`：返回项目摘要、last_poll_at、next_poll_at、被过滤任务、queued、running、validating、reworking、retrying、branch_ready、failed、诊断提示、Codex token 汇总和速率限制摘要。项目摘要会显示 running、retrying、failure_count、suspended、last_error_stage 和 last_error_category；被过滤任务会显示 filter_reason，包括因 Symphony 状态 label 被跳过的 issue；branch-ready 和 failed 任务会显示验证摘要、保留工作区或验证意见等接管信息。
- `GET /api/v1/issues/{id}`：从当前快照查询单个任务详情。
- `POST /api/v1/refresh`：显式触发一次调度刷新。

接口只监听本机地址。不要把状态接口直接暴露到公网。

## 安全与失败处理

- Symphony 主进程可以读取 Gitea token；Codex 和 External Validator 只继承 allowlist 环境变量。
- Git clone / push 使用临时凭据，不把 token 写入 remote URL 或 `.git/config`。
- workspace root 不能是符号链接，workspace 不能位于正在运行的 Symphony 源码树内。
- Codex 正常退出后必须产生 Git diff 才会进入验证。
- `.env`、私钥、日志、Codex 原始会话文件或疑似密钥内容会触发显式失败。
- External Validator 只能只读检查 workspace；若修改 workspace，验证结果无效。
- `fail` verdict 最多触发配置允许的有限返工；`blocked` verdict 直接交给人工处理。
- 验证通过后，Symphony 生成中文 commit message，写入 `.symphony/execution.json`，并推送稳定 execution branch。
- push 前会校验远端同名 branch 的 execution metadata；归属不匹配时不会覆盖。
- 同一项目连续失败达到 `scheduler.project_failure_threshold` 后会暂停该项目，其他项目继续运行。
- 写回失败会记录诊断，不会静默降级。

## 验证命令

提交前建议执行：

```bash
gofmt -w $(git ls-files '*.go')
go test -count=1 ./...
go build ./cmd/symphony
git diff --check
git status --short
```

更多 systemd、环境变量、日志、状态 API 和故障恢复说明见 `docs/operations/symphony-service.md`。

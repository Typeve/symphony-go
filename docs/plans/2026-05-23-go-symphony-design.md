# Go 版 Symphony 系统设计

## 背景

本项目实现一个 Go 版 Symphony 编排服务，参考 `openai/symphony` 的语言无关规格。服务负责持续读取 Linear Issue，为每个 Issue 创建隔离工作区，并在工作区内启动 Codex app-server 执行任务。

当前实现目标是覆盖 Core Conformance，后续扩展 HTTP Dashboard、SSH worker 与更多 tracker 适配。

## 设计目标

- 以 Go 实现长期运行的调度/执行服务。
- 使用 `WORKFLOW.md` 作为仓库内工作流契约。
- 对每个 Issue 使用稳定、安全、可复用的独立工作区。
- 对 Linear、工作区、Hook、Prompt、Codex app-server、重试和状态同步进行清晰分层。
- 所有失败显式暴露，不使用静默降级或伪造结果。
- 提供结构化日志和可测试的核心状态机。

## 非目标

- 初版不实现完整 Web Dashboard。
- 初版不实现 SSH 远程 worker。
- 初版不实现多 issue tracker。
- 初版不持久化运行中 session 或 retry queue。

## 系统架构

```text
cmd/symphony
  -> internal/workflow
  -> internal/config
  -> internal/tracker/linear
  -> internal/workspace
  -> internal/prompt
  -> internal/agent
  -> internal/orchestrator
  -> internal/logging
  -> internal/httpapi        可选基础状态 API
```

### cmd/symphony

CLI 入口，负责解析参数、加载配置、初始化日志、启动 orchestrator。默认读取当前目录 `WORKFLOW.md`，也支持传入显式路径。

### internal/workflow

读取 `WORKFLOW.md`，支持 YAML front matter 和 Markdown prompt body。解析失败返回类型化错误。prompt body 会被 trim，front matter 不是 map 时直接失败。

### internal/config

将 workflow raw config 转为强类型配置：

- `tracker.kind`
- `tracker.endpoint`
- `tracker.api_key`
- `tracker.project_slug`
- active/terminal states
- polling interval
- workspace root
- hooks 与 hook timeout
- agent concurrency、turn、retry backoff
- codex command、timeout、sandbox pass-through 字段

配置解析只对声明为 `$VAR` 的字段解析环境变量，不做全局 env override。workspace 路径支持 `~`、`$VAR` 和相对路径解析。

### internal/tracker/linear

实现三个核心接口：

- `FetchCandidateIssues(ctx)`
- `FetchIssuesByStates(ctx, states)`
- `FetchIssueStatesByIDs(ctx, ids)`

使用 Linear GraphQL，按 project slug 和 active states 查询候选 Issue，支持分页，归一化 labels、blockers、priority、时间字段和状态。

### internal/workspace

负责根据 Issue identifier 生成安全 workspace key，只允许 `[A-Za-z0-9._-]`，其他字符替换为 `_`。workspace path 必须处于 workspace root 内。支持 after_create、before_run、after_run、before_remove hook，并按规格处理失败语义。

### internal/prompt

使用严格模板渲染。输入变量为 `issue` 与 `attempt`。未知变量或未知模板行为必须失败，不能生成不完整 prompt。

### internal/agent

以 `bash -lc <codex.command>` 在 issue workspace 中启动 Codex app-server。启动前校验 cwd 等于 workspace path 且 workspace path 位于 workspace root 内。Agent runner 负责首轮 prompt、后续 continuation prompt、turn timeout、read timeout、stderr 分离、事件解析、token/rate-limit 事件上报。

### internal/orchestrator

单一状态所有者。持有 running、claimed、retry queue、completed、codex totals、rate limits 等状态。每个 tick 执行：

1. reconcile running issues
2. dispatch preflight validation
3. fetch candidate issues
4. sort by priority、created_at、identifier
5. dispatch until slots exhausted
6. emit state/log updates

正常 worker 退出后安排短 continuation retry。异常退出按指数退避重试。状态变为 terminal 时停止 worker 并清理 workspace，状态变为非 active 且非 terminal 时停止 worker 但保留 workspace。

### internal/logging

输出结构化日志，Issue 相关日志包含 `issue_id` 和 `issue_identifier`，Codex session 相关日志包含 `session_id`。日志不输出 API token 或环境变量密文。

## 数据流

```text
WORKFLOW.md
  -> WorkflowDefinition
  -> EffectiveConfig
  -> Linear candidates
  -> Orchestrator dispatch decision
  -> WorkspaceManager create/reuse
  -> PromptRenderer render
  -> AgentRunner starts Codex app-server
  -> Agent events update Orchestrator state
  -> Worker outcome schedules retry/release/cleanup
```

## 错误处理

- workflow/config 错误：启动时失败；运行中 reload 失败时保留最后一次有效配置并记录错误。
- tracker 错误：本 tick 跳过 dispatch 或保留 running worker，等待下次重试。
- workspace/hook 错误：按 hook 语义失败或记录后继续。
- prompt 错误：当前 run attempt 失败并进入 retry。
- agent 错误：当前 run attempt 失败并进入 retry。
- stall：orchestrator 按配置终止 worker 并进入 retry。

## 安全边界

- Codex 只能在 per-issue workspace cwd 中启动。
- workspace path 必须位于 workspace root 内。
- workspace key 必须经过确定性清洗。
- token 只用于请求认证，不进入日志。
- hook 是受信任配置，但必须有超时。

## 测试策略

采用 TDD：

- Workflow loader：front matter、缺失文件、非 map、prompt trim。
- Config resolver：默认值、env 解析、路径解析、字段校验。
- Workspace manager：路径清洗、root containment、hook 语义。
- Prompt renderer：成功渲染、未知变量失败。
- Linear tracker：GraphQL query、分页、payload 归一化、错误映射。
- Orchestrator：排序、并发、per-state limit、blocker、retry、reconcile、stall。
- Agent runner：cwd 校验、命令启动、timeout、事件解析、失败映射。
- CLI：默认路径、显式路径、启动失败、正常信号退出。

## 交付顺序

1. 项目骨架和领域模型。
2. Workflow 与 Config。
3. Workspace 与 Prompt。
4. Linear Tracker。
5. Orchestrator 核心状态机。
6. Agent Runner。
7. CLI 与基础状态快照。
8. 集成验证与文档。

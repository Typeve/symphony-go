# Gitea Tracker 设计

## 背景

NAS 上已有 Gitea，可以用零额外成本提供 issue、label、milestone 与代码仓库一体化能力。Go Symphony 目前只支持 Linear tracker；新增 Gitea tracker 后，可以形成“Gitea Issue → Symphony → Codex → 分支/PR 回 Gitea”的内网闭环。

## 目标

新增 tracker.kind = "gitea"，让调度器能够从指定 Gitea 仓库读取 issue、刷新 issue 状态，并复用现有 workspace、prompt、agent 与状态接口链路。

## 非目标

- 本次不在编排层自动创建 PR 或写回 issue 评论；这些动作仍由 Codex 根据工作流提示执行。
- 本次不引入 Gitea webhook；仍使用现有轮询模型。
- 本次不新增数据库或持久化状态。

## 配置

Gitea 复用现有 tracker 配置结构：

~~~yaml
tracker:
  kind: gitea
  endpoint: "http://nas.local:3000"
  api_key: "$GITEA_TOKEN"
  project_slug: "owner/repo"
  active_states: ["open"]
  terminal_states: ["closed"]
~~~

字段解释：

- tracker.endpoint：Gitea 根地址，用户不需要填写 /api/v1。
- tracker.api_key：Gitea token，建议通过环境变量注入。
- tracker.project_slug：在 Gitea 模式下解释为 owner/repo。
- active_states / terminal_states：Gitea 原生 issue 状态建议使用 open / closed。

## 架构

新增 internal/tracker/gitea 包，实现现有 tracker.Client 接口：

- FetchCandidateIssues(ctx)：按 active_states 拉取候选 issue。
- FetchIssuesByStates(ctx, states)：按给定状态拉取 issue。
- FetchIssueStatesByIDs(ctx, ids)：按 issue number 刷新状态。

cmd/symphony 根据 tracker.kind 创建 Linear 或 Gitea client。config.Resolve 与 ValidateDispatch 扩展为允许 gitea，并对 Gitea 必需字段做显式校验。

## API 映射

Gitea REST API：

- 列表：GET /api/v1/repos/{owner}/{repo}/issues?state={open|closed|all}&page=N&limit=50
- 单条：GET /api/v1/repos/{owner}/{repo}/issues/{index}

映射规则：

- 跳过带 pull_request 字段的记录，避免把 PR 当 issue 调度。
- domain.Issue.ID 使用 issue number 的字符串形式，例如 "123"。
- domain.Issue.Identifier 使用 owner/repo#123。
- Title、Description、State、URL、Labels、CreatedAt、UpdatedAt 从 Gitea payload 映射。
- Priority 暂不映射，保持 nil。
- BlockedBy 暂不映射，保持空列表；阻塞关系可后续通过 label 或 issue dependency 扩展。

## 错误处理

遵守 Debug-First：

- 缺少 endpoint、api_key、project_slug，或 project_slug 不是 owner/repo，启动校验失败。
- HTTP 请求失败、非 2xx 状态、JSON 解析失败、payload 缺少必要字段，全部返回显式错误。
- 错误信息不包含 token；请求使用 Authorization: token <token> header。
- Gitea 列表分页检测重复页，避免异常分页导致死循环。

## 测试

采用 TDD：

1. 配置解析与校验测试：允许 gitea，拒绝缺失或格式错误配置。
2. Gitea client 单元测试：请求路径、鉴权、分页、跳过 PR、字段映射、错误处理。
3. CLI 启动接线测试：tracker.kind=gitea 时创建 Gitea client。
4. 全量验证：go test -count=1 ./...、go vet ./...、go build ./cmd/symphony。

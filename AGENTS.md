# Symphony — Issue-Driven AI Coding Scheduler

Go 项目，自动扫描 Gitea issues → 调 Codex 开发 → 调 Claude 评审 → git push。

## 目标

**大幅简化现有代码库**。当前 19K 行（含测试 6.8K），目标源码 ~1500 行 + 测试 ~500 行。砍掉过度设计，保留核心链路。

## 必守规则

- 只保留 Gitea tracker，删除 Linear 和 multi-client
- 删除 httpapi 包（不需要 HTTP 状态查询接口）
- 删除 logging 包，用标准 `log/slog` 替代
- 删除 validator 包的 Read-Only Guard（MVP 不需要）
- config 包用 `yaml.Unmarshal` 到 struct，不手写解析
- 状态机只保留 3 态：`pending → running → done/failed`
- 不需要 reconcile、指数退避 retry、project failure threshold
- 不需要 blocked-by 依赖检查
- 不需要 per-state 并发限制
- 环境变量白名单保留基本过滤即可（不传 GITEA_TOKEN 给 Codex）
- 密钥脱敏只做最基础的（日志里不打印完整 token）
- 不需要 JSON-RPC 双向流（用简单的 stdin/stdout 行协议即可）

## 简化后目标结构

```
cmd/symphony/main.go          # 入口：加载配置 → 启动调度循环
internal/config/config.go      # yaml.Unmarshal + 环境变量
internal/domain/domain.go      # 数据结构（精简）
internal/tracker/gitea/        # Gitea REST 客户端（保留，简化）
internal/scheduler/scheduler.go # 定时扫描 + 队列
internal/agent/runner.go       # Codex CLI 调用
internal/reviewer/reviewer.go  # Claude CLI 调用评审
internal/git/git.go            # branch/commit/push
internal/workspace/workspace.go # workspace 创建/清理
```

## 从这里开始

1. 读 `CONTEXT.md` 了解领域概念
2. 读现有代码理解核心逻辑
3. 按模块逐步简化，每步确保 `go build ./...` 通过
4. 简化完一个模块后删掉对应的旧测试
5. 最后写核心路径测试

## 文档地图

- `CONTEXT.md` — 领域术语和设计约束
- `WORKFLOW.md` — 工作流说明
- `WORKFLOW.example.md` — 配置示例

## 常用验证

```bash
go build ./...
go test ./...
go vet ./...
```

## 配置格式（简化后）

```yaml
gitea:
  endpoint: "https://gitea.example.com"
  token: "$GITEA_TOKEN"
  projects:
    - id: my-project
      repo_url: "https://gitea.example.com/user/repo.git"
      active_states: ["Todo", "In Progress"]

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
  root: /tmp/symphony-workspaces
```

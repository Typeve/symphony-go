# Gitea 完成标记防重复派发设计

## 背景

当前 Symphony 不依赖数据库保存完成态，调度层也不会关闭 Gitea issue。Codex 正常结束后，如果 Gitea issue 仍是 `open`，下一轮轮询会再次把它当成候选任务。现有 `ApplyWorkerExit` 还会为正常退出安排 continuation retry，使重复派发更快发生。

## 目标

- Codex 正常完成后，不再重复执行同一个 issue。
- 不自动关闭 Gitea issue，避免误关任务。
- 由 Symphony 在 Gitea 写入确定性的完成标记：label + comment。
- 候选任务读取时跳过已带完成 label 的 issue。
- 写回失败必须显式暴露，不静默忽略。

## 方案

### 配置

新增 `completion` 配置：

```yaml
completion:
  enabled: true
  label: symphony-completed
  comment: 任务已完成处理，后续不会重复派发。
```

Gitea tracker 默认启用 completion 标记，默认 label 为 `symphony-completed`，默认 comment 使用用户友好的完成说明。Linear 默认不启用，避免在没有写回实现时启动失败。若显式启用但当前 tracker 不支持完成标记，启动时失败。

### Tracker 能力

新增可选完成标记能力：

```go
type CompletionMarker interface {
    MarkIssueCompleted(ctx context.Context, issue domain.Issue) error
}
```

Gitea client 实现该能力：

1. 确保仓库存在完成 label；不存在则创建。
2. 给 issue 添加完成 label。
3. 给 issue 添加完成评论。
4. 错误信息不包含 token。

### 候选过滤

Gitea `FetchCandidateIssues` / `FetchIssuesByStates` 映射 issue labels 后，如果包含 completion label，则不返回给调度器。

### 调度行为

- Codex/Agent 正常结束后，Symphony 调用 `MarkIssueCompleted`。
- 正常结束不再安排 continuation retry。
- 如果 marker 写回失败，调度状态保留本地 completed/claimed 保护并记录 diagnostic，避免同进程内重复消耗 Codex；错误在状态中显式可见。
- 下一次进程重启后，远端 label 是跨进程防重复依据。

## 错误处理

- Gitea label/comment 写回失败：记录清晰 diagnostic，不把失败包装成 Codex 执行失败，不触发重新运行 Codex。
- Gitea API 非 2xx：返回状态错误，不泄露 token。
- 候选读取遇到异常 payload：按现有 Debug-First 策略返回错误。

## 测试

- Gitea candidate fetch 跳过完成 label issue。
- Gitea completion marker 创建 label、添加 label、添加评论。
- Gitea marker HTTP 错误不泄露 token。
- Runtime 正常 worker 退出后不再安排 retry，也不会在下一轮重新派发同一 open issue。
- Runtime marker 失败时记录 diagnostic 且不重新派发 Codex。
- Config 解析 completion 默认值和显式配置。

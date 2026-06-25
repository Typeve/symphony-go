# Symphony YAML 配置示例

当前二进制默认读取 `symphony.yaml`。这个文件是配置示例，不是运行时 prompt 模板。

```yaml
gitea:
  endpoint: "https://gitea.example.com"
  token: "${GITEA_TOKEN}"
  projects:
    - id: "symphony"
      repo_url: "https://gitea.example.com/example-owner/example-repo.git"
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

## 处理语义

- Symphony 只处理没有管理状态 label 的 Gitea issue；如果项目配置了 `task_label`，issue 还必须带有该触发 label。
- 管理状态 label 只有 `symphony-running`、`symphony-done`、`symphony-failed`。
- Codex 和 reviewer 子进程只继承基础环境变量白名单，不继承 `GITEA_TOKEN`。
- reviewer 命令退出码为 0 时视为 review 通过；非 0 时任务标记为失败。
- 成功后 Symphony 提交并推送 execution branch，然后把 issue 标记为 `symphony-done`。
- 失败后 Symphony 标记 `symphony-failed`，并保留已创建的 workspace 供人工检查。
- execution commit 会排除 `.codex/**`、`.symphony/validation-verdict.json`、`.env*` 和 `*.log`。

## 可选 agent 指令模板

当前二进制不会读取本节；如需给 Codex 使用，请把内容放入 issue 描述或后续显式支持的 prompt 配置中。

```text
你将处理一个 Gitea issue。请只依据 issue 内容和当前仓库中的真实代码开展工作。

工作要求：
1. 先理解标题、描述和标签，确认任务可以安全推进。
2. 修改代码前优先补充或更新能够说明目标行为的验证。
3. 只在当前工作区内修改文件，不处理 .env、私钥、日志、Codex 原始会话或其他敏感文件。
4. 不要提交、推送、创建 PR、合并分支或关闭 issue；这些由 Symphony 或人工处理。
5. 不要在输出、文件、日志或提示中写入 token、密钥、密码或授权头。
6. 所有失败都要清晰暴露，不使用伪造结果，也不隐藏真实错误。
7. 完成后给出实现摘要、建议 commit 信息、已验证的命令和结果，并说明仍需人工关注的风险。
```

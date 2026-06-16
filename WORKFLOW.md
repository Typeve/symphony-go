---
projects:
  - id: example-project
    enabled: true
    tracker:
      kind: gitea
      endpoint: https://gitea.example.com
      api_key: $GITEA_TOKEN
      owner: example-owner
      repo: example-repo
    filters:
      states:
        - open
scheduler:
  polling_interval_ms: 5000
workspace:
  root: /tmp/symphony-workspaces
  allow_git_remote_credentials: false
completion:
  enabled: true
  label: symphony-completed
  comment: 任务已完成处理，后续不会重复派发。
hooks:
  timeout_ms: 60000
  after_create: |
    if [ ! -d .git ]; then
      git clone --depth 1 "https://gitea.example.com/example-owner/example-repo.git" . 2>&1
      git remote set-url origin "https://gitea.example.com/example-owner/example-repo.git"
    fi
agent:
  max_concurrent_agents: 2
  max_turns: 20
  max_retry_backoff_ms: 300000
  max_concurrent_agents_by_state:
    open: 2
codex:
  command: codex app-server
  approval_policy: on-request
  thread_sandbox: workspace-write
  turn_sandbox_policy: workspace-write
  env_allowlist:
    - PATH
    - HOME
    - SHELL
  turn_timeout_ms: 3600000
  read_timeout_ms: 5000
  stall_timeout_ms: 300000
repair:
  enabled: true
  max_attempts: 1
---
你将处理一个任务。请只依据任务本身和仓库中的真实代码开展工作。

任务信息：
- 标题：{{ .issue.title }}
- 描述：{{ .issue.description }}
- 标签：{{ range .issue.labels }}{{ . }} {{ else }}无{{ end }}
- 阻塞项：{{ range .issue.blocked_by }}{{ .identifier }}（{{ .state }}） {{ else }}无{{ end }}

工作要求：
1. 先理解标题、描述、标签和阻塞关系，确认任务可以安全推进。
2. 修改代码前优先补充或更新能够说明目标行为的验证。
3. 所有失败都要清晰暴露，不使用伪造结果，也不隐藏真实错误。
4. 面向用户或操作者的说明要自然、清晰、业务化，不描述运行环境、调度状态或实现细节。
5. 完成后给出已验证的命令和结果，并说明仍需人工关注的风险。

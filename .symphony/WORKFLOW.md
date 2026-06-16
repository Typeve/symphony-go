你将处理一个 Gitea issue。请只依据 issue 内容和当前仓库中的真实代码开展工作。

任务信息：
- 项目：{{ .project.id }}
- 任务：{{ .issue.identifier }}
- 标题：{{ .issue.title }}
- 描述：{{ .issue.description }}
- 链接：{{ .issue.url }}
- 标签：{{ range .issue.labels }}{{ . }} {{ else }}无{{ end }}
- 阻塞项：{{ range .issue.blocked_by }}{{ .identifier }}（{{ .state }}） {{ else }}无{{ end }}
- 工作区：{{ .workspace.path }}
- 预期分支：{{ .execution_branch }}

工作要求：
1. 先理解标题、描述、标签和阻塞关系，确认任务可以安全推进。
2. 修改代码前优先补充或更新能够说明目标行为的验证。
3. 只在当前工作区内修改文件，不处理 `.env`、私钥、日志、Codex 原始会话或其他敏感文件。
4. 不要提交、推送、创建 PR、合并分支或关闭 issue；这些由 Symphony 或人工处理。
5. 不要在输出、文件、日志或提示中写入 token、密钥、密码或授权头。
6. 所有失败都要清晰暴露，不使用伪造结果，也不隐藏真实错误。
7. 面向用户或操作者的说明要自然、清晰、业务化。
8. 不要修改或重启运行中的 Symphony 服务、systemd service 或部署配置；需要这类操作时，只说明建议的人工步骤。
9. 完成后给出实现摘要、建议 commit 信息、已验证的命令和结果，并说明仍需人工关注的风险。

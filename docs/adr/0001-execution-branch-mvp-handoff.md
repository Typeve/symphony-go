# Use execution branches as the MVP handoff

For the multi-project MVP, Symphony will not create PRs or merge code automatically. Symphony will orchestrate Codex implementation, run the configured reviewer command, then commit and push a stable execution branch only when that reviewer command exits successfully. This keeps Symphony as a deterministic orchestrator while leaving PR and merge decisions to humans.

## Considered Options

- Codex creates commits, pushes branches, and opens PRs directly.
- Symphony opens PRs as part of the MVP.
- Symphony publishes only a reviewer-approved execution branch and writes the result back to the issue.

## Consequences

The MVP has a smaller and safer surface area: Codex and reviewer processes do not receive Gitea push credentials, failed work is not pushed, and review workflow remains manual. A later milestone can add PR creation without changing the core boundary between Symphony, Codex, the reviewer command, and human review.

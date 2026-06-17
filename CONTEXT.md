# Go Symphony

Go Symphony manages issue-driven coding work across repositories. It coordinates projects, issues, workspaces, branches, and agent runs while keeping review and merge decisions visible to humans.

## Language

**Managed Project**:
A repository that Symphony is allowed to monitor and execute work for. Each **Managed Project** owns many **Task Issues**.
_Avoid_: tracker project, repo config, watched repo

**Task Issue**:
A tracker issue selected as work for an agent run. A **Task Issue** belongs to one **Managed Project** and may produce one **Execution Branch**.
_Avoid_: ticket, job, task when referring to the tracker record

**Task Issue Execution**:
The in-process module that runs one **Task Issue** from `symphony-running` through Agent Run, Review Gate, Execution Branch publish, and Done Handoff or failure marking.
_Avoid_: scheduler, worker, background job when referring to the per-issue execution sequence

**Execution Branch**:
A remote Git branch containing the work produced for one **Task Issue**. It is the MVP handoff artifact for human review.
_Avoid_: PR branch, temporary branch

**Done Handoff**:
The state where an **Execution Branch** has been pushed and the **Task Issue** is marked with `symphony-done`. It means the branch is ready for human review; it does not mean the work is merged or the issue is closed.
_Avoid_: merged, closed

**Completion Marker**:
A tracker-visible managed status label that prevents Symphony from dispatching the same **Task Issue** again after it has been claimed, completed, or failed. In the current MVP these labels are `symphony-running`, `symphony-done`, and `symphony-failed`.
_Avoid_: closed issue, merged status

**Agent Run**:
One Codex execution attempt inside an isolated workspace for a **Task Issue**.
_Avoid_: session when discussing product state

**Reviewer Command**:
A configured command that runs inside the workspace after an **Agent Run**. In the simplified MVP, exit status 0 means the review gate passed; any non-zero exit marks the **Task Issue** failed.
_Avoid_: structured review parser, verdict parser

**Review Gate**:
The rule that an **Execution Branch** is committed and pushed only after the **Reviewer Command** exits successfully. Failed work remains in the local workspace for human inspection.
_Avoid_: best-effort push, self-approved work

**Push Credential**:
The Gitea token that Symphony reads from the host environment only for tracker writes and Git push operations. It is never passed to Codex, the **Reviewer Command**, issue comments, logs, or Git remote configuration.
_Avoid_: agent token, workspace credential

**Agent Environment**:
The allowlisted environment variables passed to Codex or the **Reviewer Command**. It excludes **Push Credentials** even when those credentials are exported in the Symphony service environment.
_Avoid_: inherited server environment

## Flagged ambiguities

**Done** must not mean "merged" or "closed". In the current MVP, `symphony-done` only means the execution branch was pushed and is ready for human review.

**Review** in the simplified MVP means running the configured **Reviewer Command** and checking its exit status. Symphony does not parse structured review output in this version.

**Publishing** means pushing a remote **Execution Branch** only after the **Review Gate** passes. It does not include PR creation or merge.

**Branch naming** is deterministic but not ownership proof. If a remote branch collision or push conflict occurs, the current MVP fails the task and leaves resolution to a human.

**Credential access** is different for Symphony and agents. Symphony may read **Push Credentials** from the host environment, while Codex and the **Reviewer Command** receive only an **Agent Environment** that excludes those credentials.

## Example dialogue

Developer: "This Managed Project has an ai-ready Task Issue."

Operator: "Start an Agent Run in an isolated workspace. When it succeeds, push the Execution Branch and mark the issue `symphony-done`."

Developer: "Should Symphony create a PR?"

Operator: "Not in the MVP. The Execution Branch is the handoff artifact; a human decides whether to open a PR or merge."

Developer: "Who decides whether the branch is safe to publish?"

Operator: "The Reviewer Command must exit successfully. Symphony only pushes the Execution Branch after that Review Gate passes."

Developer: "What happens when review fails?"

Operator: "Do not push a remote branch. Keep the workspace as the failure scene, mark the Task Issue failed, and wait for human intervention."

Developer: "The server exports the Gitea token. Can agents read it?"

Operator: "No. Symphony uses the Push Credential for tracker writes and pushing the Execution Branch, but Codex and the Reviewer Command receive a credential-free Agent Environment."

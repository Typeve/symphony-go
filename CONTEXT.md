# Go Symphony

Go Symphony manages issue-driven coding work across repositories. It coordinates projects, issues, workspaces, branches, and agent runs while keeping review and merge decisions visible to humans.

## Language

**Managed Project**:
A repository that Symphony is allowed to monitor and execute work for. Each **Managed Project** owns many **Task Issues**.
_Avoid_: tracker project, repo config, watched repo

**Task Issue**:
A tracker issue selected as work for an agent run. A **Task Issue** belongs to one **Managed Project** and may produce one **Execution Branch**.
_Avoid_: ticket, job, task when referring to the tracker record

**Execution Branch**:
A remote Git branch containing the work produced for one **Task Issue**. It is the MVP handoff artifact for human review.
_Avoid_: PR branch, temporary branch

**Execution Metadata**:
A small repository file committed on an **Execution Branch** that identifies the **Managed Project**, **Task Issue**, and branch owner. Symphony uses it to decide whether an existing branch can be safely reused.
_Avoid_: hidden state, database record

**Branch Ownership Check**:
The check that combines the tracker comment trail and **Execution Metadata** to confirm an existing **Execution Branch** belongs to the same **Task Issue** before Symphony pushes updates to it.
_Avoid_: branch-name match

**Branch Ready**:
The state where an **Execution Branch** has been pushed and the **Task Issue** has enough information for a human to continue review. It does not mean the work is merged or the issue is closed.
_Avoid_: completed, done

**Completion Marker**:
A tracker-visible marker that prevents Symphony from dispatching the same **Task Issue** again after the intended handoff is reached.
_Avoid_: closed issue, merged status

**Agent Run**:
One Codex execution attempt inside an isolated workspace for a **Task Issue**.
_Avoid_: session when discussing product state

**External Validator**:
A separate agent or toolchain that reviews or verifies the workspace after an **Agent Run** before Symphony publishes the **Execution Branch**. Symphony may invoke it and record the result, but the validation judgment comes from this independent validator.
_Avoid_: Symphony validation, self-check

**Validation Verdict**:
The structured pass, fail, or blocked result produced by an **External Validator** for a **Task Issue**. A passing **Validation Verdict** is required before the MVP handoff can become **Branch Ready**.
_Avoid_: test log, agent summary

**Verdict File**:
The JSON file written by an **External Validator** that contains the **Validation Verdict**. Symphony reads this file for decisions; validator stdout and stderr are logs only.
_Avoid_: stdout verdict, free-form validation output

**Read-Only Guard**:
The protection that detects whether an **External Validator** changed the workspace. In the MVP this is a before-and-after Git diff check, not a full OS sandbox.
_Avoid_: trusted validator, implicit read-only

**Publish Gate**:
The rule that an **Execution Branch** is pushed only after a passing **Validation Verdict**. Failed or blocked work remains in the local workspace for human inspection instead of being published as a remote branch.
_Avoid_: debug branch, best-effort push

**Push Credential**:
The Gitea token that Symphony reads from the host environment only for tracker writes and Git push operations. It is never passed to Codex, an **External Validator**, issue comments, logs, or Git remote configuration.
_Avoid_: agent token, workspace credential

**Agent Environment**:
The allowlisted environment variables passed to Codex or an **External Validator**. It excludes **Push Credentials** even when those credentials are exported in the Symphony service environment.
_Avoid_: inherited server environment

## Flagged ambiguities

**Completed** must not mean both "branch is ready for review" and "work is merged". Use **Branch Ready** for the MVP handoff and reserve "completed" for a final workflow state only when that state is explicitly defined.

**Validation** must distinguish execution from judgment. Symphony can orchestrate a validation step, but the MVP validation judgment should come from an **External Validator** instead of Codex self-report or Symphony business logic.

**Publishing** means pushing a remote **Execution Branch** only after the **Publish Gate** passes. It does not include PR creation or merge.

**Validator output** must be a **Verdict File**. Natural-language stdout is not authoritative because it is hard to parse, hard to test, and risky to write back to issues.

**Validator read-only behavior** is enforced by the **Read-Only Guard** in the MVP. If the guard detects workspace changes, Symphony treats the validation as invalid and does not publish.

**Branch reuse** requires a **Branch Ownership Check**. A branch name match alone is not enough because a human-created branch could collide with Symphony's naming convention.

**Credential access** is different for Symphony and agents. Symphony may read **Push Credentials** from the host environment, while Codex and **External Validators** receive only an **Agent Environment** that excludes those credentials.

## Example dialogue

Developer: "This Managed Project has an ai-ready Task Issue."

Operator: "Start an Agent Run in an isolated workspace. When it succeeds, push the Execution Branch and mark the issue Branch Ready."

Developer: "Should Symphony create a PR?"

Operator: "Not in the MVP. The Execution Branch is the handoff artifact; a human decides whether to open a PR or merge."

Developer: "Who decides whether the branch is safe to publish?"

Operator: "An External Validator produces a Validation Verdict. Symphony records the verdict and only pushes the Execution Branch when the verdict passes."

Developer: "What happens when validation fails?"

Operator: "Do not push a remote branch. Keep the workspace as the failure scene, write the Validation Verdict to the Task Issue, and wait for human intervention after the allowed rework attempts are exhausted."

Developer: "The server exports the Gitea token. Can agents read it?"

Operator: "No. Symphony uses the Push Credential for tracker writes and pushing the Execution Branch, but Codex and External Validators receive a credential-free Agent Environment."

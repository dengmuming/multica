# MNL-002 ADR — Agent Native Loop orchestration boundaries

Status: Accepted for P0 planning

Branch: `feature/manifold-agent-native-loop`

## Context

MNL-001 found that Multica already provides several orchestration primitives we originally planned to build ourselves:

- parent/child Issues
- ordered `issue.stage` barrier groups
- agent/squad assignees
- Task/Run lifecycle and retries
- runtime daemon execution
- agent readiness and blocking semantics
- review/inbox/activity infrastructure
- Autopilot scheduling/triggering

The key architectural question is therefore whether Manifold P0 should build a separate workflow engine or compile an Agent Native Loop into Multica's existing issue/task execution model.

## Decision

For P0, Manifold Agent Native Loop will use a **Template Compiler + staged Issues + existing Task/Run execution** architecture.

We will NOT introduce independent `workflow_runs` / `stage_runs` execution tables in the first version.

The canonical runtime model is:

```text
Loop Template
    ↓ compile
Parent Issue (Loop Instance)
    ↓
Child Issues grouped by issue.stage
    ↓
Agent / Squad assignees
    ↓
Existing Task / Run
    ↓
Runtime daemon / agent CLI
```

The parent Issue acts as the P0 Mission / Loop Instance. Child Issues are executable nodes. `issue.stage` represents ordered barrier groups. Existing Task/Run records remain the canonical agent execution history.

## Why this option

This preserves existing Multica behavior for permissions, activity, comments, assignment, task execution, retry, runtime readiness, inbox, project context, search, board UI and audit history.

It avoids creating a second source of truth for execution state.

It also gives P0 deterministic fan-out/fan-in behavior through the existing stage barrier mechanism while keeping agent autonomy inside each issue/task.

## Rejected alternative: separate workflow engine

Rejected P0 shape:

```text
Mission
  ↓
Workflow Run
  ↓
Stage Run
  ↓
Issue
  ↓
Task / Run
```

Reasons for rejection in P0:

- duplicates Issue state and stage state
- duplicates retry/cancellation relationships
- creates reconciliation problems between Stage Run and Task/Run
- bypasses existing Multica parent/child stage barriers
- significantly increases persistence and recovery complexity before a real gap is proven

This option may be revisited later for cross-project, cross-repository, long-running workflows that cannot be represented as one parent Issue tree.

## 1. Loop Template compilation

A Loop Template is a versioned declarative definition. It is configuration, not an execution engine.

Example:

```yaml
name: feature-development
version: 1
stages:
  - key: product
    stage: 10
    role: product

  - key: architecture
    stage: 20
    role: architect

  - key: development
    stage: 30
    parallel:
      - role: backend
      - role: frontend
      - role: embedded
        optional: true

  - key: review
    stage: 40
    role: review

  - key: test
    stage: 50
    role: test

  - key: release_approval
    stage: 60
    approval: human

  - key: release
    stage: 70
    role: release
```

Compilation creates:

- one parent Issue containing loop metadata
- one child Issue per executable node
- child `stage` values matching barrier groups
- agent/squad assignees resolved from role bindings
- policy metadata that identifies template key/version and node key

The compiler must be idempotent. Re-running compilation for the same loop instance must not create duplicate children.

## 2. Canonical ownership

| Concern | Canonical owner in P0 |
| --- | --- |
| product intent / loop instance | Parent Issue |
| executable work node | Child Issue |
| ordered/parallel grouping | `issue.stage` |
| agent assignment | existing Issue assignee |
| single agent execution | existing Task/Run |
| task execution retry | existing Task retry path |
| runtime / CLI execution | existing Runtime/Daemon |
| loop template | new Manifold Template definition |
| structured test/review verdict | new Evaluation record |
| human approval | new Approval record |
| artifact/provenance links | new lightweight relation/artifact records if existing primitives are insufficient |

## 3. Task Retry vs Workflow Retry

These are separate concepts and MUST remain separate.

### Task Retry

Task Retry means the same Issue work attempt failed because the execution mechanism failed or became unusable.

Examples:

- agent CLI inactivity timeout
- transient runtime/network failure
- provider failure
- interrupted process
- session resume failure

Task Retry remains owned by existing Multica Task/Run logic. It may reuse workdir/session according to the existing retry semantics.

It does NOT change the logical workflow node and does NOT create a new development cycle.

```text
Backend Issue
  └ Task #1 failed operationally
       ↓
    Task Retry #2
       ↓
    same logical Backend Issue
```

### Workflow Retry

Workflow Retry means execution succeeded, but a later evaluator found the produced outcome unacceptable.

Example:

```text
Backend completed
Frontend completed
       ↓
Test Agent runs
       ↓
Evaluation = FAIL
owner = backend
reason = API returns 500
       ↓
Policy reopens / reactivates Backend node
       ↓
new Backend work attempt
       ↓
Test runs again
```

Workflow Retry is controlled by Manifold deterministic policy, not Task retry code.

The retry lineage must preserve:

- failing Evaluation
- target node / Issue
- prior successful Task/Run references
- new execution attempt
- retry reason
- retry count and configured max

P0 must not allow an unbounded validation loop.

## 4. Structured Evaluation

Review/Test/Security agents must not directly decide workflow transitions through free-form comments.

They produce validated structured Evaluation output.

Minimum P0 shape:

```json
{
  "verdict": "fail",
  "summary": "API validation failed",
  "findings": [
    {
      "severity": "high",
      "owner_role": "backend",
      "reason": "POST /licenses returns 500 for duplicate token"
    }
  ],
  "evidence": [
    {
      "type": "task_run",
      "ref": "..."
    }
  ]
}
```

The server validates this structure before any workflow transition.

An evaluator may report evidence and recommended ownership, but the server-side policy decides whether the target route is legal.

Agents cannot arbitrarily jump to unrelated stages.

## 5. Deterministic routing policy

P0 routing rules are server-owned and deterministic.

Example policy:

```text
Evaluation PASS
  → close Test Issue
  → allow next stage barrier to advance

Evaluation FAIL(owner=backend)
  → increment workflow retry count
  → reactivate Backend Issue
  → mark Test pending for re-run after fix

Evaluation FAIL(owner=frontend)
  → reactivate Frontend Issue

Retry count > limit
  → block parent loop
  → notify human inbox
```

The policy must reject unknown roles/node keys and illegal backward transitions.

## 6. Human Approval

Human Approval is NOT represented by:

- an agent comment
- a human comment
- an issue status alone
- an agent-generated `LGTM`

P0 introduces an explicit Approval record because the system needs a durable authenticated decision.

Minimum conceptual fields:

```text
id
workspace_id
parent_issue_id
node_issue_id
approval_key
state: pending/approved/rejected/cancelled
requested_from_member_id or required_role
requested_at
decided_by_member_id
decided_at
rationale
revision / idempotency key
```

Only an authenticated authorized human principal can transition `pending → approved|rejected`.

Agents and system actors cannot write an approval decision.

Approval state controls whether the following staged Issue can activate.

Release and production-impacting templates should require approval by default.

## 7. Parent Issue as Mission in P0

P0 does not add a separate `missions` table.

The parent Issue carries Loop Instance metadata such as:

```json
{
  "manifold_loop": {
    "template_key": "feature-development",
    "template_version": 1,
    "state": "running",
    "retry_count": 1
  }
}
```

Exact storage location is deferred to MNL-003: existing issue metadata may be sufficient, but queryability/indexing requirements must be checked before choosing JSON-only storage.

A separate Mission entity becomes justified when one business outcome must span multiple independent Issue trees/projects/workspaces or needs lifecycle semantics that no longer fit Issue ownership.

## 8. Stage activation model

Template compilation may create all child Issues up front, but only the first runnable barrier should be active.

Later stages should remain inert until policy permits them.

P0 should reuse existing Issue statuses rather than introduce a second stage-state machine.

Conceptually:

```text
future stage → backlog
ready stage  → todo / in-progress according to assignment path
completed    → done
failed evaluation target → reactivated to todo
blocked loop → parent/blocker state + inbox notification
```

Exact status transitions must respect existing custom status/category behavior.

## 9. Parent coordinator

The existing parent Issue wake mechanism is useful, but the P0 architecture must not depend on an LLM parent agent being the sole workflow authority.

The parent Agent/Squad may:

- summarize results
- prepare context
- make bounded planning suggestions
- create comments/status reports

But deterministic server policy owns legal stage activation, retry routing and approval gating.

This avoids planner-agent ping-pong and makes recovery/replay behavior predictable.

## 10. Idempotency

Every Manifold orchestration side effect must have an idempotency identity.

Required cases:

- template compilation
- evaluation processing
- workflow retry routing
- approval decision
- next-stage activation

Duplicate WebSocket/event delivery or repeated HTTP requests must not:

- duplicate child Issues
- create duplicate retries
- activate a stage twice
- consume an approval twice

## 11. Cancellation

P0 semantics:

- cancelling the parent Loop Issue prevents future stage activation
- queued/running Tasks should use existing cancellation behavior where available
- completed child Issues remain historical evidence
- cancellation must not delete evaluations/approvals/artifacts

A cancelled child counts as terminal for existing stage barrier behavior; Manifold policy must therefore distinguish deliberate loop cancellation from a normal successful barrier when deciding whether to activate the next stage.

## 12. Permissions

The Template Compiler and Policy layer cannot grant more authority than the underlying user/agent already has.

Starting a loop requires permission to create/assign the generated Issues.

Workflow retry must re-check agent access/readiness.

Approval requires an authenticated authorized human.

Release/production stages must not bypass existing runtime/agent permission boundaries.

## 13. New P0 components

After this ADR, the smallest expected new domain becomes:

1. **Loop Template** — versioned declarative definition
2. **Loop Compiler** — materializes parent/children/stages
3. **Loop Policy Service** — deterministic stage/routing rules
4. **Evaluation** — structured validator result
5. **Approval** — authenticated human gate
6. **Provenance/Artifact relation** — only where current Issue/Task/attachment relations are insufficient

Notably absent in P0:

- Mission table
- Workflow Run table
- Stage Run table
- separate scheduler
- separate agent queue
- separate execution runtime
- Kafka/NATS
- graph database

## 14. Open questions for MNL-003

MNL-003 must resolve these implementation details before migrations:

1. whether Loop Template definitions deserve tables or can begin as repo-backed/config records
2. whether parent Issue metadata is sufficiently queryable for loop state or needs a small indexed loop-instance table
3. whether existing attachment/source-context models can represent artifact lineage
4. exact Evaluation schema and indexing
5. exact Approval schema and authorization model
6. how to represent node attempt history without duplicating Task/Run
7. whether re-running Test reuses the same Test Issue with new Tasks or creates a new child Issue attempt
8. cleanup behavior when parent/child Issues are deleted

## Consequences

Positive:

- significantly smaller P0
- maximum reuse of Multica
- less dual-state synchronization
- native compatibility with existing UI/activity/runtime behavior
- faster internal pilot

Trade-offs:

- Issue tree becomes part of the workflow representation
- advanced arbitrary graph workflows are intentionally unsupported
- cross-project Mission support is deferred
- Evaluation/Approval still require new persistent concepts

## Decision checkpoint

Proceed to MNL-003 only under this assumption:

> Manifold P0 is a deterministic orchestration layer that compiles an engineering loop into Multica's existing work/execution primitives; it is not a second workflow/execution platform.

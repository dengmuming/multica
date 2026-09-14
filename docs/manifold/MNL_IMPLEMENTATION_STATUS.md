# Manifold Agent Native Loop — Implementation Status

> Branch: `feature/manifold-agent-native-loop`  
> Scope: P0 `feature-development` vertical slice  
> Updated: 2026-09-14

## 1. Architecture baseline

P0 deliberately extends Multica instead of creating a parallel workflow/runtime stack:

- parent Issue = Loop instance
- child Issue = Loop node
- `issue.stage` = sequential/parallel barrier
- Agent / Squad / Member = role binding
- `agent_task_queue` = execution attempt/history
- Runtime/Daemon = existing Multica execution plane
- new Manifold persistence only where Multica has no equivalent:
  - Loop Template
  - Evaluation
  - Approval
  - Artifact / provenance
- deterministic server policy owns legal transitions

No `mission`, `workflow_run`, or `stage_run` table is introduced in P0.

Naming is fixed:

- **Manifold Agent** = product
- **Manifold Agent Native Loop** = lifecycle/orchestration architecture
- **Multica** = current workforce/control-plane foundation

## 2. Implemented

### Persistence source

Migrations and SQL source are committed for:

- `loop_template`
- `loop_evaluation`
- `loop_approval`
- `loop_artifact`
- concurrent indexes `469`–`480`
- `481_loop_instance_key_uq`

`481` enforces one Loop instance key per `(workspace_id, project_id)` through the parent Issue metadata expression index.

All MNL concurrent indexes are registered with Multica's invalid-index cleanup hook.

SQLC sources:

- `loop_template.sql`
- `loop_evaluation.sql`
- `loop_approval.sql`
- `loop_artifact.sql`
- `loop_issue.sql`

### Template / Compiler Core

Implemented `server/internal/looptemplate`:

- P0 schema types
- template validation
- role-binding validation
- optional/required role behavior
- legal recovery-target validation
- deterministic `Compile()`
- same-stage parallelism
- initial `todo` / later `backlog`
- server-owned Approval nodes

### Structured Evaluation Core

Implemented `server/internal/loopevaluation`:

- exact evaluation kind
- verdict allowlist
- score validation
- structured findings/evidence
- owner role/node consistency
- legal deterministic recovery target
- fallback routing
- P0 rejection of ambiguous multi-target recovery

### Deterministic Policy Core

Implemented `server/internal/looppolicy`:

- sequential stage advancement
- parallel barriers
- Evaluation FAIL recovery
- downstream re-parking
- workflow retry budgets
- Approval request/rejection recovery
- blocked/completed terminal handling
- stale gate protection
- idempotent recovery-in-progress behavior

### Loop instantiation application layer

Implemented:

- exact template-version preflight
- workspace-scoped role bindings
- deterministic compile
- instance-key preflight
- creator identity propagation (`member` or `agent`)
- `CompiledPlan → Parent/Child Issue graph`
- Parent/Child metadata mapping
- Approval nodes remain server-owned

### Concrete Issue graph persistence

Implemented `MulticaIssueGraphStore` using existing generated Multica Issue queries rather than waiting for new Loop SQLC output.

The transaction:

1. validates workspace/project;
2. allocates ordinary Multica Issue numbers;
3. allocates ordinary column positions;
4. creates Parent Issue;
5. writes `manifold.loop.instance_key` first so concurrent requests hit the DB unique constraint early;
6. writes the remaining Parent metadata;
7. creates all Child Issues with parent/stage/assignee/creator semantics;
8. writes Child metadata;
9. commits the complete graph.

No Task is inserted inside this transaction.

### Canonical dispatch adapter

Implemented explicit dispatch of already-committed Agent/Squad Issues through `IssueService`:

`Issue → AgentReadiness → TaskService → Runtime/Daemon`

Properties:

- no direct `agent_task_queue` inserts
- Agent/Squad readiness rechecked at dispatch time
- archived Squad rejected
- pending Squad work treated idempotently
- post-commit dispatch failure surfaced as a recoverable condition

### Binding scope adapter

Implemented real workspace validation for:

- `agent → agent.id`
- `squad → squad.id`
- `member → user.id`

Archived Agent/Squad bindings are rejected.

### Policy Application Service

Implemented:

- `PolicyProjectionReader`
- pure `looppolicy.Evaluate()` invocation
- action validation
- `PolicyMutationSink`
- no-op ticks without writes
- post-commit dispatch semantics

### Concrete Policy projection reader

Implemented `MulticaPolicyProjectionReader`.

It reconstructs a running Loop from durable state:

```text
Parent Issue metadata
  → pinned template key/version/policy version
Child Issues
  → node key / role / assignee / status / retry count
actual Child assignees
  → RoleBindings
pinned immutable template
  → Compile()
Evaluation/Approval repository
  → gate state
```

A running Loop therefore never silently switches to a newly published template version.

The projection also captures Parent and Child Issue revisions for stale-decision protection.

### Concrete Policy mutation store

Implemented `MulticaPolicyMutationStore` using existing Issue SQLC operations.

Before mutation it:

1. locks Parent Issue;
2. verifies pinned `policy_version`;
3. verifies expected Parent revision;
4. locks every referenced Child Issue;
5. verifies Parent/Child lineage and `node_key` metadata;
6. verifies expected Child revisions;
7. only then applies mutations.

Supported durable mutations:

- activate node → `backlog → todo`
- reopen node → increment workflow retry + `todo`
- park node → `backlog`
- request approval → activate Approval Issue + transactional approval hook
- set Parent loop state
- block Loop
- complete Loop and mark Parent Issue done

Runnable Agent/Squad nodes are returned as post-commit dispatch requests. Tasks are never created inside the mutation transaction.

Important recovery invariant fixed during concrete wiring:

> Approval Issue must leave `backlog` when approval is requested; otherwise a later rejected Approval would look like stale evidence and be ignored by recovery policy.

### Stale projection guard

`PolicyProjection` now carries Issue revision snapshots into `PolicyMutationBatch`.

If another Task/policy/user mutation changes a referenced Issue between projection and write, the transaction rejects the decision with `ErrStalePolicyProjection` instead of re-opening or parking newer state.

### Evaluation Application Service

Implemented:

`Task lineage → server-owned node/template context → structured validation → persistence → Policy Tick`

The caller cannot choose authoritative node/template lineage independently of the Task.

If Evaluation persistence succeeds but Policy Tick fails, the durable Evaluation is returned so retry continues control-plane advancement rather than creating a second authoritative result.

### Human Approval Application Service

Implemented:

- pending approval lookup
- `approved` / `rejected` only
- authenticated human `decided_by`
- requested approver enforcement when configured
- persistence then Policy Tick
- retry-safe partial-success semantics

### HTTP contract

Implemented `server/internal/loophttp`:

```text
POST /api/loops
POST /api/tasks/{taskId}/loop-evaluation
POST /api/loop-approvals/{approvalId}/decision
```

Security semantics:

- Loop creation uses authenticated workspace context and trusted creator identity.
- Task-token Evaluation requires server-stamped `X-Task-ID` to match URL Task ID.
- Task-token Evaluation actor is server-stamped `X-Agent-ID`.
- Human Evaluation/Approval uses authenticated `X-User-ID`.
- Approval rejects machine actors.
- unknown body fields are rejected.
- duplicate Loop instance → `409`.
- persisted Evaluation/Approval with failed Policy Tick → `202 Accepted`.

The routes are intentionally not mounted in production until all persistence dependencies are available.

## 3. Still reused from Multica

Do not create parallel implementations for:

- Issue model
- Task queue / Run history
- Task retry
- Runtime/Daemon selection
- Agent/Squad identity
- Skills
- execution logs
- cancellation
- Inbox transport
- VCS integration

Task Retry and Workflow Retry remain separate:

```text
Task Retry
  transient CLI/runtime/transport failure
  → existing Multica retry lineage

Workflow Retry
  successful execution but Review/Test/Approval rejects result
  → Manifold Policy reopens responsible Issue
  → canonical Multica dispatch creates a new Task
```

## 4. Current hard blocker: SQLC generation

New Loop query source files are committed, but the corresponding generated Go files are still absent.

In particular, the branch does not yet contain generated files such as:

- `loop_template.sql.go`
- `loop_evaluation.sql.go`
- `loop_approval.sql.go`
- `loop_artifact.sql.go`
- `loop_issue.sql.go`

Run in an environment that can resolve the pinned generator:

```bash
make sqlc
```

Do not manually edit files marked `Code generated by sqlc`.

Because Issue graph and Policy Issue mutations now reuse existing generated Multica queries, this blocker has been narrowed to the new Loop tables rather than the whole runtime path.

## 5. Remaining backend work

### A. Generate SQLC output

Run `make sqlc`, review generated changes, and commit them.

### B. Template repository

Implement concrete SQLC adapter for:

- exact version load
- active version load
- draft create/update
- publish immutable version
- archive prior active version

This adapter will satisfy `PinnedLoopTemplateLoader`.

### C. Evaluation / Approval / gate-state repositories

Using generated Loop SQLC:

- Task/Issue/Loop lineage lookup
- idempotent Evaluation persistence
- latest Evaluation projection
- pending/decided Approval projection
- transactional `EnsurePendingApproval`
- Approval decision persistence
- evidence/artifact authorization

These adapters will satisfy `GateStateProjectionLoader` and `TransactionalApprovalRequester`.

### D. Inbox integration

On first pending Approval creation, create one human-attention Inbox item. Do not duplicate on policy retries.

### E. Production router wiring

Construct:

```text
Template repository
        ↓
InstantiateService
        ↓
MulticaIssueGraphStore
        ↓
DurableIssueGraphGateway

PolicyProjectionReader
        ↓
PolicyApplicationService
        ↓
MulticaPolicyMutationStore
        ↓
DurablePolicyMutationSink

EvaluationApplicationService
ApprovalApplicationService
        ↓
loophttp.Handler
```

Mount inside the existing authenticated `RequireWorkspaceMember` group.

### F. Automatic policy tick trigger

Evaluation and Approval already trigger Policy Tick explicitly. Ordinary Agent-node completion also needs a Manifold hook so Product → Architecture → Dev stage advancement does not depend on a manual HTTP call.

The hook should recognize `manifold.loop.node_key` metadata on a completed child Issue and enqueue/reconcile a Policy Tick for its Parent Issue after the Task/Issue completion transaction commits.

## 6. Frontend / pilot

### MNL-011 — Loop UI

Web/Desktop:

- Loop list/detail
- stage graph/timeline
- current stage
- node Agent/Task state
- Evaluation findings
- Approval card
- workflow recovery history

### MNL-012 — internal pilot

Recommended first real pilot: MindCloudX token licensing or Odin safety workflow.

Measure separately:

- requirement → release lead time
- first-pass Review/Test rate
- workflow retry count
- human intervention count
- transient Task retry count
- total Agent cost

## 7. P0 done condition

```text
Feature Intent
  ↓
Product
  ↓
Architecture
  ↓
Backend / Frontend / optional Embedded
  ↓
Review
  ↓
Test
  ├─ FAIL → deterministic recovery → Review → Test
  └─ PASS
       ↓
Human Approval
       ├─ Reject → deterministic recovery
       └─ Approve
            ↓
Release
            ↓
Loop Completed
```

P0 is done only when this path runs against real Multica Issues/Tasks with deterministic routing, bounded workflow retry, explicit human authority, provenance, and existing Runtime/Daemon semantics.

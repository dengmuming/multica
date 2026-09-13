# Manifold Agent Native Loop — Implementation Status

> Branch: `feature/manifold-agent-native-loop`  
> Scope: P0 `feature-development` vertical slice  
> Updated: 2026-09-13

## 1. Current architecture

The implementation follows the reduced-infrastructure direction established by MNL-001 through MNL-004:

- parent Issue = loop instance
- child Issue = loop node
- `issue.stage` = sequential/parallel barrier
- Agent / Squad / Member = role binding
- `agent_task_queue` = execution attempt/history
- Runtime/Daemon = existing Multica execution plane
- new Manifold state exists only where Multica has no equivalent:
  - Loop Template
  - Evaluation
  - Approval
  - Artifact / provenance
- deterministic server policy, not an Agent, owns legal workflow transitions

No `mission`, `workflow_run`, or `stage_run` table is introduced in P0.

Product naming is also fixed:

- **Manifold Agent** = product
- **Manifold Agent Native Loop** = lifecycle/orchestration architecture
- **Multica** = current workforce/control-plane foundation

---

## 2. Implemented

### MNL-005A — persistence source

Implemented schema migrations:

- `468_manifold_loop_persistence`
  - `loop_template`
  - `loop_evaluation`
  - `loop_approval`
  - `loop_artifact`
- `469`–`480`
  - one concurrent index per migration
  - matching rollback migration for each index
  - active-template uniqueness
  - template version uniqueness
  - evaluation parent/node/task lookups
  - approval pending uniqueness / assignee lookup
  - artifact parent/node/task lookups

All MNL concurrent index migrations are registered with Multica's invalid-index cleanup hook so interrupted `CREATE INDEX CONCURRENTLY` builds can be safely retried.

Implemented SQLC query sources:

- `pkg/db/queries/loop_template.sql`
- `pkg/db/queries/loop_evaluation.sql`
- `pkg/db/queries/loop_approval.sql`
- `pkg/db/queries/loop_artifact.sql`

### MNL-005B — template domain and validation

Implemented `server/internal/looptemplate`:

- canonical P0 template types
- role definitions / role bindings
- Agent / Evaluation / Approval node types
- evaluation and approval config types
- schema validation
- binding validation
- legal recovery-target validation
- required/optional role behavior
- P0 approval constraints
- unit tests

### Compiler Core

Implemented a side-effect-free compiler:

```text
Loop Template + Role Bindings
        ↓
Validation
        ↓
CompiledPlan
        ↓
ordered CompiledNodes
```

Compiler behavior:

- deterministic stage/key ordering
- omitted optional role-bound nodes when no binding exists
- required binding enforcement
- same-stage parallel structure preserved
- minimum included stage starts `todo`
- later stages start `backlog`
- Approval nodes remain server-owned and do not require Agent binding

The compiler deliberately does not allocate database IDs or write Issues.

### Structured Evaluation boundary

Implemented `server/internal/loopevaluation`.

Agent output is accepted by workflow policy only after validating:

- exact evaluation kind
- allowed verdict
- score range
- finding code/severity/summary
- owner node / owner role
- owner role/node consistency
- legal non-forward recovery target
- deterministic fallback target

P0 rejects multiple distinct recovery targets in one evaluation. This avoids silently allowing one Agent response to create an unreviewed arbitrary graph mutation. Multi-target fan-out can be added after the first pilot.

### Deterministic Policy Core

Implemented `server/internal/looppolicy`.

Pure input:

```text
CompiledPlan
+ child Issue state projection
+ latest validated Evaluation state
+ Approval state
```

Pure output:

```text
Action[]
```

Supported policy actions:

- activate node
- reopen node
- park downstream node
- request approval
- set parent state
- block loop
- complete loop

Covered transitions include:

- sequential stage advancement
- parallel barrier advancement
- Evaluation FAIL → responsible node recovery
- downstream re-park after recovery
- workflow retry budget exhaustion → blocked
- Human Approval request
- Approval rejection → recovery
- successful Release → complete loop
- idempotency guard while recovery target is already active
- stale failed Evaluation ignored after its gate is parked for a new workflow attempt

### MNL-006/007 — instantiate application layer and Issue graph boundary

Implemented `server/internal/loopservice` application boundaries for loop creation:

- exact Template version preflight
- role-binding validation
- workspace-scope validation hook
- deterministic compile
- instance-key duplicate preflight
- write-time instance-key idempotency requirement
- explicit `LoopInstanceWriter`
- `CompiledPlan → Parent/Child Issue graph` mapping
- node/stage/assignee/metadata mapping
- Approval node kept server-owned rather than encoded as an approval-by-assignment shortcut

Parent metadata includes pinned template, policy and instance identity. Child metadata includes node identity/type/role/retry information.

The Issue graph gateway contract requires:

1. create the complete Parent/Child graph atomically;
2. commit it before execution begins;
3. dispatch only runnable first-stage Agent/Squad nodes through Multica's canonical Issue→Task path;
4. never direct-insert a parallel Task/Run model;
5. expose initial dispatch failure as a recoverable post-commit condition rather than pretending the graph was never created.

### MNL-008 — Policy Application Service

Implemented `PolicyApplicationService` with:

- authoritative `PolicyProjectionReader`
- pure `looppolicy.Evaluate()` invocation
- action validation against the compiled plan and persisted node→Issue mapping
- one `PolicyMutationSink` boundary per deterministic decision
- no-op ticks that do not write
- idempotency/stale-projection requirements documented at the mutation boundary

This is the server-owned control loop used after node completion, Evaluation and Approval decisions.

### MNL-009 — Evaluation Application Service

Implemented `EvaluationApplicationService`:

```text
Task ID
  ↓
server-owned Task/Issue/Loop lineage lookup
  ↓
structured Evaluation validation
  ↓
artifact/evidence authorization
  ↓
idempotent Evaluation persistence
  ↓
Policy Tick
```

Security properties:

- caller cannot choose the authoritative node or pinned template version independently of the Task
- Agent prose is not a transition command
- recovery target is deterministic structured data
- evidence authorization is a separate workspace-aware boundary
- if Evaluation persistence succeeds but policy application fails, the durable Evaluation is returned so a retry can continue policy rather than create a second authoritative result

### MNL-010 — Human Approval Application Service

Implemented `ApprovalApplicationService`:

- authoritative pending-approval context lookup
- only `approved` / `rejected` decisions
- `decided_by` comes from authenticated member identity
- requested approver is enforced when configured
- persisted decision followed by Policy Tick
- retry-safe behavior when approval committed but policy application failed

A request body can never supply/forge `decided_by`.

### HTTP application contract

Implemented `server/internal/loophttp` as an adapter over the application services, without importing the not-yet-generated loop SQLC code.

Current contract:

```text
POST /api/loops
POST /api/tasks/{taskId}/loop-evaluation
POST /api/loop-approvals/{approvalId}/decision
```

HTTP/security semantics:

- Loop creation consumes the authenticated workspace context and must be mounted under Multica's existing `RequireWorkspaceMember` group.
- Evaluation authenticated by an Agent task token uses the server-stamped `X-Agent-ID` and requires server-stamped `X-Task-ID` to equal the URL Task ID.
- Human Evaluation uses authenticated `X-User-ID`.
- Approval rejects `task_token` and `cloud_pat` machine actors and uses only authenticated `X-User-ID` as the decision actor.
- unknown JSON fields are rejected, so `decided_by` spoofing is not silently ignored.
- duplicate Loop instance → `409 Conflict`.
- newly created Evaluation → `201 Created`; idempotent existing result → `200 OK`.
- Evaluation/Approval already persisted but follow-up Policy Tick failed → `202 Accepted` with the durable resource and a warning. This explicitly means "the engineering decision is stored; control-plane advancement remains retryable."

Unit tests cover actor boundaries, task-token cross-task rejection, workspace propagation, identity spoofing, duplicate creation and partial-success HTTP semantics.

The adapter is intentionally not mounted in the production router until concrete DB/application dependencies are available. When wired, all three routes belong inside the existing authenticated workspace-member group so workspace headers are never trusted without Multica's membership guard.

---

## 3. Still intentionally reused from Multica

Do not add parallel implementations for:

- issue/task queue
- Task/Run retry
- daemon/runtime selection
- Agent/Squad identity
- Skills
- execution logs
- task cancellation
- issue activity/comments
- Inbox notification transport
- VCS integration

Task retry and workflow retry remain separate:

```text
Task Retry
  transient runtime / CLI / transport execution failure
  -> existing Multica retry lineage

Workflow Retry
  successful execution but Review/Test/Approval rejects the result
  -> Manifold Policy reopens an Issue
  -> normal Multica dispatch creates a new Task
```

---

## 4. Required SQLC generation step

The new SQLC source files are committed, but generated Go files are not manually edited.

Run in an environment that can resolve/download the pinned SQLC module:

```bash
make sqlc
```

The repository target invokes the pinned generator from `server/sqlc.yaml` and should produce/update:

- `server/pkg/db/generated/models.go`
- `server/pkg/db/generated/querier.go`
- generated query files for the four new loop query sources

Generated files should be reviewed and committed before concrete loop repositories import the new query methods.

This remains a hard boundary: do not hand-maintain code marked `Code generated by sqlc` merely to bypass an unavailable generator.

---

## 5. Remaining backend integration batch

The workflow/domain/application logic is now substantially implemented. Remaining P0 backend work is mostly Multica infrastructure wiring.

### A. Generate SQLC code

Run `make sqlc` and commit generated query/model updates.

### B. Template repository/service

Implement DB-backed:

- create/update draft
- publish immutable version
- archive previous active version transactionally
- list/get active/exact version

### C. Canonical Issue graph store + dispatch adapter

Implement concrete adapters for:

- atomic Parent/Child Issue creation
- issue numbering / position allocation using existing Multica semantics
- initial activity/provenance writes
- post-commit canonical Agent/Squad dispatch
- instance-key race/idempotency enforcement

### D. Policy projection + mutation adapters

`PolicyProjectionReader`:

- parent pinned metadata
- child Issue states/retry metadata
- latest authoritative Evaluation
- pending/decided Approval

`PolicyMutationSink`:

- activate/reopen/park through canonical Issue semantics
- increment workflow retry metadata only for workflow recovery
- create pending `loop_approval` and Inbox item once
- block/complete parent idempotently
- dispatch newly activated Agent/Squad nodes after durable state mutation

### E. Evaluation / Approval repositories

Implement lineage/evidence/persistence adapters using generated loop queries and existing Task/Issue/Attachment data.

### F. Mount HTTP routes

Inject the completed application services into `loophttp.Handler` and register it inside the existing authenticated `RequireWorkspaceMember` route group.

---

## 6. Frontend / pilot after backend wiring

### MNL-011 — Loop UI

Web/Desktop:

- loop list
- execution graph/timeline
- current stage
- per-node Agent/Task status
- Evaluation findings
- Approval card
- recovery/retry history

### MNL-012 — internal pilot

Use one real Manifold feature (recommended: MindCloudX token licensing or Odin safety workflow) and measure:

- requirement → release lead time
- first-pass Review/Test rate
- workflow retry count
- human intervention count
- task/runtime failure retry count separately
- total agent cost

---

## 7. Definition of P0 done

P0 is complete only when a user can create one feature intent and observe this real closed loop:

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
  ├─ FAIL → deterministic developer recovery → Review → Test
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

The acceptance criterion is not “multiple Agents can run.”

The acceptance criterion is: **a validated engineering result can move through the complete loop with deterministic routing, bounded retry, explicit human authority, provenance, and existing Multica runtime semantics.**

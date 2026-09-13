# Manifold Agent Native Loop — Implementation Status

> Branch: `feature/manifold-agent-native-loop`  
> Scope: P0 `feature-development` vertical slice  
> Updated: 2026-09-13

## 1. Current architecture

The implementation now follows the reduced-infrastructure direction established by MNL-001 through MNL-004:

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

Database/workspace authorization of Task, Issue and artifact references remains an application-service responsibility.

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
- stale failed Evaluation is ignored after its gate is parked for a new workflow attempt

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

## 4. Required generation step before DB-backed handlers

The new SQLC source files are committed, but generated Go files are not manually edited.

Run in an environment that can resolve/download the pinned SQLC module:

```bash
make sqlc
```

The repository target invokes the pinned generator from `server/sqlc.yaml` and should produce/update:

- `server/pkg/db/generated/models.go`
- `server/pkg/db/generated/querier.go`
- generated query files for the four new loop query sources

Generated files should be reviewed and committed before handlers/services start importing the new queries.

This is intentionally a hard boundary: do not hand-maintain code marked `Code generated by sqlc` merely to bypass an unavailable generator.

---

## 5. Next implementation batch

### MNL-006 — Template API + service

After `make sqlc`:

1. Template service
   - create draft
   - update draft
   - publish immutable version
   - archive previous active version transactionally
   - list/get active/exact version
2. HTTP API
3. permission checks
4. API tests

### MNL-007 — Issue persistence adapter / Loop instantiate API

Use Compiler Core output to atomically create:

- parent orchestration Issue
- included child Issues
- exact `stage`
- resolved assignees
- loop metadata bindings

Preflight all workspace-scoped Agent/Squad/Member bindings before the first write.

### MNL-008 — Policy application service

Bridge pure Policy Actions to idempotent Multica mutations:

- `activate_node` → backlog → todo through normal Issue update/dispatch path
- `reopen_node` → reopen target + increment workflow retry metadata
- `park_node` → reset downstream node to backlog
- `request_approval` → create `loop_approval` + Inbox item
- `block_loop` → parent metadata/state + Inbox escalation
- `complete_loop` → close parent Issue

### MNL-009 — Evaluation API

- prove Task belongs to the Evaluation child Issue
- validate submitted structured payload with `loopevaluation`
- prove artifact visibility
- persist `loop_evaluation`
- invoke policy
- persist policy action/target for audit

### MNL-010 — Approval API + Inbox

- explicit member-authenticated approve/reject
- no Agent approval
- no comment-as-approval
- decision audit actor and rationale
- invoke policy after decision

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

## 6. Definition of P0 done

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

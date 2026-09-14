# Manifold Agent Native Loop — Implementation Status

> Branch: `feature/manifold-agent-native-loop`  
> Scope: P0 `feature-development` vertical slice  
> Updated: 2026-09-14

## 1. Architecture baseline

P0 extends Multica rather than creating a parallel workflow/runtime stack:

- parent Issue = Loop instance
- child Issue = Loop node
- `issue.stage` = ordered/parallel barrier
- Agent / Squad / Member = role binding
- `agent_task_queue` = execution attempts
- Runtime/Daemon = existing Multica execution plane
- Manifold-specific durable state only where Multica has no equivalent:
  - Loop Template
  - Evaluation
  - Approval
  - Artifact / provenance
- deterministic server policy owns legal transitions

No `mission`, `workflow_run`, or `stage_run` table exists in P0.

Naming is fixed:

- **Manifold Agent** = product
- **Manifold Agent Native Loop** = lifecycle/orchestration architecture
- **Multica** = workforce/control-plane foundation

## 2. Implemented core

### Template / Compiler

`server/internal/looptemplate` implements:

- template schema + validation
- role definitions / role bindings
- Agent / Evaluation / Approval nodes
- optional vs required role behavior
- legal recovery targets
- deterministic compile
- same-stage parallelism
- first included stage `todo`, later stages `backlog`
- server-owned Approval gates

### Structured Evaluation

`server/internal/loopevaluation` validates:

- exact evaluation kind
- verdict allowlist
- score range
- finding structure/severity
- owner role/node consistency
- deterministic recovery target
- fallback routing
- no ambiguous multi-target recovery in P0

### Deterministic Policy

`server/internal/looppolicy` implements:

- sequential advancement
- parallel barriers
- Evaluation FAIL recovery
- downstream parking
- bounded workflow retries
- Approval request/rejection recovery
- stale gate protection
- recovery-in-progress idempotency
- blocked/completed/cancelled/failed terminal handling

`blocked` is explicitly terminal, preventing repeated `block_loop` decisions on every Tick.

## 3. Persistence and concurrency controls

### Loop tables

Migration 468 creates:

- `loop_template`
- `loop_evaluation`
- `loop_approval`
- `loop_artifact`

Indexes 469–480 cover template/evaluation/approval/artifact access patterns.

### Loop instance uniqueness

Migration 481 adds:

```text
idx_issue_manifold_loop_instance_key
(workspace_id, project_id, metadata->>'manifold.loop.instance_key')
```

This makes Parent Issue metadata the database-level Loop-instance identity and closes the concurrent-create race.

### Policy version alignment

Migration 482 changes Evaluation/Approval `policy_version` from integer to TEXT because the application contract treats policy version as an opaque pinned identifier such as `policy-v1`.

The migration drops the old numeric CHECK constraints before the type change.

### Evaluation uniqueness

Migration 483 adds:

```text
idx_loop_evaluation_one_per_task
```

A canonical Task can therefore own at most one authoritative Evaluation result.

Every MNL `CREATE INDEX CONCURRENTLY` migration is registered with Multica's invalid-index cleanup hook.

## 4. Concrete Loop creation path

Implemented:

```text
POST /api/loops
  ↓
InstantiateService
  ↓
exact published Template version
  ↓
workspace-scoped Role Bindings
  ↓
Compile
  ↓
MulticaIssueGraphStore
  ↓
atomic Parent + Child Issues
  ↓ commit
MulticaInitialIssueDispatcher
  ↓
IssueService → AgentReadiness → TaskService → Runtime/Daemon
```

Properties:

- trusted creator identity is propagated to Parent and every Child
- Parent/Child issue graph is committed before any Task is created
- Issue number/position/project/creator/stage semantics reuse Multica
- `instance_key` is written first so concurrent losers fail early at the unique index
- Agent/Squad dispatch reuses existing runtime/task semantics
- archived Agent/Squad bindings are rejected
- dispatch failures after graph commit are recoverable and must not create a second graph

## 5. Concrete policy projection and mutation

### Projection

`MulticaPolicyProjectionReader` reconstructs current state from:

- Parent pinned metadata
- actual Child Issues
- actual persisted role bindings
- exact pinned Template
- current-attempt Evaluation/Approval evidence
- Parent/Child revisions

The exact pinned template is recompiled from durable bindings; a running Loop never silently moves to a newly published template version.

`RawLoopStateRepository` provides temporary raw-PGX access for pinned templates and gate evidence until generated Loop SQLC files are available.

Evaluation evidence is attempt-scoped to the newest Task for that Evaluation Issue. Approval evidence from a parked gate is hidden from the current attempt.

### Mutation

`MulticaPolicyMutationStore`:

1. locks Parent Issue;
2. validates pinned policy version + expected Parent revision;
3. locks all referenced Child Issues;
4. validates Parent/Child/node-key lineage + expected revisions;
5. applies all policy mutations atomically;
6. commits;
7. returns runnable Agent/Squad nodes for canonical post-commit dispatch.

Supported mutations:

- activate node
- reopen node + increment workflow retry once
- park node
- request human approval
- set Parent state
- block Loop
- complete Loop + mark Parent Issue done

Task creation never occurs inside this transaction.

## 6. Concrete Evaluation path

Implemented `RawEvaluationRepository`:

- Task → Issue → Parent lineage resolution
- rejects non-Loop child Tasks
- rejects stale Tasks that are no longer the newest Task for the node
- resolves current pinned Loop projection
- verifies node→Issue mapping
- persists normalized Evaluation
- database uniqueness enforces one authoritative Evaluation per Task
- duplicate delivery is accepted only when authoritative fields match exactly

Implemented `RawEvaluationEvidenceAuthorizer`:

- every Evidence / Finding artifact ref must resolve to `loop_artifact`
- same workspace + Parent Loop required
- accepts registered artifact id, ref_id, or ref_uri
- arbitrary unregistered URLs are not trusted as evidence

Evaluation persistence is followed by the same deterministic Policy Tick.

## 7. Concrete Human Approval path

Implemented `MulticaApprovalRequester`:

- pending Approval inserted inside the same transaction as gate activation
- one pending record per node/key
- repeat Policy Tick does not duplicate Approval or Inbox
- Parent human creator is P0 fallback requested approver
- `loop_approval_required` Inbox item is created atomically

Implemented `MulticaApprovalRepository`:

- loads authoritative Approval/Issue lineage
- checks Approval key against node metadata
- pending → approved/rejected only
- `decided_by` is authenticated human identity
- exact workspace/parent/node/key/policy lineage
- exact idempotent replay only
- marks Approval Issue done
- archives matching Approval Inbox item in the same transaction

Approval decision is then followed by Policy Tick.

## 8. Template administration

Implemented `TemplateAdminService` + `RawTemplateAdminRepository`:

- create draft
- update draft
- publish draft
- deterministic Template validation before persistence
- advisory lock per `(workspace, template_key)`
- serialized version allocation
- atomic archive-current-active + publish-new-active

HTTP contract:

```text
POST /api/loop-templates
PUT  /api/loop-templates/{templateKey}/versions/{version}
POST /api/loop-templates/{templateKey}/versions/{version}/publish
```

Template administration is human-only.

## 9. Automatic Loop advancement

Evaluation and Approval already invoke Policy Tick directly.

Normal Product/Architecture/Backend/Frontend/Release nodes now also have an event-driven advancement bridge:

```text
issue:updated(status=done) ─┐
                            ├→ verify Manifold child metadata
 task:completed ────────────┘
                                  ↓
                         PolicyApplicationService.Tick(parent)
```

This uses Multica's existing synchronous post-commit `events.Bus`.

Both events are intentionally observed because `CompleteTask()` does not itself mark the Issue done; Agents manage Issue status through the CLI. Policy Tick is idempotent, so whichever event arrives first may no-op and the other safely retries progression.

## 10. HTTP application contract

Implemented:

```text
POST /api/loops
POST /api/tasks/{taskId}/loop-evaluation
POST /api/loop-approvals/{approvalId}/decision
```

Security properties:

- trusted workspace/actor headers come from existing Auth middleware
- task token Evaluation must match its server-stamped Task ID
- task token evaluator identity is server-stamped Agent ID
- Human Approval rejects machine actors
- unknown JSON fields are rejected
- duplicate Loop → 409
- durable Evaluation/Approval with subsequent Policy Tick failure → 202 Accepted

`server/cmd/server/manifold_agent_routes.go` now assembles the concrete production dependency graph, including Template admin, Loop creation, Policy projection/mutation, Evaluation, Approval, evidence authorization and lifecycle event advancement.

## 11. SQLC status

Committed source files exist for:

- `loop_template.sql`
- `loop_evaluation.sql`
- `loop_approval.sql`
- `loop_artifact.sql`
- `loop_issue.sql`

Generated Loop Go files are still absent because this execution environment cannot resolve/download the pinned SQLC generator.

Run in a normal development environment:

```bash
make sqlc
```

Do **not** manually edit files marked `Code generated by sqlc`.

To keep P0 implementation moving, concrete adapters currently use raw PGX only for the new Loop tables while continuing to reuse existing generated Multica queries for Issue/Task/Inbox operations. These raw adapters are intentionally behind stable application interfaces and can later be replaced by generated SQLC implementations without changing the Loop engine.

## 12. Remaining backend integration work

### A. Physical production Router mount

The complete dependency bundle exists, but `router.go` still needs the small final mount because the current GitHub connector only supports whole-file replacement and `router.go` is a very large file; it was not safely overwritten merely to insert a few lines.

Required integration shape:

```go
manifoldRoutes := buildManifoldAgentRoutes(pool, queries, h)
```

Then, inside the existing authenticated `RequireWorkspaceMember(queries)` route group:

```go
registerManifoldAgentRoutes(r, manifoldRoutes)
```

This is intentionally the existing Multica Auth + workspace boundary, not a second Manifold authentication system.

### B. Real build / test / migration verification

This branch has been implemented through the GitHub connector and has not yet received a real Go compiler / PostgreSQL migration run in this environment.

Before calling the backend runnable:

```bash
cd server
make sqlc
go test ./internal/looptemplate/...
go test ./internal/loopevaluation/...
go test ./internal/looppolicy/...
go test ./internal/loopservice/...
go test ./internal/loophttp/...
go test ./cmd/migrate/...
go test ./cmd/server/...
```

Then apply migrations against a disposable PostgreSQL database and run one full Feature Development E2E.

### C. Provenance write path

Evidence authorization is implemented, but Artifact/provenance creation APIs still need to be wired so Commit/PR/Test/Build/Deployment references can be registered into `loop_artifact` without manual database writes.

## 13. Frontend / pilot next

### MNL-011 — Manifold Agent Loop UI

- Loop list/detail
- stage graph/timeline
- current stage
- Agent/Task state per node
- Evaluation findings
- Approval card
- workflow retry/recovery history
- artifact/provenance panel

### MNL-012 — internal pilot

Recommended first pilot: MindCloudX token licensing or Odin safety workflow.

Measure separately:

- requirement → release lead time
- first-pass Review/Test rate
- workflow retry count
- human intervention count
- transient Task retry count
- total Agent cost

## 14. P0 done condition

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

P0 is done only when this runs against real Multica Issues/Tasks with deterministic routing, bounded workflow retry, explicit human authority, provenance, and existing Runtime/Daemon semantics.

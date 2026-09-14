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

`RawLoopStateRepository` provides raw-PGX access for pinned templates and gate evidence while generated Loop SQLC repositories remain absent.

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

`DurablePolicyMutationSink` dispatches only after the mutation transaction commits. Tests explicitly cover transaction failure → no dispatch and post-commit dispatch failure → recoverable `PolicyDispatchError`.

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

## 9. Artifact / provenance path

Implemented `ArtifactApplicationService`, `RawArtifactRepository`, and `loophttp.ArtifactHandler`.

HTTP contract:

```text
GET  /api/loops/{parentIssueId}/artifacts
POST /api/loops/{parentIssueId}/artifacts
```

Properties:

- artifact references are scoped to workspace + Parent Loop
- optional node/task lineage is validated by the application/repository boundary
- authenticated human actors and task-token Agents can register provenance
- task-token Agents remain bound to their authoritative server-stamped Task identity
- `cloud_pat` machine credentials cannot author loop artifacts
- registered artifact ids / ref_ids / ref_uris can subsequently be used as trusted Evaluation evidence

This closes the P0 provenance write/read loop for Commit/PR/Test/Build/Deployment-style references without making external systems cease to be canonical.

## 10. Automatic Loop advancement

Evaluation and Approval invoke Policy Tick directly.

Normal Product/Architecture/Backend/Frontend/Release nodes also have an event-driven advancement bridge:

```text
issue:updated(status=done) ─┐
                            ├→ verify Manifold child metadata
 task:completed ────────────┘
                                  ↓
                         PolicyApplicationService.Tick(parent)
```

This uses Multica's existing synchronous post-commit `events.Bus`.

Both events are intentionally observed because `CompleteTask()` does not itself mark the Issue done; Agents manage Issue status through the CLI. Policy Tick is idempotent, so whichever event arrives first may no-op and the other safely retries progression.

Dedicated server tests pin the cross-module event contracts:

- `issue:updated` accepts both the canonical map payload and typed `handler.IssueResponse`
- malformed issue payloads are ignored
- Task lifecycle payloads use canonical `issue_id`
- non-Loop parent metadata does not trigger a child-policy Tick

## 11. HTTP application contract and production mount

The current P0 HTTP surface contains ten routes:

```text
GET  /api/loops
GET  /api/loops/{parentIssueId}
GET  /api/loops/{parentIssueId}/artifacts
POST /api/loops
POST /api/tasks/{taskId}/loop-evaluation
POST /api/loop-approvals/{approvalId}/decision
POST /api/loop-templates
PUT  /api/loop-templates/{templateKey}/versions/{version}
POST /api/loop-templates/{templateKey}/versions/{version}/publish
POST /api/loops/{parentIssueId}/artifacts
```

Security properties:

- trusted workspace/actor headers come from existing Auth middleware
- every mounted route is behind existing Multica Auth + `RequireWorkspaceMember`
- task token Evaluation must match its server-stamped Task ID
- task token evaluator identity is server-stamped Agent ID
- Human Approval rejects machine actors
- Template administration is human-only
- Artifact task-token authorship remains bound to the authenticated Task
- unknown JSON fields are rejected
- duplicate Loop → 409
- durable Evaluation/Approval with subsequent Policy Tick failure → 202 Accepted

Production composition is complete:

```text
main()
  ↓
NewRouter(...)
  ↓
mountManifoldAgentRoutes(...)
  ↓
Auth
  ↓
RequireWorkspaceMember
  ↓
registerManifoldAgentRoutes(...)
```

`server/cmd/server/manifold_agent_routes.go` assembles the concrete dependency graph, including Template admin, Loop creation, Policy projection/mutation, Evaluation, Approval, artifacts/evidence authorization, read APIs and lifecycle event advancement.

`server/cmd/server/manifold_agent_routes_test.go` pins all ten P0 routes so accidental route loss becomes a test failure. Dedicated event tests pin the Multica event payload contract used by automatic Policy Tick.

## 12. SQLC status

Committed query sources exist for:

- `loop_template.sql`
- `loop_evaluation.sql`
- `loop_approval.sql`
- `loop_artifact.sql`
- `loop_issue.sql`

Generated Loop Go files are still absent in this branch because the original implementation environment could not resolve/download the pinned SQLC generator.

Run in a normal development environment:

```bash
make sqlc
```

Do **not** manually edit files marked `Code generated by sqlc`.

The P0 runtime no longer depends on those missing generated Loop query files: new Loop tables are accessed through narrow raw-PGX repositories, while Issue/Task/Inbox behavior continues to reuse existing generated Multica queries/services. The raw repositories remain behind stable application interfaces and can later converge to generated SQLC implementations without changing the Loop engine or HTTP contract.

SQLC generation is therefore a code-generation consistency / typed-repository convergence step, not a runtime architecture blocker.

## 13. Remaining backend verification work

The backend feature path is now physically mounted and composed. Remaining work is verification, not missing orchestration architecture.

### A. Real Go build / test verification

This branch was primarily implemented through the GitHub connector and still needs a real Go compiler run in a normal checkout/CI environment:

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

### B. PostgreSQL migration verification

Apply migrations 468–483 against a disposable PostgreSQL database, including interruption/retry validation for the concurrent indexes.

### C. Full Feature Development E2E

Run one real Loop through:

```text
create/publish Template
  → instantiate Loop
  → Product
  → Architecture
  → parallel development
  → Review
  → Test FAIL
  → responsible-node recovery
  → Review
  → Test PASS
  → Human Approval
  → Release
  → Loop Completed
```

Also exercise Approval Reject recovery, post-commit dispatch failure/retry, duplicate instance creation and stale Evaluation rejection.

## 14. MNL-011 — Manifold Agent UI

Backend read APIs are now available for the first UI slice:

- Loop list
- Loop detail
- artifact/provenance list

The first UI should expose:

- Loop list/detail
- stage graph/timeline
- current stage
- Agent/Task state per node
- Evaluation findings
- Approval card
- workflow retry/recovery history
- artifact/provenance panel

Do not build a generic BPMN editor for P0.

## 15. MNL-012 — internal pilot

Recommended first pilot: MindCloudX token licensing or Odin safety workflow.

Measure separately:

- requirement → release lead time
- first-pass Review/Test rate
- workflow retry count
- human intervention count
- transient Task retry count
- total Agent cost

## 16. P0 done condition

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

Backend implementation now contains the concrete path required for this flow. P0 should only be called operationally complete after the real compiler/migration/E2E verification above succeeds against Multica Issues/Tasks/Runtime.
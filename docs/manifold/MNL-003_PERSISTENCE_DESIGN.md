# MNL-003 — Persistence Design for Manifold Agent Native Loop

> Status: Draft for implementation planning  
> Branch: `feature/manifold-agent-native-loop`  
> Depends on: MNL-001 capability map, MNL-002 orchestration ADR

## 1. Decision summary

P0 will **not** introduce `missions`, `workflow_runs`, or `stage_runs` tables.

The canonical loop instance remains a Multica **parent Issue**. Workflow nodes are compiled into **child Issues**, and ordered/parallel execution continues to use the existing `issue.stage` barrier model. Agent execution remains canonical in `agent_task_queue` and the existing runtime/daemon stack.

The minimum new durable state for P0 is:

1. `loop_template` — versioned definition of a reusable engineering loop.
2. `loop_evaluation` — structured Test/Review/Security verdicts that can drive deterministic routing.
3. `loop_approval` — explicit, auditable human approval/rejection records.
4. `loop_artifact` — typed references that connect requirement/design/code/test/release evidence without creating a new document store.

Loop-instance binding and lightweight node identity are stored on existing Issues through flat issue metadata. Rich or historical state must not be placed in issue metadata.

## 2. Existing persistence we will reuse

### 2.1 Issue as loop instance / node

Existing Issue already provides the durable identity and lifecycle required by P0:

- workspace scoping
- project binding
- parent/child hierarchy
- stage ordering/barriers
- status
- assignee (human/agent/squad through current product semantics)
- metadata
- properties
- comments / activity / inbox integration

The parent Issue is the loop instance. Child Issues are executable workflow nodes.

### 2.2 Task/Run as execution history

`agent_task_queue` is the canonical execution record. It already carries substantial execution and lineage state, including:

- issue_id
- agent_id / runtime_id
- status
- result / error / failure_reason
- attempt / max_attempts
- parent_task_id
- retry_of_task_id
- rerun_of_task_id
- session/workdir/branch context
- squad_id
- trigger evidence
- accountable/originator identity
- cancellation attribution

Therefore Manifold must not create a parallel `stage_run` table for agent execution in P0.

### 2.3 Activity log

Use existing activity logging for human-readable/auditable timeline events such as:

- loop instantiated
- evaluation recorded
- node reopened by policy
- approval requested
- approval approved/rejected
- loop completed/cancelled

Activity is an audit/event projection, not the source of truth for evaluation or approval state.

### 2.4 Inbox

Use existing inbox infrastructure to surface actionable workflow events:

- approval requested
- decision required
- loop blocked
- retry budget exhausted

Inbox is notification state only. It must never become approval source of truth.

### 2.5 Attachments / Source Context

Existing attachments remain the binary/file transport and association mechanism. Existing source context remains suitable for immutable external/context snapshots where its semantics already fit.

Manifold `loop_artifact` stores typed provenance references and may point to existing attachments, source-context records, issues, tasks, commits, PRs, builds or external URLs. It does not duplicate file bytes.

## 3. Why issue metadata is useful — and where it stops

Existing Issue metadata is intentionally a **small flat JSONB KV bag** for pipeline state. It is constrained to primitive values, max 50 keys, max 8KB, and single-key atomic mutation.

That makes it a good fit for stable lookup/binding fields, but a bad fit for nested workflow state, history, findings or approvals.

### Allowed Manifold metadata keys

Recommended parent-Issue keys:

```text
manifold.loop.template_key       string
manifold.loop.template_version   number
manifold.loop.instance_key       string
manifold.loop.state              string
manifold.loop.policy_version     number
manifold.loop.retry_count        number
```

Recommended child-Issue keys:

```text
manifold.loop.node_key           string
manifold.loop.node_type          string
manifold.loop.role               string
manifold.loop.retry_count        number
manifold.loop.generated          bool
```

Optional policy hints, if required:

```text
manifold.loop.required           bool
manifold.loop.max_retries        number
```

### Must NOT be stored in metadata

Do not store:

- workflow definition JSON
- evaluation findings arrays
- approval history
- artifact lists
- transition history
- agent execution result
- test logs
- nested policy objects

These exceed metadata's intended semantics and would create race/history problems.

## 4. Table design

Repository rules apply: **no database foreign keys/cascades** for new schema. Relationships are validated and cleaned in application code. Index creation must use `CREATE [UNIQUE] INDEX CONCURRENTLY` in separate single-statement migrations.

### 4.1 `loop_template`

A row is an immutable published template version. Draft editing can either use rows with `status=draft` or a later separate draft model; P0 keeps one table.

```sql
CREATE TABLE loop_template (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    template_key TEXT NOT NULL,
    version INT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    status TEXT NOT NULL,
    definition JSONB NOT NULL,
    policy JSONB NOT NULL DEFAULT '{}',
    created_by_type TEXT NOT NULL,
    created_by_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ,
    archived_at TIMESTAMPTZ
);
```

Recommended checks:

```text
status ∈ draft | active | archived
created_by_type ∈ member | agent
version > 0
jsonb_typeof(definition) = object
jsonb_typeof(policy) = object
```

Important invariant:

- `(workspace_id, template_key, version)` is unique.
- Once `status=active`, `definition` and `policy` are immutable.
- Publishing a change creates `version + 1`.

Indexes (separate migrations):

```sql
CREATE UNIQUE INDEX CONCURRENTLY ...
ON loop_template(workspace_id, template_key, version);

CREATE INDEX CONCURRENTLY ...
ON loop_template(workspace_id, status, template_key);
```

### 4.2 `loop_evaluation`

Structured verdict produced by Test/Review/Security/CI or other evaluators.

```sql
CREATE TABLE loop_evaluation (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    parent_issue_id UUID NOT NULL,
    node_issue_id UUID NOT NULL,
    task_id UUID,
    evaluator_type TEXT NOT NULL,
    evaluator_id UUID,
    evaluation_kind TEXT NOT NULL,
    verdict TEXT NOT NULL,
    score DOUBLE PRECISION,
    findings JSONB NOT NULL DEFAULT '[]',
    evidence JSONB NOT NULL DEFAULT '[]',
    policy_action TEXT,
    policy_target_node_key TEXT,
    policy_version INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Suggested evaluator types:

```text
member | agent | system | integration
```

Suggested verdicts:

```text
pass | fail | warn | inconclusive
```

`findings` is structured evidence from the evaluator; it does not itself mutate workflow state.

Example finding:

```json
{
  "code": "api_contract_failure",
  "severity": "high",
  "owner_role": "backend",
  "summary": "POST /token returns 500 for expired license",
  "artifact_refs": ["..."]
}
```

The server validates findings against the evaluation contract before policy consumes them.

Recommended indexes:

```sql
CREATE INDEX CONCURRENTLY ...
ON loop_evaluation(parent_issue_id, created_at DESC);

CREATE INDEX CONCURRENTLY ...
ON loop_evaluation(node_issue_id, created_at DESC);

CREATE INDEX CONCURRENTLY ...
ON loop_evaluation(task_id)
WHERE task_id IS NOT NULL;
```

### 4.3 `loop_approval`

Explicit human decision. This table is authoritative; comments/inbox/activity are projections.

```sql
CREATE TABLE loop_approval (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    parent_issue_id UUID NOT NULL,
    node_issue_id UUID NOT NULL,
    approval_key TEXT NOT NULL,
    state TEXT NOT NULL,
    requested_from_type TEXT,
    requested_from_id UUID,
    requested_by_type TEXT NOT NULL,
    requested_by_id UUID,
    requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_by UUID,
    decided_at TIMESTAMPTZ,
    rationale TEXT,
    policy_version INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

P0 states:

```text
pending | approved | rejected | cancelled | expired
```

Security invariant:

- `decided_by` is always the authenticated **member/user principal**.
- Agent/system identities cannot set `approved` or `rejected`.
- Approval mutation is handled by a server endpoint that re-checks workspace membership and required role/scope.

Recommended uniqueness:

```text
Only one pending approval for the same (node_issue_id, approval_key).
```

Use a partial unique concurrent index:

```sql
CREATE UNIQUE INDEX CONCURRENTLY ...
ON loop_approval(node_issue_id, approval_key)
WHERE state = 'pending';
```

Other indexes:

```sql
CREATE INDEX CONCURRENTLY ...
ON loop_approval(parent_issue_id, created_at DESC);

CREATE INDEX CONCURRENTLY ...
ON loop_approval(requested_from_id, state)
WHERE requested_from_id IS NOT NULL;
```

### 4.4 `loop_artifact`

Typed reference/provenance record. P0 stores references only; external systems remain canonical for their payload.

```sql
CREATE TABLE loop_artifact (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    parent_issue_id UUID NOT NULL,
    node_issue_id UUID,
    task_id UUID,
    artifact_type TEXT NOT NULL,
    relation TEXT NOT NULL,
    title TEXT,
    ref_kind TEXT NOT NULL,
    ref_id UUID,
    ref_uri TEXT,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_by_type TEXT NOT NULL,
    created_by_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Suggested artifact types:

```text
requirement
product_spec
architecture
api_spec
code_change
commit
pull_request
test_plan
test_result
review_report
build
deployment
runtime_evidence
external_document
```

Suggested `relation` values:

```text
input | output | evidence | supersedes | derived_from
```

Suggested `ref_kind` values:

```text
issue | task | attachment | source_context | commit | pull_request | build | deployment | url
```

Exactly one of `ref_id` / `ref_uri` is normally expected depending on `ref_kind`; validate this in application code instead of forcing complicated DB polymorphic constraints.

Recommended indexes:

```sql
CREATE INDEX CONCURRENTLY ...
ON loop_artifact(parent_issue_id, created_at);

CREATE INDEX CONCURRENTLY ...
ON loop_artifact(node_issue_id, created_at)
WHERE node_issue_id IS NOT NULL;

CREATE INDEX CONCURRENTLY ...
ON loop_artifact(task_id)
WHERE task_id IS NOT NULL;
```

## 5. Tables deliberately NOT added in P0

### No `mission`

Parent Issue is the P0 Mission/Loop Instance.

Add a first-class Mission only when a real requirement cannot be represented cleanly by one issue tree, for example one outcome spanning several independent Multica projects/workspaces or multiple root issue trees.

### No `workflow_run`

The parent Issue plus its generated child tree is the durable loop instance. Template version is pinned in parent metadata.

### No `stage_run`

Child Issue + `issue.stage` + canonical Task/Run already expresses node lifecycle and agent execution.

### No `workflow_event`

Activity log + existing realtime event patterns are sufficient initially. If orchestration later requires guaranteed outbox/event replay semantics, add an outbox/event ledger based on measured failure modes.

### No graph database

Provenance starts as typed relational references. Introduce graph storage only when concrete cross-entity query patterns prove PostgreSQL insufficient.

## 6. Loop compiler persistence contract

Given an active template version, compiler performs one atomic application transaction as far as the repository's transaction patterns allow:

1. Validate template and role bindings.
2. Create parent Issue or bind an existing eligible parent Issue.
3. Stamp parent metadata:
   - template key/version
   - instance key
   - initial state
   - policy version
4. Create child Issues with:
   - `parent_issue_id = parent.id`
   - `stage`
   - role-specific assignee
   - generated title/description/context
5. Stamp each child with node key/type/role/generated marker.
6. Create initial artifact links for requirement/source inputs.
7. Write loop-instantiated activity entry.
8. Activate only the first runnable stage; later stages remain parked according to existing Issue semantics.

Compiler must be idempotent by `manifold.loop.instance_key` and node key. Replaying the same compile request must not duplicate child Issues.

## 7. Node identity and idempotency

Stable node identity is required for retries and policy routing.

For every generated child Issue:

```text
manifold.loop.node_key = "backend"
manifold.loop.generated = true
```

A template version must not contain duplicate node keys.

Compiler lookup key:

```text
(parent_issue_id, metadata[manifold.loop.node_key])
```

If metadata lookup performance becomes a measured problem, add a dedicated lightweight `loop_node_binding` table later. Do not add it pre-emptively.

## 8. Task Retry vs Workflow Retry persistence

### Task Retry

Existing `agent_task_queue` remains canonical:

```text
retry_of_task_id
attempt
max_attempts
failure_reason
```

Used for transient/system/agent execution failures.

### Workflow Retry

Used when execution completed but validation failed.

Persist workflow retry through:

- `loop_evaluation(verdict=fail)`
- activity entry describing policy decision
- child Issue metadata retry counter
- Issue status transition/reopen
- a **new canonical task** created by existing dispatch path

The new task is not necessarily a `retry_of_task_id` Task Retry. It is a new workflow attempt caused by policy. If explicit machine-readable linkage becomes necessary, prefer artifact/activity/evaluation linkage first; add a dedicated task cause field only with a clear runtime/reporting consumer.

This separation prevents CI/test failure from being confused with CLI/network/timeout retry statistics.

## 9. Evaluation → deterministic policy transition

Evaluator cannot directly reopen/complete arbitrary Issues.

Sequence:

```text
Test Agent Task completes
        ↓
structured evaluation submitted
        ↓
server validates schema + actor + issue/task lineage
        ↓
insert loop_evaluation
        ↓
policy engine reads pinned template/policy version
        ↓
legal transition resolved
        ↓
server changes target Issue state / triggers dispatch
        ↓
activity + realtime events
```

Policy action examples:

```text
continue
reopen_node:backend
reopen_node:frontend
request_human_decision
fail_loop
```

Unknown action, unknown node key, exhausted retry budget or ambiguous ownership => stop and require human intervention.

## 10. Approval transaction contract

Request:

1. Insert `loop_approval(state=pending)`.
2. Publish activity.
3. Create inbox item for authorized human(s).
4. Keep next release/deploy node parked.

Approve:

1. Authenticate current human user.
2. Load approval + parent/node Issue and verify workspace/permission.
3. Atomically transition pending → approved with `decided_by`, timestamp, rationale.
4. Advance legal next node exactly once.
5. Publish activity/realtime/inbox updates.

Reject is equivalent except policy routes to configured target or leaves loop blocked.

Duplicate decisions return conflict/idempotent canonical state; they must not advance twice.

## 11. Artifact/provenance model

P0 target chain:

```text
Requirement artifact
   ↓ input
Product Issue / Task
   ↓ output
Product Spec artifact
   ↓ input
Architecture Issue / Task
   ↓ output
Architecture artifact
   ↓
Backend / Frontend / Embedded Issues
   ↓ output
Commit / PR artifacts
   ↓ evidence
Review + Test evaluations
   ↓ evidence
Build / deployment artifact
```

This is sufficient to answer:

- Which requirement caused this change?
- Which agent/task produced this PR?
- Which test evidence approved/rejected it?
- Which approval allowed release?
- What failure caused a node to reopen?

without copying external system payloads into Multica.

## 12. Cleanup and integrity rules

Because new tables must not depend on DB foreign keys:

- deleting/archive behavior is explicit service logic;
- workspace deletion cleanup includes all loop tables;
- hard-deleting a parent Issue must delete loop evaluations/approvals/artifacts associated with its loop;
- hard-deleting a child Issue must clean node-linked rows or reject deletion while an active loop depends on it;
- template versions referenced by active/historical loops are archived, not deleted;
- references to external artifacts can become unavailable; provenance row remains and UI renders it as missing/unavailable.

Prefer archival over destructive deletion for audit-bearing template/evaluation/approval data.

## 13. Migration plan

Actual migration numbers must be allocated from current repository head at implementation time.

Recommended sequence:

1. `create_loop_template`
2. unique concurrent index for `(workspace_id, template_key, version)`
3. active/template lookup concurrent index
4. `create_loop_evaluation`
5. evaluation indexes, each in its own migration
6. `create_loop_approval`
7. approval partial unique/index migrations
8. `create_loop_artifact`
9. artifact indexes, each in its own migration

Do not combine concurrent index creation into table-creation migrations.

After SQL changes:

```text
make sqlc
make test
pnpm typecheck
make check
```

as applicable to touched layers.

## 14. API boundary implications

Likely P0 APIs:

```text
GET/POST   /workspaces/{ws}/loop-templates
GET        /workspaces/{ws}/loop-templates/{key}/versions/{version}
POST       /workspaces/{ws}/issues/{id}/loops/instantiate
GET        /workspaces/{ws}/issues/{id}/loop
POST       /workspaces/{ws}/issues/{id}/evaluations
GET        /workspaces/{ws}/issues/{id}/evaluations
POST       /workspaces/{ws}/issues/{id}/approvals/{approvalId}/approve
POST       /workspaces/{ws}/issues/{id}/approvals/{approvalId}/reject
GET        /workspaces/{ws}/issues/{id}/artifacts
```

Exact route naming should be checked against repository route conventions before implementation.

Every UI-consumed response must follow the repository's zod/`parseWithFallback` compatibility rule.

## 15. P0 schema verdict

| Concept | Decision | Canonical storage |
| --- | --- | --- |
| Mission | Reuse | Parent Issue |
| Workflow instance | Reuse | Parent + child Issue tree |
| Stage | Reuse | `issue.stage` |
| Node | Reuse + metadata | Child Issue |
| Agent execution | Reuse | `agent_task_queue` |
| Runtime | Reuse | Agent Runtime / daemon |
| Task retry | Reuse | existing retry lineage |
| Workflow retry | Extend | Evaluation + Issue transition + metadata counter |
| Template | **New** | `loop_template` |
| Evaluation | **New** | `loop_evaluation` |
| Approval | **New** | `loop_approval` |
| Artifact provenance | **New** | `loop_artifact` |
| Event ledger | Deferred | existing activity/realtime first |
| Graph DB | Deferred | typed relational provenance first |

## 16. Go/no-go before implementation

Proceed to implementation design only if these remain true after review:

1. One P0 loop can be represented by a single parent Issue tree.
2. Existing stage barriers cover P0 sequential + parallel execution.
3. Task execution/retry remains canonical in Multica.
4. Issue metadata is used only for small primitive bindings.
5. Evaluations and approvals require historical/auditable tables.
6. Artifact provenance can be references rather than copied payloads.

If any pilot requirement violates (1) or (2), revisit MNL-002 before adding more tables around the wrong execution model.

## 17. Next task

MNL-004 should define the **Loop Template schema and compiler contract** against this persistence design, including:

- versioning/immutability
- node keys
- stage compilation
- role/agent binding
- optional nodes
- evaluation contract
- failure routing
- retry budget
- approval nodes
- artifact input/output declarations
- compile idempotency

Only after MNL-004 should the first migrations/handlers be implemented.

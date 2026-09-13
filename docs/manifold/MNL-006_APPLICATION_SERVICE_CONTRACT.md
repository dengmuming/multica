# MNL-006 — Application Service Contract

> Product: Manifold Agent
> Foundation: Multica
> Scope: bridge the pure Loop engine to existing Multica persistence and dispatch without creating a parallel runtime.

## 1. Service boundary

MNL-006 introduces application services, not a new workflow runtime.

```text
HTTP / UI
  ↓
Manifold Loop Application Service
  ├── TemplateService
  ├── InstantiateService
  ├── EvaluationService
  ├── ApprovalService
  └── PolicyApplicationService
       ↓
existing Multica Issue / Agent / Task / Inbox / Activity APIs
```

The pure packages remain authoritative for validation/decisions:

- `looptemplate` validates and compiles
- `loopevaluation` validates structured evaluator output
- `looppolicy` decides legal actions

Application services own authorization, database lineage checks, transactions, idempotency and translation into existing Multica mutations.

## 2. TemplateService

Responsibilities:

- create draft template
- validate canonical definition before persistence
- publish immutable version
- archive/replace active version transactionally
- fetch active/exact version
- list workspace templates

Rules:

- active template versions are immutable
- every loop instance pins exact template key/version and policy version
- authoring YAML may exist later, but persisted canonical definition is normalized JSON

## 3. InstantiateService

Input:

- workspace/project
- template key/version
- feature intent/title/description
- role bindings
- optional compile-time flags
- idempotency/instance key

Preflight before first write:

1. authorize caller
2. load exact published template
3. validate template
4. validate all required role bindings
5. prove bound Agent/Squad/Member belongs to allowed workspace scope
6. compile deterministic plan
7. reject if an instance with the same instance key already exists

Atomic write intent:

- create parent orchestration Issue
- stamp parent loop metadata
- create included child Issues with stable node keys/stages
- assign existing Agent/Squad/Member bindings
- park future stages in `backlog`
- activate first stage through normal Multica Issue semantics
- create initial provenance references/activity

No custom Task is created directly by the compiler. Existing Multica assignment/dispatch creates execution Tasks.

## 4. EvaluationService

Input must identify:

- parent loop Issue
- evaluation node Issue
- Task/Run producing the evaluation
- structured evaluation payload
- optional evidence/artifact references

Server proves:

- Task belongs to node Issue
- node belongs to parent loop
- node type/kind matches template
- template version is the pinned instance version
- referenced evidence is visible in the workspace

Then:

1. normalize/validate via `loopevaluation`
2. persist immutable `loop_evaluation`
3. load runtime projection
4. evaluate `looppolicy`
5. apply actions transactionally/idempotently where possible
6. record activity and realtime events

Agent free text never directly advances/reopens a stage.

## 5. ApprovalService

Only authenticated human members can decide an approval.

Approve/reject transaction:

1. authorize member
2. lock/check pending approval
3. transition pending → approved/rejected once
4. persist actor/time/rationale
5. evaluate policy
6. apply resulting actions once
7. record activity/realtime event

Agent/system identities cannot forge approve/reject. Comments are not approvals.

## 6. PolicyApplicationService

Maps pure `looppolicy.Action` to existing Multica mutations:

| Policy action | Application mutation |
| --- | --- |
| activate node | child Issue `backlog → todo`, normal dispatch path |
| reopen node | reopen responsible Issue, increment workflow retry metadata |
| park node | downstream Issue → `backlog`, invalidate stale gate evidence by state |
| request approval | create pending `loop_approval` + Inbox notification |
| set parent state | small parent metadata update |
| block loop | parent blocked state + human Inbox escalation |
| complete loop | close/complete parent Issue |

Policy application must tolerate duplicate delivery. Re-running the same state projection must not create duplicate pending approvals, duplicate Tasks, or double-increment workflow retry.

## 7. Transaction boundary

Do not wrap external Agent execution in a DB transaction. Transactions cover only durable control-plane mutations.

Recommended pattern:

```text
DB transaction
  mutate Issue/approval/evaluation/control state
  write activity/outbox intent
commit
  ↓
normal Multica dispatcher / realtime / daemon execution
```

If existing Multica code has no transactional outbox, P0 may reuse its current post-commit dispatch/event pattern, but all durable state transitions must remain idempotent.

## 8. API direction

Exact route names must follow existing handler conventions, but the resource model is:

- workspace loop templates
- issue loop instantiate/read
- loop evaluation submit/list
- approval approve/reject
- loop artifacts/read model

UI should consume a loop read model rather than reconstructing policy state independently.

## 9. Read model

The loop detail response should eventually include:

- parent intent/state
- pinned template/version
- ordered nodes/stages
- assignee role and concrete Agent/Squad/Member
- latest Task status per node
- latest Evaluation verdict/findings
- pending/decided Approval
- workflow retry count
- artifact/provenance links
- derived current stage and blocked reason

The server is authoritative for derived state.

## 10. Implementation dependency

DB-backed service code must wait for generated SQLC output from the committed migrations/query sources. Run:

```bash
make sqlc
```

Do not hand-edit generated SQLC files.

Pure engine work and service interfaces/tests can proceed independently, but handlers importing the new queries should land only after generation succeeds.

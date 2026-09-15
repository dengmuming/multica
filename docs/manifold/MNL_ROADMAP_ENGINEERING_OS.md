# Manifold Agent — Engineering OS Roadmap

> Decision: preserve the current MNL P0 Delivery Loop and extend outward into Organization, Feedback, Context, and Learning rather than replacing the Loop Engine.

## 1. Current position

The branch already implements the P0 Loop Engine around:

```text
Template → Compiler → Issue Graph → Agent Execution
         → Evaluation → Policy → Approval → Release
```

This is the **Delivery Loop kernel**. It should not be rewritten into a generic workflow engine.

The next product milestone is to turn that kernel into:

```text
Agent Organization + Native Loop + Engineering Context
```

## 2. Roadmap

### Phase 0 — Delivery Loop stabilization

Goal: make the existing P0 operationally trustworthy.

- [ ] Run `make sqlc` and commit generated consistency changes if required.
- [ ] Run all Loop Go unit/integration tests in a real checkout.
- [ ] Apply migrations 468–483 against disposable PostgreSQL.
- [ ] Execute one full Feature Development E2E.
- [ ] Verify Test FAIL deterministic recovery.
- [ ] Verify Approval Reject deterministic recovery.
- [ ] Verify duplicate instance and stale Evaluation protection.
- [ ] Finish Loop list/detail UI and Approval/Evaluation/Artifact visibility.

Exit criterion: `Intent → Release → Completed` is repeatable with durable evidence.

### Phase 1 — Organization + Work Graph product model

Goal: make Manifold Agent look like an AI engineering organization rather than a workflow page.

#### MNL-013 — Work Graph projection

Introduce a product-facing Work Graph projection over the existing Template/Issue graph. Do **not** duplicate persistence in P1 unless a proven query/performance requirement appears.

Minimum read model:

```text
WorkGraph
├── intent
├── nodes[]
│   ├── role
│   ├── responsibility
│   ├── assignee Agent/Squad/Human
│   ├── dependencies
│   ├── state
│   ├── current Task
│   ├── Evaluation
│   └── artifacts
└── gates[]
```

Acceptance:

- Existing Loop Template compiles unchanged.
- Existing Issues remain canonical work items.
- UI can render organization/work graph without knowing Loop storage details.

#### MNL-014 — Agent Organization profile

Add a Manifold-facing organization projection for specialist workers.

```text
OrganizationAgent
├── role
├── instructions ref
├── skills
├── tools/runtime
├── model
├── permissions
├── context scope
└── responsibility
```

Prefer mapping existing Multica Agent/Squad/Skill records before adding new durable tables.

Acceptance:

- Product/Architect/Backend/Frontend/Test/Review/Release roles can be inspected as an organization.
- Role binding is visible independently from a specific Loop run.
- Runtime-specific fields remain behind an adapter boundary.

#### MNL-015 — AgentRuntime adapter contract

Extract the smallest interface currently assumed by MNL from Multica execution.

Expected capabilities:

```text
ResolveWorker
DispatchWorkItem
ReadExecution
CancelExecution
ReadWorkerCapabilities
```

Multica is the first adapter. No Pi/other adapter needs to ship in this issue.

Acceptance:

- Loop policy/compiler do not import concrete alternative runtimes.
- Multica behavior remains unchanged.
- Contract is narrow enough for a future peer runtime.

### Phase 2 — Feedback Loop

Goal: make Release a transition into observation rather than the conceptual endpoint.

#### MNL-016 — Observe node and runtime evidence

Add an optional `observe` semantic after release. Runtime/deployment systems remain canonical; Manifold stores references/evidence.

Evidence examples:

- deployment id/version
- environment
- health/check result
- error/incident reference
- metric snapshot reference
- customer/tester feedback reference

Acceptance:

- A released Loop can enter `observing`.
- Evidence is provenance-scoped to the parent Loop.
- Observation timeout/failed health can create deterministic follow-up decisions.

#### MNL-017 — Feedback → Intent

Introduce a normalized feedback intake model:

```text
Feedback
├── source
├── type: bug | runtime | user | test | requirement
├── evidence refs
├── severity
├── related release/work graph
└── proposed intent
```

Acceptance:

- Feedback can be triaged by Human/Product Agent.
- Accepted feedback creates a new Intent/Loop linked to its originating release.
- History forms a navigable lineage rather than mutating the completed delivery record.

#### MNL-018 — Repair Loop

Provide a first-class repair template that starts from runtime evidence and scopes recovery to responsible engineering roles.

Acceptance:

```text
Runtime Failure
 → Triage
 → Responsible Agent
 → Review
 → Test
 → Approval if policy requires
 → Release
 → Observe
```

### Phase 3 — Engineering Context Graph

Goal: connect engineering knowledge without copying every external system into Manifold.

#### MNL-019 — Context entity/ref model

Canonical external systems remain canonical. Store typed references and relationships:

```text
Intent ↔ PRD ↔ Work Item ↔ API ↔ Code/Commit/PR
       ↔ Test ↔ Build ↔ Deployment ↔ Runtime Evidence
```

Initial entity types:

- document
- issue
- api
- repository
- commit
- pull_request
- test_result
- build
- deployment
- runtime_evidence

Acceptance:

- Context can be queried by Loop, node, task, artifact and release.
- Every reference records source, external identity and provenance.
- Agents receive scoped context, not an unrestricted global dump.

#### MNL-020 — Context Pack compiler

Compile role/task-specific context before dispatch.

```text
Intent + Work Graph + Role + Current Node + Related Evidence
                         ↓
                    Context Pack
```

Acceptance:

- Product Agent receives requirement/customer context.
- Backend Agent receives architecture/API/code context relevant to its work.
- Test Agent receives acceptance criteria, changed artifacts and prior findings.
- Context Pack contents are auditable.

### Phase 4 — Learning Loop

Goal: improve the organization from historical execution rather than only storing history.

#### MNL-021 — Loop outcome metrics

Persist/derive:

- lead time
- first-pass Review/Test rate
- workflow retry count
- transient execution retry count
- human intervention count
- evaluation findings by class
- release/observe outcome
- agent/model cost where available

#### MNL-022 — Improvement proposals

Generate proposals from historical evidence; do not silently self-modify production policies.

Proposal targets:

- Agent instructions
- Skill selection
- Context Pack rules
- Work Graph template
- Evaluation rubric
- recovery policy

Acceptance:

- Proposal cites historical evidence.
- Human can approve/reject changes.
- Approved change creates a new version; running Loops remain pinned.

## 3. Product UI direction

The primary UI should evolve toward three connected views:

```text
Organization View  — who can do what
Work Graph View    — how this Intent is being completed
Context View       — why decisions/results are grounded
```

Loop list/detail remains useful but becomes an implementation-backed delivery view, not the entire product identity.

Do not build a BPMN editor. Prefer intent-driven graph generation plus constrained human editing.

## 4. Data model rule

Before adding a Manifold table, ask:

1. Does Multica already own a canonical entity for this?
2. Is this state durable product truth or only a projection?
3. Can an external system remain canonical with a provenance reference?
4. Does the Loop require this state for deterministic recovery/audit?

Only persist new Manifold state when the answer requires it.

## 5. Suggested development order

```text
P0 verification + UI
        ↓
MNL-013 Work Graph projection
        ↓
MNL-014 Organization profile
        ↓
MNL-015 Runtime adapter boundary
        ↓
MNL-016 Observe / Runtime Evidence
        ↓
MNL-017 Feedback → Intent
        ↓
MNL-018 Repair Loop
        ↓
MNL-019/020 Context Graph + Context Pack
        ↓
MNL-021/022 Learning Loop
```

The architectural principle is simple: **expand around the working Loop kernel; do not replace it.**
# Manifold Agent Native Loop — Development Issue Backlog

This file is the planning backlog for `docs/manifold/AGENT_NATIVE_LOOP_PLAN.md`.

**Rule:** do not start implementation issues until MNL-001 and MNL-002 establish which existing Multica primitives are reused. Issue numbers below are planning IDs, not GitHub issue numbers.

## Milestone 0 — Discovery / architecture

### MNL-001 — Discovery: map Multica primitives to the Native Loop

**Goal**  
Produce a code-level map of Issue, Task/Run, Agent, Squad, Skill, Project, Autopilot, Inbox, review and runtime/daemon behavior relevant to orchestration.

**Work**
- Identify handlers/services/queries/models for each primitive.
- Trace Issue assignment → task creation → daemon claim → execution → completion/retry.
- Trace Autopilot trigger → dispatch → run.
- Trace review/inbox notification behavior.
- Identify existing generic relationship, artifact, approval and event abstractions.
- Record extension points and invariants that the workflow engine must preserve.

**Acceptance criteria**
- Architecture map contains concrete source paths.
- Every proposed P0 table/object is marked `reuse`, `extend`, or `new` with rationale.
- No schema/API implementation is included.

**Tests**: none; documentation review.

**Depends on**: none.

---

### MNL-002 — ADR: Mission/Workflow ownership and boundaries

**Goal**  
Freeze the boundary between Manifold orchestration and existing Multica execution.

**Decisions required**
- Mission vs Project vs Issue ownership.
- Workflow Run vs Task/Run ownership.
- Stage → Issue/Task linkage.
- Actor/audit model.
- Event delivery/idempotency model.
- Artifact/evaluation/approval ownership.
- Cancellation and retry propagation.

**Acceptance criteria**
- ADR documents rejected alternatives.
- No execution log/token/runtime data is duplicated by workflow tables.
- Human approval cannot be represented as an agent action.
- Server restart/replayed event behavior is specified.

**Depends on**: MNL-001.

---

### MNL-003 — Design: P0 schema and migration plan

**Goal**  
Design minimal persistent state after MNL-001/002.

**Acceptance criteria**
- Final table/column/index list.
- Cleanup/relationship rules respect repository no-FK convention.
- Concurrent indexes are split into legal migrations.
- State enums and terminal states documented.
- Migration rollback/recovery notes included.

**Depends on**: MNL-002.

---

### MNL-004 — Design: Workflow Definition schema

**Goal**  
Define a versioned, validated workflow JSON format.

**P0 node types**: agent, parallel, gate, approval, wait.

**Acceptance criteria**
- JSON schema/Go type design.
- Validation catches missing nodes, illegal edges, cycles unsupported by P0, missing assignees and invalid retry policy.
- Existing active workflow versions are immutable.
- Example `feature-development` fixture included.

**Depends on**: MNL-002.

## Milestone 1 — Mission foundation

### MNL-005 — Backend: Mission CRUD and permissions

**Scope**
- DB queries/models.
- Workspace-scoped handlers/service.
- create/read/list/update draft/cancel where legal.
- permission checks.

**Acceptance criteria**
- Workspace isolation tests.
- malformed UUID/API boundary tests.
- illegal state transition tests.
- cleanup behavior tested.

**Depends on**: MNL-003.

---

### MNL-006 — Backend: Workflow Definition CRUD/versioning

**Acceptance criteria**
- Draft definition can be created/validated/activated.
- Updating active workflow creates a new version rather than mutating historical runs.
- Mission pins exact definition version.
- Permission and malformed payload tests.

**Depends on**: MNL-003, MNL-004.

---

### MNL-007 — Frontend: Mission list and detail skeleton

**Scope**
- Shared API schemas/hooks in `packages/core`.
- Shared pages in `packages/views`.
- Web + desktop route wiring.
- Mission metadata and planned stages; no live graph yet.

**Acceptance criteria**
- API responses parsed with zod/fallback conventions.
- Same business view shared by web/desktop.
- Loading/empty/error/long-title states covered.

**Depends on**: MNL-005, MNL-006.

## Milestone 2 — Sequential orchestrator

### MNL-008 — Backend: durable Workflow Run / Stage Run state machine

**Goal**  
Implement deterministic state transitions and recovery.

**Acceptance criteria**
- Start/pause/resume/cancel.
- Idempotent transition commands.
- Duplicate events cannot duplicate a stage attempt.
- Restart recovery test.
- Transition actor/reason recorded.

**Depends on**: MNL-003, MNL-004, MNL-005, MNL-006.

---

### MNL-009 — Backend: agent stage adapter to existing execution

**Goal**  
Dispatch a stage through existing Multica Issue/Task/Run infrastructure.

**Acceptance criteria**
- No new daemon protocol required for P0 unless discovery proves otherwise.
- Stage links to canonical Issue/Task/Run.
- Existing execution logs/token/cost remain canonical.
- Completion/failure advances workflow exactly once.
- Cancellation behavior documented/tested.

**Depends on**: MNL-001, MNL-008.

---

### MNL-010 — E2E: Product → Architect → Backend sequential workflow

**Acceptance criteria**
- Mission starts from a fixture workflow.
- Three different configured agents can execute sequentially.
- Output of prior stage is available as bounded context/artifact to next stage.
- Workflow survives server restart between stages.
- Final mission reaches waiting/completed expected state.

**Depends on**: MNL-009.

## Milestone 3 — Graph execution and verification loop

### MNL-011 — Backend: parallel fan-out/fan-in

**Acceptance criteria**
- Independent stages can start concurrently.
- Join waits for required children.
- One failed child follows configured failure policy.
- Duplicate completion cannot release join twice.

**Depends on**: MNL-008, MNL-009.

---

### MNL-012 — Backend: Evaluation and deterministic gates

**Acceptance criteria**
- Structured pass/fail/score/findings/evidence.
- Evaluator can be configured agent or deterministic integration.
- Invalid agent output cannot directly mutate workflow state.
- Gate routing uses validated evaluation only.

**Depends on**: MNL-008, MNL-009.

---

### MNL-013 — Backend: bounded retry and failure routing

**Acceptance criteria**
- Retry policy specifies max attempts.
- Test failure can route to responsible development node and then re-test.
- Attempt lineage is visible.
- Infinite retry is impossible in P0 configuration.

**Depends on**: MNL-012.

---

### MNL-014 — Frontend: execution graph/timeline

**Acceptance criteria**
- Displays sequential and parallel stages.
- Active, waiting, failed, blocked, approval and completed states are distinguishable.
- Stage drawer links canonical Issue/Task/Run and evidence.
- Retry attempt/history visible.
- UI remains usable for large/long labels.

**Depends on**: MNL-008, MNL-011.

## Milestone 4 — Human control

### MNL-015 — Backend/UI: explicit Approval stage

**Acceptance criteria**
- Only authenticated authorized humans can approve/reject.
- Decision/rationale/timestamp/principal audited.
- Agent/system cannot forge approval.
- Rejection follows configured route.
- Production/release example requires approval.

**Depends on**: MNL-008.

---

### MNL-016 — Inbox: workflow approval/blocker actions

**Acceptance criteria**
- Existing Inbox surfaces approval request and intervention-required notifications.
- Opening notification lands on exact mission/stage.
- Resolved approval is no longer actionable.
- Notification replay does not duplicate decisions.

**Depends on**: MNL-015.

## Milestone 5 — Provenance and metrics

### MNL-017 — Artifacts: typed references and provenance

**Acceptance criteria**
- Requirement/spec, PR/commit, test result, build/deployment and external document can be referenced.
- Artifact records actor/source and stage/mission lineage.
- Existing canonical systems remain source of truth; P0 does not copy full documents/repos.

**Depends on**: MNL-008.

---

### MNL-018 — Context relationships v1

**Goal**  
Create queryable provenance relationships in PostgreSQL before considering a graph DB.

**Acceptance criteria**
- Can traverse Mission → Stage → Issue/Task → Artifact/Evaluation.
- Can answer which requirement/decision produced a PR/test result.
- Relationship semantics documented and indexed.

**Depends on**: MNL-017.

---

### MNL-019 — Metrics: delivery outcomes

**Acceptance criteria**
- Mission lead time.
- Stage duration/retries.
- first-pass verification rate.
- human intervention count.
- agent/model/runtime and cost/token rollups where existing data allows.
- failed/recovered stage counts.

**Depends on**: MNL-008, MNL-012.

## Milestone 6 — Real delivery loop

### MNL-020 — Integration: CI/test/deployment event adapter

**Acceptance criteria**
- External result is authenticated/validated.
- Replayed event is idempotent.
- CI/test/deploy evidence can satisfy configured wait/evaluation stages.
- Provider-specific logic is behind an adapter.

**Depends on**: MNL-008, MNL-012, MNL-017.

---

### MNL-021 — E2E: Feature Development workflow

**Scenario**

Product → Architect → parallel Backend/Frontend/(optional Embedded) → Review → Test → failure route/retry → Human Approval → Release.

**Acceptance criteria**
- At least four specialist agents.
- Parallel stage demonstrated.
- Failed evaluation routes back and recovers.
- Human approval mandatory.
- PR/test evidence linked.
- Mission history can explain intent → outcome.

**Depends on**: MNL-011 through MNL-020.

---

### MNL-022 — Internal pilot on a real Manifold feature

Recommended: MindCloudX token licensing or Odin low-battery return-to-home.

**Acceptance criteria**
- Real repository and team.
- Real PR and test evidence.
- Lead time/intervention/retry metrics captured.
- Pilot retrospective identifies what to productize, remove or redesign.

**Depends on**: MNL-021.

## Recommended first development batch

Only create/assign implementation work for these first:

1. MNL-001
2. MNL-002
3. MNL-003
4. MNL-004

After architecture review, proceed with MNL-005–MNL-010. This keeps the fork maintainable and minimizes conflict with upstream Multica evolution.

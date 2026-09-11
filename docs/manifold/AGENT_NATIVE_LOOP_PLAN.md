# Manifold Agent Native Loop — Product & Implementation Plan

> Status: Proposal / planning baseline  
> Branch: `feature/manifold-agent-native-loop`  
> Goal: evolve the Multica fork into an agent-native software delivery loop without rebuilding Multica's existing agent workforce/runtime capabilities.

## 1. Product thesis

**Manifold Nexus** is an agent-native software engineering platform. **Manifold Agent Native Loop** is the operating model underneath it.

The product should not be another AI coding chat or a menu of specialist agents. It should turn software delivery into an observable, controllable loop:

`Intent → Product Spec → Technical Plan → Build → Review → Test → Approval → Release → Observe → Feedback → Next Intent`

Humans own goals, trade-offs, approvals and accountability. Agents own bounded planning, execution, verification, retries and reporting.

### Three-loop model

1. **Model loop** — reasoning/generation provided by GPT, Claude, DeepSeek, etc.
2. **Agent loop** — tool-use loop provided by Pi, Codex, Claude Code, OpenCode and other harnesses.
3. **Engineering loop** — cross-role SDLC orchestration owned by Manifold.

Multica already bridges agent loops into team work. Manifold's differentiator should therefore live primarily at level 3.

## 2. What we inherit from Multica

Do not duplicate working Multica primitives unless a concrete gap is demonstrated.

Reuse:

- Workspaces and projects
- Human and agent assignees
- Agents, squads and skills
- Issues and task/run lifecycle
- Runtime daemon and local/cloud execution
- Provider/CLI abstraction (Pi, Codex, Claude Code, etc.)
- Execution logs, token usage, retries and timeouts
- Review gates and inbox
- Git/VCS integration
- Channels and external triggers
- Autopilots/scheduled work
- Existing roles/access scopes
- Web/desktop/mobile surfaces and shared package architecture

The first implementation must extend these concepts rather than introduce parallel `manifold_*` versions of Issue, Agent, Task or Runtime.

## 3. New Manifold domain layer

The minimum new domain is:

### Mission

A Mission represents a product/engineering outcome that may require multiple issues, roles and runs.

Examples:

- Add token-based licensing to MindCloudX
- Add low-battery return-to-home to Odin
- Fix production login regression and verify release

A Mission is **not** another Issue. Issues remain executable units of work. A Mission groups intent, workflow state, artifacts and acceptance state across issues.

### Workflow Definition

A versioned graph/template describing stages and routing.

Initial node types:

- `agent` — dispatch an existing Multica agent/squad
- `parallel` — fan out independent stages
- `gate` — deterministic condition/evaluation
- `approval` — require a human decision
- `wait` — wait for an external event

Initial control semantics:

- sequential execution
- parallel fan-out/fan-in
- conditional routing
- retry with bounded attempts
- human approval/reject
- pause/resume/cancel

Do **not** introduce a general-purpose BPMN engine in P0.

### Workflow Run

Durable instance of a workflow attached to a Mission. It records current stage, stage attempts, transitions, inputs/outputs and reasons.

### Stage Run

One execution of a workflow stage. For agent stages it should reference existing Multica Issue/Task/Run records instead of duplicating execution logs.

### Artifact

A typed reference to work produced or consumed by a stage. P0 should store references/metadata, not build a new document platform.

Candidate types:

- requirement/spec
- architecture/plan
- issue
- repository/branch/commit/PR
- test plan/result
- build/deployment
- review report
- external URL/document

### Evaluation

Machine- or human-produced verdict used by a gate:

- pass/fail
- score
- structured findings
- evaluator identity
- evidence/artifact references

### Approval

Explicit human decision bound to a stage and principal. Approval is auditable and cannot be inferred from a comment.

## 4. Role model

Roles are templates/configuration over existing Multica agents, not hard-coded new execution engines.

Initial recommended roles:

- Product Agent — requirement/spec and acceptance criteria
- Architect Agent — architecture and implementation plan
- Backend Agent
- Frontend Agent
- Embedded Agent
- Test Agent
- Review Agent
- Release Agent

Later: Security, SRE, DBA, Data, Documentation, Support.

A role can be backed by any supported runtime/provider. Pi should be a first-class recommended harness, not a hard dependency.

Example logical configuration:

```yaml
role: backend
agent: manifold-backend
runtime: pi
skills:
  - mt-go
  - mt-api
  - mt-mysql
  - mt-security-review
permissions:
  production_deploy: deny
```

## 5. Skills strategy

Evolve the existing Manifold `ai-codespec` work into an enterprise **Agent Skill Registry**.

Suggested groups:

- Engineering: `mt-go`, `mt-python`, `mt-vue`, `mt-embedded`
- Quality: `mt-test`, `mt-code-review`, `mt-security-review`
- Product: `mt-prd`, `mt-requirement-review`, `mt-acceptance`
- DevOps: `mt-docker`, `mt-k8s`, `mt-jenkins`
- Company: `mt-api-spec`, `mt-git-workflow`, `mt-release-spec`

Prefer Multica's existing Skill model/storage/distribution. Add Manifold-specific registry metadata only if needed for versioning, ownership, compatibility or policy.

## 6. Context layer / Context Graph

This is a strategic differentiator, but should be incremental.

Target relationship graph:

`Requirement → Decision → Issue → Agent Run → Code/PR → Build → Test → Deployment → Bug/Feedback`

P0 must **not** start by deploying a separate graph database. First create stable typed relationships and provenance in PostgreSQL, then evaluate graph storage/search after real queries exist.

Every workflow transition should preserve:

- original mission intent
- decisions and approval rationale
- input/output artifacts
- issue/task/run lineage
- evaluator evidence
- actor (human/agent/system)

This allows later agents to answer not only "what code is relevant?" but "why does this exist and what previously failed?"

## 7. Event model

The macro loop must be event-driven. Candidate events:

- `mission.created`
- `workflow.started`
- `stage.ready`
- `stage.started`
- `stage.completed`
- `stage.failed`
- `evaluation.completed`
- `approval.requested`
- `approval.approved`
- `approval.rejected`
- `issue.status_changed`
- `task.completed`
- `pr.opened`
- `ci.completed`
- `deployment.completed`
- `runtime.alert_created`
- `feedback.created`

P0 should reuse Multica's existing service/event/WebSocket patterns. Do not introduce Kafka/NATS until durability/throughput requirements prove the need.

## 8. P0 reference workflow

The first vertical slice is **Feature Development**:

```text
Mission
  ↓
Product Agent — spec + acceptance criteria
  ↓
Architect Agent — implementation plan
  ↓
┌──────────────┬──────────────┬──────────────┐
Frontend Agent Backend Agent  Embedded Agent (optional by project)
└──────────────┴──────────────┴──────────────┘
  ↓
Review Agent
  ↓
Test Agent
  ↓
FAIL ──→ responsible development stage ──→ Test
  ↓ PASS
Human Approval
  ↓
Release Agent
  ↓
Done
```

The workflow must support projects that do not have all development roles.

## 9. P0 UX

### Mission list

Show mission title, project, workflow, owner, overall state, active stage, blockers and progress.

### Mission detail / Execution Graph

Primary UI is a workflow graph/timeline, not a chat window.

Each stage exposes:

- assigned agent/human
- state and attempt count
- linked Issue/Task/Run
- artifacts
- evaluation result
- blocker/retry reason
- timestamps/cost where available

### Human Inbox

Extend the existing inbox with actionable workflow items:

- approval requested
- decision/blocker requested
- failed stage needs intervention
- release gate requested

Human work should converge on goals, decisions, approvals and exceptions.

## 10. Eval and product metrics

P0 instrumentation should make the system measurable from day one.

Per stage/run:

- success/failure
- retry count
- duration
- agent/model/runtime
- token/cost where available
- human intervention

Per mission:

- lead time
- first-pass review/test rate
- number of agent runs
- number of human interventions
- failed/recovered stages
- requirement-to-release duration

Do not optimize for agent activity volume. Optimize for verified delivery outcomes.

## 11. Architecture fit with this repository

Follow existing repository boundaries and `CLAUDE.md` rules.

Expected backend additions (names provisional):

```text
server/
  migrations/
  internal/
    handler/       # mission/workflow endpoints
    service/       # orchestration/application logic
    db/queries/    # SQL/sqlc queries
```

Expected shared frontend additions:

```text
packages/core/     # API schemas, queries/mutations, headless workflow state
packages/views/    # mission list/detail, graph/timeline, approval UI
packages/ui/       # only reusable primitives if genuinely missing
apps/web/          # route/platform wiring
apps/desktop/      # route/platform wiring
```

Mobile parity is not required for the first vertical slice; API semantics must remain compatible with installed clients.

Database rules from the repository remain mandatory: no foreign keys/cascades; concurrent indexes in isolated migrations; relationships/cleanup enforced in application code.

## 12. Proposed P0 data model

Names are deliberately provisional until code-level design.

### `missions`

- id
- workspace_id
- project_id nullable
- title
- description
- state: draft/running/blocked/waiting_approval/completed/cancelled/failed
- workflow_definition_id
- workflow_definition_version
- created_by
- created_at / updated_at / completed_at

### `workflow_definitions`

- id
- workspace_id
- name
- slug
- version
- definition_json
- status: draft/active/archived
- created_by
- timestamps

### `workflow_runs`

- id
- mission_id
- workflow_definition_id
- definition_version
- state
- started_at / completed_at
- current_summary

### `stage_runs`

- id
- workflow_run_id
- node_key
- stage_type
- state
- attempt
- assignee/agent reference where relevant
- issue_id nullable
- task_id nullable
- input_json / output_json
- failure_reason
- started_at / completed_at

### `workflow_artifacts`

- id
- mission_id
- stage_run_id nullable
- artifact_type
- title
- uri/reference
- metadata_json
- created_by actor metadata
- created_at

### `workflow_evaluations`

- id
- stage_run_id
- evaluator actor
- verdict
- score nullable
- findings_json
- evidence_json
- created_at

### `workflow_approvals`

- id
- stage_run_id
- requested_from
- state
- decision_by
- rationale
- requested_at / decided_at

Before migrations are written, verify whether existing generic relation/artifact/approval primitives can be reused.

## 13. Orchestrator rules

P0 orchestrator must be deterministic at workflow level:

- Agent autonomy is inside a bounded stage.
- The workflow engine, not an LLM, decides which legal transitions exist.
- LLM/evaluator output may feed a gate only through structured output validated by the server.
- Every transition is idempotent.
- Duplicate/replayed events must not start duplicate stage runs.
- Cancellation propagates to active child work where supported.
- Retry policy is explicit and bounded.
- Human approvals cannot be auto-generated by agents.
- Production-impacting stages default to a human gate.

## 14. Security and permissions

P0 requirements:

- Mission/workflow access is workspace-scoped.
- Starting/cancelling a mission requires explicit permission.
- Workflow definitions cannot escalate the underlying agent/runtime permissions.
- Approval stage records the authenticated human principal.
- Secrets stay in existing runtime/agent secret mechanisms.
- Agent-generated URLs/artifacts are untrusted input.
- Audit actor must distinguish human, agent and system transitions.

## 15. Implementation phases

### Phase 0 — Discovery & ADR

Map existing Multica primitives to the proposed domain and remove duplicated concepts before coding.

Deliverables:

- architecture map
- domain/ownership ADR
- final P0 schema
- API/event contract
- P0 workflow fixture

### Phase 1 — Mission foundation

Implement Mission CRUD/state, Workflow Definition storage/versioning and read-only Mission UI.

Exit: a user can create a mission from a workflow template and inspect its planned stages.

### Phase 2 — Orchestrator vertical slice

Implement sequential agent stages, durable stage state and linkage to existing Issue/Task/Run execution.

Exit: Product → Architect → Backend can execute automatically and resume after server restart.

### Phase 3 — Parallel + evaluation + retry

Add fan-out/fan-in, Test/Review evaluations, deterministic routing and bounded retry.

Exit: Backend + Frontend can run in parallel; failed test routes back and re-tests.

### Phase 4 — Human approval + inbox

Add approval stages and actionable inbox notifications.

Exit: release cannot continue until an authorized human approves.

### Phase 5 — Artifacts + provenance + metrics

Add artifact lineage, mission timeline, cost/time/intervention metrics and initial context relationships.

Exit: a mission provides an auditable requirement-to-result history.

### Phase 6 — Integrations / release loop

Connect CI/deployment/external events as needed for internal pilot.

Exit: one real Manifold project can run requirement → verified release through the loop.

## 16. Internal pilot

Use a real, bounded feature rather than a Todo demo.

Recommended candidates:

1. MindCloudX token licensing feature
2. Odin low-battery return-to-home feature

Success criteria:

- at least 4 specialist agent roles participate
- at least one parallel development stage
- at least one automated evaluation
- a failure can route back to the responsible stage and recover
- at least one human approval is mandatory
- code/PR/test evidence remains linked to the mission
- final mission timeline is understandable without reconstructing terminal/chat history

## 17. Non-goals for P0

- Fully autonomous company/team
- General BPMN replacement
- Separate graph database
- Kafka/NATS by default
- Replacing Multica Issue/Task/Runtime
- Hard-coding Pi as the only harness
- Building a new Git hosting system
- Building a new document editor
- Automatic production approval
- Dozens of specialist roles before the first vertical slice works

## 18. Development sequence / issue plan

Create implementation issues in this order after this proposal is reviewed:

1. **Discovery: map existing Multica domain to Agent Native Loop**
2. **ADR: Mission/Workflow ownership and execution boundaries**
3. **Design: P0 database schema + migrations**
4. **Design: Workflow definition schema and validation**
5. **Backend: Mission CRUD + permissions**
6. **Backend: Workflow Definition CRUD/versioning**
7. **Backend: durable Workflow Run / Stage Run state machine**
8. **Backend: agent-stage adapter to existing Issue/Task/Run**
9. **Frontend: Mission list + Mission detail skeleton**
10. **Frontend: workflow execution graph/timeline**
11. **Backend: parallel fan-out/fan-in**
12. **Backend: Evaluation + deterministic gates**
13. **Backend: bounded retry / failure routing**
14. **Backend + UI: human Approval stage**
15. **Inbox: workflow approvals/blockers**
16. **Artifacts: typed references + provenance**
17. **Metrics: mission/stage delivery metrics**
18. **Integration: CI/test/deployment event adapter**
19. **E2E: Feature Development workflow**
20. **Pilot: run one real Manifold feature through the loop**

Each implementation issue must include scope, dependencies, acceptance criteria, tests and explicit non-goals.

## 19. Go / no-go checkpoints

### Checkpoint A — after discovery

Do not proceed if Mission/Workflow duplicates an existing Multica primitive that can be extended cleanly.

### Checkpoint B — after sequential vertical slice

Do not add graph DB/event bus complexity unless persistence/recovery requirements demonstrate a gap.

### Checkpoint C — after internal pilot

Measure lead time, intervention rate, retry recovery and first-pass verification. Productize based on evidence rather than number of available agent roles.

## 20. Definition of success

The first milestone is successful when a human can provide a product intent and the platform can visibly coordinate multiple specialized agents through a deterministic, auditable software-delivery workflow, ask humans only at explicit decision/approval points, recover from a verification failure, and preserve the evidence linking requirement to shipped result.

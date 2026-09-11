# MNL-001 — Multica Capability Map for Manifold Agent Native Loop

> Status: discovery baseline  
> Branch: `feature/manifold-agent-native-loop`  
> Purpose: identify what Multica already provides before adding new Mission/Workflow concepts.

## Executive finding

The initial proposal assumed Manifold would need to add a new workflow/orchestrator layer relatively early. Code-level discovery shows Multica already contains more orchestration semantics than the product README alone suggests.

Most importantly, **Issue hierarchy + staged child issues already form a lightweight workflow primitive**:

- an Issue can have `parent_issue_id`
- child issues can have an integer `stage`
- siblings in the same stage form a barrier
- the parent is notified/woken only when the lowest unfinished stage closes
- multiple child issues in a stage can execute independently/in parallel
- terminal child statuses include done/cancelled
- parent agent/squad wake-up is server-controlled and guarded against duplicate/premature cascades

This means P0 should **extend the existing issue orchestration model before introducing a second general workflow state machine**.

The working recommendation after MNL-001 is therefore:

> Treat a top-level orchestration Issue as the initial Mission/Loop instance, staged child Issues as executable workflow nodes, existing Task/Run as attempts/execution, existing Agent/Squad as workers, and add only the missing deterministic policies, typed outputs/evaluations, approvals and provenance.

A dedicated `missions` / `workflow_runs` schema should remain an option, but it is no longer the default first implementation.

---

## 1. Existing product/runtime primitives

### Workspace

Existing tenant/team boundary. Manifold should stay workspace-scoped and should not create a parallel organization boundary.

### Project

Existing grouping/context primitive. It can scope a Native Loop instance and associated repository/docs context.

### Issue

This is substantially richer than a basic ticket and is the strongest candidate for the P0 orchestration record.

Observed fields/behavior include:

- workspace
- project
- status + custom status semantics
- priority
- assignee type/id
- creator
- `parent_issue_id`
- `stage`
- start/due dates
- metadata KV
- custom properties
- labels / attachments / source context
- revision / activity tracking

The important discovery is that `stage` is explicitly documented in code as grouping sub-issues under the same parent into ordered barrier groups.

### Child Issue Stage Barrier

`issue_child_done.go` implements server-side barrier semantics.

Behavior:

1. Child transitions from non-terminal into terminal.
2. Server loads the parent and siblings.
3. If the parent is already terminal or deliberately parked in backlog, nothing advances.
4. Human-assigned parents are not automatically woken.
5. For staged siblings, completion only wakes the parent when all siblings in the active/lowest unfinished stage are terminal.
6. For unstaged siblings, all siblings act as one implicit stage.
7. Parent agent/squad receives a server-generated notification/wake after the barrier closes.
8. The parent agent decides whether to activate/promote the next stage.

This is already close to:

`fan-out → wait-all → fan-in → coordinator wake → next stage`

and is directly reusable for a Feature Development loop.

### Agent / Squad

Existing executable teammates. A workflow node should point at an existing Agent or Squad; Manifold must not create separate BackendAgent/TestAgent tables.

Role names such as Product, Architect, Backend, Frontend, Embedded, Test and Review should be templates/configuration over existing agents.

### Runtime / Daemon

Existing execution plane. Agent readiness already has explicit semantics:

- **Available** — can run now
- **Waitable** — runtime offline but work may queue until it returns
- **Blocked** — agent/runtime configuration requires human intervention

Readiness logic is already shared across autopilot, squad and direct trigger paths. Native Loop orchestration should call/reuse the same admission/readiness service instead of inventing a second runtime health model.

### Task / Run

Existing per-agent execution attempt and history layer. Native Loop stage execution should link to Task/Run rather than duplicate logs, model/provider usage, retry history or CLI execution state.

### Skills

Existing reusable agent know-how. `ai-codespec` / Manifold engineering standards should enter through this model wherever possible.

### Autopilot

Existing scheduled/webhook/non-interactive dispatch primitive with quota/principal/admission behavior. This is a useful model for external triggers and recurring loop entry, but should not be stretched into the entire SDLC workflow abstraction.

### Inbox / Review

Existing human attention/review surface. Native Loop approvals, blockers and exception handling should extend this surface rather than create a second notification center.

---

## 2. Existing execution chain — conceptual map

The precise call graph spans handlers/services/daemon protocol, but the current ownership boundaries are clear enough for architectural planning:

```text
Human / Channel / Autopilot / parent-stage wake
                    │
                    ▼
                 Issue
             assignee = Agent/Squad
                    │
                    ▼
           admission/readiness
                    │
          ┌─────────┼─────────┐
          │         │         │
       blocked    queued    runnable
                    │
                    ▼
                 Task/Run
                    │
                    ▼
             Runtime / Daemon
                    │
                    ▼
        Agent CLI / harness execution
     Pi / Codex / Claude Code / etc.
                    │
                    ▼
       logs / comments / result / status
                    │
                    ▼
          child stage barrier check
                    │
          if barrier is closed
                    ▼
            wake parent assignee
                    │
                    └──→ next stage / decision
```

For Manifold, this is already a usable **micro-to-macro handoff path**.

---

## 3. Revised domain mapping

The original plan proposed new `Mission`, `WorkflowRun` and `StageRun` records immediately. After this discovery, P0 should test the following mapping first.

| Manifold concept | Prefer existing Multica primitive in P0 |
|---|---|
| Mission / loop instance | top-level orchestration Issue |
| Workflow node | child Issue |
| Stage / parallel barrier | child Issue `stage` |
| Worker | Agent / Squad |
| Agent attempt | Task / Run |
| Agent runtime | Runtime / Daemon |
| Role knowledge | Skill |
| Human attention | Inbox / existing review surfaces |
| Project context | Project + Issue source context / metadata |
| Trigger | Issue assignment / comment / Autopilot / channel / webhook |
| Loop state | parent + child Issue statuses and metadata |

This should be the default unless MNL-002 demonstrates a concrete requirement that cannot be expressed safely with these primitives.

---

## 4. What is actually missing

The Native Loop value is therefore not primarily another task engine. The meaningful gaps are:

### 4.1 Workflow Definition / Template

There is staged issue execution, but we still need a reusable declarative definition that can instantiate a known engineering loop, for example:

```text
Product
  ↓
Architecture
  ↓
Backend ─┬─ Frontend ─┬─ Embedded(optional)
         └─────────────┘
  ↓
Review
  ↓
Test
  ↓
Approval
  ↓
Release
```

P0 may compile a template into a parent Issue + staged child Issues rather than create a separate runtime graph.

### 4.2 Deterministic Transition Policy

Existing stage advancement intentionally lets the parent agent decide what to promote next. Manifold needs stronger SDLC rules in some places:

- legal next stage(s)
- retry target
- bounded retry count
- pass/fail gate
- optional role/node
- manual approval gate
- cancellation semantics

Recommendation: add deterministic policy around existing staged Issues, not replace them.

### 4.3 Structured Evaluation

Comments/status alone are insufficient for reliable Test/Review gates.

Need a validated machine-readable result such as:

```json
{
  "verdict": "pass",
  "findings": [],
  "evidence": ["artifact://..."]
}
```

The server should own validation and transition consequences.

### 4.4 Human Approval

A comment or ordinary status change should not equal approval for release/security-sensitive gates.

Need explicit authenticated decision + rationale + audit actor.

### 4.5 Artifact / Provenance Relationships

Need typed links between:

`intent → issue → task/run → branch/commit/PR → test evidence → build/deploy → feedback`

P0 should keep PostgreSQL and references; no graph database yet.

### 4.6 Workflow-level metrics

Existing run/task usage is valuable but Native Loop needs aggregate delivery measures:

- loop lead time
- first-pass Test/Review success
- stage retries
- human intervention count/time
- requirement-to-release duration

---

## 5. Important architectural consequence

### Do not create a parallel execution stack

Avoid in P0:

- `manifold_tasks`
- `manifold_agent_runs`
- separate runtime queue
- separate daemon
- separate Agent model
- separate Skill model
- separate review inbox

All of these would duplicate mature Multica behavior.

### Defer dedicated Mission tables

A separate Mission entity may still become useful when one product intent must span multiple independent parent Issues/projects/repos/releases. But that requirement should be proven by the pilot.

For the first vertical slice, a top-level orchestration Issue has major advantages:

- existing permissions
- existing comments/activity
- existing project attachment
- existing assignee semantics
- existing parent/child model
- existing board/search/inbox visibility
- existing API/client compatibility patterns
- existing stage barrier behavior

---

## 6. Proposed P0 architecture after discovery

```text
                 Manifold Loop Template
                          │
                          │ instantiate
                          ▼
                 Parent Orchestration Issue
                          │
         ┌────────────────┼────────────────┐
         │ stage 10       │ stage 20       │ stage 30
         ▼                ▼                ▼
   Product Issue    Development Issues    Review/Test Issues
                        │  │  │
                        ▼  ▼  ▼
                   Agent/Squad assignees
                        │
                        ▼
                    Task / Run
                        │
                        ▼
                 Runtime / Daemon
                        │
                        ▼
                Pi/Codex/Claude/...
                        │
                        ▼
               structured result/evidence
                        │
                        ▼
                 stage barrier closes
                        │
                        ▼
              deterministic loop policy
                        │
            ┌───────────┴───────────┐
            ▼                       ▼
        next stage              human gate
```

Use sparse stage numbers (`10, 20, 30...`) so templates can insert steps later without renumbering semantics if stage values become user-visible/configurable.

---

## 7. P0 Feature Development template — revised

Suggested compilation:

### Parent Issue

`[Loop] <feature title>`

Responsibilities:

- original product intent
- project scope
- loop/template version metadata
- high-level acceptance state
- coordinator assignee (human or planning/orchestrator agent)

### Stage 10 — Product

- Product child Issue
- assignee: Product Agent
- output: structured requirement + acceptance criteria artifact

### Stage 20 — Architecture

- Architecture child Issue
- assignee: Architect Agent
- consumes Product output

### Stage 30 — Development (parallel barrier)

- Backend child Issue
- Frontend child Issue
- Embedded child Issue when applicable
- all use `stage=30`

### Stage 40 — Review

- Review child Issue
- structured evaluation

### Stage 50 — Test

- Test child Issue
- structured evaluation
- fail can deterministically reopen/route to responsible development child and later return to Test

### Stage 60 — Human Approval

- server-owned approval gate, not an agent-completable Issue alone

### Stage 70 — Release

- Release child Issue
- production permission remains controlled by underlying agent/runtime credentials plus approval policy

---

## 8. Questions MNL-002 must answer

1. Can a top-level Issue safely serve as the P0 Mission without harming normal Issue semantics/UI?
2. Should workflow/template identity live in Issue metadata initially, or does it justify first-class columns/tables?
3. How should a workflow node become active: status promotion, assignment trigger, explicit dispatch, or an existing server path?
4. Where should deterministic transition policy live so it does not conflict with current parent-agent-driven stage advancement?
5. Can explicit approvals extend an existing review/notification model, or require one small dedicated approval table?
6. What is the smallest typed artifact/evaluation representation that survives retries and remains queryable?
7. How are task completion and issue status causally tied today across every supported CLI, and where is the safest hook for structured stage completion?
8. How should workflow retries coexist with existing task automatic retries? They are different levels and must not double-retry accidentally.
9. How does cancellation propagate through parent Issue → child Issue → active Task/Run today?
10. Which existing WebSocket events can drive the execution graph UI without adding a parallel event bus?

---

## 9. Changes to the previous plan

### Previous assumption

Create early:

- missions
- workflow_definitions
- workflow_runs
- stage_runs
- workflow_artifacts
- workflow_evaluations
- workflow_approvals

### Revised P0 bias

Create only what cannot be expressed with current primitives.

Likely first additions:

1. **Loop Template / Definition** — exact storage TBD in MNL-002.
2. **Loop instance metadata** on/around the parent Issue.
3. **Structured Evaluation** storage.
4. **Explicit Approval** storage.
5. **Artifact/Provenance references** if existing attachment/source-context models are insufficient.
6. **Transition policy/orchestrator service** that operates existing Issues/Tasks.

Dedicated Mission/WorkflowRun/StageRun tables are deferred until a concrete gap is demonstrated.

---

## 10. Immediate recommendation

Before writing migrations, MNL-002 should produce an ADR comparing exactly two implementation strategies:

### Option A — Extend existing staged Issues (recommended baseline)

Template compiles to parent + child Issues; orchestrator adds deterministic policy/eval/approval.

### Option B — New Mission/Workflow state machine

New domain records own execution and link out to Issues/Tasks.

Compare them on:

- persistence/restart safety
- retry semantics
- UI impact
- permissions
- realtime events
- auditability
- external integrations
- compatibility with upstream Multica changes
- ability to span multiple projects/repos
- migration/maintenance cost

Unless Option B has a clear requirement-level advantage, choose Option A for the first internal pilot.

---

## 11. MNL-001 conclusion

**Go forward, but reduce the amount of new infrastructure.**

Multica already provides the agent workforce, runtime execution, task history and—critically—a staged parent/child Issue coordination mechanism. Manifold's P0 should turn that mechanism into a repeatable, deterministic software-delivery loop by adding templates, policy, structured verification, approvals and provenance.

That gives us a much smaller and safer first vertical slice while preserving the option to promote Mission/Workflow into first-class entities later, based on real pilot constraints rather than architectural speculation.

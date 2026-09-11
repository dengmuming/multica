# MNL-004 — Loop Template Schema + Compiler Contract

> Status: Proposed implementation contract  
> Branch: `feature/manifold-agent-native-loop`  
> Depends on: MNL-001 capability map, MNL-002 orchestration ADR, MNL-003 persistence design

## 1. Decision summary

P0 introduces a **Loop Template** as a deterministic, versioned description of an engineering delivery loop.

A Loop Template is **not** an executable workflow runtime. It is compiled into existing Multica primitives:

- one parent Issue = loop instance / mission
- child Issues = loop nodes
- `issue.stage` = ordered barrier group
- Issue assignee = role binding to existing Agent / Squad / Member
- Task/Run = existing agent execution history
- Evaluation / Approval / Artifact = Manifold-specific structured records from MNL-003

The compiler is responsible for producing a legal, deterministic Issue tree. The runtime policy layer is responsible for activating/reopening nodes based on current issue state, structured evaluations, approvals, and retry budgets.

The LLM/Agent is never the source of truth for legal workflow transitions.

---

## 2. Design goals

The P0 schema must support the first real `feature-development` loop:

`Product → Architecture → Development(parallel) → Review → Test → Human Approval → Release`

with these semantics:

- sequential stages
- parallel nodes inside one stage
- optional nodes such as Embedded
- agent / squad / member role binding
- deterministic evaluation gates
- failure routing back to a named node/role
- bounded workflow retry
- explicit human approval
- typed outputs/artifacts
- template version pinning
- no arbitrary graph cycles in P0

The schema should be intentionally smaller than BPMN / Temporal / LangGraph. We can widen it after internal pilot evidence demonstrates a need.

---

## 3. Canonical JSON form

Although authoring tools may later support YAML, `loop_template.definition` stores canonical JSON.

Example:

```json
{
  "schema_version": 1,
  "key": "feature-development",
  "name": "Feature Development",
  "description": "Standard Manifold feature delivery loop",
  "roles": {
    "product": { "required": true },
    "architect": { "required": true },
    "backend": { "required": false },
    "frontend": { "required": false },
    "embedded": { "required": false },
    "review": { "required": true },
    "test": { "required": true },
    "release": { "required": true }
  },
  "nodes": [
    {
      "key": "product",
      "type": "agent",
      "stage": 10,
      "role": "product",
      "title": "Product specification",
      "required": true,
      "outputs": ["product_spec", "acceptance_criteria"]
    },
    {
      "key": "architecture",
      "type": "agent",
      "stage": 20,
      "role": "architect",
      "title": "Architecture and implementation plan",
      "required": true,
      "inputs": ["product_spec", "acceptance_criteria"],
      "outputs": ["architecture_plan"]
    },
    {
      "key": "backend",
      "type": "agent",
      "stage": 30,
      "role": "backend",
      "title": "Backend implementation",
      "required": false,
      "inputs": ["architecture_plan"],
      "outputs": ["code_change"]
    },
    {
      "key": "frontend",
      "type": "agent",
      "stage": 30,
      "role": "frontend",
      "title": "Frontend implementation",
      "required": false,
      "inputs": ["architecture_plan"],
      "outputs": ["code_change"]
    },
    {
      "key": "embedded",
      "type": "agent",
      "stage": 30,
      "role": "embedded",
      "title": "Embedded implementation",
      "required": false,
      "inputs": ["architecture_plan"],
      "outputs": ["code_change"]
    },
    {
      "key": "review",
      "type": "evaluation",
      "stage": 40,
      "role": "review",
      "title": "Engineering review",
      "evaluation": {
        "kind": "engineering_review",
        "allowed_verdicts": ["pass", "fail", "warn", "inconclusive"],
        "on_fail": {
          "strategy": "route_by_finding_owner",
          "fallback_node": "architecture",
          "max_workflow_retries": 2
        }
      }
    },
    {
      "key": "test",
      "type": "evaluation",
      "stage": 50,
      "role": "test",
      "title": "Verification",
      "evaluation": {
        "kind": "test",
        "allowed_verdicts": ["pass", "fail", "inconclusive"],
        "on_fail": {
          "strategy": "route_by_finding_owner",
          "fallback_node": "backend",
          "max_workflow_retries": 3
        }
      }
    },
    {
      "key": "release_approval",
      "type": "approval",
      "stage": 60,
      "title": "Release approval",
      "approval": {
        "required_role": "member",
        "min_approvals": 1,
        "on_reject": {
          "target_node": "architecture",
          "max_workflow_retries": 1
        }
      }
    },
    {
      "key": "release",
      "type": "agent",
      "stage": 70,
      "role": "release",
      "title": "Release",
      "required": true,
      "inputs": ["test_result", "approval"],
      "outputs": ["deployment"]
    }
  ]
}
```

---

## 4. Template structure

### 4.1 Root fields

Required:

```text
schema_version   integer
key              string
name             string
roles            object
nodes            array
```

Optional:

```text
description      string
labels           string[]
defaults         object
```

Rules:

- `schema_version` must be `1` in P0.
- `key` must be stable across template versions.
- `key` format: `^[a-z][a-z0-9-]{1,62}$`.
- `nodes` must contain at least one node.
- maximum P0 node count: 64.
- root definition is immutable once the corresponding template version is active.

### 4.2 Roles

Roles are symbolic execution slots. They do **not** hard-code an Agent ID into the reusable template.

```json
"roles": {
  "backend": {
    "required": false,
    "description": "Backend implementation owner",
    "allowed_assignee_types": ["agent", "squad"]
  }
}
```

P0 role fields:

```text
required                 boolean
allowed_assignee_types   optional array: agent | squad | member
```

Role binding happens when instantiating a loop.

Example instance bindings:

```json
{
  "product":   { "type": "agent", "id": "..." },
  "architect": { "type": "agent", "id": "..." },
  "backend":   { "type": "agent", "id": "..." },
  "frontend":  { "type": "agent", "id": "..." },
  "test":      { "type": "agent", "id": "..." },
  "review":    { "type": "agent", "id": "..." },
  "release":   { "type": "agent", "id": "..." }
}
```

A missing binding is legal only when every node using that role is optional (`required=false`). Those nodes are omitted during compilation.

---

## 5. Node types

P0 supports three persisted node types:

```text
agent
 evaluation
 approval
```

There is intentionally no separate `parallel` node. Parallelism is represented by multiple nodes sharing the same stage.

There is intentionally no generic `condition` or `wait` node in P0. Evaluation and Approval provide the two branching/waiting semantics required for the initial vertical slice.

### 5.1 Agent node

Represents an Issue assigned to an Agent/Squad/Member and executed through normal Multica semantics.

Fields:

```text
key            string, required
 type           "agent"
 stage          positive integer
 role           string, required
 title          string, required
 description    string, optional
 required       boolean, default true
 inputs         string[], optional artifact kinds
 outputs        string[], optional artifact kinds
 max_retries    integer, optional workflow/business retry cap
```

`max_retries` here is **workflow retry**, not `agent_task_queue.max_attempts`.

### 5.2 Evaluation node

An executable evaluator followed by a structured `loop_evaluation` record.

Most P0 evaluations are expected to be Agent-backed, so an evaluation node also has a role binding and therefore compiles to a normal Issue.

Fields:

```text
key
 type = evaluation
 stage
 role
 title
 required
 evaluation
```

Evaluation config:

```text
kind
 allowed_verdicts
 on_fail.strategy
 on_fail.target_node (for fixed_target)
 on_fail.fallback_node
 on_fail.max_workflow_retries
 on_inconclusive (optional: block | retry_evaluator)
```

Allowed fail strategies in P0:

```text
fixed_target
 route_by_finding_owner
 block
```

`route_by_finding_owner` consumes only server-validated `finding.owner_role` / `finding.owner_node_key` fields. Arbitrary agent prose must never be interpreted as a route.

### 5.3 Approval node

Represents a deterministic human gate.

It compiles to an Issue so it remains visible in the same parent tree, but completion is controlled by `loop_approval` rather than an Agent saying "done".

Fields:

```text
key
 type = approval
 stage
 title
 approval
```

Approval config:

```text
required_role       P0: member
min_approvals       P0: 1 only
on_reject.target_node
on_reject.max_workflow_retries
```

P0 explicitly rejects:

- approval by agent
- implicit approval from comments
- quorum > 1
- time-based auto-approval

These can be considered after the first pilot.

---

## 6. Stage semantics

`stage` maps directly onto existing `issue.stage`.

Rules:

1. Stage is a positive integer.
2. Stage numbers need not be contiguous.
3. Recommended authoring convention is `10, 20, 30...` to allow future insertion.
4. Nodes with the same stage are a parallel barrier group.
5. A later stage must not be activated until the lowest unfinished earlier stage is terminal according to Manifold policy.
6. The compiler must preserve stage number exactly; it must not renumber an active template version.

Important compatibility note:

Multica's current child-done logic already detects when all siblings in the lowest unfinished stage are terminal and wakes the parent assignee. The Manifold policy layer should reuse the barrier calculation but become the authority for generated loop instances, rather than asking the parent Agent to invent the next transition.

For generated loops, parent wake is a useful notification/coordinator signal, not the workflow truth source.

---

## 7. Compilation contract

### 7.1 Inputs

Compiler input:

```text
workspace_id
project_id? 
template_key
template_version
parent title
description / intent
creator principal
role_bindings
optional initial context/artifacts
```

### 7.2 Preflight validation

Before writing anything, compiler must validate:

- template is active
- requested exact version exists
- workspace owns the template
- caller may instantiate loops
- every required role has a binding
- each bound Agent/Squad/Member exists in the workspace
- assignee type is allowed by role
- every node key is unique
- every stage is > 0
- every role reference exists
- every route target exists
- route target must point to an earlier or same logical development/recovery node; arbitrary forward jumps are rejected in P0
- no evaluation/approval retry budget is negative
- there is at least one terminal forward path
- omitted optional nodes do not leave an impossible stage

Instantiation should fail before creating the parent Issue if any validation fails.

### 7.3 Atomicity

Parent Issue + all generated child Issues + metadata bindings should be created in one application transaction where current repository transaction patterns permit.

Because the repository's current design avoids database FKs, the compiler service owns cleanup/integrity.

A partially compiled loop must never become visible as a valid running loop.

### 7.4 Parent Issue

Compiler creates one parent Issue with:

```text
title        human-provided mission/feature title
project_id   selected project
status       todo or in_progress according to instantiate/start API
assignee     optional coordinator Agent/Squad; not workflow authority
```

Metadata:

```text
manifold.loop.template_key
manifold.loop.template_version
manifold.loop.instance_key
manifold.loop.state
manifold.loop.policy_version
manifold.loop.retry_count
```

Recommended P0 `manifold.loop.state` values:

```text
ready
running
blocked
waiting_approval
completed
failed
cancelled
```

This state is a denormalized summary. Canonical node truth remains child Issue state + Evaluation/Approval records.

### 7.5 Child Issues

For each included node, compiler creates exactly one child Issue.

Child metadata:

```text
manifold.loop.node_key
manifold.loop.node_type
manifold.loop.role      # omitted for approval node
manifold.loop.retry_count
manifold.loop.generated = true
manifold.loop.required
manifold.loop.max_retries
```

Issue fields:

```text
parent_issue_id = parent.id
stage           = node.stage
assignee        = resolved role binding for executable nodes
```

Node title is generated from template title, with optional later customization.

### 7.6 Initial statuses

P0 compilation rule:

- the minimum stage included in the compiled graph becomes `todo`
- every later stage starts as `backlog`
- parent becomes `in_progress` when started

This matches existing Multica semantics where a parent in backlog intentionally suppresses auto-wake, and keeps not-yet-active child nodes inert.

The policy service, not an Agent, promotes the next stage from `backlog` to `todo` when its predecessor barrier is legally satisfied.

---

## 8. Runtime policy contract

Compiler creates structure; Policy advances it.

Policy inputs:

```text
parent Issue
child Issues
latest loop_evaluation per relevant evaluation node
current loop_approval per approval node
loop template version
activity / task evidence when required
```

Policy returns an explicit command plan, for example:

```json
{
  "actions": [
    {"type": "activate_node", "node_key": "review"},
    {"type": "set_parent_state", "state": "running"}
  ],
  "reason": "stage_30_barrier_closed"
}
```

Policy evaluation itself must be deterministic and side-effect free. A separate application service applies the returned commands transactionally/idempotently.

### Required action types for P0

```text
activate_node
reopen_node
complete_node
cancel_node
set_parent_state
request_approval
block_loop
complete_loop
```

No `goto arbitrary-node` API should be exposed to Agents.

---

## 9. Evaluation contract

An Evaluation Agent can write prose/comments, but workflow routing consumes only a structured evaluation payload accepted by the server.

P0 submission shape:

```json
{
  "evaluation_kind": "test",
  "verdict": "fail",
  "score": 0.62,
  "findings": [
    {
      "code": "api_contract_failure",
      "severity": "high",
      "owner_role": "backend",
      "owner_node_key": "backend",
      "summary": "POST /licenses/token returns 500 for expired token",
      "artifact_refs": ["artifact-id"]
    }
  ],
  "evidence": ["artifact-id"]
}
```

Server validation rules:

- evaluation node must match the submitting Task/Issue context
- `verdict` must be allowed by the node contract
- `evaluation_kind` must equal template config
- owner role/node must exist in this compiled loop
- finding target must be legal according to template policy
- artifact refs must be visible in the workspace/loop
- result cannot name a retry count or mutate statuses itself

### Test failure example

```text
Test task COMPLETED successfully at execution layer
        ↓
Evaluation verdict = FAIL
        ↓
Policy resolves owner_node_key = backend
        ↓
backend workflow_retry_count < max?
        ├─ no → block loop / human intervention
        └─ yes
            ↓
          reopen backend Issue
            ↓
          increment backend workflow retry
            ↓
          keep later dependent stages inactive/reopen as policy requires
            ↓
          normal Multica dispatch creates a NEW task
```

This is deliberately separate from infrastructure task retry.

---

## 10. Workflow retry semantics

### 10.1 Retry accounting

Each generated node carries a workflow retry count in issue metadata. Rich history is visible through Activity + Evaluation + Task lineage.

Increment only when a deterministic loop policy deliberately reopens the node after a completed business/evaluation attempt.

Do not increment workflow retry for:

- CLI crash
- provider timeout
- daemon disconnect
- transient tool failure that Multica auto-retries

Those remain `agent_task_queue.retry_of_task_id` / `attempt` semantics.

### 10.2 Invalidation of downstream work

P0 chooses a conservative rule:

If a node is reopened due to Review/Test failure, every **later-stage generated node that semantically depends on its output** is reset to `backlog` unless already terminal and explicitly marked reusable by a future policy feature.

For P0, keep this simple:

- reopening Stage 30 development resets Stage 40+ generated nodes
- previous Tasks/Evaluations are never deleted
- old evaluations remain historical; policy always selects the latest valid evaluation for the current node attempt/generation

This avoids accidentally releasing from stale test evidence.

---

## 11. Approval contract

When an approval node becomes active:

1. Approval Issue becomes `todo` / waiting-human state according to current status catalog semantics.
2. Server creates a `loop_approval(state=pending)` record.
3. Parent summary state becomes `waiting_approval`.
4. Existing Inbox sends action-required notification to the intended human recipient.
5. Only an authenticated authorized member can call approve/reject.

On approve:

```text
loop_approval.state = approved
approval Issue -> done
activity logged
next stage promoted
```

On reject:

```text
loop_approval.state = rejected
approval Issue -> terminal/current historical state
policy applies configured reject target + retry budget
```

A comment such as `LGTM`, Agent completion, or changing the approval Issue directly to done must not create an approved decision.

Implementation should guard generated approval Issues so normal Issue status mutation cannot bypass pending approval policy.

---

## 12. Optional-node semantics

A node is omitted during compilation when:

- `required=false`, and
- its role has no instance binding.

Example: Odin feature may bind `embedded`; a RAG backend-only feature may not.

The compiler must calculate active stage groups after omission.

Example template:

```text
Stage 30: backend(optional), frontend(optional), embedded(optional)
```

If only backend is bound:

```text
Stage 30 compiled children: backend only
```

If none are bound and at least one of those nodes is needed for every valid path, instantiation fails rather than creating an empty meaningless stage.

P0 does not support runtime conditional inclusion after compilation. Optionality is resolved once at loop creation.

---

## 13. Artifact input/output semantics

`inputs` and `outputs` are declarative artifact kinds, not file paths.

P0 compiler stores them as node contract metadata in the template; it does not copy artifacts during compilation.

Before activating a node, policy/context assembly may verify required inputs exist in `loop_artifact`.

Example:

```text
Product outputs: product_spec, acceptance_criteria
Architecture requires: product_spec
Backend requires: architecture_plan
Test requires: code_change
Release requires: test_result, approval
```

P0 behavior when a required artifact is missing:

```text
block node activation
parent -> blocked
activity + inbox explanation
```

Do not silently ask an Agent to infer missing contractual inputs from chat history.

---

## 14. Idempotency

The compiler and policy layer must be replay-safe.

### Compiler

Instantiation request receives/derives an `instance_key`.

Within a workspace, repeated create with the same `instance_key` must return the existing parent loop or conflict; it must not create a second Issue tree.

### Policy

Every transition command must check current state before writing.

Examples:

- activating an already active/done node is a no-op
- barrier-close event replay must not create duplicate Tasks
- evaluation replay must not reopen a node twice
- approval decision endpoint must be compare-and-set from pending

Existing Task trigger dedup/idempotency patterns should be reused where possible.

---

## 15. Permissions

P0 permission rules:

- template create/edit/publish: workspace admin/owner or future explicit permission
- instantiate loop: member with permission to create Issues and invoke every required bound Agent/Squad
- template binding cannot escalate an Agent's existing invocation permission
- approval: authenticated workspace member satisfying the requested approval policy
- direct Agent calls cannot approve
- policy executor acts as system but records the originating evidence/principal

Instantiation must preflight all required role bindings instead of creating a loop that can never dispatch because one Agent is forbidden to the caller.

---

## 16. Schema validation algorithm

Validation order:

1. parse JSON
2. check schema version
3. validate root/key/limits
4. validate roles
5. validate node shape by type
6. enforce unique node keys
7. enforce positive stage
8. validate referenced roles
9. validate evaluation/approval routes
10. validate retry budgets
11. validate artifact kind names
12. derive stage groups
13. assert at least one legal forward completion path

Suggested Go package:

```text
server/internal/looptemplate/
  types.go
  validate.go
  compile.go
  policy.go        # may begin later; keep package boundary clear
```

Do not place schema validation in HTTP handlers.

---

## 17. Proposed Go types

Illustrative only; names can adapt to repository conventions during implementation.

```go
type Definition struct {
    SchemaVersion int                 `json:"schema_version"`
    Key           string              `json:"key"`
    Name          string              `json:"name"`
    Description   string              `json:"description,omitempty"`
    Roles         map[string]RoleSpec `json:"roles"`
    Nodes         []Node              `json:"nodes"`
}

type RoleSpec struct {
    Required             bool     `json:"required"`
    AllowedAssigneeTypes []string `json:"allowed_assignee_types,omitempty"`
}

type Node struct {
    Key         string          `json:"key"`
    Type        string          `json:"type"`
    Stage       int32           `json:"stage"`
    Role        string          `json:"role,omitempty"`
    Title       string          `json:"title"`
    Description string          `json:"description,omitempty"`
    Required    *bool           `json:"required,omitempty"`
    Inputs      []string        `json:"inputs,omitempty"`
    Outputs     []string        `json:"outputs,omitempty"`
    MaxRetries  *int32          `json:"max_retries,omitempty"`
    Evaluation  *EvaluationSpec `json:"evaluation,omitempty"`
    Approval    *ApprovalSpec   `json:"approval,omitempty"`
}
```

Use explicit custom validation after JSON decode; do not rely on loose `map[string]any` throughout business logic.

---

## 18. API contract for first implementation

P0 endpoints (paths provisional; follow repository route conventions when implemented):

```text
POST   /workspaces/{ws}/loop-templates
GET    /workspaces/{ws}/loop-templates
GET    /workspaces/{ws}/loop-templates/{key}/versions/{version}
POST   /workspaces/{ws}/loop-templates/{id}/publish

POST   /workspaces/{ws}/loops/instantiate
GET    /workspaces/{ws}/loops/{parent_issue_id}

POST   /workspaces/{ws}/loops/{parent_issue_id}/evaluations
POST   /workspaces/{ws}/loops/{parent_issue_id}/approvals/{approval_id}/approve
POST   /workspaces/{ws}/loops/{parent_issue_id}/approvals/{approval_id}/reject
```

The exact public routes are deferred to implementation review because Multica may prefer resource loaders or issue-centric nesting. The important contract is ownership and semantics, not the provisional URL spelling.

---

## 19. Explicit non-goals / rejected features for P0

Rejected from the initial DSL:

- arbitrary directed graph edges
- cycles in template definition
- generic script nodes
- arbitrary expressions / embedded JavaScript
- LLM-chosen next node
- BPMN compatibility
- dynamic runtime node creation by Agents
- runtime conditional optional-node inclusion
- quorum/multi-party approvals
- timers / wait-until nodes
- compensation transactions
- cross-workspace loops
- sub-workflows / nested loops

Why: all add orchestration surface before the first engineering loop is proven. Existing staged Issues already cover the primary sequencing and fan-out/fan-in problem.

---

## 20. P0 compiler example

Input:

```text
Template: feature-development@1
Title: Add token-based licensing
Bindings:
  product   -> Product Agent
  architect -> Architect Agent
  backend   -> Backend Agent
  frontend  -> Frontend Agent
  embedded  -> (none)
  review    -> Review Agent
  test      -> Test Agent
  release   -> Release Agent
```

Compiled result:

```text
Parent: [Loop] Add token-based licensing

Stage 10
  Product specification          Product Agent       todo

Stage 20
  Architecture                   Architect Agent     backlog

Stage 30
  Backend implementation         Backend Agent       backlog
  Frontend implementation        Frontend Agent      backlog
  # Embedded omitted

Stage 40
  Engineering review             Review Agent        backlog

Stage 50
  Verification                   Test Agent          backlog

Stage 60
  Release approval               human gate          backlog

Stage 70
  Release                        Release Agent       backlog
```

Execution:

```text
Product done
  ↓ policy
Architecture todo
  ↓
Architecture done
  ↓ policy
Backend + Frontend todo in parallel
  ↓ barrier
Review todo
  ↓
Review pass
  ↓
Test todo
  ↓
Test fail(owner=backend)
  ↓ policy
Backend reopened; Stage 40+ reset/backlog
  ↓
Backend fixes
  ↓
Review → Test again
  ↓
Test pass
  ↓
Approval requested
  ↓ human approve
Release todo
  ↓
Release done
  ↓
Parent completed
```

---

## 21. Implementation split after this ADR

With MNL-004 complete, implementation should no longer begin with the old generic Mission CRUD backlog. Recommended next engineering sequence is:

1. **MNL-005A — DB migrations/sqlc for loop_template, loop_evaluation, loop_approval, loop_artifact**
2. **MNL-005B — `looptemplate` Go types + validator**
3. **MNL-006 — Template CRUD/publish/versioning**
4. **MNL-007 — Loop compiler / instantiate transaction**
5. **MNL-008 — Deterministic stage policy + activation**
6. **MNL-009 — Evaluation submission + fail routing**
7. **MNL-010 — Approval + Inbox gate**
8. **MNL-011 — Shared Web/Desktop loop view**
9. **MNL-012 — E2E feature-development pilot**

This supersedes the original assumption that P0 requires Mission, WorkflowRun and StageRun implementation first.

---

## 22. Definition of done for MNL-004

MNL-004 is complete when the team agrees that:

- one stable JSON schema can describe the initial engineering loop
- parallelism is represented by shared Multica stages
- optional roles are resolved at compile time
- role bindings point only to existing Multica principals
- evaluation failure routing is deterministic and bounded
- approval is an explicit human-controlled gate
- Task retry and Workflow retry remain separate
- compiler output is fully representable with existing Parent/Child Issues + Task/Run
- no general-purpose workflow engine is required for the first pilot

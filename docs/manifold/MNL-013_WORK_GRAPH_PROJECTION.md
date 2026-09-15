# MNL-013 — Work Graph Projection

> Status: Ready for implementation after P0 verification

## Problem

MNL currently has the correct execution primitives (`Loop Template`, parent/child Issues, stages, Tasks, Evaluations, Approvals and Artifacts), but those primitives expose the Loop Engine mental model. The Manifold Agent product should present an **Intent-centered Work Graph**.

This issue introduces a read/projection boundary. It does not replace Loop Template or Issue persistence.

## Goal

Expose one stable product contract:

```text
Intent → Work Graph → Work Nodes → Execution / Evidence / Gates
```

while continuing to source truth from the existing P0 data model.

## Proposed contract

```ts
type WorkGraph = {
  id: string
  workspaceId: string
  projectId: string
  intent: {
    title: string
    description?: string
    source?: string
  }
  status: 'planning' | 'running' | 'blocked' | 'awaiting_approval' | 'released' | 'observing' | 'completed' | 'failed' | 'cancelled'
  currentStage?: string
  nodes: WorkGraphNode[]
  edges: WorkGraphEdge[]
  gates: WorkGraphGate[]
  provenanceSummary: {
    artifactCount: number
    evaluationCount: number
  }
}

type WorkGraphNode = {
  key: string
  kind: 'agent' | 'evaluation' | 'approval' | 'release' | 'observe'
  role?: string
  responsibility?: string
  stage: string
  state: string
  assignee?: {
    type: 'agent' | 'squad' | 'human'
    id: string
    name?: string
  }
  currentTask?: {
    id: string
    status: string
    attempt?: number
  }
  evaluation?: {
    verdict: string
    score?: number
    findingCount: number
  }
  artifactCount: number
}

type WorkGraphEdge = {
  from: string
  to: string
  kind: 'dependency' | 'recovery' | 'gate'
}

type WorkGraphGate = {
  nodeKey: string
  type: 'human_approval' | 'evaluation'
  state: string
}
```

The concrete language may be Go + TypeScript in their respective boundaries; the semantic contract above is normative.

## Projection sources

Use existing canonical state:

```text
Parent Issue metadata
+ Child Issues
+ pinned Loop Template
+ persisted role bindings
+ newest Task per execution node
+ current-attempt Evaluation
+ Approval
+ Artifact counts
        ↓
WorkGraph projection
```

Do not add a `work_graph` table in this issue.

## Backend tasks

- [ ] Add `loopservice.WorkGraphReader` interface.
- [ ] Implement `MulticaWorkGraphProjectionReader` using existing Loop read/projection repositories.
- [ ] Add deterministic node/edge construction from the exact pinned template.
- [ ] Map parent Loop state into product-facing Work Graph status.
- [ ] Include current authoritative Task only; never surface a stale Task as current.
- [ ] Include current-attempt Evaluation only.
- [ ] Include Approval gate state.
- [ ] Include artifact/evaluation summary counts.
- [ ] Add `GET /api/loops/{parentIssueId}/work-graph`.
- [ ] Reuse existing Auth + workspace membership middleware.
- [ ] Add unit tests for parallel nodes, recovery edge, stale Task, approval gate and terminal Loop.

## Frontend tasks

- [ ] Add `WorkGraph` API/types under the existing Loop/core package boundary.
- [ ] Build a Work Graph renderer using product terms, not BPMN terms.
- [ ] Render role/assignee/state on nodes.
- [ ] Distinguish dependency, recovery and gate edges.
- [ ] Clicking a node opens Task/Evaluation/Artifact details.
- [ ] Keep existing Loop detail available while Work Graph matures.

## Non-goals

- Generic workflow/BPMN editor.
- New orchestration database.
- Replacing Issue as canonical work item.
- Replacing Template compiler.
- Dynamic graph mutation by an LLM without policy validation.
- Context Graph persistence; that is MNL-019.

## Acceptance scenarios

### Parallel development

```text
Architecture
   ├── Backend
   └── Frontend
        ↓ barrier
      Review
```

The projection must expose two dependency edges out of Architecture and preserve the barrier semantics visible in the compiled graph.

### Test failure

```text
Test FAIL → Backend repair → Review → Test
```

The projection must expose the configured recovery relationship without rewriting historical evidence.

### Human approval

The Approval node must show pending/approved/rejected state and remain clearly different from an Agent execution node.

### Runtime independence

No Work Graph API consumer should need to know whether an execution was dispatched through Multica, Pi, or a future runtime adapter.

## Done condition

A product UI can answer, from one API response:

> What is the engineering intent, who is responsible for each part, what depends on what, what is running now, what failed, what evidence exists, and what gate is blocking delivery?

without reconstructing those semantics in the browser.
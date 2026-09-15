# Manifold Agent — Product Boundary and Architecture

> Status: product/architecture decision after MNL P0 implementation and whiteboard review

## 1. Product definition

**Manifold Agent is not a single Agent and is not a generic Workflow Engine.**

It is an **AI-native Engineering Organization / Engineering OS** that coordinates humans and specialized AI agents around real engineering goals and continuously closes the loop from intent to delivery, runtime feedback, and learning.

Recommended definition:

> **Manifold Agent is an AI-native engineering system that organizes humans and specialized AI agents to continuously plan, build, verify, approve, release, observe, and improve software around real engineering intent.**

中文：

> **Manifold Agent 是一个 AI-native Engineering Organization / Engineering OS：围绕真实研发目标组织 Human + AI Agents，持续完成计划、开发、验证、审批、发布、运行反馈与改进闭环。**

## 2. Core product model

```text
Manifold Agent
  = Agent Organization
  + Agent Native Loop
  + Engineering Context
```

These answer three different questions:

```text
Agent Organization   Who does the work?
Agent Native Loop    How does the organization complete and recover work?
Engineering Context  What does the organization know and what evidence does it retain?
```

The existing MNL implementation is primarily the **Loop Engine**. The complete product is the larger **Agent Organization** around that engine.

## 3. Four-layer architecture

```text
┌──────────────────────────────────────────────────────────┐
│ 1. Intent Layer                                          │
│ Human Goal / Requirement / Bug / Feedback                │
└──────────────────────────┬───────────────────────────────┘
                           ↓
┌──────────────────────────────────────────────────────────┐
│ 2. Agent Organization                                    │
│ Human Lead                                                │
│ Product / Architect / Backend / Frontend / Test / ...    │
│ Role / Skills / Runtime / Model / Permissions / Context  │
└──────────────────────────┬───────────────────────────────┘
                           ↓
┌──────────────────────────────────────────────────────────┐
│ 3. Agent Native Loop                                     │
│ Plan → Build → Verify → Approve → Release → Observe      │
│          ↑          FAIL / Reject          │              │
│          └──────── Repair / Retry ─────────┘              │
└──────────────────────────┬───────────────────────────────┘
                           ↓
┌──────────────────────────────────────────────────────────┐
│ 4. Engineering Context                                   │
│ PRD ↔ Issue ↔ API ↔ Code ↔ Test ↔ Deploy ↔ Runtime      │
│                    Evidence / Provenance                  │
└──────────────────────────┬───────────────────────────────┘
                           └──────────────→ Next Intent
```

## 4. Agent Organization

An Agent is not merely a prompt. Product-level specialist workers should eventually be modeled as:

```text
Agent
├── Role
├── Instructions
├── Skills
├── Tools
├── Runtime
├── Model
├── Permissions
├── Context Scope
├── Memory
└── Responsibility
```

A software engineering organization can therefore contain:

```text
Team
├── Human Lead
├── Product Agent
├── Architect Agent
├── Backend Agent
├── Frontend Agent
├── Embedded Agent
├── Test Agent
├── Review Agent
└── Release Agent
```

Multica remains the current workforce/control-plane foundation for these concepts.

## 5. Work Graph is the product language

Avoid exposing a traditional BPM/workflow mental model as the primary product abstraction.

At product level:

```text
Intent
  ↓
Work Graph
  ├── Product
  ↓
Architecture
  ├───────────────┐
  ↓               ↓
Backend        Frontend
  └───────┬───────┘
          ↓
        Review
          ↓
         Test
          ↓
       Approval
          ↓
       Release
```

Implementation mapping:

| Product language | Current implementation language |
| --- | --- |
| Intent | Loop instantiation input / parent Issue |
| Work Graph | Loop Template + compiled issue graph |
| Work Item | Issue |
| Execution | Task / Run |
| Agent | Multica Agent / Squad |
| Verification | Evaluation |
| Decision | Loop Policy |
| Human Gate | Approval |
| Evidence | Artifact / provenance |

`Loop Template` remains the correct internal implementation abstraction; `Work Graph` should become the user-facing/product abstraction.

## 6. Multica boundary

```text
Manifold Agent
│
├── Intent                         ← Manifold
├── Work Graph                     ← Manifold
├── Native Loop Policy             ← Manifold
├── Evaluation / Approval          ← Manifold
├── Provenance                     ← Manifold
├── Engineering Context            ← Manifold
├── Observe / Feedback / Learning  ← Manifold
│
└── Agent Workforce
    └── Multica
        ├── Agent / Squad / Skill
        ├── Workspace / Project / Issue
        ├── Task / Run
        └── Runtime / Daemon
```

The runtime boundary must remain replaceable:

```text
Manifold Agent
      │
AgentRuntime Adapter
   /      |       \
Multica   Pi      Other
```

P0 remains inside the Multica fork. Do not create a separate `manifold-agent-server` merely for branding. Extraction becomes justified when Manifold Agent must orchestrate multiple independent workforce installations, support peer workforce backends, or own an independently deployable enterprise Context Graph/control plane.

## 7. Three nested loops

### P0 — Delivery Loop

```text
Intent → Product → Architecture → Build → Review → Test → Approval → Release
```

This is the current implementation target and remains valid.

### P1 — Feedback Loop

```text
Release → Observe → Runtime Evidence → Bug/Feedback → New Intent → Repair → Release
```

Release is therefore a milestone, not the final product boundary.

### P2 — Learning Loop

```text
Historical Work Graph
+ Evaluation
+ Artifact / Provenance
+ Runtime Result
        ↓
Improve Agent / Skill / Template / Evaluation / Policy
```

The product's long-term loop is:

```text
Human Intent
    ↓
Agent Organization
    ↓
Work Graph
    ↓
Execute
    ↓
Verify
    ↓
Deliver
    ↓
Observe
    ↓
Learn
    ↓
Feedback ───────────────↺
```

## 8. Naming rules

- `Manifold Agent` = product / Engineering OS
- `Manifold Agent Native Loop` = lifecycle/orchestration architecture
- `MNL` = internal implementation/work-item prefix
- `Work Graph` = product-facing representation of coordinated work
- `Loop Template` = implementation/configuration representation compiled into work
- `Backend Agent`, `Frontend Agent`, `Test Agent`, etc. = specialist workers
- Multica `Agent` = existing generic workforce entity; do not rename it for branding

## 9. Architectural invariant

The key boundary to protect is:

> **Intent, Work Graph, Context, Policy, Evaluation, Approval, Provenance and Feedback belong to Manifold Agent; workforce execution is accessed through an adapter and may evolve independently.**

This keeps today's Multica + MNL route valid while preventing the product from becoming permanently equivalent to `Multica + Workflow`.
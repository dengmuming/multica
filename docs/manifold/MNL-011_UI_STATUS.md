# MNL-011 — Manifold Agent Loop UI Status

> Product: **Manifold Agent**
> Architecture: **Manifold Agent Native Loop**
> Branch: `feature/manifold-agent-native-loop`
> Updated: 2026-09-14

## Implemented shared Web/Desktop view layer

A new shared feature surface exists under:

```text
packages/views/loops/
```

and is exported as:

```text
@multica/views/loops
```

Implemented pieces:

- `LoopList`
  - loop title / instance key
  - loop state
  - pinned template version
  - updated time
  - selection callback for host routing
- `LoopDetailPanel`
  - Parent Loop identity and state
  - horizontal stage graph
  - same-stage nodes rendered as parallel work
  - per-node Issue status
  - role + assignee
  - latest Task status
  - workflow retry count / budget
  - Evaluation verdict and recovery target
  - structured findings
  - Evaluation evidence
  - Human Approval state
  - Artifact / provenance list
  - host callbacks for opening canonical Issues and external artifacts
- stage projection helpers
  - deterministic stage grouping
  - Approval satisfaction uses authoritative approval state
  - Evaluation satisfaction requires done + pass/warn
  - current unsatisfied stage
  - workflow retry aggregation
- unit tests for parallel grouping, Evaluation/Approval satisfaction, current-stage selection and workflow retry totals.

## UI boundary

The shared view layer is deliberately transport-independent. It consumes typed view models instead of calling HTTP itself.

This preserves the repository architecture:

```text
Web / Desktop route
        ↓
@multica/core query / mutation adapter
        ↓
Manifold Agent HTTP API
        ↓
@multica/views/loops
```

The shared view does **not** own authentication, workspace selection, URL routing, React Query cache keys or network retries.

## Remaining MNL-011 integration

1. Add typed Manifold Loop API models/query adapters in `@multica/core`.
2. Add Web route/page that loads `GET /api/loops` and `GET /api/loops/{parentIssueId}`.
3. Add Desktop route pointing at the same shared view.
4. Wire Approval action to `POST /api/loop-approvals/{approvalId}/decision`.
5. Wire provenance refresh/listing.
6. Add realtime invalidation/refetch on relevant Issue/Task/Approval/Evaluation events.
7. Add navigation entry using the **Manifold Agent** product terminology.

## Backend prerequisite still visible

The complete Manifold Agent server route bundle exists in `server/cmd/server/manifold_agent_routes.go`, but the final production `router.go` physical mount must still be made inside the existing authenticated `RequireWorkspaceMember(queries)` group.

Do not expose Loop routes through a parallel auth stack or outside the existing workspace membership boundary.

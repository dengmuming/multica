# MNL-013 — Work Graph Implementation Status

> Branch: `feature/manifold-agent-native-loop`
> Status: implementation complete; CI/compiler verification pending

## Implemented

- Product-facing Go `WorkGraph` contract and runtime-independent reader boundary.
- Matching TypeScript contract and `getWorkGraph()` client/query options.
- Server-owned projection from pinned Template + canonical Loop/Multica state.
- Parallel stage dependency edges and Evaluation/Approval recovery edges.
- Optional compiled-out nodes excluded from each instance graph.
- Latest Task/current-attempt Evaluation and Approval gate projection.
- Artifact provenance projected globally and per node.
- Authenticated `GET /api/loops/{parentIssueId}/work-graph` production route.
- Loop Detail Work Graph renderer with stage groups, parallel nodes, state, tasks, evaluation, provenance and recovery paths.
- Graceful UI fallback keeps canonical Loop detail usable if graph projection is unavailable.
- Backend projection, provenance, HTTP and route-surface tests.

## Verification remaining

The GitHub connector can modify and inspect repository content but does not provide a shell for this checkout. Run the repository's normal Go tests plus `@multica/core` / `@multica/views` typecheck and tests in CI or a local checkout. Any compiler/test findings should be fixed before merge.

## Follow-up product work

Assignee display-name resolution is intentionally deferred: Work Graph currently exposes stable assignee type/ID without coupling the product projection to workforce-specific identity storage. This can be added through a dedicated identity projection boundary when the Organization layer lands.

## Invariant

No `work_graph` persistence table exists. Work Graph remains a product-facing projection over canonical Loop/Multica state, and the browser does not reconstruct graph semantics.
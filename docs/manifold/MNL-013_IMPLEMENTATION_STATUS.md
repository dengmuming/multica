# MNL-013 — Work Graph Implementation Status

> Branch: `feature/manifold-agent-native-loop`
> Status: backend/API slice implemented; UI and real compiler verification remain

## Implemented

- Product-facing Go `WorkGraph` contract and `WorkGraphReader` boundary.
- Matching TypeScript Work Graph types exported by `packages/core/loops`.
- `MulticaWorkGraphProjectionReader` over canonical Loop/Multica state.
- Exact pinned Template version drives graph semantics.
- Missing optional compiled-out nodes are excluded from the instance projection.
- Stage dependency edges preserve parallel development/barrier structure.
- Evaluation FAIL and Approval Reject targets are exposed as recovery edges.
- Latest Task/current-attempt Evaluation semantics reuse `RawLoopReadRepository`.
- Evaluation and human-approval gates are projected separately from Agent nodes.
- Product status/current-stage projection.
- Authenticated `GET /api/loops/{parentIssueId}/work-graph` mounted in the production Manifold route bundle.
- Core client `getWorkGraph()` API.
- Projection tests for parallel graph, recovery and optional nodes.
- HTTP route/workspace tests.
- Production route-surface test updated to pin the Work Graph endpoint.

## Remaining MNL-013 work

1. Add artifact counts/provenance summary through an efficient aggregate query (current contract is present but counts remain zero).
2. Resolve/display assignee names where useful without making the graph depend on runtime internals.
3. Build the first Work Graph renderer in Loop detail.
4. Run Go compiler/tests in a real checkout and fix any integration/compiler findings.
5. Run frontend typecheck/tests.

## Invariant

No `work_graph` persistence table is introduced. Work Graph remains a product-facing projection over existing canonical Loop/Multica state. The browser must not reconstruct graph semantics itself.
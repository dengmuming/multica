# MNL-013 — Work Graph Implementation Status

> Branch: `feature/manifold-agent-native-loop`

## Implemented

- Product-facing Go `WorkGraph` contract under `server/internal/loopservice/work_graph.go`.
- `WorkGraphReader` boundary so product/API code does not depend directly on Multica runtime persistence.
- Matching TypeScript Work Graph types under `packages/core/loops/work-graph.ts`.
- Core package exports the Work Graph contract.

## Next implementation slice

1. Implement `MulticaWorkGraphProjectionReader` from the existing pinned Loop projection/read repositories.
2. Construct deterministic dependency/recovery/gate edges from the pinned template.
3. Project newest authoritative Task and current-attempt Evaluation only.
4. Add Approval and provenance summaries.
5. Add authenticated `GET /api/loops/{parentIssueId}/work-graph`.
6. Add backend projection/API tests.
7. Add first Work Graph UI renderer to Loop detail.

## Invariant

No `work_graph` persistence table is introduced. The Work Graph is a product-facing projection over existing canonical Loop/Multica state.
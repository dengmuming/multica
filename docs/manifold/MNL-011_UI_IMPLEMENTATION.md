# MNL-011 — Loop UI implementation status

## Current decision

The official shared UI surface is `@multica/views/loops`.

Do not create a second Manifold-specific Loop component tree. Web and Desktop should share the same Loop list/detail components and the same stage projection semantics.

## Implemented

### Shared Loop list

`packages/views/loops/components/loop-list.tsx`

- renders loop title, instance key, template version, state and updated time;
- supports selected state and navigation callback;
- distinguishes `completed`, `waiting_approval`, `blocked` / `failed`, and ordinary running states visually;
- remains data-injected and transport-agnostic.

### Shared Loop detail

`packages/views/loops/components/loop-detail-panel.tsx`

- renders the Parent Issue loop summary;
- renders ordered stage barriers;
- preserves same-stage parallel child Issues;
- renders node role, required/optional state, assignee, latest task status and workflow retry count;
- renders evaluation verdict and recovery target;
- renders approval state;
- renders structured findings/evidence;
- renders provenance artifacts and supports artifact navigation callbacks.

### Stage projection semantics

`packages/views/loops/loop-utils.ts`

The UI now derives an explicit stage state:

- `backlog`
- `ready`
- `running`
- `waiting_approval`
- `failed`
- `done`

The projection is deterministic:

1. nodes are grouped by `issue.stage`;
2. stages are ordered numerically;
3. nodes inside a stage are ordered by `node_key`;
4. Evaluation `fail` and Approval `rejected` surface as `failed`;
5. pending human approval surfaces as `waiting_approval`;
6. Evaluation is satisfied only by `done + pass|warn`;
7. Approval is satisfied only by authoritative `approved` state;
8. ordinary nodes use terminal Issue state;
9. workflow retry counts are aggregated separately from Task retry;
10. required-node progress ignores optional nodes.

Tests pin parallel-stage ordering, gate semantics, fail/approval state projection, retry totals, current stage and required-node progress.

## Consolidation performed

A temporary `packages/views/manifold-agent/*` implementation was created while inspecting the frontend, then removed after discovering the existing official `packages/views/loops` surface.

The useful stage-projection semantics were merged into `packages/views/loops`; duplicate components/types were removed. There is now one shared Loop UI implementation.

## Remaining integration

### 1. Production backend Router mount

`server/cmd/server/manifold_agent_routes.go` already builds and registers the complete Manifold route bundle, but `server/cmd/server/router.go` still needs the physical production mount:

```go
manifoldRoutes := buildManifoldAgentRoutes(pool, queries, h)
```

and, inside the existing authenticated workspace-member route group:

```go
registerManifoldAgentRoutes(r, manifoldRoutes)
```

This should be a two-line integration in the existing Router composition. Do not add a parallel middleware/auth boundary.

The connected GitHub write API only supports whole-file replacement and `router.go` is about 116 KB, so this change is intentionally not applied through a truncated file payload.

### 2. Unified frontend transport

The shared UI is deliberately transport-agnostic. The Loop endpoints should be added to the existing `ApiClient` so Web and Desktop inherit the same auth, workspace, CSRF, request-id and client-identity behavior.

Required reads:

- `GET /api/loops`
- `GET /api/loops/{parentIssueId}`
- `GET /api/loops/{parentIssueId}/artifacts`

Required mutations for the first full Mission Control surface:

- instantiate loop;
- submit evaluation;
- decide approval;
- attach artifact;
- template draft/update/publish for admin tooling.

Do not introduce a standalone `fetch` implementation just for Loop UI; that would bypass Desktop bearer-token and shared ApiClient behavior.

### 3. Web/Desktop pages

Once the unified ApiClient methods exist:

- add a workspace Loop list route;
- add a Loop detail route/panel using `@multica/views/loops`;
- connect Issue navigation and Task transcript navigation;
- connect Approval action UI to the explicit human approval endpoint;
- invalidate/refetch Loop detail on relevant realtime Issue/Task/Inbox events.

## Verification status

No GitHub Actions workflow currently runs for `feature/manifold-agent-native-loop`, so repository-wide TypeScript/Go test success is not claimed here.

Before the internal pilot, run at minimum:

```bash
make sqlc
cd server && go test ./...
pnpm --filter @multica/views test
pnpm --filter @multica/views typecheck
pnpm --filter @multica/core typecheck
```

Then apply migrations on a disposable PostgreSQL database and run the Feature Development pilot end-to-end.

## MNL-011 exit condition

MNL-011 is complete when a workspace member can open the Loop surface and observe, from the same shared Web/Desktop view:

`Intent → Product → Architecture → parallel Dev → Review → Test → recovery or approval → Release`

with visible workflow retries, structured verification evidence, human approval authority and artifact provenance.

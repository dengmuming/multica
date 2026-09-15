# MNL-014 — Agent Organization Layer

> Branch: `feature/manifold-agent-native-loop`
> Status: contract/design slice implemented

## Goal

Turn Multica's existing workforce primitives into the product-facing organization model used by Manifold Agent Native Loop without creating a second Agent/Skill/Runtime execution system.

The product model is:

`Organization → Role → Agent/Squad → Skills → Runtime → Permissions → Work Graph`

A **Role** is a Manifold responsibility/profile. An **Agent** remains the existing Multica executable identity. A **Squad** remains the existing Multica team primitive. Skills, runtimes and permissions remain canonical in Multica.

## Product contract

```text
AgentOrganization
  workspace_id
  roles[]

OrganizationRole
  key
  name
  description
  responsibilities[]
  required_skills[]
  recommended_skills[]
  capabilities[]
  bindings[]

OrganizationBinding
  type: agent | squad
  id
  name
  runtime
  skills[]
  permissions[]
  availability
```

The organization is a **projection/configuration layer**, not a workforce database.

## P0 role catalog

- `product` — requirement, scope and acceptance criteria
- `architect` — architecture, decomposition and implementation plan
- `backend` — API/service/data implementation
- `frontend` — web/desktop product implementation
- `embedded` — device/robot/edge implementation
- `review` — code/architecture/security review
- `test` — verification, regression and evidence
- `release` — build/release/deployment execution

Later roles can include Security, SRE, DBA, Data, Documentation and Support without changing the Work Graph contract.

## Binding rules

1. Templates reference `role`, never a hard-coded runtime/provider.
2. Workspace/project configuration resolves a role to an existing Agent or Squad.
3. A binding is valid only when the target belongs to the same workspace and is active.
4. Required skills are validated before dispatch; recommended skills are advisory.
5. Runtime/model selection remains owned by the bound Multica Agent/Squad.
6. Permissions are intersected with workspace/project policy; a role cannot grant authority its target does not already have.
7. Human approval is never resolved through an agent role.

## Resolution order

For a Work Graph node with `role=backend`:

1. project role binding
2. workspace role binding
3. template default binding (only when explicitly configured)
4. unresolved → node/loop blocked with a product-facing reason

No name-based agent guessing is allowed.

## Runtime boundary

Manifold owns:

- role semantics
- organization view
- role-to-work mapping
- binding validation
- product-facing capability/permission projection

Multica owns:

- Agent and Squad lifecycle
- Skill installation/distribution
- runtime/provider/model configuration
- task execution
- logs/tokens/cost
- runtime presence and concurrency

## Work Graph integration

Work Graph assignees should evolve from `{type,id}` to a resolved product projection that may include display name, role, runtime and availability. The graph still references canonical target IDs and never persists a duplicate workforce identity.

## API target

- `GET /api/manifold/organization` — projected organization/roles/bindings
- `PUT /api/manifold/organization/roles/{roleKey}/binding` — bind existing Agent/Squad
- `DELETE /api/manifold/organization/roles/{roleKey}/binding` — remove explicit binding
- template instantiate preflight validates every required role

## Persistence decision

Do **not** add `manifold_agent`, `manifold_skill`, or `manifold_runtime` tables.

If persistence is required, store only role binding/configuration metadata, conceptually:

```text
workspace/project + role_key → assignee_type + assignee_id + policy metadata
```

The referenced assignee is always canonical Multica state.

## Acceptance criteria

- Product can list the engineering organization by role.
- One role can bind an existing Agent or Squad.
- Same role can resolve differently by project.
- Missing/archived/foreign-workspace targets are rejected.
- Required skills can block instantiate/dispatch before a task is created.
- Work Graph can display resolved assignee identity without depending on runtime internals.
- No duplicate Agent/Skill/Runtime persistence is introduced.

## Implementation order

1. product-facing Go/TypeScript organization contracts
2. Multica projection reader for Agent/Squad/Skill/Runtime/permission summaries
3. minimal role-binding persistence
4. authenticated organization API
5. Organization UI
6. instantiate/dispatch role resolver
7. Work Graph enriched assignee projection
8. compiler/test verification

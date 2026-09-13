-- MNL-005A: durable persistence for the Manifold Agent Native Loop.
--
-- P0 intentionally reuses Multica Issue/Task/Run as the loop instance,
-- workflow node, and execution history. These tables store only state that
-- existing primitives cannot express cleanly: versioned loop templates,
-- structured evaluations, explicit human approvals, and typed provenance.
--
-- No foreign keys are introduced. Cross-entity integrity is enforced by the
-- application layer in line with current repository conventions.

CREATE TABLE loop_template (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    template_key TEXT NOT NULL,
    version INT NOT NULL CHECK (version > 0),
    name TEXT NOT NULL,
    description TEXT,
    status TEXT NOT NULL CHECK (status IN ('draft', 'active', 'archived')),
    definition JSONB NOT NULL CHECK (jsonb_typeof(definition) = 'object'),
    policy JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(policy) = 'object'),
    created_by_type TEXT NOT NULL CHECK (created_by_type IN ('member', 'agent')),
    created_by_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ,
    archived_at TIMESTAMPTZ
);

CREATE TABLE loop_evaluation (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    parent_issue_id UUID NOT NULL,
    node_issue_id UUID NOT NULL,
    task_id UUID,
    evaluator_type TEXT NOT NULL CHECK (evaluator_type IN ('member', 'agent', 'system', 'integration')),
    evaluator_id UUID,
    evaluation_kind TEXT NOT NULL,
    verdict TEXT NOT NULL CHECK (verdict IN ('pass', 'fail', 'warn', 'inconclusive')),
    score DOUBLE PRECISION,
    findings JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(findings) = 'array'),
    evidence JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(evidence) = 'array'),
    policy_action TEXT,
    policy_target_node_key TEXT,
    policy_version INT CHECK (policy_version IS NULL OR policy_version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE loop_approval (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    parent_issue_id UUID NOT NULL,
    node_issue_id UUID NOT NULL,
    approval_key TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('pending', 'approved', 'rejected', 'cancelled', 'expired')),
    requested_from_type TEXT CHECK (requested_from_type IS NULL OR requested_from_type = 'member'),
    requested_from_id UUID,
    requested_by_type TEXT NOT NULL CHECK (requested_by_type IN ('member', 'agent', 'system')),
    requested_by_id UUID,
    requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_by UUID,
    decided_at TIMESTAMPTZ,
    rationale TEXT,
    policy_version INT CHECK (policy_version IS NULL OR policy_version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((requested_from_type IS NULL) = (requested_from_id IS NULL)),
    CHECK ((state IN ('approved', 'rejected') AND decided_by IS NOT NULL AND decided_at IS NOT NULL)
        OR (state NOT IN ('approved', 'rejected')))
);

CREATE TABLE loop_artifact (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    parent_issue_id UUID NOT NULL,
    node_issue_id UUID,
    task_id UUID,
    artifact_type TEXT NOT NULL,
    relation TEXT NOT NULL CHECK (relation IN ('input', 'output', 'evidence', 'supersedes', 'derived_from')),
    title TEXT,
    ref_kind TEXT NOT NULL CHECK (ref_kind IN ('issue', 'task', 'attachment', 'source_context', 'commit', 'pull_request', 'build', 'deployment', 'url')),
    ref_id UUID,
    ref_uri TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    created_by_type TEXT NOT NULL CHECK (created_by_type IN ('member', 'agent', 'system', 'integration')),
    created_by_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (ref_id IS NOT NULL OR ref_uri IS NOT NULL)
);

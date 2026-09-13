-- name: CreateLoopApproval :one
INSERT INTO loop_approval (
    workspace_id, parent_issue_id, node_issue_id, approval_key, state,
    requested_from_type, requested_from_id, requested_by_type, requested_by_id,
    policy_version
) VALUES (
    sqlc.arg('workspace_id'), sqlc.arg('parent_issue_id'), sqlc.arg('node_issue_id'),
    sqlc.arg('approval_key'), 'pending', sqlc.narg('requested_from_type'),
    sqlc.narg('requested_from_id'), sqlc.arg('requested_by_type'),
    sqlc.narg('requested_by_id'), sqlc.narg('policy_version')
)
RETURNING *;

-- name: GetLoopApprovalInWorkspace :one
SELECT *
FROM loop_approval
WHERE id = sqlc.arg('id')
  AND workspace_id = sqlc.arg('workspace_id');

-- name: GetPendingLoopApprovalForNode :one
SELECT *
FROM loop_approval
WHERE workspace_id = sqlc.arg('workspace_id')
  AND node_issue_id = sqlc.arg('node_issue_id')
  AND approval_key = sqlc.arg('approval_key')
  AND state = 'pending';

-- name: ListLoopApprovalsByParent :many
SELECT *
FROM loop_approval
WHERE workspace_id = sqlc.arg('workspace_id')
  AND parent_issue_id = sqlc.arg('parent_issue_id')
ORDER BY created_at ASC, id ASC;

-- name: ListPendingLoopApprovalsForMember :many
SELECT *
FROM loop_approval
WHERE workspace_id = sqlc.arg('workspace_id')
  AND requested_from_type = 'member'
  AND requested_from_id = sqlc.arg('requested_from_id')
  AND state = 'pending'
ORDER BY requested_at ASC, id ASC;

-- name: DecideLoopApproval :one
UPDATE loop_approval
SET state = sqlc.arg('state'),
    decided_by = sqlc.arg('decided_by'),
    decided_at = now(),
    rationale = sqlc.narg('rationale'),
    updated_at = now()
WHERE id = sqlc.arg('id')
  AND workspace_id = sqlc.arg('workspace_id')
  AND state = 'pending'
  AND sqlc.arg('state')::text IN ('approved', 'rejected')
RETURNING *;

-- name: CancelLoopApproval :one
UPDATE loop_approval
SET state = 'cancelled',
    updated_at = now()
WHERE id = sqlc.arg('id')
  AND workspace_id = sqlc.arg('workspace_id')
  AND state = 'pending'
RETURNING *;

-- name: ExpireLoopApproval :one
UPDATE loop_approval
SET state = 'expired',
    updated_at = now()
WHERE id = sqlc.arg('id')
  AND workspace_id = sqlc.arg('workspace_id')
  AND state = 'pending'
RETURNING *;

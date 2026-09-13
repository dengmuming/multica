-- Manifold Agent Native Loop Issue persistence helpers.
--
-- These queries intentionally live beside the loop_* sources instead of
-- changing the generic CreateIssue query. The graph writer needs metadata to be
-- present on the INSERT itself so the partial unique instance-key index can
-- arbitrate concurrent creators before either transaction commits.

-- name: GetLoopInstanceIssueByKey :one
SELECT *
FROM issue
WHERE workspace_id = sqlc.arg(workspace_id)
  AND project_id = sqlc.arg(project_id)
  AND metadata ->> 'manifold.loop.instance_key' = sqlc.arg(instance_key)
LIMIT 1;

-- name: CreateLoopIssue :one
INSERT INTO issue (
    id,
    workspace_id,
    title,
    description,
    status,
    priority,
    assignee_type,
    assignee_id,
    creator_type,
    creator_id,
    parent_issue_id,
    position,
    start_date,
    due_date,
    number,
    project_id,
    metadata,
    stage,
    last_activity_at
) VALUES (
    sqlc.arg(id),
    sqlc.arg(workspace_id),
    sqlc.arg(title),
    sqlc.narg(description),
    sqlc.arg(status),
    sqlc.arg(priority),
    sqlc.narg(assignee_type),
    sqlc.narg(assignee_id),
    sqlc.arg(creator_type),
    sqlc.arg(creator_id),
    sqlc.narg(parent_issue_id),
    sqlc.arg(position),
    NULL,
    NULL,
    sqlc.arg(number),
    sqlc.arg(project_id),
    sqlc.arg(metadata)::jsonb,
    sqlc.narg(stage),
    now()
)
RETURNING *;

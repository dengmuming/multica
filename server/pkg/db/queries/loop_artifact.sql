-- name: CreateLoopArtifact :one
INSERT INTO loop_artifact (
    workspace_id, parent_issue_id, node_issue_id, task_id,
    artifact_type, relation, title, ref_kind, ref_id, ref_uri,
    metadata, created_by_type, created_by_id
) VALUES (
    sqlc.arg('workspace_id'), sqlc.arg('parent_issue_id'), sqlc.narg('node_issue_id'),
    sqlc.narg('task_id'), sqlc.arg('artifact_type'), sqlc.arg('relation'),
    sqlc.narg('title'), sqlc.arg('ref_kind'), sqlc.narg('ref_id'),
    sqlc.narg('ref_uri'), sqlc.arg('metadata'), sqlc.arg('created_by_type'),
    sqlc.narg('created_by_id')
)
RETURNING *;

-- name: GetLoopArtifactInWorkspace :one
SELECT *
FROM loop_artifact
WHERE id = sqlc.arg('id')
  AND workspace_id = sqlc.arg('workspace_id');

-- name: ListLoopArtifactsByParent :many
SELECT *
FROM loop_artifact
WHERE workspace_id = sqlc.arg('workspace_id')
  AND parent_issue_id = sqlc.arg('parent_issue_id')
ORDER BY created_at ASC, id ASC;

-- name: ListLoopArtifactsByNode :many
SELECT *
FROM loop_artifact
WHERE workspace_id = sqlc.arg('workspace_id')
  AND node_issue_id = sqlc.arg('node_issue_id')
ORDER BY created_at ASC, id ASC;

-- name: ListLoopArtifactsByTask :many
SELECT *
FROM loop_artifact
WHERE workspace_id = sqlc.arg('workspace_id')
  AND task_id = sqlc.arg('task_id')
ORDER BY created_at ASC, id ASC;

-- name: ListLoopArtifactsByType :many
SELECT *
FROM loop_artifact
WHERE workspace_id = sqlc.arg('workspace_id')
  AND parent_issue_id = sqlc.arg('parent_issue_id')
  AND artifact_type = sqlc.arg('artifact_type')
ORDER BY created_at ASC, id ASC;

-- name: CreateLoopTemplate :one
INSERT INTO loop_template (
    workspace_id, template_key, version, name, description, status,
    definition, policy, created_by_type, created_by_id
) VALUES (
    sqlc.arg('workspace_id'), sqlc.arg('template_key'), sqlc.arg('version'),
    sqlc.arg('name'), sqlc.narg('description'), sqlc.arg('status'),
    sqlc.arg('definition'), sqlc.arg('policy'), sqlc.arg('created_by_type'),
    sqlc.arg('created_by_id')
)
RETURNING *;

-- name: GetLoopTemplateInWorkspace :one
SELECT *
FROM loop_template
WHERE id = sqlc.arg('id')
  AND workspace_id = sqlc.arg('workspace_id');

-- name: GetLoopTemplateVersion :one
SELECT *
FROM loop_template
WHERE workspace_id = sqlc.arg('workspace_id')
  AND template_key = sqlc.arg('template_key')
  AND version = sqlc.arg('version');

-- name: GetActiveLoopTemplate :one
SELECT *
FROM loop_template
WHERE workspace_id = sqlc.arg('workspace_id')
  AND template_key = sqlc.arg('template_key')
  AND status = 'active';

-- name: ListLoopTemplates :many
SELECT *
FROM loop_template
WHERE workspace_id = sqlc.arg('workspace_id')
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
ORDER BY template_key ASC, version DESC;

-- name: GetNextLoopTemplateVersion :one
SELECT COALESCE(MAX(version), 0)::int + 1 AS next_version
FROM loop_template
WHERE workspace_id = sqlc.arg('workspace_id')
  AND template_key = sqlc.arg('template_key');

-- name: UpdateLoopTemplateDraft :one
UPDATE loop_template
SET name = sqlc.arg('name'),
    description = sqlc.narg('description'),
    definition = sqlc.arg('definition'),
    policy = sqlc.arg('policy')
WHERE id = sqlc.arg('id')
  AND workspace_id = sqlc.arg('workspace_id')
  AND status = 'draft'
RETURNING *;

-- name: ArchiveActiveLoopTemplates :many
UPDATE loop_template
SET status = 'archived',
    archived_at = now()
WHERE workspace_id = sqlc.arg('workspace_id')
  AND template_key = sqlc.arg('template_key')
  AND status = 'active'
  AND id <> sqlc.arg('except_id')
RETURNING *;

-- name: PublishLoopTemplate :one
UPDATE loop_template
SET status = 'active',
    published_at = COALESCE(published_at, now()),
    archived_at = NULL
WHERE id = sqlc.arg('id')
  AND workspace_id = sqlc.arg('workspace_id')
  AND status = 'draft'
RETURNING *;

-- name: ArchiveLoopTemplate :one
UPDATE loop_template
SET status = 'archived',
    archived_at = COALESCE(archived_at, now())
WHERE id = sqlc.arg('id')
  AND workspace_id = sqlc.arg('workspace_id')
  AND status <> 'archived'
RETURNING *;

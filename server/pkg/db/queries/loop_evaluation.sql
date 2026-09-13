-- name: CreateLoopEvaluation :one
INSERT INTO loop_evaluation (
    workspace_id, parent_issue_id, node_issue_id, task_id,
    evaluator_type, evaluator_id, evaluation_kind, verdict, score,
    findings, evidence, policy_action, policy_target_node_key, policy_version
) VALUES (
    sqlc.arg('workspace_id'), sqlc.arg('parent_issue_id'), sqlc.arg('node_issue_id'),
    sqlc.narg('task_id'), sqlc.arg('evaluator_type'), sqlc.narg('evaluator_id'),
    sqlc.arg('evaluation_kind'), sqlc.arg('verdict'), sqlc.narg('score'),
    sqlc.arg('findings'), sqlc.arg('evidence'), sqlc.narg('policy_action'),
    sqlc.narg('policy_target_node_key'), sqlc.narg('policy_version')
)
RETURNING *;

-- name: GetLoopEvaluationInWorkspace :one
SELECT *
FROM loop_evaluation
WHERE id = sqlc.arg('id')
  AND workspace_id = sqlc.arg('workspace_id');

-- name: ListLoopEvaluationsByParent :many
SELECT *
FROM loop_evaluation
WHERE workspace_id = sqlc.arg('workspace_id')
  AND parent_issue_id = sqlc.arg('parent_issue_id')
ORDER BY created_at ASC, id ASC;

-- name: ListLoopEvaluationsByNode :many
SELECT *
FROM loop_evaluation
WHERE workspace_id = sqlc.arg('workspace_id')
  AND node_issue_id = sqlc.arg('node_issue_id')
ORDER BY created_at DESC, id DESC;

-- name: ListLoopEvaluationsByTask :many
SELECT *
FROM loop_evaluation
WHERE workspace_id = sqlc.arg('workspace_id')
  AND task_id = sqlc.arg('task_id')
ORDER BY created_at DESC, id DESC;

-- name: GetLatestLoopEvaluationForNode :one
SELECT *
FROM loop_evaluation
WHERE workspace_id = sqlc.arg('workspace_id')
  AND node_issue_id = sqlc.arg('node_issue_id')
  AND evaluation_kind = sqlc.arg('evaluation_kind')
ORDER BY created_at DESC, id DESC
LIMIT 1;

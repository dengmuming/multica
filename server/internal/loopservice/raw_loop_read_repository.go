package loopservice

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type RawLoopReadRepository struct {
	db LoopStateDB
}

func NewRawLoopReadRepository(db LoopStateDB) *RawLoopReadRepository {
	return &RawLoopReadRepository{db: db}
}

func (r *RawLoopReadRepository) ListLoops(ctx context.Context, query ListLoopsQuery) ([]LoopSummary, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("loop read database is required")
	}
	workspaceID, err := parseRequiredUUID("workspace id", query.WorkspaceID)
	if err != nil {
		return nil, err
	}
	var projectID pgtype.UUID
	if strings.TrimSpace(query.ProjectID) != "" {
		projectID, err = parseRequiredUUID("project id", query.ProjectID)
		if err != nil {
			return nil, err
		}
	}
	rows, err := r.db.Query(ctx, `
		SELECT i.id,
		       i.project_id,
		       i.title,
		       COALESCE(i.description, ''),
		       i.status,
		       COALESCE(i.metadata ->> 'manifold.loop.state', 'running'),
		       i.metadata ->> 'manifold.loop.template_key',
		       COALESCE((i.metadata ->> 'manifold.loop.template_version')::integer, 0),
		       COALESCE(i.metadata ->> 'manifold.loop.policy_version', ''),
		       i.metadata ->> 'manifold.loop.instance_key',
		       i.created_at,
		       i.updated_at
		FROM issue i
		WHERE i.workspace_id = $1
		  AND i.metadata ? 'manifold.loop.instance_key'
		  AND NOT (i.metadata ? 'manifold.loop.node_key')
		  AND ($2::uuid IS NULL OR i.project_id = $2)
		ORDER BY i.updated_at DESC, i.id DESC
		LIMIT $3`, workspaceID, projectID, query.Limit)
	if err != nil {
		return nil, fmt.Errorf("list loop instances: %w", err)
	}
	defer rows.Close()

	out := make([]LoopSummary, 0)
	for rows.Next() {
		var id, project pgtype.UUID
		var summary LoopSummary
		if err := rows.Scan(
			&id,
			&project,
			&summary.Title,
			&summary.Description,
			&summary.IssueStatus,
			&summary.State,
			&summary.TemplateKey,
			&summary.TemplateVersion,
			&summary.PolicyVersion,
			&summary.InstanceKey,
			&summary.CreatedAt,
			&summary.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan loop instance: %w", err)
		}
		summary.ParentIssueID = uuidString(id)
		summary.ProjectID = uuidString(project)
		out = append(out, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate loop instances: %w", err)
	}
	return out, nil
}

func (r *RawLoopReadRepository) GetLoop(ctx context.Context, workspaceID, parentIssueID string) (LoopDetail, error) {
	if r == nil || r.db == nil {
		return LoopDetail{}, errors.New("loop read database is required")
	}
	wid, err := parseRequiredUUID("workspace id", workspaceID)
	if err != nil {
		return LoopDetail{}, err
	}
	pid, err := parseRequiredUUID("parent issue id", parentIssueID)
	if err != nil {
		return LoopDetail{}, err
	}

	var project pgtype.UUID
	var detail LoopDetail
	err = r.db.QueryRow(ctx, `
		SELECT i.project_id,
		       i.title,
		       COALESCE(i.description, ''),
		       i.status,
		       COALESCE(i.metadata ->> 'manifold.loop.state', 'running'),
		       i.metadata ->> 'manifold.loop.template_key',
		       COALESCE((i.metadata ->> 'manifold.loop.template_version')::integer, 0),
		       COALESCE(i.metadata ->> 'manifold.loop.policy_version', ''),
		       i.metadata ->> 'manifold.loop.instance_key',
		       i.created_at,
		       i.updated_at
		FROM issue i
		WHERE i.id = $1
		  AND i.workspace_id = $2
		  AND i.metadata ? 'manifold.loop.instance_key'
		  AND NOT (i.metadata ? 'manifold.loop.node_key')`, pid, wid).Scan(
		&project,
		&detail.Title,
		&detail.Description,
		&detail.IssueStatus,
		&detail.State,
		&detail.TemplateKey,
		&detail.TemplateVersion,
		&detail.PolicyVersion,
		&detail.InstanceKey,
		&detail.CreatedAt,
		&detail.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return LoopDetail{}, ErrLoopNotFound
		}
		return LoopDetail{}, fmt.Errorf("load loop instance: %w", err)
	}
	detail.ParentIssueID = parentIssueID
	detail.ProjectID = uuidString(project)

	nodes, err := r.loadLoopNodes(ctx, wid, pid)
	if err != nil {
		return LoopDetail{}, err
	}
	detail.Nodes = nodes
	return detail, nil
}

func (r *RawLoopReadRepository) loadLoopNodes(ctx context.Context, workspaceID, parentID pgtype.UUID) ([]LoopNodeRead, error) {
	rows, err := r.db.Query(ctx, `
		SELECT i.id,
		       i.metadata ->> 'manifold.loop.node_key',
		       i.metadata ->> 'manifold.loop.node_type',
		       COALESCE(i.metadata ->> 'manifold.loop.role', ''),
		       i.title,
		       COALESCE(i.description, ''),
		       i.status,
		       COALESCE(i.stage, 0),
		       COALESCE((i.metadata ->> 'manifold.loop.required')::boolean, true),
		       COALESCE((i.metadata ->> 'manifold.loop.retry_count')::integer, 0),
		       COALESCE((i.metadata ->> 'manifold.loop.max_retries')::integer, 0),
		       COALESCE(i.assignee_type, ''),
		       i.assignee_id,
		       latest_task.id,
		       COALESCE(latest_task.status, ''),
		       COALESCE(current_eval.verdict, ''),
		       COALESCE(current_eval.policy_target_node_key, ''),
		       COALESCE(current_approval.state, '')
		FROM issue i
		LEFT JOIN LATERAL (
			SELECT t.id, t.status
			FROM agent_task_queue t
			WHERE t.issue_id = i.id
			ORDER BY t.created_at DESC, t.id DESC
			LIMIT 1
		) latest_task ON true
		LEFT JOIN LATERAL (
			SELECT e.verdict, e.policy_target_node_key
			FROM loop_evaluation e
			WHERE e.workspace_id = i.workspace_id
			  AND e.parent_issue_id = i.parent_issue_id
			  AND e.node_issue_id = i.id
			  AND i.status <> 'backlog'
			  AND (latest_task.id IS NULL OR e.task_id = latest_task.id)
			ORDER BY e.created_at DESC, e.id DESC
			LIMIT 1
		) current_eval ON true
		LEFT JOIN LATERAL (
			SELECT a.state
			FROM loop_approval a
			WHERE a.workspace_id = i.workspace_id
			  AND a.parent_issue_id = i.parent_issue_id
			  AND a.node_issue_id = i.id
			  AND i.status <> 'backlog'
			ORDER BY a.requested_at DESC, a.created_at DESC, a.id DESC
			LIMIT 1
		) current_approval ON true
		WHERE i.workspace_id = $1
		  AND i.parent_issue_id = $2
		  AND i.metadata ? 'manifold.loop.node_key'
		ORDER BY i.stage ASC NULLS LAST, i.number ASC, i.id ASC`, workspaceID, parentID)
	if err != nil {
		return nil, fmt.Errorf("load loop nodes: %w", err)
	}
	defer rows.Close()

	out := make([]LoopNodeRead, 0)
	for rows.Next() {
		var issueID, assigneeID, taskID pgtype.UUID
		var node LoopNodeRead
		if err := rows.Scan(
			&issueID,
			&node.NodeKey,
			&node.NodeType,
			&node.Role,
			&node.Title,
			&node.Description,
			&node.Status,
			&node.Stage,
			&node.Required,
			&node.RetryCount,
			&node.MaxRetries,
			&node.AssigneeType,
			&assigneeID,
			&taskID,
			&node.LatestTaskStatus,
			&node.EvaluationVerdict,
			&node.EvaluationTarget,
			&node.ApprovalState,
		); err != nil {
			return nil, fmt.Errorf("scan loop node: %w", err)
		}
		node.IssueID = uuidString(issueID)
		node.AssigneeID = uuidString(assigneeID)
		node.LatestTaskID = uuidString(taskID)
		out = append(out, node)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate loop nodes: %w", err)
	}
	return out, nil
}

var _ time.Time

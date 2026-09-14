package loopservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/looppolicy"
	"github.com/multica-ai/multica/server/internal/looptemplate"
)

// LoopStateDB is implemented by *pgxpool.Pool and pgx.Tx. It is a temporary
// compatibility seam while the committed loop SQLC queries await generation.
// Once generated methods are available, callers can replace this adapter
// without changing PolicyProjectionReader contracts.
type LoopStateDB interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

type RawLoopStateRepository struct {
	db LoopStateDB
}

func NewRawLoopStateRepository(db LoopStateDB) *RawLoopStateRepository {
	return &RawLoopStateRepository{db: db}
}

func (r *RawLoopStateRepository) LoadPinnedLoopTemplate(ctx context.Context, workspaceID, templateKey string, version int) (PinnedLoopTemplate, error) {
	if r == nil || r.db == nil {
		return PinnedLoopTemplate{}, errors.New("loop state database is required")
	}
	wid, err := parseRequiredUUID("workspace id", workspaceID)
	if err != nil {
		return PinnedLoopTemplate{}, err
	}
	if strings.TrimSpace(templateKey) == "" || version <= 0 {
		return PinnedLoopTemplate{}, errors.New("template key and positive version are required")
	}

	var definitionJSON, policyJSON []byte
	err = r.db.QueryRow(ctx, `
		SELECT definition, policy
		FROM loop_template
		WHERE workspace_id = $1
		  AND template_key = $2
		  AND version = $3
		  AND status IN ('active', 'archived')
		  AND published_at IS NOT NULL
		LIMIT 1`, wid, templateKey, version).Scan(&definitionJSON, &policyJSON)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PinnedLoopTemplate{}, fmt.Errorf("published loop template %s@%d not found", templateKey, version)
		}
		return PinnedLoopTemplate{}, fmt.Errorf("load pinned loop template: %w", err)
	}

	var definition looptemplate.Definition
	if err := json.Unmarshal(definitionJSON, &definition); err != nil {
		return PinnedLoopTemplate{}, fmt.Errorf("decode loop template definition: %w", err)
	}
	if definition.Key != templateKey {
		return PinnedLoopTemplate{}, fmt.Errorf("template definition key %q does not match persisted key %q", definition.Key, templateKey)
	}

	// policy.version is optional for backwards-compatible draft data. The
	// parent Issue remains the authoritative pinned value; when present here it
	// becomes an additional consistency check in MulticaPolicyProjectionReader.
	policyVersion := ""
	if len(policyJSON) > 0 {
		var policy map[string]any
		if err := json.Unmarshal(policyJSON, &policy); err != nil {
			return PinnedLoopTemplate{}, fmt.Errorf("decode loop template policy: %w", err)
		}
		if raw, ok := policy["version"].(string); ok {
			policyVersion = strings.TrimSpace(raw)
		}
	}
	return PinnedLoopTemplate{Definition: definition, PolicyVersion: policyVersion}, nil
}

func (r *RawLoopStateRepository) LoadGateStates(
	ctx context.Context,
	workspaceID, parentIssueID string,
	nodeIssueIDs map[string]string,
) (GateStateProjection, error) {
	if r == nil || r.db == nil {
		return GateStateProjection{}, errors.New("loop state database is required")
	}
	wid, err := parseRequiredUUID("workspace id", workspaceID)
	if err != nil {
		return GateStateProjection{}, err
	}
	pid, err := parseRequiredUUID("parent issue id", parentIssueID)
	if err != nil {
		return GateStateProjection{}, err
	}
	issueToNode := make(map[string]string, len(nodeIssueIDs))
	for nodeKey, issueID := range nodeIssueIDs {
		parsed, err := parseRequiredUUID("node issue id", issueID)
		if err != nil {
			return GateStateProjection{}, fmt.Errorf("node %q: %w", nodeKey, err)
		}
		issueToNode[uuidString(parsed)] = nodeKey
	}

	out := GateStateProjection{
		Evaluations: map[string]looppolicy.EvaluationState{},
		Approvals:   map[string]looppolicy.ApprovalState{},
	}
	if err := r.loadCurrentEvaluationStates(ctx, wid, pid, issueToNode, out.Evaluations); err != nil {
		return GateStateProjection{}, err
	}
	if err := r.loadCurrentApprovalStates(ctx, wid, pid, issueToNode, out.Approvals); err != nil {
		return GateStateProjection{}, err
	}
	return out, nil
}

// Only evidence attached to the newest Task for an active Evaluation Issue is
// authoritative. This prevents an old PASS/FAIL from controlling a later
// workflow retry after that evaluator node is parked and rerun.
func (r *RawLoopStateRepository) loadCurrentEvaluationStates(
	ctx context.Context,
	workspaceID, parentID pgtype.UUID,
	issueToNode map[string]string,
	out map[string]looppolicy.EvaluationState,
) error {
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT ON (e.node_issue_id)
		       e.node_issue_id,
		       e.verdict,
		       COALESCE(e.policy_target_node_key, '')
		FROM loop_evaluation e
		JOIN issue i
		  ON i.id = e.node_issue_id
		 AND i.workspace_id = e.workspace_id
		 AND i.parent_issue_id = e.parent_issue_id
		 AND i.status <> 'backlog'
		JOIN LATERAL (
			SELECT t.id
			FROM agent_task_queue t
			WHERE t.workspace_id = e.workspace_id
			  AND t.issue_id = e.node_issue_id
			ORDER BY t.created_at DESC, t.id DESC
			LIMIT 1
		) latest ON latest.id = e.task_id
		WHERE e.workspace_id = $1 AND e.parent_issue_id = $2
		ORDER BY e.node_issue_id, e.created_at DESC, e.id DESC`, workspaceID, parentID)
	if err != nil {
		return fmt.Errorf("load current loop evaluations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var issueID pgtype.UUID
		var verdict, target string
		if err := rows.Scan(&issueID, &verdict, &target); err != nil {
			return fmt.Errorf("scan current loop evaluation: %w", err)
		}
		if nodeKey := issueToNode[uuidString(issueID)]; nodeKey != "" {
			out[nodeKey] = looppolicy.EvaluationState{Verdict: verdict, TargetNodeKey: target}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate current loop evaluations: %w", err)
	}
	return nil
}

// Parked approval nodes hide previous approval history from the current policy
// attempt. requestLoopApproval activates the Issue and inserts a fresh pending
// row in the same transaction, so the next projection observes only the new
// gate attempt.
func (r *RawLoopStateRepository) loadCurrentApprovalStates(
	ctx context.Context,
	workspaceID, parentID pgtype.UUID,
	issueToNode map[string]string,
	out map[string]looppolicy.ApprovalState,
) error {
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT ON (a.node_issue_id)
		       a.node_issue_id, a.state
		FROM loop_approval a
		JOIN issue i
		  ON i.id = a.node_issue_id
		 AND i.workspace_id = a.workspace_id
		 AND i.parent_issue_id = a.parent_issue_id
		 AND i.status <> 'backlog'
		WHERE a.workspace_id = $1 AND a.parent_issue_id = $2
		ORDER BY a.node_issue_id, a.requested_at DESC, a.created_at DESC, a.id DESC`, workspaceID, parentID)
	if err != nil {
		return fmt.Errorf("load current loop approvals: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var issueID pgtype.UUID
		var state string
		if err := rows.Scan(&issueID, &state); err != nil {
			return fmt.Errorf("scan current loop approval: %w", err)
		}
		if nodeKey := issueToNode[uuidString(issueID)]; nodeKey != "" {
			out[nodeKey] = looppolicy.ApprovalState{State: state}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate current loop approvals: %w", err)
	}
	return nil
}

package loopservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/loopevaluation"
)

// RawEvaluationRepository implements EvaluationContextReader and EvaluationStore
// while the committed loop_evaluation SQLC queries await generation.
type RawEvaluationRepository struct {
	db         LoopStateDB
	projection PolicyProjectionReader
}

func NewRawEvaluationRepository(db LoopStateDB, projection PolicyProjectionReader) *RawEvaluationRepository {
	return &RawEvaluationRepository{db: db, projection: projection}
}

func (r *RawEvaluationRepository) LoadEvaluationContext(ctx context.Context, taskID string) (EvaluationContext, error) {
	if r == nil || r.db == nil || r.projection == nil {
		return EvaluationContext{}, errors.New("evaluation repository dependencies are incomplete")
	}
	tid, err := parseRequiredUUID("task id", taskID)
	if err != nil {
		return EvaluationContext{}, err
	}

	var issueID, parentID pgtype.UUID
	var nodeMetadata []byte
	err = r.db.QueryRow(ctx, `
		SELECT t.issue_id, i.parent_issue_id, i.metadata
		FROM agent_task_queue t
		JOIN issue i ON i.id = t.issue_id
		WHERE t.id = $1`, tid).Scan(&issueID, &parentID, &nodeMetadata)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EvaluationContext{}, errors.New("evaluation task not found")
		}
		return EvaluationContext{}, fmt.Errorf("load evaluation task lineage: %w", err)
	}
	if !parentID.Valid {
		return EvaluationContext{}, errors.New("evaluation task issue is not a loop child")
	}

	// A stale task from a previous workflow attempt must not be allowed to write
	// the authoritative result for the current node attempt.
	var latestTaskID pgtype.UUID
	err = r.db.QueryRow(ctx, `
		SELECT id
		FROM agent_task_queue
		WHERE issue_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT 1`, issueID).Scan(&latestTaskID)
	if err != nil {
		return EvaluationContext{}, fmt.Errorf("load latest node task: %w", err)
	}
	if latestTaskID != tid {
		return EvaluationContext{}, errors.New("evaluation task is not the current node attempt")
	}

	metadata, err := decodeIssueMetadata(nodeMetadata)
	if err != nil {
		return EvaluationContext{}, fmt.Errorf("decode evaluation node metadata: %w", err)
	}
	nodeKey := metadataString(metadata, "manifold.loop.node_key")
	if nodeKey == "" {
		return EvaluationContext{}, errors.New("evaluation node key is missing")
	}
	projection, err := r.projection.LoadPolicyProjection(ctx, uuidString(parentID))
	if err != nil {
		return EvaluationContext{}, fmt.Errorf("load loop projection for evaluation: %w", err)
	}
	if projection.NodeIssueIDs[nodeKey] != uuidString(issueID) {
		return EvaluationContext{}, errors.New("evaluation task node lineage does not match loop projection")
	}
	return EvaluationContext{
		WorkspaceID: projection.WorkspaceID,
		ParentIssueID: projection.ParentIssueID,
		NodeIssueID: uuidString(issueID),
		NodeKey: nodeKey,
		TaskID: uuidString(tid),
		PolicyVersion: projection.PolicyVersion,
		Plan: projection.Plan,
	}, nil
}

func (r *RawEvaluationRepository) PersistEvaluation(ctx context.Context, cmd PersistEvaluationCommand) (PersistedEvaluation, error) {
	if r == nil || r.db == nil {
		return PersistedEvaluation{}, errors.New("evaluation database is required")
	}
	workspaceID, err := parseRequiredUUID("workspace id", cmd.WorkspaceID)
	if err != nil {
		return PersistedEvaluation{}, err
	}
	parentID, err := parseRequiredUUID("parent issue id", cmd.ParentIssueID)
	if err != nil {
		return PersistedEvaluation{}, err
	}
	nodeID, err := parseRequiredUUID("node issue id", cmd.NodeIssueID)
	if err != nil {
		return PersistedEvaluation{}, err
	}
	taskID, err := parseRequiredUUID("task id", cmd.TaskID)
	if err != nil {
		return PersistedEvaluation{}, err
	}
	evaluatorID := pgtype.UUID{}
	if strings.TrimSpace(cmd.EvaluatorID) != "" {
		evaluatorID, err = parseRequiredUUID("evaluator id", cmd.EvaluatorID)
		if err != nil {
			return PersistedEvaluation{}, err
		}
	}
	if strings.TrimSpace(cmd.PolicyVersion) == "" {
		return PersistedEvaluation{}, errors.New("policy version is required")
	}

	findingsJSON, err := json.Marshal(cmd.Normalized.Findings)
	if err != nil {
		return PersistedEvaluation{}, fmt.Errorf("encode evaluation findings: %w", err)
	}
	evidenceJSON, err := json.Marshal(cmd.Normalized.Evidence)
	if err != nil {
		return PersistedEvaluation{}, fmt.Errorf("encode evaluation evidence: %w", err)
	}
	policyAction := evaluationPolicyAction(cmd.Normalized)
	var evaluationID pgtype.UUID
	err = r.db.QueryRow(ctx, `
		INSERT INTO loop_evaluation (
			workspace_id, parent_issue_id, node_issue_id, task_id,
			evaluator_type, evaluator_id, evaluation_kind, verdict, score,
			findings, evidence, policy_action, policy_target_node_key, policy_version
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,NULLIF($13,''),$14)
		ON CONFLICT (task_id) WHERE task_id IS NOT NULL
		DO NOTHING
		RETURNING id`,
		workspaceID, parentID, nodeID, taskID,
		cmd.EvaluatorType, evaluatorID, cmd.Normalized.EvaluationKind, cmd.Normalized.Verdict, cmd.Normalized.Score,
		findingsJSON, evidenceJSON, policyAction, cmd.Normalized.RecoveryTarget, cmd.PolicyVersion,
	).Scan(&evaluationID)
	if err == nil {
		return PersistedEvaluation{EvaluationID: uuidString(evaluationID), Created: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return PersistedEvaluation{}, fmt.Errorf("insert loop evaluation: %w", err)
	}

	var existingID pgtype.UUID
	var existingKind, existingVerdict, existingTarget, existingPolicyVersion, existingEvaluatorType string
	var existingEvaluatorID pgtype.UUID
	err = r.db.QueryRow(ctx, `
		SELECT id, evaluation_kind, verdict, COALESCE(policy_target_node_key,''), policy_version, evaluator_type, evaluator_id
		FROM loop_evaluation
		WHERE task_id = $1`, taskID).Scan(
		&existingID, &existingKind, &existingVerdict, &existingTarget, &existingPolicyVersion, &existingEvaluatorType, &existingEvaluatorID,
	)
	if err != nil {
		return PersistedEvaluation{}, fmt.Errorf("reload idempotent evaluation: %w", err)
	}
	if existingKind != cmd.Normalized.EvaluationKind || existingVerdict != cmd.Normalized.Verdict ||
		existingTarget != cmd.Normalized.RecoveryTarget || existingPolicyVersion != cmd.PolicyVersion ||
		existingEvaluatorType != cmd.EvaluatorType || existingEvaluatorID != evaluatorID {
		return PersistedEvaluation{}, errors.New("evaluation conflict: task already has a different authoritative result")
	}
	return PersistedEvaluation{EvaluationID: uuidString(existingID), Created: false}, nil
}

func evaluationPolicyAction(in loopevaluation.Normalized) string {
	switch in.Verdict {
	case "pass", "warn":
		return "continue"
	case "fail":
		if in.RecoveryTarget != "" {
			return "recover"
		}
		return "block"
	case "inconclusive":
		return "request_human_decision"
	default:
		return ""
	}
}

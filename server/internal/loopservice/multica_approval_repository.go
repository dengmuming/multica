package loopservice

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	multicaservice "github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// MulticaApprovalRepository implements both approval context loading and the
// authoritative pending -> approved/rejected transition without depending on
// generated loop_approval SQLC code. The write uses IssueService's transaction
// starter so the approval row, gate Issue status and Inbox cleanup are atomic.
type MulticaApprovalRepository struct {
	issues *multicaservice.IssueService
}

func NewMulticaApprovalRepository(issues *multicaservice.IssueService) *MulticaApprovalRepository {
	return &MulticaApprovalRepository{issues: issues}
}

func (r *MulticaApprovalRepository) LoadPendingApprovalContext(ctx context.Context, approvalID string) (ApprovalContext, error) {
	if r == nil || r.issues == nil || r.issues.TxStarter == nil {
		return ApprovalContext{}, errors.New("approval repository dependencies are incomplete")
	}
	id, err := parseRequiredUUID("approval id", approvalID)
	if err != nil {
		return ApprovalContext{}, err
	}
	tx, err := r.issues.TxStarter.Begin(ctx)
	if err != nil {
		return ApprovalContext{}, fmt.Errorf("begin approval read transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var workspaceID, parentID, nodeID, requestedFromID pgtype.UUID
	var approvalKey, state, policyVersion string
	var nodeMetadata []byte
	err = tx.QueryRow(ctx, `
		SELECT a.workspace_id,
		       a.parent_issue_id,
		       a.node_issue_id,
		       a.approval_key,
		       a.state,
		       a.policy_version,
		       a.requested_from_id,
		       i.metadata
		FROM loop_approval a
		JOIN issue i
		  ON i.id = a.node_issue_id
		 AND i.workspace_id = a.workspace_id
		 AND i.parent_issue_id = a.parent_issue_id
		WHERE a.id = $1`, id).Scan(
		&workspaceID, &parentID, &nodeID, &approvalKey, &state, &policyVersion, &requestedFromID, &nodeMetadata,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ApprovalContext{}, errors.New("approval not found")
		}
		return ApprovalContext{}, fmt.Errorf("load approval context: %w", err)
	}
	metadata, err := decodeIssueMetadata(nodeMetadata)
	if err != nil {
		return ApprovalContext{}, fmt.Errorf("decode approval node metadata: %w", err)
	}
	nodeKey := metadataString(metadata, "manifold.loop.node_key")
	if nodeKey == "" || nodeKey != approvalKey {
		return ApprovalContext{}, errors.New("approval node lineage mismatch")
	}
	requestedFrom := ""
	if requestedFromID.Valid {
		requestedFrom = uuidString(requestedFromID)
	}
	return ApprovalContext{
		WorkspaceID: uuidString(workspaceID),
		ParentIssueID: uuidString(parentID),
		NodeIssueID: uuidString(nodeID),
		NodeKey: nodeKey,
		ApprovalKey: approvalKey,
		State: state,
		PolicyVersion: policyVersion,
		RequestedFromID: requestedFrom,
	}, nil
}

func (r *MulticaApprovalRepository) DecideApproval(ctx context.Context, cmd DecideApprovalCommand) (PersistedApprovalDecision, error) {
	if r == nil || r.issues == nil || r.issues.Queries == nil || r.issues.TxStarter == nil {
		return PersistedApprovalDecision{}, errors.New("approval repository dependencies are incomplete")
	}
	approvalID, err := parseRequiredUUID("approval id", cmd.ApprovalID)
	if err != nil {
		return PersistedApprovalDecision{}, err
	}
	workspaceID, err := parseRequiredUUID("workspace id", cmd.WorkspaceID)
	if err != nil {
		return PersistedApprovalDecision{}, err
	}
	parentID, err := parseRequiredUUID("parent issue id", cmd.ParentIssueID)
	if err != nil {
		return PersistedApprovalDecision{}, err
	}
	nodeID, err := parseRequiredUUID("node issue id", cmd.NodeIssueID)
	if err != nil {
		return PersistedApprovalDecision{}, err
	}
	decidedBy, err := parseRequiredUUID("decided by", cmd.DecidedBy)
	if err != nil {
		return PersistedApprovalDecision{}, err
	}
	if cmd.Decision != "approved" && cmd.Decision != "rejected" {
		return PersistedApprovalDecision{}, errors.New("approval decision must be approved or rejected")
	}
	if strings.TrimSpace(cmd.ApprovalKey) == "" || strings.TrimSpace(cmd.PolicyVersion) == "" {
		return PersistedApprovalDecision{}, errors.New("approval key and policy version are required")
	}

	tx, err := r.issues.TxStarter.Begin(ctx)
	if err != nil {
		return PersistedApprovalDecision{}, fmt.Errorf("begin approval decision transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := r.issues.Queries.WithTx(tx)

	var persistedID pgtype.UUID
	var persistedState string
	err = tx.QueryRow(ctx, `
		UPDATE loop_approval
		SET state = $1,
		    decided_by = $2,
		    decided_at = now(),
		    rationale = NULLIF($3, ''),
		    updated_at = now()
		WHERE id = $4
		  AND workspace_id = $5
		  AND parent_issue_id = $6
		  AND node_issue_id = $7
		  AND approval_key = $8
		  AND policy_version = $9
		  AND state = 'pending'
		RETURNING id, state`,
		cmd.Decision, decidedBy, strings.TrimSpace(cmd.Rationale), approvalID,
		workspaceID, parentID, nodeID, cmd.ApprovalKey, cmd.PolicyVersion,
	).Scan(&persistedID, &persistedState)
	changed := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		var existingDecider pgtype.UUID
		err = tx.QueryRow(ctx, `
			SELECT id, state, decided_by
			FROM loop_approval
			WHERE id=$1 AND workspace_id=$2 AND parent_issue_id=$3 AND node_issue_id=$4
			  AND approval_key=$5 AND policy_version=$6`,
			approvalID, workspaceID, parentID, nodeID, cmd.ApprovalKey, cmd.PolicyVersion,
		).Scan(&persistedID, &persistedState, &existingDecider)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return PersistedApprovalDecision{}, errors.New("approval not found or lineage changed")
			}
			return PersistedApprovalDecision{}, fmt.Errorf("reload approval decision: %w", err)
		}
		if persistedState != cmd.Decision || !existingDecider.Valid || existingDecider != decidedBy {
			return PersistedApprovalDecision{}, fmt.Errorf("approval conflict: current state is %q", persistedState)
		}
		changed = false
	} else if err != nil {
		return PersistedApprovalDecision{}, fmt.Errorf("persist approval decision: %w", err)
	}

	if _, err := qtx.UpdateIssueStatus(ctx, db.UpdateIssueStatusParams{ID: nodeID, Status: "done", WorkspaceID: workspaceID}); err != nil {
		return PersistedApprovalDecision{}, fmt.Errorf("complete approval node issue: %w", err)
	}
	if _, err := qtx.ArchiveInboxByIssueAndType(ctx, db.ArchiveInboxByIssueAndTypeParams{
		WorkspaceID: workspaceID,
		IssueID: nodeID,
		Type: "loop_approval_required",
	}); err != nil {
		return PersistedApprovalDecision{}, fmt.Errorf("archive approval inbox item: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return PersistedApprovalDecision{}, fmt.Errorf("commit approval decision: %w", err)
	}
	return PersistedApprovalDecision{ApprovalID: uuidString(persistedID), State: persistedState, Changed: changed}, nil
}

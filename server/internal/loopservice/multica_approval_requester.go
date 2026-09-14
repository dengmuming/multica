package loopservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
)

// MulticaApprovalRequester is the temporary raw-PGX bridge for loop_approval
// while the committed loop SQLC sources have not yet been regenerated. It uses
// the caller's transaction, so the approval row, approval Issue activation and
// Inbox notification remain one atomic policy transition.
//
// Once `make sqlc` is available, the INSERT can be replaced with the generated
// CreateLoopApproval query without changing the application contract.
type MulticaApprovalRequester struct{}

func NewMulticaApprovalRequester() *MulticaApprovalRequester { return &MulticaApprovalRequester{} }

func (r *MulticaApprovalRequester) EnsurePendingApproval(
	ctx context.Context,
	tx pgx.Tx,
	qtx *db.Queries,
	req PendingApprovalRequest,
) error {
	if tx == nil || qtx == nil {
		return errors.New("approval requester transaction dependencies are incomplete")
	}
	if !req.WorkspaceID.Valid || !req.ParentIssueID.Valid || !req.NodeIssueID.Valid {
		return errors.New("approval workspace, parent, and node ids are required")
	}
	if strings.TrimSpace(req.ApprovalKey) == "" || strings.TrimSpace(req.PolicyVersion) == "" {
		return errors.New("approval key and policy version are required")
	}

	var approvalID pgtype.UUID
	var err error
	if req.FallbackRecipientID.Valid {
		err = tx.QueryRow(ctx, `
			INSERT INTO loop_approval (
				workspace_id, parent_issue_id, node_issue_id, approval_key, state,
				requested_from_type, requested_from_id,
				requested_by_type, requested_at, policy_version
			) VALUES ($1, $2, $3, $4, 'pending', 'member', $5, 'system', now(), $6)
			ON CONFLICT (node_issue_id, approval_key) WHERE state = 'pending'
			DO NOTHING
			RETURNING id`,
			req.WorkspaceID, req.ParentIssueID, req.NodeIssueID, req.ApprovalKey,
			req.FallbackRecipientID, req.PolicyVersion,
		).Scan(&approvalID)
	} else {
		err = tx.QueryRow(ctx, `
			INSERT INTO loop_approval (
				workspace_id, parent_issue_id, node_issue_id, approval_key, state,
				requested_by_type, requested_at, policy_version
			) VALUES ($1, $2, $3, $4, 'pending', 'system', now(), $5)
			ON CONFLICT (node_issue_id, approval_key) WHERE state = 'pending'
			DO NOTHING
			RETURNING id`,
			req.WorkspaceID, req.ParentIssueID, req.NodeIssueID, req.ApprovalKey, req.PolicyVersion,
		).Scan(&approvalID)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		// Existing pending approval is the idempotent success case. Do not create
		// another Inbox row on a replay.
		return nil
	}
	if err != nil {
		return fmt.Errorf("insert pending loop approval: %w", err)
	}

	if !req.FallbackRecipientID.Valid {
		return nil
	}
	details, err := json.Marshal(map[string]any{
		"approval_id":      uuidString(approvalID),
		"approval_key":     req.ApprovalKey,
		"parent_issue_id":  uuidString(req.ParentIssueID),
		"policy_version":   req.PolicyVersion,
		"reason":           req.Reason,
	})
	if err != nil {
		return fmt.Errorf("encode approval inbox details: %w", err)
	}
	_, err = qtx.CreateInboxItem(ctx, db.CreateInboxItemParams{
		ID:            dbid.NewV7(),
		WorkspaceID:   req.WorkspaceID,
		RecipientType: "member",
		RecipientID:   req.FallbackRecipientID,
		Type:          "loop_approval_required",
		Severity:      "info",
		IssueID:       req.NodeIssueID,
		Title:         "Approval required",
		Body:          pgtype.Text{String: "A Manifold Agent loop is waiting for your approval.", Valid: true},
		ActorType:     pgtype.Text{String: "system", Valid: true},
		Details:       details,
	})
	if err != nil {
		return fmt.Errorf("create loop approval inbox item: %w", err)
	}
	return nil
}

package loopservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	multicaservice "github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

var ErrStalePolicyProjection = errors.New("stale loop policy projection")

// TransactionalApprovalRequester is the narrow bridge to loop_approval. The
// implementation must use qtx so the pending approval is committed atomically
// with the Issue state transition. The SQLC-backed implementation can be added
// once the new loop queries are generated; the Issue mutation store itself does
// not need to wait for generated loop table code.
type TransactionalApprovalRequester interface {
	EnsurePendingApproval(ctx context.Context, qtx *db.Queries, req PendingApprovalRequest) error
}

type PendingApprovalRequest struct {
	WorkspaceID   pgtype.UUID
	ParentIssueID pgtype.UUID
	NodeIssueID   pgtype.UUID
	ApprovalKey   string
	PolicyVersion string
	Reason        string
}

// MulticaPolicyMutationStore applies one deterministic policy decision against
// existing Issue rows in a single transaction. Agent Tasks are deliberately
// absent here: dispatch requests are returned only after the transaction has
// committed and DurablePolicyMutationSink sends them through IssueService.
type MulticaPolicyMutationStore struct {
	issues    *multicaservice.IssueService
	approvals TransactionalApprovalRequester
}

func NewMulticaPolicyMutationStore(issues *multicaservice.IssueService, approvals TransactionalApprovalRequester) *MulticaPolicyMutationStore {
	return &MulticaPolicyMutationStore{issues: issues, approvals: approvals}
}

func (s *MulticaPolicyMutationStore) ApplyMutationBatchAtomically(ctx context.Context, batch PolicyMutationBatch) (PolicyMutationCommit, error) {
	if s == nil || s.issues == nil || s.issues.Queries == nil || s.issues.TxStarter == nil {
		return PolicyMutationCommit{}, errors.New("Multica policy mutation store dependencies are incomplete")
	}
	workspaceID, err := parseRequiredUUID("workspace id", batch.WorkspaceID)
	if err != nil {
		return PolicyMutationCommit{}, err
	}
	parentID, err := parseRequiredUUID("parent issue id", batch.ParentIssueID)
	if err != nil {
		return PolicyMutationCommit{}, err
	}
	if strings.TrimSpace(batch.PolicyVersion) == "" {
		return PolicyMutationCommit{}, errors.New("policy version is required")
	}
	if len(batch.Mutations) == 0 {
		return PolicyMutationCommit{}, errors.New("policy mutation batch is empty")
	}

	tx, err := s.issues.TxStarter.Begin(ctx)
	if err != nil {
		return PolicyMutationCommit{}, fmt.Errorf("begin policy mutation transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := s.issues.Queries.WithTx(tx)

	parent, err := qtx.LockIssueForDescriptionUpdate(ctx, db.LockIssueForDescriptionUpdateParams{ID: parentID, WorkspaceID: workspaceID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PolicyMutationCommit{}, errors.New("loop parent issue not found in workspace")
		}
		return PolicyMutationCommit{}, fmt.Errorf("lock loop parent issue: %w", err)
	}
	if batch.ExpectedParentRevision != nil && parent.Revision != *batch.ExpectedParentRevision {
		return PolicyMutationCommit{}, fmt.Errorf("%w: parent revision changed from %d to %d", ErrStalePolicyProjection, *batch.ExpectedParentRevision, parent.Revision)
	}
	parentMetadata, err := decodeIssueMetadata(parent.Metadata)
	if err != nil {
		return PolicyMutationCommit{}, fmt.Errorf("decode parent metadata: %w", err)
	}
	if got := metadataString(parentMetadata, "manifold.loop.policy_version"); got != batch.PolicyVersion {
		return PolicyMutationCommit{}, fmt.Errorf("%w: policy version changed from %q to %q", ErrStalePolicyProjection, batch.PolicyVersion, got)
	}

	// Lock every referenced child before mutating anything. This makes revision
	// validation a true all-or-nothing stale-projection guard rather than a
	// partial check performed after earlier mutations have already changed rows.
	locked := make(map[string]db.Issue)
	for _, mutation := range batch.Mutations {
		if mutation.NodeKey == "" {
			continue
		}
		if _, ok := locked[mutation.NodeKey]; ok {
			continue
		}
		issueID, err := parseRequiredUUID("node issue id", mutation.IssueID)
		if err != nil {
			return PolicyMutationCommit{}, err
		}
		issue, err := qtx.LockIssueForDescriptionUpdate(ctx, db.LockIssueForDescriptionUpdateParams{ID: issueID, WorkspaceID: workspaceID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return PolicyMutationCommit{}, fmt.Errorf("node %q issue not found in workspace", mutation.NodeKey)
			}
			return PolicyMutationCommit{}, fmt.Errorf("lock node %q issue: %w", mutation.NodeKey, err)
		}
		if !issue.ParentIssueID.Valid || issue.ParentIssueID != parentID {
			return PolicyMutationCommit{}, fmt.Errorf("node %q is not a child of loop parent", mutation.NodeKey)
		}
		metadata, err := decodeIssueMetadata(issue.Metadata)
		if err != nil {
			return PolicyMutationCommit{}, fmt.Errorf("decode node %q metadata: %w", mutation.NodeKey, err)
		}
		if metadataString(metadata, "manifold.loop.node_key") != mutation.NodeKey {
			return PolicyMutationCommit{}, fmt.Errorf("node issue metadata mismatch for %q", mutation.NodeKey)
		}
		if expected, ok := batch.ExpectedNodeRevisions[mutation.NodeKey]; ok && issue.Revision != expected {
			return PolicyMutationCommit{}, fmt.Errorf("%w: node %s revision changed from %d to %d", ErrStalePolicyProjection, mutation.NodeKey, expected, issue.Revision)
		}
		locked[mutation.NodeKey] = issue
	}

	commit := PolicyMutationCommit{}
	for _, mutation := range batch.Mutations {
		switch mutation.Type {
		case MutationActivateNode:
			issue := locked[mutation.NodeKey]
			if issue.Status != "backlog" {
				continue
			}
			updated, err := qtx.UpdateIssueStatus(ctx, db.UpdateIssueStatusParams{ID: issue.ID, Status: "todo", WorkspaceID: workspaceID})
			if err != nil {
				return PolicyMutationCommit{}, fmt.Errorf("activate node %q: %w", mutation.NodeKey, err)
			}
			appendDispatchIfExecutable(&commit, batch.WorkspaceID, mutation.NodeKey, updated, "manifold_loop_stage_activation")

		case MutationReopenNode:
			issue := locked[mutation.NodeKey]
			if issue.Status == "todo" || issue.Status == "in_progress" || issue.Status == "in_review" {
				// Idempotent recovery retry: the previous decision already made the
				// target runnable, so do not increment workflow retry again.
				continue
			}
			if mutation.IncrementRetry {
				metadata, err := decodeIssueMetadata(issue.Metadata)
				if err != nil {
					return PolicyMutationCommit{}, err
				}
				next := metadataInt(metadata, "manifold.loop.retry_count") + 1
				if err := setIssueMetadataValue(ctx, qtx, workspaceID, issue.ID, "manifold.loop.retry_count", next); err != nil {
					return PolicyMutationCommit{}, fmt.Errorf("increment workflow retry for %q: %w", mutation.NodeKey, err)
				}
			}
			updated, err := qtx.UpdateIssueStatus(ctx, db.UpdateIssueStatusParams{ID: issue.ID, Status: "todo", WorkspaceID: workspaceID})
			if err != nil {
				return PolicyMutationCommit{}, fmt.Errorf("reopen node %q: %w", mutation.NodeKey, err)
			}
			appendDispatchIfExecutable(&commit, batch.WorkspaceID, mutation.NodeKey, updated, "manifold_loop_workflow_retry")

		case MutationParkNode:
			issue := locked[mutation.NodeKey]
			if issue.Status == "backlog" {
				continue
			}
			if _, err := qtx.UpdateIssueStatus(ctx, db.UpdateIssueStatusParams{ID: issue.ID, Status: "backlog", WorkspaceID: workspaceID}); err != nil {
				return PolicyMutationCommit{}, fmt.Errorf("park node %q: %w", mutation.NodeKey, err)
			}

		case MutationRequestApproval:
			if s.approvals == nil {
				return PolicyMutationCommit{}, errors.New("transactional approval requester is required")
			}
			issue := locked[mutation.NodeKey]
			// Approval gates must become active. Recovery evaluation deliberately
			// ignores parked gates, so leaving this Issue in backlog would make a
			// later rejection invisible to policy.
			if issue.Status == "backlog" {
				if _, err := qtx.UpdateIssueStatus(ctx, db.UpdateIssueStatusParams{ID: issue.ID, Status: "todo", WorkspaceID: workspaceID}); err != nil {
					return PolicyMutationCommit{}, fmt.Errorf("activate approval node %q: %w", mutation.NodeKey, err)
				}
			}
			if err := s.approvals.EnsurePendingApproval(ctx, qtx, PendingApprovalRequest{
				WorkspaceID: workspaceID, ParentIssueID: parentID, NodeIssueID: issue.ID,
				ApprovalKey: mutation.NodeKey, PolicyVersion: batch.PolicyVersion, Reason: mutation.Reason,
			}); err != nil {
				return PolicyMutationCommit{}, fmt.Errorf("request approval for %q: %w", mutation.NodeKey, err)
			}

		case MutationSetParentState, MutationBlockLoop, MutationCompleteLoop:
			state := strings.TrimSpace(mutation.ParentState)
			if state == "" {
				return PolicyMutationCommit{}, fmt.Errorf("parent state is required for %s", mutation.Type)
			}
			if err := setIssueMetadataValue(ctx, qtx, workspaceID, parentID, "manifold.loop.state", state); err != nil {
				return PolicyMutationCommit{}, fmt.Errorf("set loop parent state %q: %w", state, err)
			}
			if mutation.Type == MutationCompleteLoop && parent.Status != "done" {
				if _, err := qtx.UpdateIssueStatus(ctx, db.UpdateIssueStatusParams{ID: parentID, Status: "done", WorkspaceID: workspaceID}); err != nil {
					return PolicyMutationCommit{}, fmt.Errorf("complete parent issue: %w", err)
				}
			}

		default:
			return PolicyMutationCommit{}, fmt.Errorf("unsupported policy mutation type %q", mutation.Type)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return PolicyMutationCommit{}, fmt.Errorf("commit policy mutation: %w", err)
	}
	return commit, nil
}

func appendDispatchIfExecutable(commit *PolicyMutationCommit, workspaceID, nodeKey string, issue db.Issue, reason string) {
	if commit == nil || !issue.AssigneeType.Valid || !issue.AssigneeID.Valid {
		return
	}
	if issue.AssigneeType.String != "agent" && issue.AssigneeType.String != "squad" {
		return
	}
	commit.Dispatch = append(commit.Dispatch, InitialDispatchRequest{
		WorkspaceID: workspaceID,
		IssueID: uuidString(issue.ID),
		NodeKey: nodeKey,
		AssigneeType: issue.AssigneeType.String,
		AssigneeID: uuidString(issue.AssigneeID),
		Reason: reason,
	})
}

func setIssueMetadataValue(ctx context.Context, qtx *db.Queries, workspaceID, issueID pgtype.UUID, key string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = qtx.SetIssueMetadataKey(ctx, db.SetIssueMetadataKeyParams{Key: key, Value: encoded, ID: issueID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		// SetIssueMetadataKey returns no row both for a missing Issue and for an
		// idempotent no-op. The Issue is already locked/validated by this store,
		// so a no-row result is an acceptable no-op here.
		return nil
	}
	return err
}

func decodeIssueMetadata(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

func metadataString(metadata map[string]any, key string) string {
	value, _ := metadata[key].(string)
	return value
}

func metadataInt(metadata map[string]any, key string) int {
	value, ok := metadata[key]
	if !ok {
		return 0
	}
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	default:
		return 0
	}
}

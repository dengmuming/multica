package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/dispatch"
	"github.com/multica-ai/multica/server/internal/issuestatus"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

var (
	ErrIssueDispatchNotFound         = errors.New("issue not found in workspace")
	ErrIssueDispatchParked           = errors.New("issue is parked in backlog")
	ErrIssueDispatchAssigneeMismatch = errors.New("issue assignee does not match dispatch request")
	ErrIssueDispatchUnavailable      = errors.New("assigned worker is not currently dispatchable")
)

// ExistingIssueDispatchRequest describes a post-commit activation of an already
// durable Issue. It is used by orchestration/recovery code that must enter work
// through the same TaskService and AgentReadiness path as ordinary Issue
// assignment, while still receiving an explicit error when dispatch fails.
//
// ExpectedAssigneeType/ID are required concurrency guards. A stale workflow
// projection must never enqueue work after a human or another policy action has
// reassigned the Issue.
type ExistingIssueDispatchRequest struct {
	WorkspaceID          pgtype.UUID
	IssueID              pgtype.UUID
	ExpectedAssigneeType string
	ExpectedAssigneeID   pgtype.UUID
}

// DispatchExistingAssignedIssue dispatches an already-committed assigned Issue
// through Multica's canonical Agent/Squad execution path.
//
// Unlike maybeEnqueueOnAssign (which is create-path best effort and logs enqueue
// failures), this method returns failures to the caller so a workflow control
// plane can reconcile the existing Issue instead of creating a duplicate graph.
// Task creation itself remains owned by TaskService; no orchestration caller may
// insert agent_task_queue rows directly.
func (s *IssueService) DispatchExistingAssignedIssue(ctx context.Context, req ExistingIssueDispatchRequest) (pgtype.UUID, error) {
	if s == nil || s.Queries == nil || s.TaskService == nil {
		return pgtype.UUID{}, errors.New("issue dispatch service dependencies are incomplete")
	}
	if !req.WorkspaceID.Valid || !req.IssueID.Valid || !req.ExpectedAssigneeID.Valid {
		return pgtype.UUID{}, errors.New("workspace, issue, and expected assignee ids are required")
	}
	if req.ExpectedAssigneeType != "agent" && req.ExpectedAssigneeType != "squad" {
		return pgtype.UUID{}, fmt.Errorf("unsupported assignee type %q", req.ExpectedAssigneeType)
	}

	issue, err := s.Queries.GetIssueInWorkspace(ctx, db.GetIssueInWorkspaceParams{
		ID:          req.IssueID,
		WorkspaceID: req.WorkspaceID,
	})
	if err != nil || !issue.ID.Valid {
		return pgtype.UUID{}, ErrIssueDispatchNotFound
	}
	if !issue.AssigneeType.Valid || !issue.AssigneeID.Valid ||
		issue.AssigneeType.String != req.ExpectedAssigneeType || issue.AssigneeID != req.ExpectedAssigneeID {
		return pgtype.UUID{}, ErrIssueDispatchAssigneeMismatch
	}
	if issuestatus.Effective(ctx, s.Queries, issue.WorkspaceID, issue.Status) == "backlog" {
		return pgtype.UUID{}, ErrIssueDispatchParked
	}

	switch req.ExpectedAssigneeType {
	case "agent":
		verdict, admitted := agentAssigneeVerdict(ctx, s.runtimeLookup(s.Queries), issue)
		if !admitted {
			if verdict.Reason == dispatch.ReasonRuntimeUnusable {
				s.noteRuntimeUnusable(ctx, issue, verdict)
			}
			return pgtype.UUID{}, fmt.Errorf("%w: agent %s (%s)", ErrIssueDispatchUnavailable, util.UUIDToString(issue.AssigneeID), verdict.Reason)
		}
		task, err := s.TaskService.EnqueueTaskForIssue(ctx, issue)
		if err != nil {
			return pgtype.UUID{}, fmt.Errorf("enqueue agent task: %w", err)
		}
		return task.ID, nil

	case "squad":
		squad, err := s.Queries.GetSquadInWorkspace(ctx, db.GetSquadInWorkspaceParams{
			ID:          issue.AssigneeID,
			WorkspaceID: issue.WorkspaceID,
		})
		if err != nil {
			return pgtype.UUID{}, fmt.Errorf("%w: squad lookup failed", ErrIssueDispatchUnavailable)
		}
		if squad.ArchivedAt.Valid {
			return pgtype.UUID{}, fmt.Errorf("%w: squad %s is archived", ErrIssueDispatchUnavailable, util.UUIDToString(squad.ID))
		}
		agent, err := s.Queries.GetAgent(ctx, squad.LeaderID)
		if err != nil {
			return pgtype.UUID{}, fmt.Errorf("%w: squad leader lookup failed", ErrIssueDispatchUnavailable)
		}
		verdict, err := AgentReadiness(ctx, s.runtimeLookup(s.Queries), agent)
		if err != nil || !verdict.Ready() {
			return pgtype.UUID{}, fmt.Errorf("%w: squad leader %s is not ready", ErrIssueDispatchUnavailable, util.UUIDToString(squad.LeaderID))
		}

		hasPending, err := s.Queries.HasPendingTaskForIssueAndAgent(ctx, db.HasPendingTaskForIssueAndAgentParams{
			IssueID: issue.ID,
			AgentID: squad.LeaderID,
			HeadSha: headShaText(s.TaskService.ResolveIssueReviewSHA(ctx, issue.ID)),
		})
		if err != nil {
			return pgtype.UUID{}, fmt.Errorf("check pending squad task: %w", err)
		}
		if hasPending {
			// Existing pending work is the idempotent success case. The caller
			// only needs to know the Issue has canonical work in flight.
			return pgtype.UUID{}, nil
		}
		task, err := s.TaskService.EnqueueTaskForSquadLeader(ctx, issue, squad.LeaderID, squad.ID, pgtype.UUID{})
		if err != nil {
			return pgtype.UUID{}, fmt.Errorf("enqueue squad leader task: %w", err)
		}
		return task.ID, nil
	}

	return pgtype.UUID{}, fmt.Errorf("unsupported assignee type %q", req.ExpectedAssigneeType)
}

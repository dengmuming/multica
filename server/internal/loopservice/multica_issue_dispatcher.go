package loopservice

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
)

// ExistingAssignedIssueDispatcher is the narrow Multica service contract needed
// by the Agent Native Loop. Keeping the adapter on this interface makes the loop
// application layer testable without duplicating IssueService internals.
type ExistingAssignedIssueDispatcher interface {
	DispatchExistingAssignedIssue(ctx context.Context, req service.ExistingIssueDispatchRequest) (pgtype.UUID, error)
}

// MulticaInitialIssueDispatcher converts string application IDs into Multica's
// UUID boundary and enters work through IssueService. It is intentionally thin:
// admission, runtime readiness, squad-leader selection and Task creation remain
// canonical Multica responsibilities.
type MulticaInitialIssueDispatcher struct {
	issues ExistingAssignedIssueDispatcher
}

func NewMulticaInitialIssueDispatcher(issues ExistingAssignedIssueDispatcher) *MulticaInitialIssueDispatcher {
	return &MulticaInitialIssueDispatcher{issues: issues}
}

func (d *MulticaInitialIssueDispatcher) DispatchAssignedIssue(ctx context.Context, req InitialDispatchRequest) error {
	if d == nil || d.issues == nil {
		return fmt.Errorf("multica issue dispatcher is required")
	}
	workspaceID, err := parseRequiredUUID("workspace id", req.WorkspaceID)
	if err != nil {
		return err
	}
	issueID, err := parseRequiredUUID("issue id", req.IssueID)
	if err != nil {
		return err
	}
	assigneeID, err := parseRequiredUUID("assignee id", req.AssigneeID)
	if err != nil {
		return err
	}
	if req.AssigneeType != "agent" && req.AssigneeType != "squad" {
		return fmt.Errorf("unsupported loop assignee type %q", req.AssigneeType)
	}

	_, err = d.issues.DispatchExistingAssignedIssue(ctx, service.ExistingIssueDispatchRequest{
		WorkspaceID:          workspaceID,
		IssueID:              issueID,
		ExpectedAssigneeType: req.AssigneeType,
		ExpectedAssigneeID:   assigneeID,
	})
	if err != nil {
		return fmt.Errorf("dispatch loop node %q: %w", strings.TrimSpace(req.NodeKey), err)
	}
	return nil
}

func parseRequiredUUID(field, value string) (pgtype.UUID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return pgtype.UUID{}, fmt.Errorf("%s is required", field)
	}
	parsed, err := util.ParseUUID(value)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid %s: %w", field, err)
	}
	return parsed, nil
}

package loopservice

import (
	"context"
	"fmt"
	"strings"
)

// ApprovalContext is the authoritative server-side view of one approval gate.
// requested_from is intentionally not trusted from the HTTP body: the server
// resolves the pending approval row and verifies the authenticated human.
type ApprovalContext struct {
	WorkspaceID   string
	ParentIssueID string
	NodeIssueID   string
	NodeKey       string
	ApprovalKey   string
	State         string
	PolicyVersion string
	RequestedFromID string
}

type ApprovalContextReader interface {
	LoadPendingApprovalContext(ctx context.Context, approvalID string) (ApprovalContext, error)
}

// ApprovalStore performs the authoritative pending -> approved/rejected state
// change. The adapter must verify the row is still pending and decided_by is the
// authenticated member ID. Duplicate decisions should be idempotent or return a
// conflict; they must never advance policy twice.
type ApprovalStore interface {
	DecideApproval(ctx context.Context, cmd DecideApprovalCommand) (PersistedApprovalDecision, error)
}

type DecideApprovalCommand struct {
	ApprovalID     string
	WorkspaceID    string
	ParentIssueID  string
	NodeIssueID    string
	NodeKey        string
	ApprovalKey    string
	Decision       string
	DecidedBy      string
	Rationale      string
	PolicyVersion  string
}

type PersistedApprovalDecision struct {
	ApprovalID string
	State      string
	Changed    bool
}

type DecideApprovalRequest struct {
	ApprovalID string
	MemberID   string
	Decision   string
	Rationale  string
}

type DecideApprovalResult struct {
	Approval PersistedApprovalDecision
	Policy   PolicyTickResult
}

// ApprovalApplicationService guarantees that release authority remains human.
// Agent/system identities are not accepted by this boundary; the HTTP layer
// must pass the authenticated member ID from middleware, never a caller-chosen
// decided_by field.
type ApprovalApplicationService struct {
	contexts ApprovalContextReader
	store    ApprovalStore
	policy   *PolicyApplicationService
}

func NewApprovalApplicationService(
	contexts ApprovalContextReader,
	store ApprovalStore,
	policy *PolicyApplicationService,
) *ApprovalApplicationService {
	return &ApprovalApplicationService{contexts: contexts, store: store, policy: policy}
}

func (s *ApprovalApplicationService) Decide(ctx context.Context, req DecideApprovalRequest) (DecideApprovalResult, error) {
	if s == nil || s.contexts == nil || s.store == nil || s.policy == nil {
		return DecideApprovalResult{}, fmt.Errorf("approval application dependencies are incomplete")
	}
	req.ApprovalID = strings.TrimSpace(req.ApprovalID)
	req.MemberID = strings.TrimSpace(req.MemberID)
	if req.ApprovalID == "" {
		return DecideApprovalResult{}, fmt.Errorf("approval id is required")
	}
	if req.MemberID == "" {
		return DecideApprovalResult{}, fmt.Errorf("authenticated member id is required")
	}
	if req.Decision != "approved" && req.Decision != "rejected" {
		return DecideApprovalResult{}, fmt.Errorf("approval decision must be approved or rejected")
	}

	approval, err := s.contexts.LoadPendingApprovalContext(ctx, req.ApprovalID)
	if err != nil {
		return DecideApprovalResult{}, fmt.Errorf("load pending approval: %w", err)
	}
	if err := validateApprovalContext(approval); err != nil {
		return DecideApprovalResult{}, err
	}
	if approval.RequestedFromID != "" && approval.RequestedFromID != req.MemberID {
		return DecideApprovalResult{}, fmt.Errorf("authenticated member is not the requested approver")
	}

	persisted, err := s.store.DecideApproval(ctx, DecideApprovalCommand{
		ApprovalID:    req.ApprovalID,
		WorkspaceID:   approval.WorkspaceID,
		ParentIssueID: approval.ParentIssueID,
		NodeIssueID:   approval.NodeIssueID,
		NodeKey:       approval.NodeKey,
		ApprovalKey:   approval.ApprovalKey,
		Decision:      req.Decision,
		DecidedBy:     req.MemberID,
		Rationale:     strings.TrimSpace(req.Rationale),
		PolicyVersion: approval.PolicyVersion,
	})
	if err != nil {
		return DecideApprovalResult{}, fmt.Errorf("decide approval: %w", err)
	}
	if strings.TrimSpace(persisted.ApprovalID) == "" {
		return DecideApprovalResult{}, fmt.Errorf("decide approval: store returned empty approval id")
	}
	if persisted.State != "approved" && persisted.State != "rejected" {
		return DecideApprovalResult{}, fmt.Errorf("decide approval: store returned invalid state %q", persisted.State)
	}

	// Tick even for an idempotent duplicate. A client retry after the approval
	// committed but before policy ran must still move the loop forward/recover.
	policyResult, err := s.policy.Tick(ctx, approval.ParentIssueID)
	if err != nil {
		return DecideApprovalResult{Approval: persisted}, fmt.Errorf("approval persisted but policy tick failed: %w", err)
	}
	return DecideApprovalResult{Approval: persisted, Policy: policyResult}, nil
}

func validateApprovalContext(in ApprovalContext) error {
	if strings.TrimSpace(in.WorkspaceID) == "" {
		return fmt.Errorf("approval context workspace id is required")
	}
	if strings.TrimSpace(in.ParentIssueID) == "" || strings.TrimSpace(in.NodeIssueID) == "" {
		return fmt.Errorf("approval context issue lineage is incomplete")
	}
	if strings.TrimSpace(in.NodeKey) == "" || strings.TrimSpace(in.ApprovalKey) == "" {
		return fmt.Errorf("approval context gate identity is incomplete")
	}
	if in.State != "pending" {
		return fmt.Errorf("approval is not pending")
	}
	if strings.TrimSpace(in.PolicyVersion) == "" {
		return fmt.Errorf("approval context must pin policy version")
	}
	return nil
}

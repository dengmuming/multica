package loopservice

import (
	"context"
	"fmt"
	"strings"

	"github.com/multica-ai/multica/server/internal/loopevaluation"
	"github.com/multica-ai/multica/server/internal/looptemplate"
)

// EvaluationContext is the authoritative lineage snapshot used to validate one
// evaluator submission before persistence. The repository must prove all IDs
// belong to the same workspace/loop instance and that TaskID is the canonical
// execution attempt for NodeIssueID.
type EvaluationContext struct {
	WorkspaceID   string
	ParentIssueID string
	NodeIssueID   string
	NodeKey       string
	TaskID        string
	PolicyVersion string
	Plan          looptemplate.CompiledPlan
}

// EvaluationContextReader resolves task/issue/template lineage from server-owned
// state. Callers must never be allowed to choose a template version or node key
// independently of the persisted loop instance.
type EvaluationContextReader interface {
	LoadEvaluationContext(ctx context.Context, taskID string) (EvaluationContext, error)
}

// EvaluationEvidenceAuthorizer validates that every referenced artifact/evidence
// object is visible in the same workspace and is legal evidence for this loop.
// Implementations may accept URLs/opaque refs backed by loop_artifact rows, but
// must reject cross-workspace references.
type EvaluationEvidenceAuthorizer interface {
	AuthorizeEvaluationEvidence(ctx context.Context, workspaceID, parentIssueID string, submission loopevaluation.Submission) error
}

// EvaluationStore persists the normalized structured result. PersistEvaluation
// must be idempotent for the same canonical evaluator task; duplicate delivery
// after a timeout must not create a second authoritative result for that task.
type EvaluationStore interface {
	PersistEvaluation(ctx context.Context, cmd PersistEvaluationCommand) (PersistedEvaluation, error)
}

type PersistEvaluationCommand struct {
	WorkspaceID    string
	ParentIssueID  string
	NodeIssueID    string
	NodeKey        string
	TaskID         string
	PolicyVersion  string
	EvaluatorType  string
	EvaluatorID    string
	Normalized     loopevaluation.Normalized
}

type PersistedEvaluation struct {
	EvaluationID string
	Created      bool
}

type SubmitEvaluationRequest struct {
	TaskID        string
	EvaluatorType string
	EvaluatorID   string
	Submission    loopevaluation.Submission
}

type SubmitEvaluationResult struct {
	Evaluation PersistedEvaluation
	Normalized loopevaluation.Normalized
	Policy     PolicyTickResult
}

// EvaluationApplicationService owns the complete server-side boundary for one
// structured evaluator result: resolve lineage -> validate schema -> authorize
// evidence -> persist -> run deterministic policy. Agents submit evidence, but
// they never directly choose or mutate the workflow transition.
type EvaluationApplicationService struct {
	contexts   EvaluationContextReader
	evidence   EvaluationEvidenceAuthorizer
	store      EvaluationStore
	policy     *PolicyApplicationService
}

func NewEvaluationApplicationService(
	contexts EvaluationContextReader,
	evidence EvaluationEvidenceAuthorizer,
	store EvaluationStore,
	policy *PolicyApplicationService,
) *EvaluationApplicationService {
	return &EvaluationApplicationService{contexts: contexts, evidence: evidence, store: store, policy: policy}
}

func (s *EvaluationApplicationService) Submit(ctx context.Context, req SubmitEvaluationRequest) (SubmitEvaluationResult, error) {
	if s == nil || s.contexts == nil || s.evidence == nil || s.store == nil || s.policy == nil {
		return SubmitEvaluationResult{}, fmt.Errorf("evaluation application dependencies are incomplete")
	}
	req.TaskID = strings.TrimSpace(req.TaskID)
	if req.TaskID == "" {
		return SubmitEvaluationResult{}, fmt.Errorf("task id is required")
	}
	if req.EvaluatorType != "agent" && req.EvaluatorType != "member" && req.EvaluatorType != "system" {
		return SubmitEvaluationResult{}, fmt.Errorf("unsupported evaluator type %q", req.EvaluatorType)
	}
	if req.EvaluatorType != "system" && strings.TrimSpace(req.EvaluatorID) == "" {
		return SubmitEvaluationResult{}, fmt.Errorf("evaluator id is required for %s evaluator", req.EvaluatorType)
	}

	lineage, err := s.contexts.LoadEvaluationContext(ctx, req.TaskID)
	if err != nil {
		return SubmitEvaluationResult{}, fmt.Errorf("load evaluation context: %w", err)
	}
	if err := validateEvaluationContext(lineage, req.TaskID); err != nil {
		return SubmitEvaluationResult{}, err
	}

	normalized, err := loopevaluation.Validate(lineage.Plan, lineage.NodeKey, req.Submission)
	if err != nil {
		return SubmitEvaluationResult{}, err
	}
	if err := s.evidence.AuthorizeEvaluationEvidence(ctx, lineage.WorkspaceID, lineage.ParentIssueID, req.Submission); err != nil {
		return SubmitEvaluationResult{}, fmt.Errorf("authorize evaluation evidence: %w", err)
	}

	persisted, err := s.store.PersistEvaluation(ctx, PersistEvaluationCommand{
		WorkspaceID:   lineage.WorkspaceID,
		ParentIssueID: lineage.ParentIssueID,
		NodeIssueID:   lineage.NodeIssueID,
		NodeKey:       lineage.NodeKey,
		TaskID:        lineage.TaskID,
		PolicyVersion: lineage.PolicyVersion,
		EvaluatorType: req.EvaluatorType,
		EvaluatorID:   strings.TrimSpace(req.EvaluatorID),
		Normalized:    normalized,
	})
	if err != nil {
		return SubmitEvaluationResult{}, fmt.Errorf("persist evaluation: %w", err)
	}
	if strings.TrimSpace(persisted.EvaluationID) == "" {
		return SubmitEvaluationResult{}, fmt.Errorf("persist evaluation: store returned empty evaluation id")
	}

	// Always tick policy after an idempotent persistence result. If the first
	// response was lost after persistence but before policy application, a retry
	// must still be able to advance/recover the loop.
	policyResult, err := s.policy.Tick(ctx, lineage.ParentIssueID)
	if err != nil {
		return SubmitEvaluationResult{
			Evaluation: persisted,
			Normalized: normalized,
		}, fmt.Errorf("evaluation persisted but policy tick failed: %w", err)
	}
	return SubmitEvaluationResult{
		Evaluation: persisted,
		Normalized: normalized,
		Policy:     policyResult,
	}, nil
}

func validateEvaluationContext(in EvaluationContext, requestedTaskID string) error {
	if strings.TrimSpace(in.WorkspaceID) == "" {
		return fmt.Errorf("evaluation context workspace id is required")
	}
	if strings.TrimSpace(in.ParentIssueID) == "" || strings.TrimSpace(in.NodeIssueID) == "" {
		return fmt.Errorf("evaluation context issue lineage is incomplete")
	}
	if strings.TrimSpace(in.NodeKey) == "" {
		return fmt.Errorf("evaluation context node key is required")
	}
	if strings.TrimSpace(in.TaskID) == "" || in.TaskID != requestedTaskID {
		return fmt.Errorf("evaluation context task mismatch")
	}
	if strings.TrimSpace(in.PolicyVersion) == "" {
		return fmt.Errorf("evaluation context must pin policy version")
	}
	if len(in.Plan.IncludedNodes) == 0 {
		return fmt.Errorf("evaluation context compiled plan contains no nodes")
	}
	node, ok := compiledNodeByKey(in.Plan, in.NodeKey)
	if !ok {
		return fmt.Errorf("evaluation context node %q is not in compiled plan", in.NodeKey)
	}
	if node.Type != looptemplate.NodeTypeEvaluation {
		return fmt.Errorf("evaluation context node %q is not an evaluation node", in.NodeKey)
	}
	return nil
}

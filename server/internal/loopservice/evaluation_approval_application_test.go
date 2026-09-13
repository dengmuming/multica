package loopservice

import (
	"context"
	"errors"
	"testing"

	"github.com/multica-ai/multica/server/internal/loopevaluation"
	"github.com/multica-ai/multica/server/internal/looppolicy"
	"github.com/multica-ai/multica/server/internal/looptemplate"
)

type fakeEvaluationContextReader struct {
	value EvaluationContext
	err   error
}

func (f fakeEvaluationContextReader) LoadEvaluationContext(_ context.Context, _ string) (EvaluationContext, error) {
	return f.value, f.err
}

type fakeEvaluationEvidenceAuthorizer struct {
	calls int
	err   error
}

func (f *fakeEvaluationEvidenceAuthorizer) AuthorizeEvaluationEvidence(_ context.Context, _, _ string, _ loopevaluation.Submission) error {
	f.calls++
	return f.err
}

type fakeEvaluationStore struct {
	calls int
	last  PersistEvaluationCommand
	value PersistedEvaluation
	err   error
}

func (f *fakeEvaluationStore) PersistEvaluation(_ context.Context, cmd PersistEvaluationCommand) (PersistedEvaluation, error) {
	f.calls++
	f.last = cmd
	return f.value, f.err
}

type fakeApprovalContextReader struct {
	value ApprovalContext
	err   error
}

func (f fakeApprovalContextReader) LoadPendingApprovalContext(_ context.Context, _ string) (ApprovalContext, error) {
	return f.value, f.err
}

type fakeApprovalStore struct {
	calls int
	last  DecideApprovalCommand
	value PersistedApprovalDecision
	err   error
}

func (f *fakeApprovalStore) DecideApproval(_ context.Context, cmd DecideApprovalCommand) (PersistedApprovalDecision, error) {
	f.calls++
	f.last = cmd
	return f.value, f.err
}

func evaluationPlan() looptemplate.CompiledPlan {
	return looptemplate.CompiledPlan{IncludedNodes: []looptemplate.CompiledNode{
		{Key: "backend", Type: looptemplate.NodeTypeAgent, Stage: 30},
		{
			Key:   "test",
			Type:  looptemplate.NodeTypeEvaluation,
			Stage: 50,
			Evaluation: &looptemplate.EvaluationConfig{
				Kind:            "test",
				AllowedVerdicts: []string{"pass", "fail"},
				OnFail: looptemplate.EvaluationFailConfig{
					Strategy:           "fixed_target",
					TargetNode:         "backend",
					MaxWorkflowRetries: 2,
				},
			},
		},
	}}
}

func policyForEvaluation(t *testing.T) *PolicyApplicationService {
	t.Helper()
	reader := &fakePolicyProjectionReader{value: PolicyProjection{
		WorkspaceID:   "workspace-1",
		ParentIssueID: "parent-1",
		PolicyVersion: "policy-v1",
		Plan:          evaluationPlan(),
		Runtime: looppolicy.RuntimeState{
			ParentState: "running",
			Nodes: map[string]looppolicy.NodeState{
				"backend": {Status: "done"},
				"test":    {Status: "done"},
			},
			Evaluations: map[string]looppolicy.EvaluationState{
				"test": {Verdict: "fail", TargetNodeKey: "backend"},
			},
		},
		NodeIssueIDs: map[string]string{"backend": "issue-backend", "test": "issue-test"},
	}}
	return NewPolicyApplicationService(reader, &fakePolicyMutationSink{})
}

func TestEvaluationApplicationServicePersistsNormalizedResultAndTicksPolicy(t *testing.T) {
	evidence := &fakeEvaluationEvidenceAuthorizer{}
	store := &fakeEvaluationStore{value: PersistedEvaluation{EvaluationID: "evaluation-1", Created: true}}
	service := NewEvaluationApplicationService(
		fakeEvaluationContextReader{value: EvaluationContext{
			WorkspaceID:   "workspace-1",
			ParentIssueID: "parent-1",
			NodeIssueID:   "issue-test",
			NodeKey:       "test",
			TaskID:        "task-1",
			PolicyVersion: "policy-v1",
			Plan:          evaluationPlan(),
		}},
		evidence,
		store,
		policyForEvaluation(t),
	)

	result, err := service.Submit(context.Background(), SubmitEvaluationRequest{
		TaskID:        "task-1",
		EvaluatorType: "agent",
		EvaluatorID:   "agent-test",
		Submission: loopevaluation.Submission{
			EvaluationKind: "test",
			Verdict:        "fail",
			Findings: []loopevaluation.Finding{{
				Code:         "api.contract",
				Severity:     "high",
				OwnerNodeKey: "backend",
				Summary:      "API contract mismatch",
			}},
		},
	})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if store.calls != 1 || evidence.calls != 1 {
		t.Fatalf("store/evidence calls = %d/%d, want 1/1", store.calls, evidence.calls)
	}
	if store.last.Normalized.RecoveryTarget != "backend" {
		t.Fatalf("recovery target = %q, want backend", store.last.Normalized.RecoveryTarget)
	}
	if result.Evaluation.EvaluationID != "evaluation-1" {
		t.Fatalf("evaluation id = %q", result.Evaluation.EvaluationID)
	}
	if !result.Policy.Applied {
		t.Fatal("policy applied = false, want true")
	}
}

func TestEvaluationApplicationServiceRejectsTaskLineageMismatchBeforePersist(t *testing.T) {
	store := &fakeEvaluationStore{}
	service := NewEvaluationApplicationService(
		fakeEvaluationContextReader{value: EvaluationContext{
			WorkspaceID:   "workspace-1",
			ParentIssueID: "parent-1",
			NodeIssueID:   "issue-test",
			NodeKey:       "test",
			TaskID:        "different-task",
			PolicyVersion: "policy-v1",
			Plan:          evaluationPlan(),
		}},
		&fakeEvaluationEvidenceAuthorizer{},
		store,
		policyForEvaluation(t),
	)
	_, err := service.Submit(context.Background(), SubmitEvaluationRequest{
		TaskID:        "task-1",
		EvaluatorType: "agent",
		EvaluatorID:   "agent-test",
		Submission:    loopevaluation.Submission{EvaluationKind: "test", Verdict: "pass"},
	})
	if err == nil {
		t.Fatal("Submit() error = nil, want task lineage mismatch")
	}
	if store.calls != 0 {
		t.Fatalf("store calls = %d, want 0", store.calls)
	}
}

func TestEvaluationApplicationServiceReturnsPersistedResultWhenPolicyFails(t *testing.T) {
	store := &fakeEvaluationStore{value: PersistedEvaluation{EvaluationID: "evaluation-1", Created: true}}
	policyReader := &fakePolicyProjectionReader{err: errors.New("db unavailable")}
	policy := NewPolicyApplicationService(policyReader, &fakePolicyMutationSink{})
	service := NewEvaluationApplicationService(
		fakeEvaluationContextReader{value: EvaluationContext{
			WorkspaceID: "workspace-1", ParentIssueID: "parent-1", NodeIssueID: "issue-test",
			NodeKey: "test", TaskID: "task-1", PolicyVersion: "policy-v1", Plan: evaluationPlan(),
		}},
		&fakeEvaluationEvidenceAuthorizer{}, store, policy,
	)
	result, err := service.Submit(context.Background(), SubmitEvaluationRequest{
		TaskID: "task-1", EvaluatorType: "agent", EvaluatorID: "agent-test",
		Submission: loopevaluation.Submission{EvaluationKind: "test", Verdict: "pass"},
	})
	if err == nil {
		t.Fatal("Submit() error = nil, want policy failure")
	}
	if result.Evaluation.EvaluationID != "evaluation-1" {
		t.Fatalf("persisted evaluation lost on policy error: %#v", result)
	}
}

func TestApprovalApplicationServiceUsesAuthenticatedMemberAndTicksPolicy(t *testing.T) {
	policyReader := &fakePolicyProjectionReader{value: PolicyProjection{
		WorkspaceID: "workspace-1", ParentIssueID: "parent-1", PolicyVersion: "policy-v1",
		Plan: looptemplate.CompiledPlan{IncludedNodes: []looptemplate.CompiledNode{{Key: "approval", Type: looptemplate.NodeTypeApproval, Stage: 60}}},
		Runtime: looppolicy.RuntimeState{
			ParentState: "waiting_approval",
			Nodes:       map[string]looppolicy.NodeState{"approval": {Status: "todo"}},
			Approvals:   map[string]looppolicy.ApprovalState{"approval": {State: "approved"}},
		},
		NodeIssueIDs: map[string]string{"approval": "issue-approval"},
	}}
	store := &fakeApprovalStore{value: PersistedApprovalDecision{ApprovalID: "approval-1", State: "approved", Changed: true}}
	service := NewApprovalApplicationService(
		fakeApprovalContextReader{value: ApprovalContext{
			WorkspaceID: "workspace-1", ParentIssueID: "parent-1", NodeIssueID: "issue-approval",
			NodeKey: "approval", ApprovalKey: "release", State: "pending", PolicyVersion: "policy-v1",
			RequestedFromID: "member-1",
		}},
		store,
		NewPolicyApplicationService(policyReader, &fakePolicyMutationSink{}),
	)

	result, err := service.Decide(context.Background(), DecideApprovalRequest{
		ApprovalID: "approval-1", MemberID: "member-1", Decision: "approved", Rationale: "ship it",
	})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if store.last.DecidedBy != "member-1" {
		t.Fatalf("decided_by = %q, want authenticated member", store.last.DecidedBy)
	}
	if result.Approval.State != "approved" || !result.Policy.Applied {
		t.Fatalf("result = %#v", result)
	}
}

func TestApprovalApplicationServiceRejectsDifferentRequestedApprover(t *testing.T) {
	store := &fakeApprovalStore{}
	service := NewApprovalApplicationService(
		fakeApprovalContextReader{value: ApprovalContext{
			WorkspaceID: "workspace-1", ParentIssueID: "parent-1", NodeIssueID: "issue-approval",
			NodeKey: "approval", ApprovalKey: "release", State: "pending", PolicyVersion: "policy-v1",
			RequestedFromID: "member-owner",
		}},
		store,
		NewPolicyApplicationService(&fakePolicyProjectionReader{}, &fakePolicyMutationSink{}),
	)
	_, err := service.Decide(context.Background(), DecideApprovalRequest{
		ApprovalID: "approval-1", MemberID: "member-other", Decision: "approved",
	})
	if err == nil {
		t.Fatal("Decide() error = nil, want approver mismatch")
	}
	if store.calls != 0 {
		t.Fatalf("store calls = %d, want 0", store.calls)
	}
}

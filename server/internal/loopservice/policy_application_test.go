package loopservice

import (
	"context"
	"errors"
	"testing"

	"github.com/multica-ai/multica/server/internal/looppolicy"
	"github.com/multica-ai/multica/server/internal/looptemplate"
)

type fakePolicyProjectionReader struct {
	calls int
	arg   string
	value PolicyProjection
	err   error
}

func (f *fakePolicyProjectionReader) LoadPolicyProjection(_ context.Context, parentIssueID string) (PolicyProjection, error) {
	f.calls++
	f.arg = parentIssueID
	return f.value, f.err
}

type fakePolicyMutationSink struct {
	calls int
	last  ApplyPolicyDecisionCommand
	err   error
}

func (f *fakePolicyMutationSink) ApplyPolicyDecision(_ context.Context, cmd ApplyPolicyDecisionCommand) error {
	f.calls++
	f.last = cmd
	return f.err
}

func TestPolicyApplicationServiceActivatesNextStage(t *testing.T) {
	plan := looptemplate.CompiledPlan{IncludedNodes: []looptemplate.CompiledNode{
		{Key: "product", Type: looptemplate.NodeTypeAgent, Stage: 10, InitialStatus: "todo"},
		{Key: "architecture", Type: looptemplate.NodeTypeAgent, Stage: 20, InitialStatus: "backlog"},
	}}
	reader := &fakePolicyProjectionReader{value: PolicyProjection{
		WorkspaceID:   "workspace-1",
		ParentIssueID: "parent-1",
		PolicyVersion: "policy-v1",
		Plan:          plan,
		Runtime: looppolicy.RuntimeState{
			ParentState: "running",
			Nodes: map[string]looppolicy.NodeState{
				"product":      {Status: "done"},
				"architecture": {Status: "backlog"},
			},
		},
		NodeIssueIDs: map[string]string{
			"product":      "issue-product",
			"architecture": "issue-architecture",
		},
	}}
	sink := &fakePolicyMutationSink{}
	service := NewPolicyApplicationService(reader, sink)

	result, err := service.Tick(context.Background(), "parent-1")
	if err != nil {
		t.Fatalf("Tick() error = %v", err)
	}
	if !result.Applied {
		t.Fatal("Applied = false, want true")
	}
	if sink.calls != 1 {
		t.Fatalf("sink calls = %d, want 1", sink.calls)
	}
	if len(sink.last.Actions) != 1 {
		t.Fatalf("actions = %#v", sink.last.Actions)
	}
	if sink.last.Actions[0].Type != looppolicy.ActionActivateNode || sink.last.Actions[0].NodeKey != "architecture" {
		t.Fatalf("action = %#v", sink.last.Actions[0])
	}
	if sink.last.NodeIssueIDs["architecture"] != "issue-architecture" {
		t.Fatalf("node issue mapping = %#v", sink.last.NodeIssueIDs)
	}
}

func TestPolicyApplicationServiceAppliesFailureRecoveryAsOneDecision(t *testing.T) {
	maxRetries := 2
	plan := looptemplate.CompiledPlan{IncludedNodes: []looptemplate.CompiledNode{
		{Key: "backend", Type: looptemplate.NodeTypeAgent, Stage: 30, InitialStatus: "backlog", MaxRetries: maxRetries},
		{Key: "review", Type: looptemplate.NodeTypeAgent, Stage: 40, InitialStatus: "backlog"},
		{
			Key:           "test",
			Type:          looptemplate.NodeTypeEvaluation,
			Stage:         50,
			InitialStatus: "backlog",
			Evaluation: &looptemplate.EvaluationConfig{
				Kind:            "test",
				AllowedVerdicts: []string{"pass", "fail"},
				OnFail: looptemplate.EvaluationFailConfig{
					Strategy:           "fixed_target",
					TargetNode:         "backend",
					MaxWorkflowRetries: maxRetries,
				},
			},
		},
	}}
	reader := &fakePolicyProjectionReader{value: PolicyProjection{
		WorkspaceID:   "workspace-1",
		ParentIssueID: "parent-1",
		PolicyVersion: "policy-v2",
		Plan:          plan,
		Runtime: looppolicy.RuntimeState{
			ParentState: "running",
			Nodes: map[string]looppolicy.NodeState{
				"backend": {Status: "done", RetryCount: 0},
				"review":  {Status: "done"},
				"test":    {Status: "done"},
			},
			Evaluations: map[string]looppolicy.EvaluationState{
				"test": {Verdict: "fail", TargetNodeKey: "backend"},
			},
		},
		NodeIssueIDs: map[string]string{
			"backend": "issue-backend",
			"review":  "issue-review",
			"test":    "issue-test",
		},
	}}
	sink := &fakePolicyMutationSink{}
	service := NewPolicyApplicationService(reader, sink)

	result, err := service.Tick(context.Background(), "parent-1")
	if err != nil {
		t.Fatalf("Tick() error = %v", err)
	}
	if !result.Applied {
		t.Fatal("Applied = false, want true")
	}
	if sink.calls != 1 {
		t.Fatalf("sink calls = %d, want one atomic policy mutation", sink.calls)
	}
	if len(sink.last.Actions) != 4 {
		t.Fatalf("actions = %#v, want reopen backend + park review + park test + parent running", sink.last.Actions)
	}
	if sink.last.Actions[0].Type != looppolicy.ActionReopenNode || sink.last.Actions[0].NodeKey != "backend" {
		t.Fatalf("first action = %#v", sink.last.Actions[0])
	}
}

func TestPolicyApplicationServiceNoOpDoesNotWrite(t *testing.T) {
	plan := looptemplate.CompiledPlan{IncludedNodes: []looptemplate.CompiledNode{
		{Key: "product", Type: looptemplate.NodeTypeAgent, Stage: 10},
	}}
	reader := &fakePolicyProjectionReader{value: PolicyProjection{
		WorkspaceID:   "workspace-1",
		ParentIssueID: "parent-1",
		PolicyVersion: "policy-v1",
		Plan:          plan,
		Runtime: looppolicy.RuntimeState{
			ParentState: "completed",
			Nodes: map[string]looppolicy.NodeState{
				"product": {Status: "done"},
			},
		},
		NodeIssueIDs: map[string]string{"product": "issue-product"},
	}}
	sink := &fakePolicyMutationSink{}
	service := NewPolicyApplicationService(reader, sink)

	result, err := service.Tick(context.Background(), "parent-1")
	if err != nil {
		t.Fatalf("Tick() error = %v", err)
	}
	if result.Applied {
		t.Fatal("Applied = true, want false")
	}
	if sink.calls != 0 {
		t.Fatalf("sink calls = %d, want 0", sink.calls)
	}
}

func TestPolicyApplicationServiceRejectsStaleParentProjection(t *testing.T) {
	reader := &fakePolicyProjectionReader{value: PolicyProjection{
		WorkspaceID:   "workspace-1",
		ParentIssueID: "parent-other",
		PolicyVersion: "policy-v1",
		Plan: looptemplate.CompiledPlan{IncludedNodes: []looptemplate.CompiledNode{
			{Key: "product", Type: looptemplate.NodeTypeAgent, Stage: 10},
		}},
		Runtime: looppolicy.RuntimeState{Nodes: map[string]looppolicy.NodeState{"product": {Status: "todo"}}},
		NodeIssueIDs: map[string]string{"product": "issue-product"},
	}}
	sink := &fakePolicyMutationSink{}
	service := NewPolicyApplicationService(reader, sink)

	_, err := service.Tick(context.Background(), "parent-1")
	if err == nil {
		t.Fatal("Tick() error = nil, want parent mismatch")
	}
	if sink.calls != 0 {
		t.Fatalf("sink calls = %d, want 0", sink.calls)
	}
}

func TestPolicyApplicationServicePropagatesAtomicApplyFailure(t *testing.T) {
	applyErr := errors.New("stale issue revision")
	plan := looptemplate.CompiledPlan{IncludedNodes: []looptemplate.CompiledNode{
		{Key: "product", Type: looptemplate.NodeTypeAgent, Stage: 10},
	}}
	reader := &fakePolicyProjectionReader{value: PolicyProjection{
		WorkspaceID:   "workspace-1",
		ParentIssueID: "parent-1",
		PolicyVersion: "policy-v1",
		Plan:          plan,
		Runtime: looppolicy.RuntimeState{
			ParentState: "running",
			Nodes:       map[string]looppolicy.NodeState{"product": {Status: "done"}},
		},
		NodeIssueIDs: map[string]string{"product": "issue-product"},
	}}
	sink := &fakePolicyMutationSink{err: applyErr}
	service := NewPolicyApplicationService(reader, sink)

	result, err := service.Tick(context.Background(), "parent-1")
	if !errors.Is(err, applyErr) {
		t.Fatalf("Tick() error = %v, want apply error", err)
	}
	if result.Applied {
		t.Fatal("Applied = true, want false")
	}
	if sink.calls != 1 {
		t.Fatalf("sink calls = %d, want 1", sink.calls)
	}
}

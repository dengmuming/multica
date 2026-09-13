package looppolicy

import (
	"testing"

	"github.com/multica-ai/multica/server/internal/looptemplate"
)

func testPlan() looptemplate.CompiledPlan {
	return looptemplate.CompiledPlan{
		TemplateKey:   "feature-development",
		SchemaVersion: 1,
		InitialStage:  10,
		IncludedNodes: []looptemplate.CompiledNode{
			{Key: "product", Type: looptemplate.NodeTypeAgent, Stage: 10, InitialStatus: "todo"},
			{Key: "backend", Type: looptemplate.NodeTypeAgent, Stage: 20, InitialStatus: "backlog"},
			{
				Key: "review", Type: looptemplate.NodeTypeEvaluation, Stage: 30, InitialStatus: "backlog",
				Evaluation: &looptemplate.EvaluationConfig{
					Kind: "review", AllowedVerdicts: []string{"pass", "fail"},
					OnFail: looptemplate.EvaluationFailConfig{Strategy: "fixed_target", TargetNode: "backend", MaxWorkflowRetries: 2},
				},
			},
			{
				Key: "test", Type: looptemplate.NodeTypeEvaluation, Stage: 40, InitialStatus: "backlog",
				Evaluation: &looptemplate.EvaluationConfig{
					Kind: "test", AllowedVerdicts: []string{"pass", "fail"},
					OnFail: looptemplate.EvaluationFailConfig{Strategy: "route_by_finding_owner", FallbackNode: "backend", MaxWorkflowRetries: 3},
				},
			},
			{
				Key: "approval", Type: looptemplate.NodeTypeApproval, Stage: 50, InitialStatus: "backlog",
				Approval: &looptemplate.ApprovalConfig{
					RequiredRole: "member", MinApprovals: 1,
					OnReject: looptemplate.ApprovalRejectConfig{TargetNode: "backend", MaxWorkflowRetries: 1},
				},
			},
			{Key: "release", Type: looptemplate.NodeTypeAgent, Stage: 60, InitialStatus: "backlog"},
		},
	}
}

func stateForPlan(plan looptemplate.CompiledPlan) RuntimeState {
	nodes := make(map[string]NodeState, len(plan.IncludedNodes))
	for _, node := range plan.IncludedNodes {
		nodes[node.Key] = NodeState{Status: node.InitialStatus}
	}
	return RuntimeState{
		ParentState: "running",
		Nodes:       nodes,
		Evaluations: map[string]EvaluationState{},
		Approvals:   map[string]ApprovalState{},
	}
}

func TestEvaluateActivatesNextStage(t *testing.T) {
	plan := testPlan()
	state := stateForPlan(plan)
	state.Nodes["product"] = NodeState{Status: "done"}

	decision, err := Evaluate(plan, state)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	assertHasAction(t, decision, ActionActivateNode, "backend")
}

func TestEvaluateRoutesFailedTestBackToOwnerAndParksDownstream(t *testing.T) {
	plan := testPlan()
	state := stateForPlan(plan)
	state.Nodes["product"] = NodeState{Status: "done"}
	state.Nodes["backend"] = NodeState{Status: "done", RetryCount: 0}
	state.Nodes["review"] = NodeState{Status: "done"}
	state.Evaluations["review"] = EvaluationState{Verdict: "pass"}
	state.Nodes["test"] = NodeState{Status: "done"}
	state.Evaluations["test"] = EvaluationState{Verdict: "fail", TargetNodeKey: "backend"}
	state.Nodes["approval"] = NodeState{Status: "todo"}
	state.Nodes["release"] = NodeState{Status: "todo"}

	decision, err := Evaluate(plan, state)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	assertHasAction(t, decision, ActionReopenNode, "backend")
	assertHasAction(t, decision, ActionParkNode, "review")
	assertHasAction(t, decision, ActionParkNode, "test")
	assertHasAction(t, decision, ActionParkNode, "approval")
	assertHasAction(t, decision, ActionParkNode, "release")
}

func TestEvaluateIgnoresOldFailedEvaluationAfterGateIsParked(t *testing.T) {
	plan := testPlan()
	state := stateForPlan(plan)
	state.Nodes["product"] = NodeState{Status: "done"}
	state.Nodes["backend"] = NodeState{Status: "done", RetryCount: 1}
	state.Nodes["review"] = NodeState{Status: "backlog"}
	state.Nodes["test"] = NodeState{Status: "backlog"}
	state.Evaluations["test"] = EvaluationState{Verdict: "fail", TargetNodeKey: "backend"}

	decision, err := Evaluate(plan, state)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	assertHasAction(t, decision, ActionActivateNode, "review")
	assertNoAction(t, decision, ActionReopenNode, "backend")
}

func TestEvaluateDoesNotDoubleApplyRecoveryWhileTargetActive(t *testing.T) {
	plan := testPlan()
	state := stateForPlan(plan)
	state.Nodes["product"] = NodeState{Status: "done"}
	state.Nodes["backend"] = NodeState{Status: "in_progress", RetryCount: 1}
	state.Nodes["review"] = NodeState{Status: "done"}
	state.Evaluations["review"] = EvaluationState{Verdict: "pass"}
	state.Nodes["test"] = NodeState{Status: "done"}
	state.Evaluations["test"] = EvaluationState{Verdict: "fail", TargetNodeKey: "backend"}

	decision, err := Evaluate(plan, state)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if len(decision.Actions) != 0 {
		t.Fatalf("Actions = %#v, want none while recovery is already active", decision.Actions)
	}
}

func TestEvaluateBlocksWhenWorkflowRetryBudgetIsExhausted(t *testing.T) {
	plan := testPlan()
	state := stateForPlan(plan)
	state.Nodes["product"] = NodeState{Status: "done"}
	state.Nodes["backend"] = NodeState{Status: "done", RetryCount: 3}
	state.Nodes["review"] = NodeState{Status: "done"}
	state.Evaluations["review"] = EvaluationState{Verdict: "pass"}
	state.Nodes["test"] = NodeState{Status: "done"}
	state.Evaluations["test"] = EvaluationState{Verdict: "fail", TargetNodeKey: "backend"}

	decision, err := Evaluate(plan, state)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	assertHasAction(t, decision, ActionBlockLoop, "")
}

func TestEvaluateRequestsHumanApproval(t *testing.T) {
	plan := testPlan()
	state := stateForPlan(plan)
	state.Nodes["product"] = NodeState{Status: "done"}
	state.Nodes["backend"] = NodeState{Status: "done"}
	state.Nodes["review"] = NodeState{Status: "done"}
	state.Evaluations["review"] = EvaluationState{Verdict: "pass"}
	state.Nodes["test"] = NodeState{Status: "done"}
	state.Evaluations["test"] = EvaluationState{Verdict: "pass"}

	decision, err := Evaluate(plan, state)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	assertHasAction(t, decision, ActionRequestApproval, "approval")
	assertParentStateAction(t, decision, "waiting_approval")
}

func TestEvaluateApprovalRejectionRoutesRecovery(t *testing.T) {
	plan := testPlan()
	state := stateForPlan(plan)
	state.Nodes["product"] = NodeState{Status: "done"}
	state.Nodes["backend"] = NodeState{Status: "done"}
	state.Nodes["review"] = NodeState{Status: "done"}
	state.Evaluations["review"] = EvaluationState{Verdict: "pass"}
	state.Nodes["test"] = NodeState{Status: "done"}
	state.Evaluations["test"] = EvaluationState{Verdict: "pass"}
	state.Nodes["approval"] = NodeState{Status: "todo"}
	state.Approvals["approval"] = ApprovalState{State: "rejected"}

	decision, err := Evaluate(plan, state)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	assertHasAction(t, decision, ActionReopenNode, "backend")
}

func TestEvaluateCompletesLoopAfterRelease(t *testing.T) {
	plan := testPlan()
	state := stateForPlan(plan)
	state.Nodes["product"] = NodeState{Status: "done"}
	state.Nodes["backend"] = NodeState{Status: "done"}
	state.Nodes["review"] = NodeState{Status: "done"}
	state.Evaluations["review"] = EvaluationState{Verdict: "pass"}
	state.Nodes["test"] = NodeState{Status: "done"}
	state.Evaluations["test"] = EvaluationState{Verdict: "pass"}
	state.Nodes["approval"] = NodeState{Status: "done"}
	state.Approvals["approval"] = ApprovalState{State: "approved"}
	state.Nodes["release"] = NodeState{Status: "done"}

	decision, err := Evaluate(plan, state)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	assertHasAction(t, decision, ActionCompleteLoop, "")
}

func assertHasAction(t *testing.T, decision Decision, actionType ActionType, nodeKey string) {
	t.Helper()
	for _, action := range decision.Actions {
		if action.Type == actionType && (nodeKey == "" || action.NodeKey == nodeKey) {
			return
		}
	}
	t.Fatalf("decision %#v has no %s action for node %q", decision, actionType, nodeKey)
}

func assertNoAction(t *testing.T, decision Decision, actionType ActionType, nodeKey string) {
	t.Helper()
	for _, action := range decision.Actions {
		if action.Type == actionType && (nodeKey == "" || action.NodeKey == nodeKey) {
			t.Fatalf("decision %#v unexpectedly has %s action for node %q", decision, actionType, nodeKey)
		}
	}
}

func assertParentStateAction(t *testing.T, decision Decision, want string) {
	t.Helper()
	for _, action := range decision.Actions {
		if action.Type == ActionSetParentState && action.ParentState == want {
			return
		}
	}
	t.Fatalf("decision %#v has no parent state action %q", decision, want)
}

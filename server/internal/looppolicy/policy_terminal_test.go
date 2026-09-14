package looppolicy

import "testing"

func TestEvaluateTreatsBlockedParentAsTerminal(t *testing.T) {
	plan := testPlan()
	state := stateForPlan(plan)
	state.ParentState = "blocked"
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
	if decision.Reason != "parent_terminal" || len(decision.Actions) != 0 {
		t.Fatalf("decision = %#v, want blocked parent terminal no-op", decision)
	}
}

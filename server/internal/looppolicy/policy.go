package looppolicy

import (
	"fmt"
	"sort"

	"github.com/multica-ai/multica/server/internal/looptemplate"
)

// ActionType is intentionally small. The application layer translates these
// deterministic commands into existing Issue/Inbox/Approval mutations.
type ActionType string

const (
	ActionActivateNode    ActionType = "activate_node"
	ActionReopenNode      ActionType = "reopen_node"
	ActionParkNode        ActionType = "park_node"
	ActionRequestApproval ActionType = "request_approval"
	ActionSetParentState  ActionType = "set_parent_state"
	ActionBlockLoop       ActionType = "block_loop"
	ActionCompleteLoop    ActionType = "complete_loop"
)

type Action struct {
	Type        ActionType `json:"type"`
	NodeKey     string     `json:"node_key,omitempty"`
	ParentState string     `json:"parent_state,omitempty"`
	Reason      string     `json:"reason,omitempty"`
}

type Decision struct {
	Actions []Action `json:"actions"`
	Reason  string   `json:"reason"`
}

// RuntimeState is the minimum state required for a pure policy decision.
// Callers build it from child Issues plus authoritative evaluation/approval
// rows. Policy never reads comments or arbitrary agent prose.
type RuntimeState struct {
	ParentState string                     `json:"parent_state"`
	Nodes       map[string]NodeState       `json:"nodes"`
	Evaluations map[string]EvaluationState `json:"evaluations,omitempty"`
	Approvals   map[string]ApprovalState   `json:"approvals,omitempty"`
}

type NodeState struct {
	Status     string `json:"status"`
	RetryCount int    `json:"retry_count"`
}

type EvaluationState struct {
	Verdict       string `json:"verdict"`
	TargetNodeKey string `json:"target_node_key,omitempty"`
}

type ApprovalState struct {
	State string `json:"state"`
}

// Evaluate returns the next idempotent command plan. It never mutates state.
// Recovery decisions are handled before normal forward stage advancement.
func Evaluate(plan looptemplate.CompiledPlan, state RuntimeState) (Decision, error) {
	if len(plan.IncludedNodes) == 0 {
		return Decision{}, fmt.Errorf("compiled plan contains no nodes")
	}
	if state.Nodes == nil {
		return Decision{}, fmt.Errorf("runtime node state is required")
	}
	for _, node := range plan.IncludedNodes {
		if _, ok := state.Nodes[node.Key]; !ok {
			return Decision{}, fmt.Errorf("runtime state missing node %q", node.Key)
		}
	}

	if state.ParentState == "cancelled" || state.ParentState == "completed" || state.ParentState == "failed" || state.ParentState == "blocked" {
		return Decision{Reason: "parent_terminal"}, nil
	}

	if decision, handled, err := evaluateRecovery(plan, state); handled || err != nil {
		return decision, err
	}

	stages := uniqueStages(plan.IncludedNodes)
	for _, stage := range stages {
		nodes := nodesAtStage(plan.IncludedNodes, stage)
		satisfied, err := stageSatisfied(nodes, state)
		if err != nil {
			return Decision{}, err
		}
		if satisfied {
			continue
		}

		if !priorStagesSatisfied(plan.IncludedNodes, stage, state) {
			return Decision{Reason: fmt.Sprintf("stage_%d_waiting_on_prior_stage", stage)}, nil
		}

		return activationDecision(stage, nodes, state), nil
	}

	if state.ParentState == "completed" {
		return Decision{Reason: "loop_already_completed"}, nil
	}
	return Decision{
		Actions: []Action{{Type: ActionCompleteLoop, ParentState: "completed", Reason: "all_stages_satisfied"}},
		Reason:  "all_stages_satisfied",
	}, nil
}

func evaluateRecovery(plan looptemplate.CompiledPlan, state RuntimeState) (Decision, bool, error) {
	// Earliest failed *active/completed* gate wins. A gate parked back to backlog
	// during recovery deliberately ignores its old evaluation/approval row; that
	// evidence belongs to the previous workflow attempt and must not fire again
	// after the target developer node finishes.
	for _, node := range plan.IncludedNodes {
		nodeState := state.Nodes[node.Key]
		if nodeState.Status == "backlog" {
			continue
		}

		switch node.Type {
		case looptemplate.NodeTypeEvaluation:
			// Structured evaluations are accepted only after the evaluator run
			// completes. Requiring done here also prevents a stale result from
			// racing a newly activated evaluator Issue.
			if nodeState.Status != "done" {
				continue
			}
			eval, ok := state.Evaluations[node.Key]
			if !ok {
				continue
			}
			switch eval.Verdict {
			case "fail":
				return evaluationFailureDecision(plan, state, node, eval), true, nil
			case "inconclusive":
				if node.Evaluation != nil && node.Evaluation.OnInconclusive == "block" {
					return blockDecision("evaluation_inconclusive:" + node.Key), true, nil
				}
				if node.Evaluation != nil && node.Evaluation.OnInconclusive == "retry_evaluator" {
					return retryEvaluatorDecision(plan, state, node), true, nil
				}
			}
		case looptemplate.NodeTypeApproval:
			approval, ok := state.Approvals[node.Key]
			if !ok {
				continue
			}
			if approval.State == "rejected" {
				return approvalRejectionDecision(plan, state, node), true, nil
			}
		}
	}
	return Decision{}, false, nil
}

func evaluationFailureDecision(plan looptemplate.CompiledPlan, state RuntimeState, gate looptemplate.CompiledNode, eval EvaluationState) Decision {
	if gate.Evaluation == nil {
		return blockDecision("evaluation_config_missing:" + gate.Key)
	}
	cfg := gate.Evaluation.OnFail
	if cfg.Strategy == "block" {
		return blockDecision("evaluation_failed:" + gate.Key)
	}

	target := eval.TargetNodeKey
	if cfg.Strategy == "fixed_target" {
		target = cfg.TargetNode
	} else if target == "" {
		target = cfg.FallbackNode
	}
	if target == "" {
		return blockDecision("evaluation_failure_has_no_target:" + gate.Key)
	}
	return recoveryDecision(plan, state, target, cfg.MaxWorkflowRetries, "evaluation_failed:"+gate.Key)
}

func approvalRejectionDecision(plan looptemplate.CompiledPlan, state RuntimeState, gate looptemplate.CompiledNode) Decision {
	if gate.Approval == nil || gate.Approval.OnReject.TargetNode == "" {
		return blockDecision("approval_rejected:" + gate.Key)
	}
	return recoveryDecision(
		plan,
		state,
		gate.Approval.OnReject.TargetNode,
		gate.Approval.OnReject.MaxWorkflowRetries,
		"approval_rejected:"+gate.Key,
	)
}

func retryEvaluatorDecision(plan looptemplate.CompiledPlan, state RuntimeState, gate looptemplate.CompiledNode) Decision {
	maxRetries := gate.MaxRetries
	if maxRetries <= 0 {
		return blockDecision("evaluation_inconclusive_retry_exhausted:" + gate.Key)
	}
	return recoveryDecision(plan, state, gate.Key, maxRetries, "evaluation_inconclusive:"+gate.Key)
}

func recoveryDecision(plan looptemplate.CompiledPlan, state RuntimeState, target string, maxRetries int, reason string) Decision {
	targetNode, ok := compiledNode(plan, target)
	if !ok {
		return blockDecision(reason + ":unknown_target")
	}
	targetState := state.Nodes[target]
	if maxRetries <= 0 || targetState.RetryCount >= maxRetries {
		return blockDecision(reason + ":retry_budget_exhausted")
	}

	// If the target is already active, the recovery command was applied on a
	// previous policy tick. Do not reopen/increment it again.
	if targetState.Status == "todo" || targetState.Status == "in_progress" || targetState.Status == "in_review" {
		return Decision{Reason: reason + ":recovery_in_progress"}
	}

	actions := []Action{{Type: ActionReopenNode, NodeKey: target, Reason: reason}}
	for _, node := range plan.IncludedNodes {
		if node.Stage <= targetNode.Stage || node.Key == target {
			continue
		}
		if state.Nodes[node.Key].Status != "backlog" {
			actions = append(actions, Action{Type: ActionParkNode, NodeKey: node.Key, Reason: reason})
		}
	}
	actions = append(actions, Action{Type: ActionSetParentState, ParentState: "running", Reason: reason})
	return Decision{Actions: actions, Reason: reason}
}

func activationDecision(stage int, nodes []looptemplate.CompiledNode, state RuntimeState) Decision {
	actions := make([]Action, 0, len(nodes)+1)
	waitingApproval := false
	for _, node := range nodes {
		nodeState := state.Nodes[node.Key]
		if node.Type == looptemplate.NodeTypeApproval {
			approval := state.Approvals[node.Key]
			if approval.State == "approved" {
				continue
			}
			waitingApproval = true
			if approval.State == "" {
				actions = append(actions, Action{Type: ActionRequestApproval, NodeKey: node.Key, Reason: fmt.Sprintf("stage_%d_ready", stage)})
			}
			continue
		}
		if nodeState.Status == "backlog" {
			actions = append(actions, Action{Type: ActionActivateNode, NodeKey: node.Key, Reason: fmt.Sprintf("stage_%d_ready", stage)})
		}
	}

	parentState := "running"
	if waitingApproval {
		parentState = "waiting_approval"
	}
	if state.ParentState != parentState {
		actions = append(actions, Action{Type: ActionSetParentState, ParentState: parentState, Reason: fmt.Sprintf("stage_%d_ready", stage)})
	}
	return Decision{Actions: actions, Reason: fmt.Sprintf("stage_%d_active", stage)}
}

func stageSatisfied(nodes []looptemplate.CompiledNode, state RuntimeState) (bool, error) {
	for _, node := range nodes {
		nodeState := state.Nodes[node.Key]
		switch node.Type {
		case looptemplate.NodeTypeAgent:
			if nodeState.Status != "done" {
				return false, nil
			}
		case looptemplate.NodeTypeEvaluation:
			eval, ok := state.Evaluations[node.Key]
			if !ok || nodeState.Status != "done" || (eval.Verdict != "pass" && eval.Verdict != "warn") {
				return false, nil
			}
		case looptemplate.NodeTypeApproval:
			approval, ok := state.Approvals[node.Key]
			if !ok || approval.State != "approved" {
				return false, nil
			}
		default:
			return false, fmt.Errorf("unsupported compiled node type %q", node.Type)
		}
	}
	return true, nil
}

func priorStagesSatisfied(nodes []looptemplate.CompiledNode, stage int, state RuntimeState) bool {
	for _, prior := range uniqueStages(nodes) {
		if prior >= stage {
			break
		}
		satisfied, err := stageSatisfied(nodesAtStage(nodes, prior), state)
		if err != nil || !satisfied {
			return false
		}
	}
	return true
}

func blockDecision(reason string) Decision {
	return Decision{
		Actions: []Action{{Type: ActionBlockLoop, ParentState: "blocked", Reason: reason}},
		Reason:  reason,
	}
}

func compiledNode(plan looptemplate.CompiledPlan, key string) (looptemplate.CompiledNode, bool) {
	for _, node := range plan.IncludedNodes {
		if node.Key == key {
			return node, true
		}
	}
	return looptemplate.CompiledNode{}, false
}

func uniqueStages(nodes []looptemplate.CompiledNode) []int {
	seen := map[int]bool{}
	stages := make([]int, 0)
	for _, node := range nodes {
		if !seen[node.Stage] {
			seen[node.Stage] = true
			stages = append(stages, node.Stage)
		}
	}
	sort.Ints(stages)
	return stages
}

func nodesAtStage(nodes []looptemplate.CompiledNode, stage int) []looptemplate.CompiledNode {
	out := make([]looptemplate.CompiledNode, 0)
	for _, node := range nodes {
		if node.Stage == stage {
			out = append(out, node)
		}
	}
	return out
}

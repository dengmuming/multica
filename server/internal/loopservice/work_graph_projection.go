package loopservice

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/multica-ai/multica/server/internal/looptemplate"
)

type WorkGraphLoopReader interface {
	GetLoop(context.Context, string, string) (LoopDetail, error)
}

type WorkGraphTemplateReader interface {
	LoadPinnedLoopTemplate(context.Context, string, string, int) (PinnedLoopTemplate, error)
}

// MulticaWorkGraphProjectionReader projects existing canonical Loop/Multica
// state into the product-facing Work Graph contract. It deliberately owns no
// Work Graph persistence.
type MulticaWorkGraphProjectionReader struct {
	loops     WorkGraphLoopReader
	templates WorkGraphTemplateReader
}

func NewMulticaWorkGraphProjectionReader(loops WorkGraphLoopReader, templates WorkGraphTemplateReader) *MulticaWorkGraphProjectionReader {
	return &MulticaWorkGraphProjectionReader{loops: loops, templates: templates}
}

func (r *MulticaWorkGraphProjectionReader) GetWorkGraph(ctx context.Context, workspaceID, parentIssueID string) (*WorkGraph, error) {
	if r == nil || r.loops == nil || r.templates == nil {
		return nil, errors.New("work graph projection dependencies are required")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	parentIssueID = strings.TrimSpace(parentIssueID)
	if workspaceID == "" || parentIssueID == "" {
		return nil, errors.New("workspace and parent issue id are required")
	}

	detail, err := r.loops.GetLoop(ctx, workspaceID, parentIssueID)
	if err != nil {
		return nil, err
	}
	pinned, err := r.templates.LoadPinnedLoopTemplate(ctx, workspaceID, detail.TemplateKey, detail.TemplateVersion)
	if err != nil {
		return nil, fmt.Errorf("load work graph pinned template: %w", err)
	}

	readByKey := make(map[string]LoopNodeRead, len(detail.Nodes))
	for _, node := range detail.Nodes {
		readByKey[node.NodeKey] = node
	}

	graph := &WorkGraph{
		ID:          detail.ParentIssueID,
		WorkspaceID: workspaceID,
		ProjectID:   detail.ProjectID,
		Intent: WorkGraphIntent{
			Title:       detail.Title,
			Description: detail.Description,
			Source:      "loop",
		},
		Status: mapWorkGraphStatus(detail.State, detail.IssueStatus),
		Nodes:  make([]WorkGraphNode, 0, len(pinned.Definition.Nodes)),
		Edges:  []WorkGraphEdge{},
		Gates:  []WorkGraphGate{},
	}

	included := make([]looptemplate.Node, 0, len(pinned.Definition.Nodes))
	for _, definitionNode := range pinned.Definition.Nodes {
		read, ok := readByKey[definitionNode.Key]
		if !ok {
			// Optional roles may compile out of an instance. Only persisted work
			// items are part of this Work Graph instance.
			continue
		}
		included = append(included, definitionNode)
		node := projectWorkGraphNode(definitionNode, read)
		graph.Nodes = append(graph.Nodes, node)
		if read.Stage >= 0 && read.Status != "backlog" {
			stage := strconv.Itoa(read.Stage)
			if graph.CurrentStage == "" || read.Stage > parseStage(graph.CurrentStage) {
				graph.CurrentStage = stage
			}
		}
		if read.EvaluationID != "" {
			graph.ProvenanceSummary.EvaluationCount++
		}
		if definitionNode.Type == looptemplate.NodeTypeEvaluation {
			graph.Gates = append(graph.Gates, WorkGraphGate{NodeKey: definitionNode.Key, Type: "evaluation", State: evaluationGateState(read)})
		}
		if definitionNode.Type == looptemplate.NodeTypeApproval {
			graph.Gates = append(graph.Gates, WorkGraphGate{NodeKey: definitionNode.Key, Type: "human_approval", State: approvalGateState(read)})
		}
	}

	graph.Edges = projectWorkGraphEdges(included)
	return graph, nil
}

func projectWorkGraphNode(def looptemplate.Node, read LoopNodeRead) WorkGraphNode {
	node := WorkGraphNode{
		Key:            def.Key,
		Kind:           mapWorkGraphNodeKind(def),
		Role:           def.Role,
		Responsibility: def.Description,
		Stage:          strconv.Itoa(read.Stage),
		State:          read.Status,
	}
	if read.AssigneeID != "" {
		node.Assignee = &WorkGraphAssignee{Type: read.AssigneeType, ID: read.AssigneeID}
	}
	if read.LatestTaskID != "" {
		node.CurrentTask = &WorkGraphTask{ID: read.LatestTaskID, Status: read.LatestTaskStatus, Attempt: read.RetryCount + 1}
	}
	if read.EvaluationID != "" {
		node.Evaluation = &WorkGraphEvaluation{Verdict: read.EvaluationVerdict, FindingCount: len(read.EvaluationFindings)}
	}
	return node
}

func projectWorkGraphEdges(nodes []looptemplate.Node) []WorkGraphEdge {
	if len(nodes) < 2 {
		return []WorkGraphEdge{}
	}
	byStage := map[int][]looptemplate.Node{}
	stages := []int{}
	seen := map[int]bool{}
	for _, node := range nodes {
		byStage[node.Stage] = append(byStage[node.Stage], node)
		if !seen[node.Stage] {
			seen[node.Stage] = true
			stages = append(stages, node.Stage)
		}
	}
	// Templates are validated/compiled in stage order, but sort locally so the
	// projection contract does not depend on persisted JSON array ordering.
	for i := 0; i < len(stages); i++ {
		for j := i + 1; j < len(stages); j++ {
			if stages[j] < stages[i] { stages[i], stages[j] = stages[j], stages[i] }
		}
	}
	edges := []WorkGraphEdge{}
	for i := 0; i+1 < len(stages); i++ {
		for _, from := range byStage[stages[i]] {
			for _, to := range byStage[stages[i+1]] {
				edges = append(edges, WorkGraphEdge{From: from.Key, To: to.Key, Kind: "dependency"})
			}
		}
	}
	for _, node := range nodes {
		if node.Evaluation != nil {
			if target := strings.TrimSpace(node.Evaluation.OnFail.TargetNode); target != "" {
				edges = append(edges, WorkGraphEdge{From: node.Key, To: target, Kind: "recovery"})
			}
		}
		if node.Approval != nil {
			if target := strings.TrimSpace(node.Approval.OnReject.TargetNode); target != "" {
				edges = append(edges, WorkGraphEdge{From: node.Key, To: target, Kind: "recovery"})
			}
		}
	}
	return edges
}

func mapWorkGraphNodeKind(node looptemplate.Node) string {
	switch node.Type {
	case looptemplate.NodeTypeEvaluation:
		return "evaluation"
	case looptemplate.NodeTypeApproval:
		return "approval"
	default:
		if strings.Contains(strings.ToLower(node.Key), "release") || strings.Contains(strings.ToLower(node.Role), "release") {
			return "release"
		}
		if strings.Contains(strings.ToLower(node.Key), "observe") || strings.Contains(strings.ToLower(node.Role), "observe") {
			return "observe"
		}
		return "agent"
	}
}

func mapWorkGraphStatus(loopState, issueStatus string) string {
	state := strings.ToLower(strings.TrimSpace(loopState))
	switch state {
	case "blocked", "completed", "failed", "cancelled", "observing", "released", "awaiting_approval", "planning", "running":
		return state
	}
	switch strings.ToLower(strings.TrimSpace(issueStatus)) {
	case "done": return "completed"
	case "cancelled": return "cancelled"
	default: return "running"
	}
}

func evaluationGateState(node LoopNodeRead) string {
	if node.EvaluationVerdict != "" { return strings.ToLower(node.EvaluationVerdict) }
	return node.Status
}

func approvalGateState(node LoopNodeRead) string {
	if node.ApprovalState != "" { return node.ApprovalState }
	return node.Status
}

func parseStage(raw string) int {
	v, _ := strconv.Atoi(raw)
	return v
}

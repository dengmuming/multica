package loopservice

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/multica-ai/multica/server/internal/looptemplate"
)

type WorkGraphLoopReader interface { GetLoop(context.Context, string, string) (LoopDetail, error) }
type WorkGraphTemplateReader interface { LoadPinnedLoopTemplate(context.Context, string, string, int) (PinnedLoopTemplate, error) }
type WorkGraphArtifactReader interface { ListArtifacts(context.Context, string, string) ([]RegisteredArtifact, error) }

type MulticaWorkGraphProjectionReader struct {
	loops WorkGraphLoopReader
	templates WorkGraphTemplateReader
	artifacts WorkGraphArtifactReader
}

func NewMulticaWorkGraphProjectionReader(loops WorkGraphLoopReader, templates WorkGraphTemplateReader, artifacts ...WorkGraphArtifactReader) *MulticaWorkGraphProjectionReader {
	r := &MulticaWorkGraphProjectionReader{loops: loops, templates: templates}
	if len(artifacts) > 0 { r.artifacts = artifacts[0] }
	return r
}

func (r *MulticaWorkGraphProjectionReader) GetWorkGraph(ctx context.Context, workspaceID, parentIssueID string) (*WorkGraph, error) {
	if r == nil || r.loops == nil || r.templates == nil { return nil, errors.New("work graph projection dependencies are required") }
	workspaceID, parentIssueID = strings.TrimSpace(workspaceID), strings.TrimSpace(parentIssueID)
	if workspaceID == "" || parentIssueID == "" { return nil, errors.New("workspace and parent issue id are required") }
	detail, err := r.loops.GetLoop(ctx, workspaceID, parentIssueID); if err != nil { return nil, err }
	pinned, err := r.templates.LoadPinnedLoopTemplate(ctx, workspaceID, detail.TemplateKey, detail.TemplateVersion); if err != nil { return nil, fmt.Errorf("load work graph pinned template: %w", err) }

	artifactCounts := map[string]int{}
	artifactTotal := 0
	if r.artifacts != nil {
		items, err := r.artifacts.ListArtifacts(ctx, workspaceID, parentIssueID); if err != nil { return nil, fmt.Errorf("load work graph artifacts: %w", err) }
		artifactTotal = len(items)
		for _, item := range items { if item.NodeIssueID != "" { artifactCounts[item.NodeIssueID]++ } }
	}
	readByKey := make(map[string]LoopNodeRead, len(detail.Nodes)); for _, n := range detail.Nodes { readByKey[n.NodeKey] = n }
	graph := &WorkGraph{ID: detail.ParentIssueID, WorkspaceID: workspaceID, ProjectID: detail.ProjectID, Intent: WorkGraphIntent{Title: detail.Title, Description: detail.Description, Source: "loop"}, Status: mapWorkGraphStatus(detail.State, detail.IssueStatus), Nodes: []WorkGraphNode{}, Edges: []WorkGraphEdge{}, Gates: []WorkGraphGate{}, ProvenanceSummary: WorkGraphProvenance{ArtifactCount: artifactTotal}}
	included := []looptemplate.Node{}
	for _, def := range pinned.Definition.Nodes {
		read, ok := readByKey[def.Key]; if !ok { continue }
		included = append(included, def)
		node := projectWorkGraphNode(def, read); node.ArtifactCount = artifactCounts[read.IssueID]; graph.Nodes = append(graph.Nodes, node)
		if read.Stage >= 0 && read.Status != "backlog" { stage := strconv.Itoa(read.Stage); if graph.CurrentStage == "" || read.Stage > parseStage(graph.CurrentStage) { graph.CurrentStage = stage } }
		if read.EvaluationID != "" { graph.ProvenanceSummary.EvaluationCount++ }
		if def.Type == looptemplate.NodeTypeEvaluation { graph.Gates = append(graph.Gates, WorkGraphGate{NodeKey: def.Key, Type: "evaluation", State: evaluationGateState(read)}) }
		if def.Type == looptemplate.NodeTypeApproval { graph.Gates = append(graph.Gates, WorkGraphGate{NodeKey: def.Key, Type: "human_approval", State: approvalGateState(read)}) }
	}
	graph.Edges = projectWorkGraphEdges(included)
	return graph, nil
}

func projectWorkGraphNode(def looptemplate.Node, read LoopNodeRead) WorkGraphNode {
	n := WorkGraphNode{Key: def.Key, Kind: mapWorkGraphNodeKind(def), Role: def.Role, Responsibility: def.Description, Stage: strconv.Itoa(read.Stage), State: read.Status}
	if read.AssigneeID != "" { n.Assignee = &WorkGraphAssignee{Type: read.AssigneeType, ID: read.AssigneeID} }
	if read.LatestTaskID != "" { n.CurrentTask = &WorkGraphTask{ID: read.LatestTaskID, Status: read.LatestTaskStatus, Attempt: read.RetryCount + 1} }
	if read.EvaluationID != "" { n.Evaluation = &WorkGraphEvaluation{Verdict: read.EvaluationVerdict, FindingCount: len(read.EvaluationFindings)} }
	return n
}

func projectWorkGraphEdges(nodes []looptemplate.Node) []WorkGraphEdge {
	if len(nodes) < 2 { return []WorkGraphEdge{} }
	byStage := map[int][]looptemplate.Node{}; stages := []int{}; seen := map[int]bool{}; keys := map[string]bool{}
	for _, n := range nodes { byStage[n.Stage] = append(byStage[n.Stage], n); keys[n.Key] = true; if !seen[n.Stage] { seen[n.Stage] = true; stages = append(stages, n.Stage) } }
	sort.Ints(stages); edges := []WorkGraphEdge{}
	for i := 0; i+1 < len(stages); i++ { for _, from := range byStage[stages[i]] { for _, to := range byStage[stages[i+1]] { edges = append(edges, WorkGraphEdge{From: from.Key, To: to.Key, Kind: "dependency"}) } } }
	for _, n := range nodes {
		if n.Evaluation != nil { if target := strings.TrimSpace(n.Evaluation.OnFail.TargetNode); target != "" && keys[target] { edges = append(edges, WorkGraphEdge{From: n.Key, To: target, Kind: "recovery"}) } }
		if n.Approval != nil { if target := strings.TrimSpace(n.Approval.OnReject.TargetNode); target != "" && keys[target] { edges = append(edges, WorkGraphEdge{From: n.Key, To: target, Kind: "recovery"}) } }
	}
	return edges
}

func mapWorkGraphNodeKind(n looptemplate.Node) string { switch n.Type { case looptemplate.NodeTypeEvaluation: return "evaluation"; case looptemplate.NodeTypeApproval: return "approval" }; key, role := strings.ToLower(n.Key), strings.ToLower(n.Role); if strings.Contains(key, "release") || strings.Contains(role, "release") { return "release" }; if strings.Contains(key, "observe") || strings.Contains(role, "observe") { return "observe" }; return "agent" }
func mapWorkGraphStatus(loopState, issueStatus string) string { state := strings.ToLower(strings.TrimSpace(loopState)); switch state { case "blocked", "completed", "failed", "cancelled", "observing", "released", "awaiting_approval", "planning", "running": return state }; switch strings.ToLower(strings.TrimSpace(issueStatus)) { case "done": return "completed"; case "cancelled": return "cancelled"; default: return "running" } }
func evaluationGateState(n LoopNodeRead) string { if n.EvaluationVerdict != "" { return strings.ToLower(n.EvaluationVerdict) }; return n.Status }
func approvalGateState(n LoopNodeRead) string { if n.ApprovalState != "" { return n.ApprovalState }; return n.Status }
func parseStage(raw string) int { v, _ := strconv.Atoi(raw); return v }

package loopservice

import (
	"context"
	"testing"

	"github.com/multica-ai/multica/server/internal/looptemplate"
)

type fakeWorkGraphLoops struct{ detail LoopDetail }
func (f fakeWorkGraphLoops) GetLoop(context.Context, string, string) (LoopDetail, error) { return f.detail, nil }

type fakeWorkGraphTemplates struct{ pinned PinnedLoopTemplate }
func (f fakeWorkGraphTemplates) LoadPinnedLoopTemplate(context.Context, string, string, int) (PinnedLoopTemplate, error) { return f.pinned, nil }

func TestWorkGraphProjectionParallelAndRecovery(t *testing.T) {
	reader := NewMulticaWorkGraphProjectionReader(
		fakeWorkGraphLoops{detail: LoopDetail{LoopSummary: LoopSummary{ParentIssueID: "parent", ProjectID: "project", Title: "Token License", State: "running", TemplateKey: "feature", TemplateVersion: 1}, Nodes: []LoopNodeRead{
			{NodeKey: "architecture", NodeType: "agent", Role: "architect", Stage: 1, Status: "done"},
			{NodeKey: "backend", NodeType: "agent", Role: "backend", Stage: 2, Status: "todo", LatestTaskID: "task-be", LatestTaskStatus: "running"},
			{NodeKey: "frontend", NodeType: "agent", Role: "frontend", Stage: 2, Status: "todo"},
			{NodeKey: "test", NodeType: "evaluation", Role: "test", Stage: 3, Status: "backlog"},
		}},
		fakeWorkGraphTemplates{pinned: PinnedLoopTemplate{Definition: looptemplate.Definition{Nodes: []looptemplate.Node{
			{Key: "architecture", Type: "agent", Stage: 1, Role: "architect"},
			{Key: "backend", Type: "agent", Stage: 2, Role: "backend"},
			{Key: "frontend", Type: "agent", Stage: 2, Role: "frontend"},
			{Key: "test", Type: "evaluation", Stage: 3, Role: "test", Evaluation: &looptemplate.EvaluationConfig{OnFail: looptemplate.EvaluationFailConfig{TargetNode: "backend"}}},
		}}}},
	)
	graph, err := reader.GetWorkGraph(context.Background(), "workspace", "parent")
	if err != nil { t.Fatal(err) }
	if len(graph.Nodes) != 4 { t.Fatalf("nodes=%d", len(graph.Nodes)) }
	if graph.CurrentStage != "2" { t.Fatalf("current stage=%q", graph.CurrentStage) }
	if len(graph.Gates) != 1 || graph.Gates[0].Type != "evaluation" { t.Fatalf("gates=%+v", graph.Gates) }
	assertEdge := func(from, to, kind string) {
		t.Helper()
		for _, edge := range graph.Edges { if edge.From == from && edge.To == to && edge.Kind == kind { return } }
		t.Fatalf("missing edge %s -> %s (%s): %+v", from, to, kind, graph.Edges)
	}
	assertEdge("architecture", "backend", "dependency")
	assertEdge("architecture", "frontend", "dependency")
	assertEdge("backend", "test", "dependency")
	assertEdge("frontend", "test", "dependency")
	assertEdge("test", "backend", "recovery")
}

func TestWorkGraphProjectionCompilesOutMissingOptionalNode(t *testing.T) {
	reader := NewMulticaWorkGraphProjectionReader(
		fakeWorkGraphLoops{detail: LoopDetail{LoopSummary: LoopSummary{ParentIssueID: "parent", Title: "x", State: "running", TemplateKey: "feature", TemplateVersion: 1}, Nodes: []LoopNodeRead{{NodeKey: "backend", Stage: 1, Status: "todo"}}}},
		fakeWorkGraphTemplates{pinned: PinnedLoopTemplate{Definition: looptemplate.Definition{Nodes: []looptemplate.Node{{Key: "embedded", Type: "agent", Stage: 1}, {Key: "backend", Type: "agent", Stage: 1}}}}},
	)
	graph, err := reader.GetWorkGraph(context.Background(), "workspace", "parent")
	if err != nil { t.Fatal(err) }
	if len(graph.Nodes) != 1 || graph.Nodes[0].Key != "backend" { t.Fatalf("nodes=%+v", graph.Nodes) }
}

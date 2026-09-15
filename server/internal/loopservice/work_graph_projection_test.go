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
type fakeWorkGraphArtifacts struct{ items []RegisteredArtifact }
func (f fakeWorkGraphArtifacts) ListArtifacts(context.Context, string, string) ([]RegisteredArtifact, error) { return f.items, nil }

func TestWorkGraphProjectionParallelRecoveryAndProvenance(t *testing.T) {
	reader := NewMulticaWorkGraphProjectionReader(
		fakeWorkGraphLoops{LoopDetail{LoopSummary: LoopSummary{ParentIssueID:"parent", ProjectID:"project", Title:"Token License", State:"running", TemplateKey:"feature", TemplateVersion:1}, Nodes: []LoopNodeRead{
			{IssueID:"arch-id", NodeKey:"architecture", Stage:1, Status:"done"}, {IssueID:"be-id", NodeKey:"backend", Stage:2, Status:"todo", LatestTaskID:"task-be", LatestTaskStatus:"running"}, {IssueID:"fe-id", NodeKey:"frontend", Stage:2, Status:"todo"}, {IssueID:"test-id", NodeKey:"test", Stage:3, Status:"backlog", EvaluationID:"eval", EvaluationVerdict:"fail"},
		}}},
		fakeWorkGraphTemplates{PinnedLoopTemplate{Definition: looptemplate.Definition{Nodes: []looptemplate.Node{
			{Key:"architecture", Type:"agent", Stage:1}, {Key:"backend", Type:"agent", Stage:2}, {Key:"frontend", Type:"agent", Stage:2}, {Key:"test", Type:"evaluation", Stage:3, Evaluation:&looptemplate.EvaluationConfig{OnFail:looptemplate.EvaluationFailConfig{TargetNode:"backend"}}},
		}}}},
		fakeWorkGraphArtifacts{[]RegisteredArtifact{{NodeIssueID:"be-id"},{NodeIssueID:"be-id"},{NodeIssueID:"test-id"}}},
	)
	graph, err := reader.GetWorkGraph(context.Background(), "workspace", "parent"); if err != nil { t.Fatal(err) }
	if graph.CurrentStage != "2" { t.Fatalf("current stage=%q", graph.CurrentStage) }
	if graph.ProvenanceSummary.ArtifactCount != 3 || graph.ProvenanceSummary.EvaluationCount != 1 { t.Fatalf("provenance=%+v", graph.ProvenanceSummary) }
	for _, node := range graph.Nodes { if node.Key == "backend" && node.ArtifactCount != 2 { t.Fatalf("backend artifacts=%d", node.ArtifactCount) } }
	assertEdge := func(from,to,kind string){ t.Helper(); for _,e:=range graph.Edges { if e.From==from&&e.To==to&&e.Kind==kind{return} }; t.Fatalf("missing edge %s -> %s (%s)",from,to,kind) }
	assertEdge("architecture","backend","dependency"); assertEdge("architecture","frontend","dependency"); assertEdge("backend","test","dependency"); assertEdge("frontend","test","dependency"); assertEdge("test","backend","recovery")
}

func TestWorkGraphProjectionCompilesOutMissingOptionalNode(t *testing.T) {
	reader := NewMulticaWorkGraphProjectionReader(fakeWorkGraphLoops{LoopDetail{LoopSummary:LoopSummary{ParentIssueID:"parent",Title:"x",State:"running",TemplateKey:"feature",TemplateVersion:1},Nodes:[]LoopNodeRead{{NodeKey:"backend",Stage:1,Status:"todo"}}}}, fakeWorkGraphTemplates{PinnedLoopTemplate{Definition:looptemplate.Definition{Nodes:[]looptemplate.Node{{Key:"embedded",Type:"agent",Stage:1},{Key:"backend",Type:"agent",Stage:1}}}}})
	graph,err:=reader.GetWorkGraph(context.Background(),"workspace","parent"); if err!=nil{t.Fatal(err)}; if len(graph.Nodes)!=1||graph.Nodes[0].Key!="backend"{t.Fatalf("nodes=%+v",graph.Nodes)}
}

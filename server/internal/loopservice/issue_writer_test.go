package loopservice

import (
	"context"
	"errors"
	"testing"

	"github.com/multica-ai/multica/server/internal/looptemplate"
)

type fakeIssueGraphGateway struct {
	calls  int
	last   CreateLoopIssueGraphCommand
	result CreatedIssueGraph
	err    error
}

func (f *fakeIssueGraphGateway) CreateLoopIssueGraph(_ context.Context, cmd CreateLoopIssueGraphCommand) (CreatedIssueGraph, error) {
	f.calls++
	f.last = cmd
	return f.result, f.err
}

func basePersistRequest() PersistLoopInstanceRequest {
	return PersistLoopInstanceRequest{
		WorkspaceID: "workspace-1", ProjectID: "project-1", InstanceKey: "feature-123",
		Title: "Feature", CreatorType: "member", CreatorID: "user-1",
		TemplateKey: "feature-development", TemplateVersion: 1,
	}
}

func TestBuildIssueGraphCommandMapsParentAndChildren(t *testing.T) {
	stage10 := 10
	stage20 := 20
	req := basePersistRequest()
	req.Title = " Add token licensing "
	req.Description = "Support Docker/Kubernetes workers."
	req.TemplateVersion = 3
	req.PolicyVersion = "policy-v2"
	req.Plan = looptemplate.CompiledPlan{
		TemplateKey: "feature-development", InitialStage: 10,
		IncludedNodes: []looptemplate.CompiledNode{
			{Key: "backend", Type: looptemplate.NodeTypeAgent, Stage: stage10, Title: "Backend", Description: "Implement token license API", Role: "backend", Binding: &looptemplate.RoleBinding{Type: "agent", ID: "agent-1"}, Required: true, MaxRetries: 2, InitialStatus: "todo"},
			{Key: "approval", Type: looptemplate.NodeTypeApproval, Stage: stage20, Title: "Release approval", Required: true, InitialStatus: "backlog"},
		},
	}
	cmd, err := BuildIssueGraphCommand(req)
	if err != nil {
		t.Fatalf("BuildIssueGraphCommand() error = %v", err)
	}
	if cmd.Parent.Title != "Add token licensing" {
		t.Fatalf("parent title = %q", cmd.Parent.Title)
	}
	if cmd.Parent.CreatorType != "member" || cmd.Parent.CreatorID != "user-1" {
		t.Fatalf("parent creator = %s:%s", cmd.Parent.CreatorType, cmd.Parent.CreatorID)
	}
	if cmd.Parent.Status != "backlog" {
		t.Fatalf("parent status = %q, want backlog", cmd.Parent.Status)
	}
	if got := cmd.Parent.Metadata["manifold.loop.template_version"]; got != 3 {
		t.Fatalf("template version metadata = %#v, want 3", got)
	}
	if got := cmd.Parent.Metadata["manifold.loop.state"]; got != "running" {
		t.Fatalf("loop state metadata = %#v, want running", got)
	}
	if len(cmd.Children) != 2 {
		t.Fatalf("children = %d, want 2", len(cmd.Children))
	}

	backend := cmd.Children[0]
	if backend.NodeKey != "backend" || backend.Status != "todo" {
		t.Fatalf("backend = %#v", backend)
	}
	if backend.CreatorType != "member" || backend.CreatorID != "user-1" {
		t.Fatalf("backend creator = %s:%s", backend.CreatorType, backend.CreatorID)
	}
	if backend.Stage == nil || *backend.Stage != 10 {
		t.Fatalf("backend stage = %#v, want 10", backend.Stage)
	}
	if backend.AssigneeType != "agent" || backend.AssigneeID != "agent-1" {
		t.Fatalf("backend assignee = %s:%s", backend.AssigneeType, backend.AssigneeID)
	}
	if backend.Metadata["manifold.loop.role"] != "backend" {
		t.Fatalf("backend role metadata = %#v", backend.Metadata["manifold.loop.role"])
	}

	approval := cmd.Children[1]
	if approval.Status != "backlog" {
		t.Fatalf("approval status = %q, want backlog", approval.Status)
	}
	if approval.AssigneeType != "" || approval.AssigneeID != "" {
		t.Fatalf("approval must not be assigned as an approval decision: %#v", approval)
	}
}

func TestBuildIssueGraphCommandRejectsExecutableNodeWithoutBinding(t *testing.T) {
	req := basePersistRequest()
	req.Plan = looptemplate.CompiledPlan{IncludedNodes: []looptemplate.CompiledNode{{Key: "backend", Type: looptemplate.NodeTypeAgent, Stage: 10, Title: "Backend", InitialStatus: "todo"}}}
	_, err := BuildIssueGraphCommand(req)
	if err == nil {
		t.Fatal("BuildIssueGraphCommand() error = nil, want missing binding error")
	}
}

func TestBuildIssueGraphCommandRejectsDuplicateNodeKeys(t *testing.T) {
	binding := &looptemplate.RoleBinding{Type: "agent", ID: "agent-1"}
	req := basePersistRequest()
	req.Plan = looptemplate.CompiledPlan{IncludedNodes: []looptemplate.CompiledNode{
		{Key: "backend", Type: looptemplate.NodeTypeAgent, Stage: 10, Title: "Backend A", InitialStatus: "todo", Binding: binding},
		{Key: "backend", Type: looptemplate.NodeTypeAgent, Stage: 20, Title: "Backend B", InitialStatus: "backlog", Binding: binding},
	}}
	_, err := BuildIssueGraphCommand(req)
	if err == nil {
		t.Fatal("BuildIssueGraphCommand() error = nil, want duplicate node key error")
	}
}

func TestBuildIssueGraphCommandRejectsMissingCreator(t *testing.T) {
	req := basePersistRequest()
	req.CreatorID = ""
	req.Plan = looptemplate.CompiledPlan{IncludedNodes: []looptemplate.CompiledNode{{Key: "approval", Type: looptemplate.NodeTypeApproval, Stage: 10, Title: "Approve", InitialStatus: "todo"}}}
	_, err := BuildIssueGraphCommand(req)
	if err == nil {
		t.Fatal("BuildIssueGraphCommand() error = nil, want creator error")
	}
}

func TestMulticaIssueWriterDelegatesToGateway(t *testing.T) {
	gateway := &fakeIssueGraphGateway{result: CreatedIssueGraph{ParentIssueID: "parent-1", NodeIssueIDs: map[string]string{"approval": "child-1"}}}
	writer := NewMulticaIssueWriter(gateway)
	req := basePersistRequest()
	req.Plan = looptemplate.CompiledPlan{IncludedNodes: []looptemplate.CompiledNode{{Key: "approval", Type: looptemplate.NodeTypeApproval, Stage: 10, Title: "Approve", Required: true, InitialStatus: "todo"}}}

	result, err := writer.PersistLoopInstance(context.Background(), req)
	if err != nil {
		t.Fatalf("PersistLoopInstance() error = %v", err)
	}
	if gateway.calls != 1 {
		t.Fatalf("gateway calls = %d, want 1", gateway.calls)
	}
	if result.ParentIssueID != "parent-1" || result.NodeIssueIDs["approval"] != "child-1" {
		t.Fatalf("result = %#v", result)
	}
	gateway.result.NodeIssueIDs["approval"] = "mutated"
	if result.NodeIssueIDs["approval"] != "child-1" {
		t.Fatalf("result map aliases gateway map: %#v", result.NodeIssueIDs)
	}
}

func TestMulticaIssueWriterPropagatesIdempotencyConflict(t *testing.T) {
	gateway := &fakeIssueGraphGateway{err: ErrLoopInstanceExists}
	writer := NewMulticaIssueWriter(gateway)
	req := basePersistRequest()
	req.InstanceKey = "feature-race"
	req.Plan = looptemplate.CompiledPlan{IncludedNodes: []looptemplate.CompiledNode{{Key: "approval", Type: looptemplate.NodeTypeApproval, Stage: 10, Title: "Approve", InitialStatus: "todo"}}}
	_, err := writer.PersistLoopInstance(context.Background(), req)
	if !errors.Is(err, ErrLoopInstanceExists) {
		t.Fatalf("PersistLoopInstance() error = %v, want ErrLoopInstanceExists", err)
	}
}

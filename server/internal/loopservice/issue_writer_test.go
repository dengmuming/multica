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

func TestBuildIssueGraphCommandMapsParentAndChildren(t *testing.T) {
	stage10 := 10
	stage20 := 20
	cmd, err := BuildIssueGraphCommand(PersistLoopInstanceRequest{
		WorkspaceID:     "workspace-1",
		ProjectID:       "project-1",
		InstanceKey:     "feature-123",
		Title:           " Add token licensing ",
		Description:     "Support Docker/Kubernetes workers.",
		TemplateKey:     "feature-development",
		TemplateVersion: 3,
		PolicyVersion:   "policy-v2",
		Plan: looptemplate.CompiledPlan{
			TemplateKey:  "feature-development",
			InitialStage: 10,
			IncludedNodes: []looptemplate.CompiledNode{
				{
					Key:           "backend",
					Type:          looptemplate.NodeTypeAgent,
					Stage:         stage10,
					Title:         "Backend",
					Description:   "Implement token license API",
					Role:          "backend",
					Binding:       &looptemplate.RoleBinding{Type: "agent", ID: "agent-1"},
					Required:      true,
					MaxRetries:    2,
					InitialStatus: "todo",
				},
				{
					Key:           "approval",
					Type:          looptemplate.NodeTypeApproval,
					Stage:         stage20,
					Title:         "Release approval",
					Required:      true,
					InitialStatus: "backlog",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildIssueGraphCommand() error = %v", err)
	}
	if cmd.Parent.Title != "Add token licensing" {
		t.Fatalf("parent title = %q", cmd.Parent.Title)
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
	_, err := BuildIssueGraphCommand(PersistLoopInstanceRequest{
		WorkspaceID:     "workspace-1",
		ProjectID:       "project-1",
		InstanceKey:     "feature-123",
		Title:           "Feature",
		TemplateKey:     "feature-development",
		TemplateVersion: 1,
		Plan: looptemplate.CompiledPlan{
			IncludedNodes: []looptemplate.CompiledNode{
				{
					Key:           "backend",
					Type:          looptemplate.NodeTypeAgent,
					Stage:         10,
					Title:         "Backend",
					InitialStatus: "todo",
				},
			},
		},
	})
	if err == nil {
		t.Fatal("BuildIssueGraphCommand() error = nil, want missing binding error")
	}
}

func TestBuildIssueGraphCommandRejectsDuplicateNodeKeys(t *testing.T) {
	binding := &looptemplate.RoleBinding{Type: "agent", ID: "agent-1"}
	_, err := BuildIssueGraphCommand(PersistLoopInstanceRequest{
		WorkspaceID:     "workspace-1",
		ProjectID:       "project-1",
		InstanceKey:     "feature-123",
		Title:           "Feature",
		TemplateKey:     "feature-development",
		TemplateVersion: 1,
		Plan: looptemplate.CompiledPlan{
			IncludedNodes: []looptemplate.CompiledNode{
				{Key: "backend", Type: looptemplate.NodeTypeAgent, Stage: 10, Title: "Backend A", InitialStatus: "todo", Binding: binding},
				{Key: "backend", Type: looptemplate.NodeTypeAgent, Stage: 20, Title: "Backend B", InitialStatus: "backlog", Binding: binding},
			},
		},
	})
	if err == nil {
		t.Fatal("BuildIssueGraphCommand() error = nil, want duplicate node key error")
	}
}

func TestMulticaIssueWriterDelegatesToGateway(t *testing.T) {
	gateway := &fakeIssueGraphGateway{result: CreatedIssueGraph{
		ParentIssueID: "parent-1",
		NodeIssueIDs:  map[string]string{"approval": "child-1"},
	}}
	writer := NewMulticaIssueWriter(gateway)

	result, err := writer.PersistLoopInstance(context.Background(), PersistLoopInstanceRequest{
		WorkspaceID:     "workspace-1",
		ProjectID:       "project-1",
		InstanceKey:     "feature-123",
		Title:           "Feature",
		TemplateKey:     "feature-development",
		TemplateVersion: 1,
		Plan: looptemplate.CompiledPlan{
			IncludedNodes: []looptemplate.CompiledNode{
				{Key: "approval", Type: looptemplate.NodeTypeApproval, Stage: 10, Title: "Approve", Required: true, InitialStatus: "todo"},
			},
		},
	})
	if err != nil {
		t.Fatalf("PersistLoopInstance() error = %v", err)
	}
	if gateway.calls != 1 {
		t.Fatalf("gateway calls = %d, want 1", gateway.calls)
	}
	if result.ParentIssueID != "parent-1" || result.NodeIssueIDs["approval"] != "child-1" {
		t.Fatalf("result = %#v", result)
	}

	// Writer must not expose a mutable alias to an adapter-owned map.
	gateway.result.NodeIssueIDs["approval"] = "mutated"
	if result.NodeIssueIDs["approval"] != "child-1" {
		t.Fatalf("result map aliases gateway map: %#v", result.NodeIssueIDs)
	}
}

func TestMulticaIssueWriterPropagatesIdempotencyConflict(t *testing.T) {
	gateway := &fakeIssueGraphGateway{err: ErrLoopInstanceExists}
	writer := NewMulticaIssueWriter(gateway)
	_, err := writer.PersistLoopInstance(context.Background(), PersistLoopInstanceRequest{
		WorkspaceID:     "workspace-1",
		ProjectID:       "project-1",
		InstanceKey:     "feature-race",
		Title:           "Feature",
		TemplateKey:     "feature-development",
		TemplateVersion: 1,
		Plan: looptemplate.CompiledPlan{
			IncludedNodes: []looptemplate.CompiledNode{
				{Key: "approval", Type: looptemplate.NodeTypeApproval, Stage: 10, Title: "Approve", InitialStatus: "todo"},
			},
		},
	})
	if !errors.Is(err, ErrLoopInstanceExists) {
		t.Fatalf("PersistLoopInstance() error = %v, want ErrLoopInstanceExists", err)
	}
}

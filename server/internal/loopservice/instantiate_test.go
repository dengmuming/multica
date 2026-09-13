package loopservice

import (
	"context"
	"errors"
	"testing"

	"github.com/multica-ai/multica/server/internal/looptemplate"
)

type fakeTemplateReader struct {
	template PublishedTemplate
	err      error
}

func (f fakeTemplateReader) LoadPublishedTemplate(context.Context, string, string, int) (PublishedTemplate, error) {
	return f.template, f.err
}

type fakeBindingScopeValidator struct {
	calls []string
	err   error
}

func (f *fakeBindingScopeValidator) ValidateBindingScope(_ context.Context, _ string, role string, _ looptemplate.RoleBinding) error {
	f.calls = append(f.calls, role)
	return f.err
}

type fakeInstanceLookup struct {
	exists bool
	err    error
}

func (f fakeInstanceLookup) LoopInstanceExists(context.Context, string, string, string) (bool, error) {
	return f.exists, f.err
}

func TestInstantiateServicePreflightCompilesPlan(t *testing.T) {
	def := validDefinition()
	bindingValidator := &fakeBindingScopeValidator{}
	service := NewInstantiateService(
		fakeTemplateReader{template: PublishedTemplate{
			Key:           def.Key,
			Version:       3,
			PolicyVersion: "p1",
			Definition:    def,
		}},
		bindingValidator,
		fakeInstanceLookup{},
	)

	bindings := looptemplate.RoleBindings{
		"backend": {Type: "agent", ID: "agent-1"},
	}
	result, err := service.Preflight(context.Background(), InstantiatePreflightRequest{
		WorkspaceID: "workspace-1",
		ProjectID:   "project-1",
		TemplateKey: def.Key,
		Version:     3,
		InstanceKey: "feature-123",
		Bindings:    bindings,
	})
	if err != nil {
		t.Fatalf("Preflight() error = %v", err)
	}

	if len(result.CompiledPlan.IncludedNodes) != 2 {
		t.Fatalf("IncludedNodes = %d, want 2", len(result.CompiledPlan.IncludedNodes))
	}
	if result.CompiledPlan.InitialStage != 10 {
		t.Fatalf("InitialStage = %d, want 10", result.CompiledPlan.InitialStage)
	}
	if result.CompiledPlan.IncludedNodes[0].InitialStatus != "todo" {
		t.Fatalf("first node status = %q, want todo", result.CompiledPlan.IncludedNodes[0].InitialStatus)
	}
	if result.CompiledPlan.IncludedNodes[1].InitialStatus != "backlog" {
		t.Fatalf("second node status = %q, want backlog", result.CompiledPlan.IncludedNodes[1].InitialStatus)
	}
	if len(bindingValidator.calls) != 1 || bindingValidator.calls[0] != "backend" {
		t.Fatalf("binding validation calls = %#v, want [backend]", bindingValidator.calls)
	}

	// The service returns a defensive copy so request-owned maps cannot mutate
	// the plan inputs after preflight.
	bindings["backend"] = looptemplate.RoleBinding{Type: "agent", ID: "mutated"}
	if got := result.RoleBindings["backend"].ID; got != "agent-1" {
		t.Fatalf("RoleBindings backend ID = %q, want agent-1", got)
	}
}

func TestInstantiateServicePreflightRejectsDuplicateInstance(t *testing.T) {
	def := validDefinition()
	service := NewInstantiateService(
		fakeTemplateReader{template: PublishedTemplate{Key: def.Key, Version: 1, Definition: def}},
		&fakeBindingScopeValidator{},
		fakeInstanceLookup{exists: true},
	)

	_, err := service.Preflight(context.Background(), InstantiatePreflightRequest{
		WorkspaceID: "workspace-1",
		ProjectID:   "project-1",
		TemplateKey: def.Key,
		Version:     1,
		InstanceKey: "feature-123",
		Bindings: looptemplate.RoleBindings{
			"backend": {Type: "agent", ID: "agent-1"},
		},
	})
	if !errors.Is(err, ErrLoopInstanceExists) {
		t.Fatalf("Preflight() error = %v, want ErrLoopInstanceExists", err)
	}
}

func TestInstantiateServicePreflightRejectsOutOfScopeBinding(t *testing.T) {
	def := validDefinition()
	wantErr := errors.New("agent belongs to another workspace")
	service := NewInstantiateService(
		fakeTemplateReader{template: PublishedTemplate{Key: def.Key, Version: 1, Definition: def}},
		&fakeBindingScopeValidator{err: wantErr},
		fakeInstanceLookup{},
	)

	_, err := service.Preflight(context.Background(), InstantiatePreflightRequest{
		WorkspaceID: "workspace-1",
		ProjectID:   "project-1",
		TemplateKey: def.Key,
		Version:     1,
		InstanceKey: "feature-123",
		Bindings: looptemplate.RoleBindings{
			"backend": {Type: "agent", ID: "agent-1"},
		},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Preflight() error = %v, want wrapped scope error", err)
	}
}

func TestInstantiateServicePreflightRejectsTemplateMismatch(t *testing.T) {
	def := validDefinition()
	service := NewInstantiateService(
		fakeTemplateReader{template: PublishedTemplate{Key: def.Key, Version: 2, Definition: def}},
		&fakeBindingScopeValidator{},
		fakeInstanceLookup{},
	)

	_, err := service.Preflight(context.Background(), InstantiatePreflightRequest{
		WorkspaceID: "workspace-1",
		ProjectID:   "project-1",
		TemplateKey: def.Key,
		Version:     1,
		InstanceKey: "feature-123",
		Bindings: looptemplate.RoleBindings{
			"backend": {Type: "agent", ID: "agent-1"},
		},
	})
	if !errors.Is(err, ErrTemplateMismatch) {
		t.Fatalf("Preflight() error = %v, want ErrTemplateMismatch", err)
	}
}

func validDefinition() looptemplate.Definition {
	return looptemplate.Definition{
		SchemaVersion: 1,
		Key:           "feature-development",
		Name:          "Feature Development",
		Roles: map[string]looptemplate.RoleDefinition{
			"backend": {
				Required:             true,
				AllowedAssigneeTypes: []string{"agent", "squad"},
			},
		},
		Nodes: []looptemplate.Node{
			{
				Key:   "backend",
				Type:  looptemplate.NodeTypeAgent,
				Stage: 10,
				Role:  "backend",
				Title: "Backend implementation",
			},
			{
				Key:   "approval",
				Type:  looptemplate.NodeTypeApproval,
				Stage: 20,
				Title: "Human approval",
				Approval: &looptemplate.ApprovalConfig{
					RequiredRole: "member",
					MinApprovals: 1,
					OnReject: looptemplate.ApprovalRejectConfig{
						TargetNode:         "backend",
						MaxWorkflowRetries: 1,
					},
				},
			},
		},
	}
}

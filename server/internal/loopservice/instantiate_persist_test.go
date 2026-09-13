package loopservice

import (
	"context"
	"errors"
	"testing"

	"github.com/multica-ai/multica/server/internal/looptemplate"
)

type fakeLoopInstanceWriter struct {
	calls  int
	last   PersistLoopInstanceRequest
	result PersistedLoopInstance
	err    error
}

func (f *fakeLoopInstanceWriter) PersistLoopInstance(_ context.Context, req PersistLoopInstanceRequest) (PersistedLoopInstance, error) {
	f.calls++
	f.last = req
	return f.result, f.err
}

func TestInstantiatorPersistsCompiledPlan(t *testing.T) {
	def := validDefinition()
	preflight := NewInstantiateService(
		fakeTemplateReader{template: PublishedTemplate{
			Key:           def.Key,
			Version:       4,
			PolicyVersion: "policy-v1",
			Definition:    def,
		}},
		&fakeBindingScopeValidator{},
		fakeInstanceLookup{},
	)
	writer := &fakeLoopInstanceWriter{result: PersistedLoopInstance{
		ParentIssueID: "parent-issue",
		NodeIssueIDs: map[string]string{
			"backend":  "backend-issue",
			"approval": "approval-issue",
		},
	}}
	service := NewInstantiator(preflight, writer)

	result, err := service.Instantiate(context.Background(), InstantiateRequest{
		WorkspaceID: "workspace-1",
		ProjectID:   "project-1",
		TemplateKey: def.Key,
		Version:     4,
		InstanceKey: "feature-456",
		Title:       "Add token licensing",
		Description: "Support dynamic Docker/Kubernetes workers.",
		Bindings: looptemplate.RoleBindings{
			"backend": {Type: "agent", ID: "agent-1"},
		},
	})
	if err != nil {
		t.Fatalf("Instantiate() error = %v", err)
	}
	if writer.calls != 1 {
		t.Fatalf("writer calls = %d, want 1", writer.calls)
	}
	if writer.last.TemplateKey != def.Key || writer.last.TemplateVersion != 4 {
		t.Fatalf("persisted template = %s@%d, want %s@4", writer.last.TemplateKey, writer.last.TemplateVersion, def.Key)
	}
	if writer.last.PolicyVersion != "policy-v1" {
		t.Fatalf("PolicyVersion = %q, want policy-v1", writer.last.PolicyVersion)
	}
	if len(writer.last.Plan.IncludedNodes) != 2 {
		t.Fatalf("persisted plan nodes = %d, want 2", len(writer.last.Plan.IncludedNodes))
	}
	if result.Instance.ParentIssueID != "parent-issue" {
		t.Fatalf("ParentIssueID = %q, want parent-issue", result.Instance.ParentIssueID)
	}
}

func TestInstantiatorDoesNotWriteWhenPreflightFails(t *testing.T) {
	def := validDefinition()
	preflight := NewInstantiateService(
		fakeTemplateReader{template: PublishedTemplate{Key: def.Key, Version: 1, Definition: def}},
		&fakeBindingScopeValidator{},
		fakeInstanceLookup{exists: true},
	)
	writer := &fakeLoopInstanceWriter{}
	service := NewInstantiator(preflight, writer)

	_, err := service.Instantiate(context.Background(), InstantiateRequest{
		WorkspaceID: "workspace-1",
		ProjectID:   "project-1",
		TemplateKey: def.Key,
		Version:     1,
		InstanceKey: "feature-duplicate",
		Title:       "Duplicate",
		Bindings: looptemplate.RoleBindings{
			"backend": {Type: "agent", ID: "agent-1"},
		},
	})
	if !errors.Is(err, ErrLoopInstanceExists) {
		t.Fatalf("Instantiate() error = %v, want ErrLoopInstanceExists", err)
	}
	if writer.calls != 0 {
		t.Fatalf("writer calls = %d, want 0", writer.calls)
	}
}

func TestInstantiatorPropagatesWriteConflict(t *testing.T) {
	def := validDefinition()
	preflight := NewInstantiateService(
		fakeTemplateReader{template: PublishedTemplate{Key: def.Key, Version: 1, Definition: def}},
		&fakeBindingScopeValidator{},
		fakeInstanceLookup{},
	)
	writer := &fakeLoopInstanceWriter{err: ErrLoopInstanceExists}
	service := NewInstantiator(preflight, writer)

	_, err := service.Instantiate(context.Background(), InstantiateRequest{
		WorkspaceID: "workspace-1",
		ProjectID:   "project-1",
		TemplateKey: def.Key,
		Version:     1,
		InstanceKey: "feature-race",
		Title:       "Concurrent request",
		Bindings: looptemplate.RoleBindings{
			"backend": {Type: "agent", ID: "agent-1"},
		},
	})
	if !errors.Is(err, ErrLoopInstanceExists) {
		t.Fatalf("Instantiate() error = %v, want wrapped ErrLoopInstanceExists", err)
	}
}

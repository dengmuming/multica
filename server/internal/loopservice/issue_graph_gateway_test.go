package loopservice

import (
	"context"
	"errors"
	"testing"
)

type fakeCanonicalIssueGraphStore struct {
	calls  int
	last   CreateLoopIssueGraphCommand
	result CreatedIssueGraph
	err    error
}

func (f *fakeCanonicalIssueGraphStore) CreateGraphAtomically(_ context.Context, cmd CreateLoopIssueGraphCommand) (CreatedIssueGraph, error) {
	f.calls++
	f.last = cmd
	return f.result, f.err
}

type fakeInitialIssueDispatcher struct {
	calls []InitialDispatchRequest
	fail  map[string]error
}

func (f *fakeInitialIssueDispatcher) DispatchAssignedIssue(_ context.Context, req InitialDispatchRequest) error {
	f.calls = append(f.calls, req)
	if f.fail == nil {
		return nil
	}
	return f.fail[req.NodeKey]
}

func TestDurableIssueGraphGatewayPersistsBeforeDispatch(t *testing.T) {
	store := &fakeCanonicalIssueGraphStore{result: CreatedIssueGraph{
		ParentIssueID: "parent-1",
		NodeIssueIDs: map[string]string{
			"product": "issue-product",
			"backend": "issue-backend",
			"approval": "issue-approval",
		},
	}}
	dispatcher := &fakeInitialIssueDispatcher{}
	gateway := NewDurableIssueGraphGateway(store, dispatcher)

	cmd := CreateLoopIssueGraphCommand{
		WorkspaceID: "workspace-1",
		ProjectID:   "project-1",
		InstanceKey: "feature-1",
		Parent: LoopIssueCommand{Title: "Feature", Status: "backlog"},
		Children: []LoopIssueCommand{
			{NodeKey: "product", Status: "todo", AssigneeType: "agent", AssigneeID: "agent-product"},
			{NodeKey: "backend", Status: "backlog", AssigneeType: "agent", AssigneeID: "agent-backend"},
			{NodeKey: "approval", Status: "backlog"},
		},
	}

	created, err := gateway.CreateLoopIssueGraph(context.Background(), cmd)
	if err != nil {
		t.Fatalf("CreateLoopIssueGraph() error = %v", err)
	}
	if store.calls != 1 {
		t.Fatalf("store calls = %d, want 1", store.calls)
	}
	if created.ParentIssueID != "parent-1" {
		t.Fatalf("parent id = %q, want parent-1", created.ParentIssueID)
	}
	if len(dispatcher.calls) != 1 {
		t.Fatalf("dispatch calls = %d, want 1", len(dispatcher.calls))
	}
	if dispatcher.calls[0].NodeKey != "product" || dispatcher.calls[0].IssueID != "issue-product" {
		t.Fatalf("dispatch request = %#v", dispatcher.calls[0])
	}
	if dispatcher.calls[0].Reason != "manifold_loop_initial_stage" {
		t.Fatalf("dispatch reason = %q", dispatcher.calls[0].Reason)
	}
}

func TestDurableIssueGraphGatewayDispatchesParallelInitialAgents(t *testing.T) {
	store := &fakeCanonicalIssueGraphStore{result: CreatedIssueGraph{
		ParentIssueID: "parent-1",
		NodeIssueIDs: map[string]string{
			"backend":  "issue-backend",
			"frontend": "issue-frontend",
			"human":    "issue-human",
		},
	}}
	dispatcher := &fakeInitialIssueDispatcher{}
	gateway := NewDurableIssueGraphGateway(store, dispatcher)

	_, err := gateway.CreateLoopIssueGraph(context.Background(), CreateLoopIssueGraphCommand{
		WorkspaceID: "workspace-1",
		InstanceKey: "feature-1",
		Parent:      LoopIssueCommand{Title: "Feature"},
		Children: []LoopIssueCommand{
			{NodeKey: "backend", Status: "todo", AssigneeType: "agent", AssigneeID: "agent-backend"},
			{NodeKey: "frontend", Status: "todo", AssigneeType: "squad", AssigneeID: "squad-frontend"},
			{NodeKey: "human", Status: "todo", AssigneeType: "member", AssigneeID: "member-1"},
		},
	})
	if err != nil {
		t.Fatalf("CreateLoopIssueGraph() error = %v", err)
	}
	if len(dispatcher.calls) != 2 {
		t.Fatalf("dispatch calls = %d, want 2", len(dispatcher.calls))
	}
	if dispatcher.calls[0].NodeKey != "backend" || dispatcher.calls[1].NodeKey != "frontend" {
		t.Fatalf("dispatch order = %#v", dispatcher.calls)
	}
}

func TestDurableIssueGraphGatewayDoesNotDispatchWhenPersistenceFails(t *testing.T) {
	persistErr := errors.New("db unavailable")
	store := &fakeCanonicalIssueGraphStore{err: persistErr}
	dispatcher := &fakeInitialIssueDispatcher{}
	gateway := NewDurableIssueGraphGateway(store, dispatcher)

	_, err := gateway.CreateLoopIssueGraph(context.Background(), CreateLoopIssueGraphCommand{
		WorkspaceID: "workspace-1",
		InstanceKey: "feature-1",
		Parent:      LoopIssueCommand{Title: "Feature"},
		Children: []LoopIssueCommand{
			{NodeKey: "backend", Status: "todo", AssigneeType: "agent", AssigneeID: "agent-1"},
		},
	})
	if !errors.Is(err, persistErr) {
		t.Fatalf("error = %v, want persist error", err)
	}
	if len(dispatcher.calls) != 0 {
		t.Fatalf("dispatch calls = %d, want 0", len(dispatcher.calls))
	}
}

func TestDurableIssueGraphGatewayReturnsRecoverableDispatchError(t *testing.T) {
	dispatchErr := errors.New("runtime temporarily unavailable")
	store := &fakeCanonicalIssueGraphStore{result: CreatedIssueGraph{
		ParentIssueID: "parent-1",
		NodeIssueIDs: map[string]string{
			"backend":  "issue-backend",
			"frontend": "issue-frontend",
		},
	}}
	dispatcher := &fakeInitialIssueDispatcher{fail: map[string]error{"backend": dispatchErr}}
	gateway := NewDurableIssueGraphGateway(store, dispatcher)

	created, err := gateway.CreateLoopIssueGraph(context.Background(), CreateLoopIssueGraphCommand{
		WorkspaceID: "workspace-1",
		InstanceKey: "feature-1",
		Parent:      LoopIssueCommand{Title: "Feature"},
		Children: []LoopIssueCommand{
			{NodeKey: "backend", Status: "todo", AssigneeType: "agent", AssigneeID: "agent-backend"},
			{NodeKey: "frontend", Status: "todo", AssigneeType: "agent", AssigneeID: "agent-frontend"},
		},
	})
	if created.ParentIssueID != "parent-1" {
		t.Fatalf("created parent id = %q", created.ParentIssueID)
	}
	var dispatchFailure *InitialDispatchError
	if !errors.As(err, &dispatchFailure) {
		t.Fatalf("error = %v, want InitialDispatchError", err)
	}
	if !errors.Is(err, dispatchErr) {
		t.Fatalf("error = %v, want wrapped dispatch error", err)
	}
	if len(dispatchFailure.Failures) != 1 || dispatchFailure.Failures[0].NodeKey != "backend" {
		t.Fatalf("failures = %#v", dispatchFailure.Failures)
	}
	if len(dispatcher.calls) != 2 {
		t.Fatalf("dispatcher should continue independent initial nodes, calls = %d", len(dispatcher.calls))
	}
}

func TestDurableIssueGraphGatewayValidatesReturnedChildIdsBeforeDispatch(t *testing.T) {
	store := &fakeCanonicalIssueGraphStore{result: CreatedIssueGraph{
		ParentIssueID: "parent-1",
		NodeIssueIDs:  map[string]string{},
	}}
	dispatcher := &fakeInitialIssueDispatcher{}
	gateway := NewDurableIssueGraphGateway(store, dispatcher)

	_, err := gateway.CreateLoopIssueGraph(context.Background(), CreateLoopIssueGraphCommand{
		WorkspaceID: "workspace-1",
		InstanceKey: "feature-1",
		Parent:      LoopIssueCommand{Title: "Feature"},
		Children: []LoopIssueCommand{
			{NodeKey: "backend", Status: "todo", AssigneeType: "agent", AssigneeID: "agent-1"},
		},
	})
	if err == nil {
		t.Fatal("error = nil, want incomplete graph validation error")
	}
	if len(dispatcher.calls) != 0 {
		t.Fatalf("dispatch calls = %d, want 0", len(dispatcher.calls))
	}
}

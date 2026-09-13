package loopservice

import (
	"context"
	"errors"
	"testing"

	"github.com/multica-ai/multica/server/internal/looppolicy"
)

type fakePolicyMutationStore struct {
	calls  int
	last   PolicyMutationBatch
	commit PolicyMutationCommit
	err    error
}

func (f *fakePolicyMutationStore) ApplyMutationBatchAtomically(_ context.Context, batch PolicyMutationBatch) (PolicyMutationCommit, error) {
	f.calls++
	f.last = batch
	return f.commit, f.err
}

type fakeInitialDispatcher struct {
	calls int
	last  []InitialDispatchRequest
	err   error
}

func (f *fakeInitialDispatcher) DispatchAssignedIssue(_ context.Context, req InitialDispatchRequest) error {
	f.calls++
	f.last = append(f.last, req)
	return f.err
}

func policyCommand(actions ...looppolicy.Action) ApplyPolicyDecisionCommand {
	return ApplyPolicyDecisionCommand{
		WorkspaceID: "workspace-1", ParentIssueID: "parent-1", PolicyVersion: "policy-v1",
		NodeIssueIDs: map[string]string{"backend": "issue-backend", "test": "issue-test", "approval": "issue-approval"},
		Actions: actions, Reason: "test_policy",
	}
}

func TestBuildPolicyMutationBatchMapsRecoveryActions(t *testing.T) {
	batch, err := BuildPolicyMutationBatch(policyCommand(
		looppolicy.Action{Type: looppolicy.ActionReopenNode, NodeKey: "backend", Reason: "test_failed"},
		looppolicy.Action{Type: looppolicy.ActionParkNode, NodeKey: "test", Reason: "test_failed"},
		looppolicy.Action{Type: looppolicy.ActionSetParentState, ParentState: "running", Reason: "test_failed"},
	))
	if err != nil {
		t.Fatalf("BuildPolicyMutationBatch() error = %v", err)
	}
	if len(batch.Mutations) != 3 {
		t.Fatalf("mutations = %d, want 3", len(batch.Mutations))
	}
	if batch.Mutations[0].Type != MutationReopenNode || batch.Mutations[0].IssueID != "issue-backend" || !batch.Mutations[0].IncrementRetry {
		t.Fatalf("reopen mutation = %#v", batch.Mutations[0])
	}
	if batch.Mutations[1].Type != MutationParkNode || batch.Mutations[1].IssueID != "issue-test" {
		t.Fatalf("park mutation = %#v", batch.Mutations[1])
	}
	if batch.Mutations[2].Type != MutationSetParentState || batch.Mutations[2].ParentState != "running" {
		t.Fatalf("parent mutation = %#v", batch.Mutations[2])
	}
}

func TestBuildPolicyMutationBatchRejectsMissingNodeIssue(t *testing.T) {
	_, err := BuildPolicyMutationBatch(ApplyPolicyDecisionCommand{
		WorkspaceID: "workspace-1", ParentIssueID: "parent-1", PolicyVersion: "policy-v1",
		NodeIssueIDs: map[string]string{},
		Actions: []looppolicy.Action{{Type: looppolicy.ActionActivateNode, NodeKey: "backend"}},
	})
	if err == nil {
		t.Fatal("BuildPolicyMutationBatch() error = nil, want missing issue id")
	}
}

func TestDurablePolicyMutationSinkCommitsBeforeDispatch(t *testing.T) {
	store := &fakePolicyMutationStore{commit: PolicyMutationCommit{Dispatch: []InitialDispatchRequest{{
		WorkspaceID: "workspace-1", IssueID: "issue-backend", NodeKey: "backend", AssigneeType: "agent", AssigneeID: "agent-1", Reason: "manifold_loop_policy",
	}}}}
	dispatcher := &fakeInitialDispatcher{}
	sink := NewDurablePolicyMutationSink(store, dispatcher)

	err := sink.ApplyPolicyDecision(context.Background(), policyCommand(
		looppolicy.Action{Type: looppolicy.ActionActivateNode, NodeKey: "backend", Reason: "stage_ready"},
	))
	if err != nil {
		t.Fatalf("ApplyPolicyDecision() error = %v", err)
	}
	if store.calls != 1 || dispatcher.calls != 1 {
		t.Fatalf("store calls=%d dispatcher calls=%d", store.calls, dispatcher.calls)
	}
	if dispatcher.last[0].IssueID != "issue-backend" {
		t.Fatalf("dispatch = %#v", dispatcher.last[0])
	}
}

func TestDurablePolicyMutationSinkDoesNotDispatchWhenTransactionFails(t *testing.T) {
	storeErr := errors.New("revision conflict")
	store := &fakePolicyMutationStore{err: storeErr}
	dispatcher := &fakeInitialDispatcher{}
	sink := NewDurablePolicyMutationSink(store, dispatcher)

	err := sink.ApplyPolicyDecision(context.Background(), policyCommand(
		looppolicy.Action{Type: looppolicy.ActionActivateNode, NodeKey: "backend"},
	))
	if !errors.Is(err, storeErr) {
		t.Fatalf("error = %v, want store error", err)
	}
	if dispatcher.calls != 0 {
		t.Fatalf("dispatcher calls = %d, want 0", dispatcher.calls)
	}
}

func TestDurablePolicyMutationSinkSurfacesPostCommitDispatchFailure(t *testing.T) {
	dispatchErr := errors.New("runtime unavailable")
	store := &fakePolicyMutationStore{commit: PolicyMutationCommit{Dispatch: []InitialDispatchRequest{{
		WorkspaceID: "workspace-1", IssueID: "issue-backend", NodeKey: "backend", AssigneeType: "agent", AssigneeID: "agent-1",
	}}}}
	dispatcher := &fakeInitialDispatcher{err: dispatchErr}
	sink := NewDurablePolicyMutationSink(store, dispatcher)

	err := sink.ApplyPolicyDecision(context.Background(), policyCommand(
		looppolicy.Action{Type: looppolicy.ActionActivateNode, NodeKey: "backend"},
	))
	var policyDispatchErr *PolicyDispatchError
	if !errors.As(err, &policyDispatchErr) || !errors.Is(err, dispatchErr) {
		t.Fatalf("error = %v, want PolicyDispatchError wrapping dispatch failure", err)
	}
	if policyDispatchErr.ParentIssueID != "parent-1" || len(policyDispatchErr.Failures) != 1 {
		t.Fatalf("policy dispatch error = %#v", policyDispatchErr)
	}
}

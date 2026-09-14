package loopservice

import (
	"context"
	"testing"
)

type fakeLoopReadRepository struct {
	listCalls int
	lastList  ListLoopsQuery
	listValue []LoopSummary
	getCalls int
	lastWorkspace string
	lastParent string
	getValue LoopDetail
}

func (f *fakeLoopReadRepository) ListLoops(_ context.Context, query ListLoopsQuery) ([]LoopSummary, error) {
	f.listCalls++
	f.lastList = query
	return f.listValue, nil
}

func (f *fakeLoopReadRepository) GetLoop(_ context.Context, workspaceID, parentIssueID string) (LoopDetail, error) {
	f.getCalls++
	f.lastWorkspace = workspaceID
	f.lastParent = parentIssueID
	return f.getValue, nil
}

func TestLoopReadServiceDefaultsAndCapsLimit(t *testing.T) {
	repo := &fakeLoopReadRepository{}
	svc := NewLoopReadService(repo)
	if _, err := svc.List(context.Background(), ListLoopsQuery{WorkspaceID: " workspace-1 "}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if repo.lastList.WorkspaceID != "workspace-1" || repo.lastList.Limit != 50 {
		t.Fatalf("query = %#v", repo.lastList)
	}

	if _, err := svc.List(context.Background(), ListLoopsQuery{WorkspaceID: "workspace-1", Limit: 1000}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if repo.lastList.Limit != 100 {
		t.Fatalf("limit = %d, want 100", repo.lastList.Limit)
	}
}

func TestLoopReadServiceRejectsMissingWorkspace(t *testing.T) {
	repo := &fakeLoopReadRepository{}
	svc := NewLoopReadService(repo)
	if _, err := svc.List(context.Background(), ListLoopsQuery{}); err == nil {
		t.Fatal("List() error = nil, want workspace validation error")
	}
	if repo.listCalls != 0 {
		t.Fatalf("repo calls = %d, want 0", repo.listCalls)
	}
}

func TestLoopReadServiceGetNormalizesIDs(t *testing.T) {
	repo := &fakeLoopReadRepository{}
	svc := NewLoopReadService(repo)
	if _, err := svc.Get(context.Background(), " workspace-1 ", " parent-1 "); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if repo.lastWorkspace != "workspace-1" || repo.lastParent != "parent-1" {
		t.Fatalf("workspace=%q parent=%q", repo.lastWorkspace, repo.lastParent)
	}
}

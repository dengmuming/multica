package loopservice

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/service"
)

type fakeExistingAssignedIssueDispatcher struct {
	got service.ExistingIssueDispatchRequest
	err error
}

func (f *fakeExistingAssignedIssueDispatcher) DispatchExistingAssignedIssue(_ context.Context, req service.ExistingIssueDispatchRequest) (pgtype.UUID, error) {
	f.got = req
	return pgtype.UUID{}, f.err
}

func TestMulticaInitialIssueDispatcherForwardsCanonicalIdentity(t *testing.T) {
	fake := &fakeExistingAssignedIssueDispatcher{}
	d := NewMulticaInitialIssueDispatcher(fake)

	err := d.DispatchAssignedIssue(context.Background(), InitialDispatchRequest{
		WorkspaceID:  "11111111-1111-4111-8111-111111111111",
		IssueID:      "22222222-2222-4222-8222-222222222222",
		NodeKey:      "backend",
		AssigneeType: "agent",
		AssigneeID:   "33333333-3333-4333-8333-333333333333",
	})
	if err != nil {
		t.Fatalf("DispatchAssignedIssue() error = %v", err)
	}
	if fake.got.ExpectedAssigneeType != "agent" {
		t.Fatalf("ExpectedAssigneeType = %q", fake.got.ExpectedAssigneeType)
	}
	if !fake.got.WorkspaceID.Valid || !fake.got.IssueID.Valid || !fake.got.ExpectedAssigneeID.Valid {
		t.Fatal("expected parsed UUIDs to be valid")
	}
}

func TestMulticaInitialIssueDispatcherRejectsInvalidUUIDBeforeService(t *testing.T) {
	fake := &fakeExistingAssignedIssueDispatcher{}
	d := NewMulticaInitialIssueDispatcher(fake)

	err := d.DispatchAssignedIssue(context.Background(), InitialDispatchRequest{
		WorkspaceID:  "not-a-uuid",
		IssueID:      "22222222-2222-4222-8222-222222222222",
		NodeKey:      "backend",
		AssigneeType: "agent",
		AssigneeID:   "33333333-3333-4333-8333-333333333333",
	})
	if err == nil {
		t.Fatal("expected invalid workspace UUID error")
	}
	if fake.got.WorkspaceID.Valid {
		t.Fatal("service should not have been called")
	}
}

func TestMulticaInitialIssueDispatcherPropagatesCanonicalDispatchFailure(t *testing.T) {
	fake := &fakeExistingAssignedIssueDispatcher{err: service.ErrIssueDispatchAssigneeMismatch}
	d := NewMulticaInitialIssueDispatcher(fake)

	err := d.DispatchAssignedIssue(context.Background(), InitialDispatchRequest{
		WorkspaceID:  "11111111-1111-4111-8111-111111111111",
		IssueID:      "22222222-2222-4222-8222-222222222222",
		NodeKey:      "frontend",
		AssigneeType: "agent",
		AssigneeID:   "33333333-3333-4333-8333-333333333333",
	})
	if !errors.Is(err, service.ErrIssueDispatchAssigneeMismatch) {
		t.Fatalf("expected assignee mismatch, got %v", err)
	}
}

package loopservice

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/looptemplate"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type fakeBindingScopeQueries struct {
	agent  db.Agent
	squad  db.Squad
	member db.Member
	err    error
	kind   string
}

func (f *fakeBindingScopeQueries) GetAgentInWorkspace(_ context.Context, _ db.GetAgentInWorkspaceParams) (db.Agent, error) {
	f.kind = "agent"
	return f.agent, f.err
}

func (f *fakeBindingScopeQueries) GetSquadInWorkspace(_ context.Context, _ db.GetSquadInWorkspaceParams) (db.Squad, error) {
	f.kind = "squad"
	return f.squad, f.err
}

func (f *fakeBindingScopeQueries) GetMemberByUserAndWorkspace(_ context.Context, _ db.GetMemberByUserAndWorkspaceParams) (db.Member, error) {
	f.kind = "member"
	return f.member, f.err
}

func TestMulticaBindingScopeValidatorAgent(t *testing.T) {
	q := &fakeBindingScopeQueries{}
	v := NewMulticaBindingScopeValidator(q)
	err := v.ValidateBindingScope(context.Background(),
		"11111111-1111-4111-8111-111111111111", "backend",
		looptemplate.RoleBinding{Type: "agent", ID: "22222222-2222-4222-8222-222222222222"},
	)
	if err != nil {
		t.Fatalf("ValidateBindingScope() error = %v", err)
	}
	if q.kind != "agent" {
		t.Fatalf("lookup kind = %q", q.kind)
	}
}

func TestMulticaBindingScopeValidatorArchivedAgent(t *testing.T) {
	q := &fakeBindingScopeQueries{agent: db.Agent{ArchivedAt: pgtype.Timestamptz{Valid: true}}}
	v := NewMulticaBindingScopeValidator(q)
	err := v.ValidateBindingScope(context.Background(),
		"11111111-1111-4111-8111-111111111111", "backend",
		looptemplate.RoleBinding{Type: "agent", ID: "22222222-2222-4222-8222-222222222222"},
	)
	if !errors.Is(err, ErrBindingTargetArchived) {
		t.Fatalf("expected archived binding error, got %v", err)
	}
}

func TestMulticaBindingScopeValidatorMemberUsesUserIdentity(t *testing.T) {
	q := &fakeBindingScopeQueries{}
	v := NewMulticaBindingScopeValidator(q)
	err := v.ValidateBindingScope(context.Background(),
		"11111111-1111-4111-8111-111111111111", "product",
		looptemplate.RoleBinding{Type: "member", ID: "33333333-3333-4333-8333-333333333333"},
	)
	if err != nil {
		t.Fatalf("ValidateBindingScope() error = %v", err)
	}
	if q.kind != "member" {
		t.Fatalf("lookup kind = %q", q.kind)
	}
}

func TestMulticaBindingScopeValidatorMapsNoRows(t *testing.T) {
	q := &fakeBindingScopeQueries{err: pgx.ErrNoRows}
	v := NewMulticaBindingScopeValidator(q)
	err := v.ValidateBindingScope(context.Background(),
		"11111111-1111-4111-8111-111111111111", "frontend",
		looptemplate.RoleBinding{Type: "squad", ID: "44444444-4444-4444-8444-444444444444"},
	)
	if !errors.Is(err, ErrBindingTargetNotFound) {
		t.Fatalf("expected not-found binding error, got %v", err)
	}
}

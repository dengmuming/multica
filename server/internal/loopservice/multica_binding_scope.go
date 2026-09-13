package loopservice

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/looptemplate"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

var (
	ErrBindingTargetNotFound = errors.New("loop binding target not found in workspace")
	ErrBindingTargetArchived = errors.New("loop binding target is archived")
)

// BindingScopeQueries is the existing Multica identity surface required to
// prove a compiled role binding belongs to the target workspace. These methods
// are all backed by pre-existing SQLC queries, so this adapter does not depend
// on generation of the new loop_* query files.
type BindingScopeQueries interface {
	GetAgentInWorkspace(ctx context.Context, arg db.GetAgentInWorkspaceParams) (db.Agent, error)
	GetSquadInWorkspace(ctx context.Context, arg db.GetSquadInWorkspaceParams) (db.Squad, error)
	GetMemberByUserAndWorkspace(ctx context.Context, arg db.GetMemberByUserAndWorkspaceParams) (db.Member, error)
}

type MulticaBindingScopeValidator struct {
	queries BindingScopeQueries
}

func NewMulticaBindingScopeValidator(queries BindingScopeQueries) *MulticaBindingScopeValidator {
	return &MulticaBindingScopeValidator{queries: queries}
}

func (v *MulticaBindingScopeValidator) ValidateBindingScope(ctx context.Context, workspaceID, role string, binding looptemplate.RoleBinding) error {
	if v == nil || v.queries == nil {
		return errors.New("binding scope queries are required")
	}
	workspaceUUID, err := parseRequiredUUID("workspace id", workspaceID)
	if err != nil {
		return err
	}
	bindingUUID, err := parseRequiredUUID("binding id", binding.ID)
	if err != nil {
		return err
	}
	role = strings.TrimSpace(role)
	if role == "" {
		return errors.New("role is required")
	}

	switch binding.Type {
	case "agent":
		agent, err := v.queries.GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{
			ID: bindingUUID, WorkspaceID: workspaceUUID,
		})
		if err != nil {
			return bindingLookupError(role, binding, err)
		}
		if agent.ArchivedAt.Valid {
			return fmt.Errorf("%w: role %q agent %s", ErrBindingTargetArchived, role, binding.ID)
		}
		return nil

	case "squad":
		squad, err := v.queries.GetSquadInWorkspace(ctx, db.GetSquadInWorkspaceParams{
			ID: bindingUUID, WorkspaceID: workspaceUUID,
		})
		if err != nil {
			return bindingLookupError(role, binding, err)
		}
		if squad.ArchivedAt.Valid {
			return fmt.Errorf("%w: role %q squad %s", ErrBindingTargetArchived, role, binding.ID)
		}
		return nil

	case "member":
		// Multica issue assignee_type='member' stores the user's UUID, not the
		// member-row UUID. Resolve the binding the same way so compiler output
		// can be written directly to issue.assignee_id without translation.
		_, err := v.queries.GetMemberByUserAndWorkspace(ctx, db.GetMemberByUserAndWorkspaceParams{
			UserID: bindingUUID, WorkspaceID: workspaceUUID,
		})
		if err != nil {
			return bindingLookupError(role, binding, err)
		}
		return nil

	default:
		return fmt.Errorf("unsupported binding type %q", binding.Type)
	}
}

func bindingLookupError(role string, binding looptemplate.RoleBinding, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: role %q %s %s", ErrBindingTargetNotFound, role, binding.Type, binding.ID)
	}
	return fmt.Errorf("lookup role %q %s %s: %w", role, binding.Type, binding.ID, err)
}

// Ensure the UUID type stays visible to compile-time interface checks in tests
// that use generated SQLC parameter structs without importing utility helpers.
var _ pgtype.UUID

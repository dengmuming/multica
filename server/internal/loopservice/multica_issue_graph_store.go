package loopservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/issueposition"
	multicaservice "github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
)

const loopInstanceUniqueIndex = "idx_issue_manifold_loop_instance_key"
const loopInstanceMetadataKey = "manifold.loop.instance_key"

// MulticaIssueGraphStore persists the compiled Parent/Child graph with existing
// generated Issue queries. It intentionally does not depend on generated
// loop_issue.sql code, so graph creation can run before the new SQLC files are
// regenerated. The dedicated loop query source remains the eventual compact
// repository surface.
//
// Metadata is written through the existing SetIssueMetadataKey query in the
// same transaction as Issue insertion. Consequently the instance-key unique
// index participates before commit and concurrent creators cannot both expose a
// loop instance.
type MulticaIssueGraphStore struct {
	issues *multicaservice.IssueService
}

func NewMulticaIssueGraphStore(issues *multicaservice.IssueService) *MulticaIssueGraphStore {
	return &MulticaIssueGraphStore{issues: issues}
}

func (s *MulticaIssueGraphStore) CreateGraphAtomically(ctx context.Context, cmd CreateLoopIssueGraphCommand) (CreatedIssueGraph, error) {
	if s == nil || s.issues == nil || s.issues.Queries == nil || s.issues.TxStarter == nil {
		return CreatedIssueGraph{}, errors.New("Multica issue graph store dependencies are incomplete")
	}
	workspaceID, err := parseRequiredUUID("workspace id", cmd.WorkspaceID)
	if err != nil {
		return CreatedIssueGraph{}, err
	}
	projectID, err := parseRequiredUUID("project id", cmd.ProjectID)
	if err != nil {
		return CreatedIssueGraph{}, err
	}
	if strings.TrimSpace(cmd.InstanceKey) == "" {
		return CreatedIssueGraph{}, errors.New("instance key is required")
	}
	if len(cmd.Children) == 0 {
		return CreatedIssueGraph{}, errors.New("loop graph requires at least one child Issue")
	}

	issueCountPolicy := multicaservice.ResolveIssueCountPolicy(ctx, s.issues.Entitlements, workspaceID)

	tx, err := s.issues.TxStarter.Begin(ctx)
	if err != nil {
		return CreatedIssueGraph{}, fmt.Errorf("begin loop graph transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := s.issues.Queries.WithTx(tx)

	if _, err := qtx.GetProjectInWorkspace(ctx, db.GetProjectInWorkspaceParams{ID: projectID, WorkspaceID: workspaceID}); err != nil {
		return CreatedIssueGraph{}, fmt.Errorf("project not found in workspace: %w", err)
	}

	parent, err := createLoopIssueRow(ctx, tx, qtx, workspaceID, projectID, pgtype.UUID{}, cmd.Parent, issueCountPolicy)
	if err != nil {
		return CreatedIssueGraph{}, mapLoopInstanceConflict(err)
	}
	if err := setLoopMetadata(ctx, qtx, workspaceID, parent.ID, cmd.Parent.Metadata, true); err != nil {
		return CreatedIssueGraph{}, mapLoopInstanceConflict(err)
	}

	created := CreatedIssueGraph{
		ParentIssueID: uuidString(parent.ID),
		NodeIssueIDs:  make(map[string]string, len(cmd.Children)),
	}
	for _, child := range cmd.Children {
		if strings.TrimSpace(child.NodeKey) == "" {
			return CreatedIssueGraph{}, errors.New("child node key is required")
		}
		if _, duplicate := created.NodeIssueIDs[child.NodeKey]; duplicate {
			return CreatedIssueGraph{}, fmt.Errorf("duplicate child node key %q", child.NodeKey)
		}
		issue, err := createLoopIssueRow(ctx, tx, qtx, workspaceID, projectID, parent.ID, child, issueCountPolicy)
		if err != nil {
			return CreatedIssueGraph{}, err
		}
		if err := setLoopMetadata(ctx, qtx, workspaceID, issue.ID, child.Metadata, false); err != nil {
			return CreatedIssueGraph{}, err
		}
		created.NodeIssueIDs[child.NodeKey] = uuidString(issue.ID)
	}

	if err := tx.Commit(ctx); err != nil {
		return CreatedIssueGraph{}, mapLoopInstanceConflict(err)
	}
	return created, nil
}

func createLoopIssueRow(
	ctx context.Context,
	tx pgx.Tx,
	qtx *db.Queries,
	workspaceID, projectID, parentIssueID pgtype.UUID,
	cmd LoopIssueCommand,
	policy multicaservice.IssueCountPolicy,
) (db.Issue, error) {
	creatorID, err := parseRequiredUUID("creator id", cmd.CreatorID)
	if err != nil {
		return db.Issue{}, err
	}
	if cmd.CreatorType != "member" && cmd.CreatorType != "agent" {
		return db.Issue{}, fmt.Errorf("unsupported creator type %q", cmd.CreatorType)
	}
	if strings.TrimSpace(cmd.Title) == "" || strings.TrimSpace(cmd.Status) == "" {
		return db.Issue{}, errors.New("loop issue title and status are required")
	}

	issueNumber, err := multicaservice.AllocateIssueNumber(ctx, qtx, workspaceID, policy)
	if err != nil {
		return db.Issue{}, fmt.Errorf("allocate issue number: %w", err)
	}
	position, err := issueposition.NextTopPosition(ctx, tx, workspaceID, cmd.Status)
	if err != nil {
		return db.Issue{}, fmt.Errorf("next issue position: %w", err)
	}

	assigneeType := pgtype.Text{}
	assigneeID := pgtype.UUID{}
	if cmd.AssigneeType != "" || cmd.AssigneeID != "" {
		if cmd.AssigneeType == "" || cmd.AssigneeID == "" {
			return db.Issue{}, errors.New("assignee type and id must be provided together")
		}
		parsedAssigneeID, err := parseRequiredUUID("assignee id", cmd.AssigneeID)
		if err != nil {
			return db.Issue{}, err
		}
		assigneeType = pgtype.Text{String: cmd.AssigneeType, Valid: true}
		assigneeID = parsedAssigneeID
	}

	description := pgtype.Text{}
	if cmd.Description != "" {
		description = pgtype.Text{String: cmd.Description, Valid: true}
	}
	stage := pgtype.Int4{}
	if cmd.Stage != nil {
		stage = pgtype.Int4{Int32: int32(*cmd.Stage), Valid: true}
	}

	return qtx.CreateIssue(ctx, db.CreateIssueParams{
		ID:            dbid.NewV7(),
		WorkspaceID:   workspaceID,
		Title:         cmd.Title,
		Description:   description,
		Status:        cmd.Status,
		Priority:      cmd.Priority,
		AssigneeType:  assigneeType,
		AssigneeID:    assigneeID,
		CreatorType:   cmd.CreatorType,
		CreatorID:     creatorID,
		ParentIssueID: parentIssueID,
		Position:      position,
		Number:        issueNumber,
		ProjectID:     projectID,
		Stage:         stage,
	})
}

func setLoopMetadata(ctx context.Context, qtx *db.Queries, workspaceID, issueID pgtype.UUID, metadata map[string]any, instanceFirst bool) error {
	keys := make([]string, 0, len(metadata))
	for key := range metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if instanceFirst {
		for i, key := range keys {
			if key == loopInstanceMetadataKey {
				keys[0], keys[i] = keys[i], keys[0]
				break
			}
	}

	for _, key := range keys {
		encoded, err := json.Marshal(metadata[key])
		if err != nil {
			return fmt.Errorf("encode metadata %q: %w", key, err)
		}
		if _, err := qtx.SetIssueMetadataKey(ctx, db.SetIssueMetadataKeyParams{
			Key: key, Value: encoded, ID: issueID, WorkspaceID: workspaceID,
		}); err != nil {
			return fmt.Errorf("set metadata %q: %w", key, err)
		}
	}
	return nil
}

func mapLoopInstanceConflict(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == loopInstanceUniqueIndex {
		return ErrLoopInstanceExists
	}
	return err
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", id.Bytes[0:4], id.Bytes[4:6], id.Bytes[6:8], id.Bytes[8:10], id.Bytes[10:16])
}

package loopservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// RawArtifactRepository is the temporary raw-PGX implementation for
// loop_artifact while its committed SQLC source awaits generation.
type RawArtifactRepository struct {
	db LoopStateDB
}

func NewRawArtifactRepository(db LoopStateDB) *RawArtifactRepository {
	return &RawArtifactRepository{db: db}
}

func (r *RawArtifactRepository) ResolveAgentTaskArtifactContext(ctx context.Context, workspaceID, parentIssueID, taskID, agentID string) (string, error) {
	if r == nil || r.db == nil {
		return "", errors.New("artifact database is required")
	}
	wid, err := parseRequiredUUID("workspace id", workspaceID)
	if err != nil { return "", err }
	pid, err := parseRequiredUUID("parent issue id", parentIssueID)
	if err != nil { return "", err }
	tid, err := parseRequiredUUID("task id", taskID)
	if err != nil { return "", err }
	aid, err := parseRequiredUUID("agent id", agentID)
	if err != nil { return "", err }

	var nodeID pgtype.UUID
	var parentMetadata, nodeMetadata []byte
	err = r.db.QueryRow(ctx, `
		SELECT i.id, p.metadata, i.metadata
		FROM agent_task_queue t
		JOIN issue i ON i.id=t.issue_id
		JOIN issue p ON p.id=i.parent_issue_id
		WHERE t.id=$1 AND t.agent_id=$2
		  AND i.workspace_id=$3 AND p.workspace_id=$3 AND p.id=$4`,
		tid, aid, wid, pid,
	).Scan(&nodeID, &parentMetadata, &nodeMetadata)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) { return "", errors.New("task token is not authorized for this loop") }
		return "", fmt.Errorf("resolve artifact task lineage: %w", err)
	}
	if !isLoopParentMetadata(parentMetadata) || !isLoopNodeMetadata(nodeMetadata) {
		return "", errors.New("artifact task does not belong to a Manifold Agent loop")
	}
	return uuidString(nodeID), nil
}

func (r *RawArtifactRepository) ValidateHumanArtifactContext(ctx context.Context, workspaceID, parentIssueID, nodeIssueID, taskID string) error {
	if r == nil || r.db == nil { return errors.New("artifact database is required") }
	wid, err := parseRequiredUUID("workspace id", workspaceID)
	if err != nil { return err }
	pid, err := parseRequiredUUID("parent issue id", parentIssueID)
	if err != nil { return err }
	var parentMetadata []byte
	if err := r.db.QueryRow(ctx, `SELECT metadata FROM issue WHERE id=$1 AND workspace_id=$2`, pid, wid).Scan(&parentMetadata); err != nil {
		if errors.Is(err, pgx.ErrNoRows) { return errors.New("loop parent issue not found in workspace") }
		return fmt.Errorf("load artifact loop parent: %w", err)
	}
	if !isLoopParentMetadata(parentMetadata) { return errors.New("target parent is not a Manifold Agent loop") }

	var nid pgtype.UUID
	if strings.TrimSpace(nodeIssueID) != "" {
		nid, err = parseRequiredUUID("node issue id", nodeIssueID)
		if err != nil { return err }
		var nodeMetadata []byte
		if err := r.db.QueryRow(ctx, `
			SELECT metadata FROM issue
			WHERE id=$1 AND workspace_id=$2 AND parent_issue_id=$3`, nid, wid, pid).Scan(&nodeMetadata); err != nil {
			if errors.Is(err, pgx.ErrNoRows) { return errors.New("artifact node is not a child of this loop") }
			return fmt.Errorf("load artifact node: %w", err)
		}
		if !isLoopNodeMetadata(nodeMetadata) { return errors.New("artifact node is not a Manifold Agent loop node") }
	}
	if strings.TrimSpace(taskID) != "" {
		tid, err := parseRequiredUUID("task id", taskID)
		if err != nil { return err }
		if !nid.Valid { return errors.New("task-scoped artifact requires node_issue_id") }
		var exists bool
		if err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_task_queue WHERE id=$1 AND issue_id=$2)`, tid, nid).Scan(&exists); err != nil {
			return fmt.Errorf("validate artifact task: %w", err)
		}
		if !exists { return errors.New("artifact task does not belong to requested loop node") }
	}
	return nil
}

func (r *RawArtifactRepository) RegisterArtifact(ctx context.Context, cmd RegisterArtifactCommand) (RegisteredArtifact, error) {
	if r == nil || r.db == nil { return RegisteredArtifact{}, errors.New("artifact database is required") }
	wid, err := parseRequiredUUID("workspace id", cmd.WorkspaceID)
	if err != nil { return RegisteredArtifact{}, err }
	pid, err := parseRequiredUUID("parent issue id", cmd.ParentIssueID)
	if err != nil { return RegisteredArtifact{}, err }
	nid := pgtype.UUID{}
	if cmd.NodeIssueID != "" { nid, err = parseRequiredUUID("node issue id", cmd.NodeIssueID); if err != nil { return RegisteredArtifact{}, err } }
	tid := pgtype.UUID{}
	if cmd.TaskID != "" { tid, err = parseRequiredUUID("task id", cmd.TaskID); if err != nil { return RegisteredArtifact{}, err } }
	refID := pgtype.UUID{}
	if cmd.RefID != "" { refID, err = parseRequiredUUID("artifact ref id", cmd.RefID); if err != nil { return RegisteredArtifact{}, err } }
	creatorID, err := parseRequiredUUID("artifact creator id", cmd.CreatedByID)
	if err != nil { return RegisteredArtifact{}, err }
	metadataJSON, err := json.Marshal(cmd.Metadata)
	if err != nil { return RegisteredArtifact{}, fmt.Errorf("encode artifact metadata: %w", err) }

	var artifactID pgtype.UUID
	var createdAt time.Time
	err = r.db.QueryRow(ctx, `
		INSERT INTO loop_artifact (
			workspace_id,parent_issue_id,node_issue_id,task_id,
			artifact_type,relation,title,ref_kind,ref_id,ref_uri,metadata,
			created_by_type,created_by_id
		) VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8,$9,NULLIF($10,''),$11,$12,$13)
		RETURNING id, created_at`,
		wid,pid,nid,tid,cmd.ArtifactType,cmd.Relation,cmd.Title,cmd.RefKind,refID,cmd.RefURI,metadataJSON,cmd.CreatedByType,creatorID,
	).Scan(&artifactID, &createdAt)
	if err != nil { return RegisteredArtifact{}, fmt.Errorf("insert loop artifact: %w", err) }
	return RegisteredArtifact{
		ID: uuidString(artifactID), WorkspaceID: cmd.WorkspaceID, ParentIssueID: cmd.ParentIssueID,
		NodeIssueID: cmd.NodeIssueID, TaskID: cmd.TaskID, ArtifactType: cmd.ArtifactType,
		Relation: cmd.Relation, Title: cmd.Title, RefKind: cmd.RefKind, RefID: cmd.RefID,
		RefURI: cmd.RefURI, Metadata: cloneAnyMap(cmd.Metadata), CreatedByType: cmd.CreatedByType, CreatedByID: cmd.CreatedByID,
		CreatedAt: createdAt,
	}, nil
}

func (r *RawArtifactRepository) ListArtifacts(ctx context.Context, workspaceID, parentIssueID string) ([]RegisteredArtifact, error) {
	if r == nil || r.db == nil { return nil, errors.New("artifact database is required") }
	wid, err := parseRequiredUUID("workspace id", workspaceID)
	if err != nil { return nil, err }
	pid, err := parseRequiredUUID("parent issue id", parentIssueID)
	if err != nil { return nil, err }

	rows, err := r.db.Query(ctx, `
		SELECT id, node_issue_id, task_id, artifact_type, relation,
		       COALESCE(title, ''), ref_kind, ref_id, COALESCE(ref_uri, ''),
		       metadata, created_by_type, created_by_id, created_at
		FROM loop_artifact
		WHERE workspace_id=$1 AND parent_issue_id=$2
		ORDER BY created_at DESC, id DESC`, wid, pid)
	if err != nil { return nil, fmt.Errorf("list loop artifacts: %w", err) }
	defer rows.Close()

	out := make([]RegisteredArtifact, 0)
	for rows.Next() {
		var id, nodeID, taskID, refID, creatorID pgtype.UUID
		var metadataJSON []byte
		var artifact RegisteredArtifact
		if err := rows.Scan(
			&id, &nodeID, &taskID, &artifact.ArtifactType, &artifact.Relation,
			&artifact.Title, &artifact.RefKind, &refID, &artifact.RefURI,
			&metadataJSON, &artifact.CreatedByType, &creatorID, &artifact.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan loop artifact: %w", err)
		}
		artifact.ID = uuidString(id)
		artifact.WorkspaceID = workspaceID
		artifact.ParentIssueID = parentIssueID
		artifact.NodeIssueID = uuidString(nodeID)
		artifact.TaskID = uuidString(taskID)
		artifact.RefID = uuidString(refID)
		artifact.CreatedByID = uuidString(creatorID)
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &artifact.Metadata); err != nil {
				return nil, fmt.Errorf("decode loop artifact metadata %s: %w", artifact.ID, err)
			}
		}
		if artifact.Metadata == nil { artifact.Metadata = map[string]any{} }
		out = append(out, artifact)
	}
	if err := rows.Err(); err != nil { return nil, fmt.Errorf("iterate loop artifacts: %w", err) }
	return out, nil
}

func isLoopParentMetadata(raw []byte) bool {
	metadata, err := decodeIssueMetadata(raw)
	return err == nil && metadataString(metadata, "manifold.loop.instance_key") != "" && metadataString(metadata, "manifold.loop.template_key") != ""
}

func isLoopNodeMetadata(raw []byte) bool {
	metadata, err := decodeIssueMetadata(raw)
	return err == nil && metadataString(metadata, "manifold.loop.node_key") != ""
}

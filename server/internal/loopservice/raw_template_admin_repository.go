package loopservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	multicaservice "github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/looptemplate"
	"github.com/multica-ai/multica/server/pkg/dbid"
)

// RawTemplateAdminRepository uses the same transaction starter as IssueService
// but raw PGX for loop_template until the committed loop SQLC sources are
// generated. Per-(workspace,key) advisory locks serialize version allocation
// and publication.
type RawTemplateAdminRepository struct {
	issues *multicaservice.IssueService
}

func NewRawTemplateAdminRepository(issues *multicaservice.IssueService) *RawTemplateAdminRepository {
	return &RawTemplateAdminRepository{issues: issues}
}

func (r *RawTemplateAdminRepository) CreateDraft(ctx context.Context, cmd CreateTemplateDraftCommand) (ManagedTemplate, error) {
	if r == nil || r.issues == nil || r.issues.TxStarter == nil {
		return ManagedTemplate{}, errors.New("template repository dependencies are incomplete")
	}
	wid, err := parseRequiredUUID("workspace id", cmd.WorkspaceID)
	if err != nil {
		return ManagedTemplate{}, err
	}
	creatorID, err := parseRequiredUUID("creator id", cmd.CreatorID)
	if err != nil {
		return ManagedTemplate{}, err
	}
	definitionJSON, policyJSON, err := encodeManagedTemplate(cmd.Definition, cmd.PolicyVersion)
	if err != nil {
		return ManagedTemplate{}, err
	}

	tx, err := r.issues.TxStarter.Begin(ctx)
	if err != nil {
		return ManagedTemplate{}, fmt.Errorf("begin template draft transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockTemplateKey(ctx, tx, wid, cmd.Definition.Key); err != nil {
		return ManagedTemplate{}, err
	}
	var nextVersion int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(version),0)+1
		FROM loop_template
		WHERE workspace_id=$1 AND template_key=$2`, wid, cmd.Definition.Key).Scan(&nextVersion); err != nil {
		return ManagedTemplate{}, fmt.Errorf("allocate template version: %w", err)
	}
	id := dbid.NewV7()
	_, err = tx.Exec(ctx, `
		INSERT INTO loop_template (
			id, workspace_id, template_key, version, name, description, status,
			definition, policy, created_by_type, created_by_id
		) VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),'draft',$7,$8,$9,$10)`,
		id, wid, cmd.Definition.Key, nextVersion, cmd.Definition.Name, cmd.Definition.Description,
		definitionJSON, policyJSON, cmd.CreatorType, creatorID,
	)
	if err != nil {
		return ManagedTemplate{}, fmt.Errorf("insert template draft: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ManagedTemplate{}, fmt.Errorf("commit template draft: %w", err)
	}
	return managedTemplateFromCommand(id, cmd.WorkspaceID, nextVersion, "draft", cmd.Definition, cmd.PolicyVersion), nil
}

func (r *RawTemplateAdminRepository) UpdateDraft(ctx context.Context, cmd UpdateTemplateDraftCommand) (ManagedTemplate, error) {
	if r == nil || r.issues == nil || r.issues.TxStarter == nil {
		return ManagedTemplate{}, errors.New("template repository dependencies are incomplete")
	}
	wid, err := parseRequiredUUID("workspace id", cmd.WorkspaceID)
	if err != nil {
		return ManagedTemplate{}, err
	}
	definitionJSON, policyJSON, err := encodeManagedTemplate(cmd.Definition, cmd.PolicyVersion)
	if err != nil {
		return ManagedTemplate{}, err
	}
	tx, err := r.issues.TxStarter.Begin(ctx)
	if err != nil {
		return ManagedTemplate{}, fmt.Errorf("begin template update transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockTemplateKey(ctx, tx, wid, cmd.TemplateKey); err != nil {
		return ManagedTemplate{}, err
	}
	var id pgtype.UUID
	err = tx.QueryRow(ctx, `
		UPDATE loop_template
		SET name=$1, description=NULLIF($2,''), definition=$3, policy=$4
		WHERE workspace_id=$5 AND template_key=$6 AND version=$7 AND status='draft'
		RETURNING id`,
		cmd.Definition.Name, cmd.Definition.Description, definitionJSON, policyJSON,
		wid, cmd.TemplateKey, cmd.Version,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ManagedTemplate{}, ErrTemplateDraftNotFound
	}
	if err != nil {
		return ManagedTemplate{}, fmt.Errorf("update template draft: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ManagedTemplate{}, fmt.Errorf("commit template update: %w", err)
	}
	return managedTemplateFromCommand(id, cmd.WorkspaceID, cmd.Version, "draft", cmd.Definition, cmd.PolicyVersion), nil
}

func (r *RawTemplateAdminRepository) PublishDraft(ctx context.Context, cmd PublishTemplateDraftCommand) (ManagedTemplate, error) {
	if r == nil || r.issues == nil || r.issues.TxStarter == nil {
		return ManagedTemplate{}, errors.New("template repository dependencies are incomplete")
	}
	wid, err := parseRequiredUUID("workspace id", cmd.WorkspaceID)
	if err != nil {
		return ManagedTemplate{}, err
	}
	tx, err := r.issues.TxStarter.Begin(ctx)
	if err != nil {
		return ManagedTemplate{}, fmt.Errorf("begin template publish transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockTemplateKey(ctx, tx, wid, cmd.TemplateKey); err != nil {
		return ManagedTemplate{}, err
	}

	var id pgtype.UUID
	var definitionJSON, policyJSON []byte
	var name string
	var description pgtype.Text
	err = tx.QueryRow(ctx, `
		SELECT id, name, description, definition, policy
		FROM loop_template
		WHERE workspace_id=$1 AND template_key=$2 AND version=$3 AND status='draft'
		FOR UPDATE`, wid, cmd.TemplateKey, cmd.Version).Scan(&id, &name, &description, &definitionJSON, &policyJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return ManagedTemplate{}, ErrTemplateDraftNotFound
	}
	if err != nil {
		return ManagedTemplate{}, fmt.Errorf("lock template draft: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE loop_template
		SET status='archived', archived_at=COALESCE(archived_at,now())
		WHERE workspace_id=$1 AND template_key=$2 AND status='active'`, wid, cmd.TemplateKey); err != nil {
		return ManagedTemplate{}, fmt.Errorf("archive active template version: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE loop_template
		SET status='active', published_at=now(), archived_at=NULL
		WHERE id=$1`, id); err != nil {
		return ManagedTemplate{}, fmt.Errorf("publish template draft: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ManagedTemplate{}, fmt.Errorf("commit template publish: %w", err)
	}

	var definition looptemplate.Definition
	if err := json.Unmarshal(definitionJSON, &definition); err != nil {
		return ManagedTemplate{}, fmt.Errorf("decode published definition: %w", err)
	}
	policyVersion, err := policyVersionFromJSON(policyJSON, cmd.TemplateKey, cmd.Version)
	if err != nil {
		return ManagedTemplate{}, err
	}
	return ManagedTemplate{
		ID: uuidString(id), WorkspaceID: cmd.WorkspaceID, Key: cmd.TemplateKey, Version: cmd.Version,
		Name: name, Description: description.String, Status: "active", Definition: definition, PolicyVersion: policyVersion,
	}, nil
}

func lockTemplateKey(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID, templateKey string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, uuidString(workspaceID)+":"+strings.TrimSpace(templateKey))
	if err != nil {
		return fmt.Errorf("lock template key: %w", err)
	}
	return nil
}

func encodeManagedTemplate(def looptemplate.Definition, policyVersion string) ([]byte, []byte, error) {
	definitionJSON, err := json.Marshal(def)
	if err != nil {
		return nil, nil, fmt.Errorf("encode template definition: %w", err)
	}
	policyJSON, err := json.Marshal(map[string]any{"version": strings.TrimSpace(policyVersion)})
	if err != nil {
		return nil, nil, fmt.Errorf("encode template policy: %w", err)
	}
	return definitionJSON, policyJSON, nil
}

func policyVersionFromJSON(raw []byte, key string, version int) (string, error) {
	fallback := fmt.Sprintf("%s@%d", key, version)
	if len(raw) == 0 {
		return fallback, nil
	}
	var policy map[string]any
	if err := json.Unmarshal(raw, &policy); err != nil {
		return "", fmt.Errorf("decode template policy: %w", err)
	}
	if value, ok := policy["version"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value), nil
	}
	return fallback, nil
}

func managedTemplateFromCommand(id pgtype.UUID, workspaceID string, version int, status string, def looptemplate.Definition, policyVersion string) ManagedTemplate {
	return ManagedTemplate{
		ID: uuidString(id), WorkspaceID: workspaceID, Key: def.Key, Version: version,
		Name: def.Name, Description: def.Description, Status: status, Definition: def,
		PolicyVersion: strings.TrimSpace(policyVersion),
	}
}

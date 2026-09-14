package loopservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/looptemplate"
)

// RawLoopTemplateRepository supplies the creation path while loop-template SQLC
// generation is unavailable. It is deliberately exact-version only.
type RawLoopTemplateRepository struct {
	db LoopStateDB
}

func NewRawLoopTemplateRepository(db LoopStateDB) *RawLoopTemplateRepository {
	return &RawLoopTemplateRepository{db: db}
}

func (r *RawLoopTemplateRepository) LoadPublishedTemplate(ctx context.Context, workspaceID, templateKey string, version int) (PublishedTemplate, error) {
	if r == nil || r.db == nil {
		return PublishedTemplate{}, errors.New("loop template database is required")
	}
	wid, err := parseRequiredUUID("workspace id", workspaceID)
	if err != nil {
		return PublishedTemplate{}, err
	}
	if strings.TrimSpace(templateKey) == "" || version <= 0 {
		return PublishedTemplate{}, errors.New("template key and positive version are required")
	}
	var definitionJSON, policyJSON []byte
	err = r.db.QueryRow(ctx, `
		SELECT definition, policy
		FROM loop_template
		WHERE workspace_id=$1 AND template_key=$2 AND version=$3
		  AND status IN ('active','archived') AND published_at IS NOT NULL
		LIMIT 1`, wid, templateKey, version).Scan(&definitionJSON, &policyJSON)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PublishedTemplate{}, fmt.Errorf("published loop template %s@%d not found", templateKey, version)
		}
		return PublishedTemplate{}, fmt.Errorf("load published loop template: %w", err)
	}
	var definition looptemplate.Definition
	if err := json.Unmarshal(definitionJSON, &definition); err != nil {
		return PublishedTemplate{}, fmt.Errorf("decode loop template definition: %w", err)
	}
	policyVersion := fmt.Sprintf("%s@%d", templateKey, version)
	if len(policyJSON) > 0 {
		var policy map[string]any
		if err := json.Unmarshal(policyJSON, &policy); err != nil {
			return PublishedTemplate{}, fmt.Errorf("decode loop template policy: %w", err)
		}
		if raw, ok := policy["version"].(string); ok && strings.TrimSpace(raw) != "" {
			policyVersion = strings.TrimSpace(raw)
		}
	}
	return PublishedTemplate{Key: templateKey, Version: version, PolicyVersion: policyVersion, Definition: definition}, nil
}

func (r *RawLoopTemplateRepository) LoopInstanceExists(ctx context.Context, workspaceID, projectID, instanceKey string) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("loop template database is required")
	}
	wid, err := parseRequiredUUID("workspace id", workspaceID)
	if err != nil {
		return false, err
	}
	pid, err := parseRequiredUUID("project id", projectID)
	if err != nil {
		return false, err
	}
	instanceKey = strings.TrimSpace(instanceKey)
	if instanceKey == "" {
		return false, errors.New("instance key is required")
	}
	var exists bool
	err = r.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM issue
			WHERE workspace_id=$1 AND project_id=$2
			  AND metadata->>'manifold.loop.instance_key'=$3
		)`, wid, pid, instanceKey).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check loop instance: %w", err)
	}
	return exists, nil
}

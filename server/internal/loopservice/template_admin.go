package loopservice

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/multica-ai/multica/server/internal/looptemplate"
)

var (
	ErrTemplateDraftNotFound = errors.New("loop template draft not found")
	ErrTemplateNotDraft      = errors.New("loop template version is not a draft")
)

type ManagedTemplate struct {
	ID            string
	WorkspaceID   string
	Key           string
	Version       int
	Name          string
	Description   string
	Status        string
	Definition    looptemplate.Definition
	PolicyVersion string
}

type TemplateAdminRepository interface {
	CreateDraft(ctx context.Context, req CreateTemplateDraftCommand) (ManagedTemplate, error)
	UpdateDraft(ctx context.Context, req UpdateTemplateDraftCommand) (ManagedTemplate, error)
	PublishDraft(ctx context.Context, req PublishTemplateDraftCommand) (ManagedTemplate, error)
}

type CreateTemplateDraftCommand struct {
	WorkspaceID   string
	CreatorType   string
	CreatorID     string
	Definition    looptemplate.Definition
	PolicyVersion string
}

type UpdateTemplateDraftCommand struct {
	WorkspaceID   string
	TemplateKey   string
	Version       int
	Definition    looptemplate.Definition
	PolicyVersion string
}

type PublishTemplateDraftCommand struct {
	WorkspaceID string
	TemplateKey string
	Version     int
}

type TemplateAdminService struct {
	repo TemplateAdminRepository
}

func NewTemplateAdminService(repo TemplateAdminRepository) *TemplateAdminService {
	return &TemplateAdminService{repo: repo}
}

func (s *TemplateAdminService) CreateDraft(ctx context.Context, cmd CreateTemplateDraftCommand) (ManagedTemplate, error) {
	if s == nil || s.repo == nil {
		return ManagedTemplate{}, errors.New("template admin repository is required")
	}
	if err := validateTemplateWrite(cmd.WorkspaceID, cmd.Definition, cmd.PolicyVersion); err != nil {
		return ManagedTemplate{}, err
	}
	if cmd.CreatorType != "member" && cmd.CreatorType != "agent" {
		return ManagedTemplate{}, fmt.Errorf("unsupported template creator type %q", cmd.CreatorType)
	}
	if strings.TrimSpace(cmd.CreatorID) == "" {
		return ManagedTemplate{}, errors.New("template creator id is required")
	}
	cmd.PolicyVersion = strings.TrimSpace(cmd.PolicyVersion)
	return s.repo.CreateDraft(ctx, cmd)
}

func (s *TemplateAdminService) UpdateDraft(ctx context.Context, cmd UpdateTemplateDraftCommand) (ManagedTemplate, error) {
	if s == nil || s.repo == nil {
		return ManagedTemplate{}, errors.New("template admin repository is required")
	}
	if err := validateTemplateWrite(cmd.WorkspaceID, cmd.Definition, cmd.PolicyVersion); err != nil {
		return ManagedTemplate{}, err
	}
	if strings.TrimSpace(cmd.TemplateKey) == "" || cmd.Version <= 0 {
		return ManagedTemplate{}, errors.New("template key and positive version are required")
	}
	if cmd.Definition.Key != cmd.TemplateKey {
		return ManagedTemplate{}, errors.New("template definition key does not match URL key")
	}
	cmd.PolicyVersion = strings.TrimSpace(cmd.PolicyVersion)
	return s.repo.UpdateDraft(ctx, cmd)
}

func (s *TemplateAdminService) PublishDraft(ctx context.Context, cmd PublishTemplateDraftCommand) (ManagedTemplate, error) {
	if s == nil || s.repo == nil {
		return ManagedTemplate{}, errors.New("template admin repository is required")
	}
	if strings.TrimSpace(cmd.WorkspaceID) == "" || strings.TrimSpace(cmd.TemplateKey) == "" || cmd.Version <= 0 {
		return ManagedTemplate{}, errors.New("workspace, template key, and positive version are required")
	}
	return s.repo.PublishDraft(ctx, cmd)
}

func validateTemplateWrite(workspaceID string, definition looptemplate.Definition, policyVersion string) error {
	if strings.TrimSpace(workspaceID) == "" {
		return errors.New("workspace id is required")
	}
	if strings.TrimSpace(policyVersion) == "" {
		return errors.New("policy version is required")
	}
	if err := looptemplate.Validate(definition); err != nil {
		return err
	}
	return nil
}

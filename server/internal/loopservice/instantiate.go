package loopservice

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/multica-ai/multica/server/internal/looptemplate"
)

var (
	ErrInvalidInstantiateRequest = errors.New("invalid loop instantiate request")
	ErrTemplateMismatch          = errors.New("loaded loop template does not match request")
	ErrLoopInstanceExists        = errors.New("loop instance already exists")
)

// PublishedTemplate is the application-layer view of one immutable published
// loop template version. Persistence adapters may populate it from SQLC without
// leaking generated DB types into the pure loop engine.
type PublishedTemplate struct {
	Key           string
	Version       int
	PolicyVersion string
	Definition    looptemplate.Definition
}

// TemplateReader loads the exact published version requested by an instance.
// Implementations must not silently fall back to another version.
type TemplateReader interface {
	LoadPublishedTemplate(ctx context.Context, workspaceID, templateKey string, version int) (PublishedTemplate, error)
}

// BindingScopeValidator proves that a symbolic role binding resolves to an
// existing assignee that is legal in the target workspace. It owns database
// identity/workspace checks that deliberately do not belong in looptemplate.
type BindingScopeValidator interface {
	ValidateBindingScope(ctx context.Context, workspaceID, role string, binding looptemplate.RoleBinding) error
}

// LoopInstanceLookup provides idempotency protection before persistence. The
// eventual write transaction must enforce the same invariant again because a
// preflight check alone cannot prevent concurrent creators.
type LoopInstanceLookup interface {
	LoopInstanceExists(ctx context.Context, workspaceID, projectID, instanceKey string) (bool, error)
}

type InstantiateService struct {
	templates TemplateReader
	bindings  BindingScopeValidator
	instances LoopInstanceLookup
}

func NewInstantiateService(templates TemplateReader, bindings BindingScopeValidator, instances LoopInstanceLookup) *InstantiateService {
	return &InstantiateService{
		templates: templates,
		bindings:  bindings,
		instances: instances,
	}
}

type InstantiatePreflightRequest struct {
	WorkspaceID string
	ProjectID   string
	TemplateKey string
	Version     int
	InstanceKey string
	Bindings    looptemplate.RoleBindings
}

type InstantiatePreflightResult struct {
	Template      PublishedTemplate
	CompiledPlan  looptemplate.CompiledPlan
	RoleBindings  looptemplate.RoleBindings
}

// Preflight performs every deterministic and workspace-scoped check that can
// be completed before the first durable write. It intentionally does not create
// Issues or Tasks. A later persistence adapter consumes CompiledPlan inside a
// transaction and lets normal Multica Issue assignment/dispatch create Tasks.
func (s *InstantiateService) Preflight(ctx context.Context, req InstantiatePreflightRequest) (InstantiatePreflightResult, error) {
	if err := validateInstantiateRequest(req); err != nil {
		return InstantiatePreflightResult{}, err
	}
	if s == nil || s.templates == nil || s.bindings == nil || s.instances == nil {
		return InstantiatePreflightResult{}, fmt.Errorf("%w: instantiate service dependencies are incomplete", ErrInvalidInstantiateRequest)
	}

	tpl, err := s.templates.LoadPublishedTemplate(ctx, req.WorkspaceID, req.TemplateKey, req.Version)
	if err != nil {
		return InstantiatePreflightResult{}, fmt.Errorf("load published loop template: %w", err)
	}
	if tpl.Key != req.TemplateKey || tpl.Version != req.Version || tpl.Definition.Key != req.TemplateKey {
		return InstantiatePreflightResult{}, fmt.Errorf(
			"%w: requested %s@%d, loaded %s@%d with definition key %q",
			ErrTemplateMismatch,
			req.TemplateKey,
			req.Version,
			tpl.Key,
			tpl.Version,
			tpl.Definition.Key,
		)
	}

	bindings := cloneRoleBindings(req.Bindings)
	if err := looptemplate.ValidateBindings(tpl.Definition, bindings); err != nil {
		return InstantiatePreflightResult{}, err
	}

	for role, binding := range bindings {
		if err := s.bindings.ValidateBindingScope(ctx, req.WorkspaceID, role, binding); err != nil {
			return InstantiatePreflightResult{}, fmt.Errorf("validate binding for role %q: %w", role, err)
		}
	}

	plan, err := looptemplate.Compile(tpl.Definition, bindings)
	if err != nil {
		return InstantiatePreflightResult{}, fmt.Errorf("compile loop template: %w", err)
	}

	exists, err := s.instances.LoopInstanceExists(ctx, req.WorkspaceID, req.ProjectID, req.InstanceKey)
	if err != nil {
		return InstantiatePreflightResult{}, fmt.Errorf("check loop instance idempotency: %w", err)
	}
	if exists {
		return InstantiatePreflightResult{}, fmt.Errorf("%w: %q", ErrLoopInstanceExists, req.InstanceKey)
	}

	return InstantiatePreflightResult{
		Template:     tpl,
		CompiledPlan: plan,
		RoleBindings: bindings,
	}, nil
}

func validateInstantiateRequest(req InstantiatePreflightRequest) error {
	missing := make([]string, 0, 5)
	if strings.TrimSpace(req.WorkspaceID) == "" {
		missing = append(missing, "workspace_id")
	}
	if strings.TrimSpace(req.ProjectID) == "" {
		missing = append(missing, "project_id")
	}
	if strings.TrimSpace(req.TemplateKey) == "" {
		missing = append(missing, "template_key")
	}
	if req.Version <= 0 {
		missing = append(missing, "positive version")
	}
	if strings.TrimSpace(req.InstanceKey) == "" {
		missing = append(missing, "instance_key")
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: missing %s", ErrInvalidInstantiateRequest, strings.Join(missing, ", "))
	}
	return nil
}

func cloneRoleBindings(in looptemplate.RoleBindings) looptemplate.RoleBindings {
	if in == nil {
		return looptemplate.RoleBindings{}
	}
	out := make(looptemplate.RoleBindings, len(in))
	for role, binding := range in {
		out[role] = binding
	}
	return out
}

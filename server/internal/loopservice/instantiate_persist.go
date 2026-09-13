package loopservice

import (
	"context"
	"fmt"
	"strings"

	"github.com/multica-ai/multica/server/internal/looptemplate"
)

// LoopInstanceWriter is the durable transaction boundary for one compiled loop
// instance. A concrete Multica adapter must create the parent Issue, all child
// Issues, metadata/provenance and initial stage state atomically enough that a
// partially-created workflow is never exposed as runnable.
//
// Implementations must also enforce instance-key idempotency at write time. The
// earlier Preflight lookup is only an optimization and cannot close the race
// between two concurrent requests.
type LoopInstanceWriter interface {
	PersistLoopInstance(ctx context.Context, req PersistLoopInstanceRequest) (PersistedLoopInstance, error)
}

type PersistLoopInstanceRequest struct {
	WorkspaceID   string
	ProjectID     string
	InstanceKey   string
	Title         string
	Description   string
	TemplateKey   string
	TemplateVersion int
	PolicyVersion string
	Plan          looptemplate.CompiledPlan
	RoleBindings  looptemplate.RoleBindings
}

type PersistedLoopInstance struct {
	ParentIssueID string
	NodeIssueIDs  map[string]string
}

type InstantiateRequest struct {
	WorkspaceID string
	ProjectID   string
	TemplateKey string
	Version     int
	InstanceKey string
	Title       string
	Description string
	Bindings    looptemplate.RoleBindings
}

type InstantiateResult struct {
	Template     PublishedTemplate
	CompiledPlan looptemplate.CompiledPlan
	Instance     PersistedLoopInstance
}

// Instantiator composes the side-effect-free preflight phase with one explicit
// persistence boundary. It still never creates Tasks directly: child Issue
// activation must flow through the existing Multica Issue assignment/dispatch
// semantics inside the concrete writer/adapter.
type Instantiator struct {
	preflight *InstantiateService
	writer    LoopInstanceWriter
}

func NewInstantiator(preflight *InstantiateService, writer LoopInstanceWriter) *Instantiator {
	return &Instantiator{preflight: preflight, writer: writer}
}

func (s *Instantiator) Instantiate(ctx context.Context, req InstantiateRequest) (InstantiateResult, error) {
	if s == nil || s.preflight == nil || s.writer == nil {
		return InstantiateResult{}, fmt.Errorf("%w: instantiator dependencies are incomplete", ErrInvalidInstantiateRequest)
	}
	if strings.TrimSpace(req.Title) == "" {
		return InstantiateResult{}, fmt.Errorf("%w: missing title", ErrInvalidInstantiateRequest)
	}

	preflight, err := s.preflight.Preflight(ctx, InstantiatePreflightRequest{
		WorkspaceID: req.WorkspaceID,
		ProjectID:   req.ProjectID,
		TemplateKey: req.TemplateKey,
		Version:     req.Version,
		InstanceKey: req.InstanceKey,
		Bindings:    req.Bindings,
	})
	if err != nil {
		return InstantiateResult{}, err
	}

	persisted, err := s.writer.PersistLoopInstance(ctx, PersistLoopInstanceRequest{
		WorkspaceID:     req.WorkspaceID,
		ProjectID:       req.ProjectID,
		InstanceKey:     req.InstanceKey,
		Title:           strings.TrimSpace(req.Title),
		Description:     req.Description,
		TemplateKey:     preflight.Template.Key,
		TemplateVersion: preflight.Template.Version,
		PolicyVersion:   preflight.Template.PolicyVersion,
		Plan:            preflight.CompiledPlan,
		RoleBindings:    preflight.RoleBindings,
	})
	if err != nil {
		return InstantiateResult{}, fmt.Errorf("persist loop instance: %w", err)
	}
	if strings.TrimSpace(persisted.ParentIssueID) == "" {
		return InstantiateResult{}, fmt.Errorf("persist loop instance: writer returned empty parent issue id")
	}

	return InstantiateResult{
		Template:     preflight.Template,
		CompiledPlan: preflight.CompiledPlan,
		Instance:     persisted,
	}, nil
}

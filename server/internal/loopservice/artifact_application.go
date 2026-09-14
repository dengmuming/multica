package loopservice

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var allowedLoopArtifactTypes = map[string]bool{
	"requirement": true, "product_spec": true, "architecture": true, "api_spec": true,
	"code_change": true, "commit": true, "pull_request": true,
	"test_plan": true, "test_result": true, "review_report": true,
	"build": true, "deployment": true, "runtime_evidence": true, "external_document": true,
}

var allowedLoopArtifactRelations = map[string]bool{
	"input": true, "output": true, "evidence": true, "supersedes": true, "derived_from": true,
}

var allowedLoopArtifactRefKinds = map[string]bool{
	"issue": true, "task": true, "attachment": true, "source_context": true,
	"commit": true, "pull_request": true, "build": true, "deployment": true, "url": true,
}

type RegisterArtifactRequest struct {
	WorkspaceID   string
	ParentIssueID string
	ActorType     string
	ActorID       string
	AuthTaskID    string
	NodeIssueID   string
	TaskID        string
	ArtifactType  string
	Relation      string
	Title         string
	RefKind       string
	RefID         string
	RefURI        string
	Metadata      map[string]any
}

type RegisterArtifactCommand struct {
	WorkspaceID   string
	ParentIssueID string
	NodeIssueID   string
	TaskID        string
	ArtifactType  string
	Relation      string
	Title         string
	RefKind       string
	RefID         string
	RefURI        string
	Metadata      map[string]any
	CreatedByType string
	CreatedByID   string
}

type RegisteredArtifact struct {
	ID            string         `json:"id"`
	WorkspaceID   string         `json:"workspace_id"`
	ParentIssueID string         `json:"parent_issue_id"`
	NodeIssueID   string         `json:"node_issue_id,omitempty"`
	TaskID        string         `json:"task_id,omitempty"`
	ArtifactType  string         `json:"artifact_type"`
	Relation      string         `json:"relation"`
	Title         string         `json:"title,omitempty"`
	RefKind       string         `json:"ref_kind"`
	RefID         string         `json:"ref_id,omitempty"`
	RefURI        string         `json:"ref_uri,omitempty"`
	Metadata      map[string]any `json:"metadata"`
	CreatedByType string         `json:"created_by_type"`
	CreatedByID   string         `json:"created_by_id,omitempty"`
}

type ArtifactRepository interface {
	RegisterArtifact(ctx context.Context, cmd RegisterArtifactCommand) (RegisteredArtifact, error)
	ResolveAgentTaskArtifactContext(ctx context.Context, workspaceID, parentIssueID, taskID, agentID string) (nodeIssueID string, err error)
	ValidateHumanArtifactContext(ctx context.Context, workspaceID, parentIssueID, nodeIssueID, taskID string) error
}

type ArtifactApplicationService struct {
	repo ArtifactRepository
}

func NewArtifactApplicationService(repo ArtifactRepository) *ArtifactApplicationService {
	return &ArtifactApplicationService{repo: repo}
}

func (s *ArtifactApplicationService) Register(ctx context.Context, req RegisterArtifactRequest) (RegisteredArtifact, error) {
	if s == nil || s.repo == nil {
		return RegisteredArtifact{}, errors.New("artifact repository is required")
	}
	trimArtifactRequest(&req)
	if req.WorkspaceID == "" || req.ParentIssueID == "" || req.ActorID == "" {
		return RegisteredArtifact{}, errors.New("workspace, parent issue, and authenticated actor are required")
	}
	if req.ActorType != "member" && req.ActorType != "agent" {
		return RegisteredArtifact{}, fmt.Errorf("unsupported artifact actor type %q", req.ActorType)
	}
	if !allowedLoopArtifactTypes[req.ArtifactType] {
		return RegisteredArtifact{}, fmt.Errorf("unsupported artifact type %q", req.ArtifactType)
	}
	if !allowedLoopArtifactRelations[req.Relation] {
		return RegisteredArtifact{}, fmt.Errorf("unsupported artifact relation %q", req.Relation)
	}
	if !allowedLoopArtifactRefKinds[req.RefKind] {
		return RegisteredArtifact{}, fmt.Errorf("unsupported artifact ref kind %q", req.RefKind)
	}
	if req.RefID == "" && req.RefURI == "" {
		return RegisteredArtifact{}, errors.New("artifact requires ref_id or ref_uri")
	}

	if req.ActorType == "agent" {
		if req.AuthTaskID == "" {
			return RegisteredArtifact{}, errors.New("agent artifact registration requires task-token context")
		}
		if req.TaskID != "" && req.TaskID != req.AuthTaskID {
			return RegisteredArtifact{}, errors.New("task token is not authorized for requested artifact task")
		}
		req.TaskID = req.AuthTaskID
		nodeIssueID, err := s.repo.ResolveAgentTaskArtifactContext(ctx, req.WorkspaceID, req.ParentIssueID, req.TaskID, req.ActorID)
		if err != nil {
			return RegisteredArtifact{}, err
		}
		if req.NodeIssueID != "" && req.NodeIssueID != nodeIssueID {
			return RegisteredArtifact{}, errors.New("task token is not authorized for requested artifact node")
		}
		req.NodeIssueID = nodeIssueID
	} else if err := s.repo.ValidateHumanArtifactContext(ctx, req.WorkspaceID, req.ParentIssueID, req.NodeIssueID, req.TaskID); err != nil {
		return RegisteredArtifact{}, err
	}

	return s.repo.RegisterArtifact(ctx, RegisterArtifactCommand{
		WorkspaceID: req.WorkspaceID, ParentIssueID: req.ParentIssueID,
		NodeIssueID: req.NodeIssueID, TaskID: req.TaskID,
		ArtifactType: req.ArtifactType, Relation: req.Relation, Title: req.Title,
		RefKind: req.RefKind, RefID: req.RefID, RefURI: req.RefURI,
		Metadata: cloneAnyMap(req.Metadata), CreatedByType: req.ActorType, CreatedByID: req.ActorID,
	})
}

func trimArtifactRequest(req *RegisterArtifactRequest) {
	if req == nil { return }
	req.WorkspaceID = strings.TrimSpace(req.WorkspaceID)
	req.ParentIssueID = strings.TrimSpace(req.ParentIssueID)
	req.ActorType = strings.TrimSpace(req.ActorType)
	req.ActorID = strings.TrimSpace(req.ActorID)
	req.AuthTaskID = strings.TrimSpace(req.AuthTaskID)
	req.NodeIssueID = strings.TrimSpace(req.NodeIssueID)
	req.TaskID = strings.TrimSpace(req.TaskID)
	req.ArtifactType = strings.TrimSpace(req.ArtifactType)
	req.Relation = strings.TrimSpace(req.Relation)
	req.Title = strings.TrimSpace(req.Title)
	req.RefKind = strings.TrimSpace(req.RefKind)
	req.RefID = strings.TrimSpace(req.RefID)
	req.RefURI = strings.TrimSpace(req.RefURI)
}

func cloneAnyMap(in map[string]any) map[string]any {
	if in == nil { return map[string]any{} }
	out := make(map[string]any, len(in))
	for k, v := range in { out[k] = v }
	return out
}

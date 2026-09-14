package loopservice

import (
	"context"
	"errors"
	"strings"
	"time"
)

var ErrLoopNotFound = errors.New("loop not found")

type LoopSummary struct {
	ParentIssueID  string    `json:"parent_issue_id"`
	ProjectID      string    `json:"project_id,omitempty"`
	Title          string    `json:"title"`
	Description    string    `json:"description,omitempty"`
	IssueStatus    string    `json:"issue_status"`
	State          string    `json:"state"`
	TemplateKey    string    `json:"template_key"`
	TemplateVersion int      `json:"template_version"`
	PolicyVersion  string    `json:"policy_version"`
	InstanceKey    string    `json:"instance_key"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type LoopNodeRead struct {
	IssueID       string `json:"issue_id"`
	NodeKey       string `json:"node_key"`
	NodeType      string `json:"node_type"`
	Role          string `json:"role,omitempty"`
	Title         string `json:"title"`
	Description   string `json:"description,omitempty"`
	Status        string `json:"status"`
	Stage         int    `json:"stage"`
	Required      bool   `json:"required"`
	RetryCount    int    `json:"retry_count"`
	MaxRetries    int    `json:"max_retries"`
	AssigneeType  string `json:"assignee_type,omitempty"`
	AssigneeID    string `json:"assignee_id,omitempty"`
	LatestTaskID  string `json:"latest_task_id,omitempty"`
	LatestTaskStatus string `json:"latest_task_status,omitempty"`
	EvaluationVerdict string `json:"evaluation_verdict,omitempty"`
	EvaluationTarget  string `json:"evaluation_target,omitempty"`
	ApprovalState     string `json:"approval_state,omitempty"`
}

type LoopDetail struct {
	LoopSummary
	Nodes []LoopNodeRead `json:"nodes"`
}

type ListLoopsQuery struct {
	WorkspaceID string
	ProjectID   string
	Limit       int
}

type LoopReadRepository interface {
	ListLoops(context.Context, ListLoopsQuery) ([]LoopSummary, error)
	GetLoop(context.Context, string, string) (LoopDetail, error)
}

type LoopReadService struct {
	repo LoopReadRepository
}

func NewLoopReadService(repo LoopReadRepository) *LoopReadService {
	return &LoopReadService{repo: repo}
}

func (s *LoopReadService) List(ctx context.Context, query ListLoopsQuery) ([]LoopSummary, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("loop read repository is required")
	}
	query.WorkspaceID = strings.TrimSpace(query.WorkspaceID)
	query.ProjectID = strings.TrimSpace(query.ProjectID)
	if query.WorkspaceID == "" {
		return nil, errors.New("workspace id is required")
	}
	if query.Limit <= 0 {
		query.Limit = 50
	}
	if query.Limit > 100 {
		query.Limit = 100
	}
	return s.repo.ListLoops(ctx, query)
}

func (s *LoopReadService) Get(ctx context.Context, workspaceID, parentIssueID string) (LoopDetail, error) {
	if s == nil || s.repo == nil {
		return LoopDetail{}, errors.New("loop read repository is required")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	parentIssueID = strings.TrimSpace(parentIssueID)
	if workspaceID == "" || parentIssueID == "" {
		return LoopDetail{}, errors.New("workspace and parent issue id are required")
	}
	return s.repo.GetLoop(ctx, workspaceID, parentIssueID)
}

package loopservice

import (
	"context"
	"fmt"
	"strings"

	"github.com/multica-ai/multica/server/internal/looptemplate"
)

const (
	loopParentInitialStatus = "backlog"
	loopIssuePriority       = "none"
)

// IssueGraphGateway is the Multica-facing boundary used by the loop writer.
//
// The concrete adapter must preserve the ordinary Issue creation semantics of
// the application, including assignment-triggered dispatch for runnable child
// Issues. MNL deliberately does not insert Tasks itself and should not bypass
// the canonical Issue command path merely to make graph creation convenient.
//
// CreateLoopIssueGraph must provide one logical all-or-nothing operation: a
// failed graph must not leave a subset of runnable child Issues visible. It
// must also enforce (workspace_id, instance_key) idempotency at write time and
// return ErrLoopInstanceExists when another request wins that race.
type IssueGraphGateway interface {
	CreateLoopIssueGraph(ctx context.Context, cmd CreateLoopIssueGraphCommand) (CreatedIssueGraph, error)
}

type CreateLoopIssueGraphCommand struct {
	WorkspaceID string
	ProjectID   string
	InstanceKey string
	Parent      LoopIssueCommand
	Children    []LoopIssueCommand
}

type CreatedIssueGraph struct {
	ParentIssueID string
	NodeIssueIDs  map[string]string
}

// LoopIssueCommand is intentionally close to Multica's existing Issue write
// shape while keeping pgx/sqlc types out of the loop application package.
type LoopIssueCommand struct {
	NodeKey      string
	Title        string
	Description  string
	Status       string
	Priority     string
	CreatorType  string
	CreatorID    string
	AssigneeType string
	AssigneeID   string
	Stage        *int
	Metadata     map[string]any
}

type MulticaIssueWriter struct {
	gateway IssueGraphGateway
}

func NewMulticaIssueWriter(gateway IssueGraphGateway) *MulticaIssueWriter {
	return &MulticaIssueWriter{gateway: gateway}
}

func (w *MulticaIssueWriter) PersistLoopInstance(ctx context.Context, req PersistLoopInstanceRequest) (PersistedLoopInstance, error) {
	if w == nil || w.gateway == nil {
		return PersistedLoopInstance{}, fmt.Errorf("issue graph gateway is required")
	}
	cmd, err := BuildIssueGraphCommand(req)
	if err != nil {
		return PersistedLoopInstance{}, err
	}
	created, err := w.gateway.CreateLoopIssueGraph(ctx, cmd)
	if err != nil {
		return PersistedLoopInstance{}, err
	}
	if strings.TrimSpace(created.ParentIssueID) == "" {
		return PersistedLoopInstance{}, fmt.Errorf("issue graph gateway returned empty parent issue id")
	}
	return PersistedLoopInstance{
		ParentIssueID: created.ParentIssueID,
		NodeIssueIDs:  cloneStringMap(created.NodeIssueIDs),
	}, nil
}

// BuildIssueGraphCommand translates the deterministic compiler output into the
// exact Parent Issue + staged Child Issue graph MNL expects Multica to create.
// It is pure so graph semantics can be exhaustively tested without a database.
func BuildIssueGraphCommand(req PersistLoopInstanceRequest) (CreateLoopIssueGraphCommand, error) {
	if strings.TrimSpace(req.WorkspaceID) == "" {
		return CreateLoopIssueGraphCommand{}, fmt.Errorf("workspace id is required")
	}
	if strings.TrimSpace(req.ProjectID) == "" {
		return CreateLoopIssueGraphCommand{}, fmt.Errorf("project id is required")
	}
	if strings.TrimSpace(req.InstanceKey) == "" {
		return CreateLoopIssueGraphCommand{}, fmt.Errorf("instance key is required")
	}
	if strings.TrimSpace(req.Title) == "" {
		return CreateLoopIssueGraphCommand{}, fmt.Errorf("title is required")
	}
	if (req.CreatorType != "member" && req.CreatorType != "agent") || strings.TrimSpace(req.CreatorID) == "" {
		return CreateLoopIssueGraphCommand{}, fmt.Errorf("trusted creator identity is required")
	}
	if strings.TrimSpace(req.TemplateKey) == "" || req.TemplateVersion <= 0 {
		return CreateLoopIssueGraphCommand{}, fmt.Errorf("pinned template key/version are required")
	}
	if len(req.Plan.IncludedNodes) == 0 {
		return CreateLoopIssueGraphCommand{}, fmt.Errorf("compiled plan contains no included nodes")
	}

	parent := LoopIssueCommand{
		Title:       strings.TrimSpace(req.Title),
		Description: req.Description,
		Status:      loopParentInitialStatus,
		Priority:    loopIssuePriority,
		CreatorType: req.CreatorType,
		CreatorID:   req.CreatorID,
		Metadata: map[string]any{
			"manifold.loop.template_key":     req.TemplateKey,
			"manifold.loop.template_version": req.TemplateVersion,
			"manifold.loop.instance_key":     req.InstanceKey,
			"manifold.loop.state":            "running",
			"manifold.loop.policy_version":   req.PolicyVersion,
			"manifold.loop.retry_count":      0,
		},
	}

	children := make([]LoopIssueCommand, 0, len(req.Plan.IncludedNodes))
	seenNodeKeys := make(map[string]struct{}, len(req.Plan.IncludedNodes))
	for _, node := range req.Plan.IncludedNodes {
		if _, duplicate := seenNodeKeys[node.Key]; duplicate {
			return CreateLoopIssueGraphCommand{}, fmt.Errorf("compiled plan repeats node key %q", node.Key)
		}
		seenNodeKeys[node.Key] = struct{}{}

		child, err := buildChildIssueCommand(node, req.CreatorType, req.CreatorID)
		if err != nil {
			return CreateLoopIssueGraphCommand{}, err
		}
		children = append(children, child)
	}

	return CreateLoopIssueGraphCommand{
		WorkspaceID: req.WorkspaceID,
		ProjectID:   req.ProjectID,
		InstanceKey: req.InstanceKey,
		Parent:      parent,
		Children:    children,
	}, nil
}

func buildChildIssueCommand(node looptemplate.CompiledNode, creatorType, creatorID string) (LoopIssueCommand, error) {
	if strings.TrimSpace(node.Key) == "" {
		return LoopIssueCommand{}, fmt.Errorf("compiled node key is required")
	}
	if node.Stage <= 0 {
		return LoopIssueCommand{}, fmt.Errorf("compiled node %q has invalid stage %d", node.Key, node.Stage)
	}
	if node.InitialStatus != "todo" && node.InitialStatus != "backlog" {
		return LoopIssueCommand{}, fmt.Errorf("compiled node %q has unsupported initial status %q", node.Key, node.InitialStatus)
	}

	stage := node.Stage
	metadata := map[string]any{
		"manifold.loop.node_key":    node.Key,
		"manifold.loop.node_type":   node.Type,
		"manifold.loop.retry_count": 0,
		"manifold.loop.generated":   true,
		"manifold.loop.required":    node.Required,
		"manifold.loop.max_retries": node.MaxRetries,
	}
	if node.Role != "" {
		metadata["manifold.loop.role"] = node.Role
	}

	child := LoopIssueCommand{
		NodeKey:      node.Key,
		Title:        node.Title,
		Description:  node.Description,
		Status:       node.InitialStatus,
		Priority:     loopIssuePriority,
		CreatorType:  creatorType,
		CreatorID:    creatorID,
		Stage:        &stage,
		Metadata:     metadata,
	}

	if node.Type == looptemplate.NodeTypeApproval {
		// Approval is a server-owned human gate. It is intentionally not encoded
		// as a normal member assignment because an Issue assignee must never be
		// treated as an approval decision.
		return child, nil
	}
	if node.Binding == nil {
		return LoopIssueCommand{}, fmt.Errorf("compiled executable node %q has no binding", node.Key)
	}
	child.AssigneeType = node.Binding.Type
	child.AssigneeID = node.Binding.ID
	return child, nil
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

package loopservice

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/looppolicy"
	"github.com/multica-ai/multica/server/internal/looptemplate"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type PinnedLoopTemplateLoader interface {
	LoadPinnedLoopTemplate(ctx context.Context, workspaceID, templateKey string, version int) (PinnedLoopTemplate, error)
}

type PinnedLoopTemplate struct {
	Definition    looptemplate.Definition
	PolicyVersion string
}

type GateStateProjectionLoader interface {
	LoadGateStates(ctx context.Context, workspaceID, parentIssueID string, nodeIssueIDs map[string]string) (GateStateProjection, error)
}

type GateStateProjection struct {
	Evaluations map[string]looppolicy.EvaluationState
	Approvals   map[string]looppolicy.ApprovalState
}

type policyIssueReader interface {
	GetIssueInWorkspace(context.Context, db.GetIssueInWorkspaceParams) (db.Issue, error)
	ListChildIssues(context.Context, pgtype.UUID) ([]db.Issue, error)
	ResolveIssueWorkspace(context.Context, pgtype.UUID) (pgtype.UUID, error)
}

// MulticaPolicyProjectionReader reconstructs the policy input from durable
// Issues plus the exact pinned template and authoritative gate records.
type MulticaPolicyProjectionReader struct {
	issues    policyIssueReader
	templates PinnedLoopTemplateLoader
	gates     GateStateProjectionLoader
}

func NewMulticaPolicyProjectionReader(issues policyIssueReader, templates PinnedLoopTemplateLoader, gates GateStateProjectionLoader) *MulticaPolicyProjectionReader {
	return &MulticaPolicyProjectionReader{issues: issues, templates: templates, gates: gates}
}

func (r *MulticaPolicyProjectionReader) LoadPolicyProjection(ctx context.Context, parentIssueID string) (PolicyProjection, error) {
	if r == nil || r.issues == nil || r.templates == nil || r.gates == nil {
		return PolicyProjection{}, errors.New("policy projection reader dependencies are incomplete")
	}
	parentID, err := parseRequiredUUID("parent issue id", parentIssueID)
	if err != nil {
		return PolicyProjection{}, err
	}
	workspaceID, err := r.issues.ResolveIssueWorkspace(ctx, parentID)
	if err != nil || !workspaceID.Valid {
		return PolicyProjection{}, errors.New("loop parent issue not found")
	}
	parent, err := r.issues.GetIssueInWorkspace(ctx, db.GetIssueInWorkspaceParams{ID: parentID, WorkspaceID: workspaceID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PolicyProjection{}, errors.New("loop parent issue not found in workspace")
		}
		return PolicyProjection{}, err
	}
	parentMetadata, err := decodeIssueMetadata(parent.Metadata)
	if err != nil {
		return PolicyProjection{}, fmt.Errorf("decode loop parent metadata: %w", err)
	}
	templateKey := metadataString(parentMetadata, "manifold.loop.template_key")
	templateVersion := metadataPositiveInt(parentMetadata, "manifold.loop.template_version")
	policyVersion := metadataString(parentMetadata, "manifold.loop.policy_version")
	parentState := metadataString(parentMetadata, "manifold.loop.state")
	if templateKey == "" || templateVersion <= 0 || policyVersion == "" || parentState == "" {
		return PolicyProjection{}, errors.New("loop parent metadata is incomplete")
	}

	workspaceIDString := uuidString(workspaceID)
	pinned, err := r.templates.LoadPinnedLoopTemplate(ctx, workspaceIDString, templateKey, templateVersion)
	if err != nil {
		return PolicyProjection{}, fmt.Errorf("load pinned loop template: %w", err)
	}
	if pinned.PolicyVersion != "" && pinned.PolicyVersion != policyVersion {
		return PolicyProjection{}, fmt.Errorf("pinned template policy version %q does not match parent %q", pinned.PolicyVersion, policyVersion)
	}

	children, err := r.issues.ListChildIssues(ctx, parentID)
	if err != nil {
		return PolicyProjection{}, fmt.Errorf("list loop child issues: %w", err)
	}
	bindings := make(looptemplate.RoleBindings)
	childByNode := make(map[string]db.Issue, len(children))
	for _, child := range children {
		if child.WorkspaceID != workspaceID {
			return PolicyProjection{}, errors.New("loop child workspace mismatch")
		}
		metadata, err := decodeIssueMetadata(child.Metadata)
		if err != nil {
			return PolicyProjection{}, fmt.Errorf("decode child metadata: %w", err)
		}
		nodeKey := metadataString(metadata, "manifold.loop.node_key")
		if nodeKey == "" {
			continue
		}
		if _, duplicate := childByNode[nodeKey]; duplicate {
			return PolicyProjection{}, fmt.Errorf("duplicate persisted loop node key %q", nodeKey)
		}
		childByNode[nodeKey] = child
		role := metadataString(metadata, "manifold.loop.role")
		if role != "" && child.AssigneeType.Valid && child.AssigneeID.Valid {
			bindings[role] = looptemplate.RoleBinding{Type: child.AssigneeType.String, ID: uuidString(child.AssigneeID)}
		}
	}

	plan, err := looptemplate.Compile(pinned.Definition, bindings)
	if err != nil {
		return PolicyProjection{}, fmt.Errorf("recompile pinned loop plan: %w", err)
	}
	nodeIssueIDs := make(map[string]string, len(plan.IncludedNodes))
	nodeRevisions := make(map[string]int64, len(plan.IncludedNodes))
	runtimeNodes := make(map[string]looppolicy.NodeState, len(plan.IncludedNodes))
	for _, node := range plan.IncludedNodes {
		child, ok := childByNode[node.Key]
		if !ok {
			return PolicyProjection{}, fmt.Errorf("persisted loop is missing node issue %q", node.Key)
		}
		metadata, err := decodeIssueMetadata(child.Metadata)
		if err != nil {
			return PolicyProjection{}, err
		}
		nodeIssueIDs[node.Key] = uuidString(child.ID)
		nodeRevisions[node.Key] = child.Revision
		runtimeNodes[node.Key] = looppolicy.NodeState{Status: child.Status, RetryCount: metadataInt(metadata, "manifold.loop.retry_count")}
	}

	gateState, err := r.gates.LoadGateStates(ctx, workspaceIDString, parentIssueID, nodeIssueIDs)
	if err != nil {
		return PolicyProjection{}, fmt.Errorf("load loop gate states: %w", err)
	}
	parentRevision := parent.Revision
	return PolicyProjection{
		WorkspaceID: workspaceIDString,
		ParentIssueID: parentIssueID,
		PolicyVersion: policyVersion,
		Plan: plan,
		Runtime: looppolicy.RuntimeState{
			ParentState: parentState,
			Nodes: runtimeNodes,
			Evaluations: gateState.Evaluations,
			Approvals: gateState.Approvals,
		},
		NodeIssueIDs: nodeIssueIDs,
		ExpectedParentRevision: &parentRevision,
		ExpectedNodeRevisions: nodeRevisions,
	}, nil
}

// DBPolicyIssueReader adapts existing generated Issue queries. Workspace
// discovery is immediately followed by workspace-scoped reads; no mutation is
// ever authorized by the unscoped lookup.
type DBPolicyIssueReader struct {
	Queries *db.Queries
}

func (r DBPolicyIssueReader) ResolveIssueWorkspace(ctx context.Context, id pgtype.UUID) (pgtype.UUID, error) {
	if r.Queries == nil {
		return pgtype.UUID{}, errors.New("issue queries are required")
	}
	issue, err := r.Queries.GetIssue(ctx, id)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return issue.WorkspaceID, nil
}

func (r DBPolicyIssueReader) GetIssueInWorkspace(ctx context.Context, p db.GetIssueInWorkspaceParams) (db.Issue, error) {
	return r.Queries.GetIssueInWorkspace(ctx, p)
}

func (r DBPolicyIssueReader) ListChildIssues(ctx context.Context, parentID pgtype.UUID) ([]db.Issue, error) {
	return r.Queries.ListChildIssues(ctx, parentID)
}

func metadataPositiveInt(metadata map[string]any, key string) int {
	if n := metadataInt(metadata, key); n > 0 {
		return n
	}
	if raw, ok := metadata[key].(string); ok {
		n, _ := strconv.Atoi(strings.TrimSpace(raw))
		if n > 0 {
			return n
		}
	}
	return 0
}

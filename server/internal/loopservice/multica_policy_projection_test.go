package loopservice

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/looppolicy"
	"github.com/multica-ai/multica/server/internal/looptemplate"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type fakePolicyIssueReader struct {
	workspace pgtype.UUID
	parent    db.Issue
	children  []db.Issue
}

func (f fakePolicyIssueReader) ResolveIssueWorkspace(context.Context, pgtype.UUID) (pgtype.UUID, error) {
	return f.workspace, nil
}
func (f fakePolicyIssueReader) GetIssueInWorkspace(context.Context, db.GetIssueInWorkspaceParams) (db.Issue, error) {
	return f.parent, nil
}
func (f fakePolicyIssueReader) ListChildIssues(context.Context, pgtype.UUID) ([]db.Issue, error) {
	return f.children, nil
}

type fakePinnedTemplateLoader struct {
	gotKey     string
	gotVersion int
	value      PinnedLoopTemplate
}

func (f *fakePinnedTemplateLoader) LoadPinnedLoopTemplate(_ context.Context, _ string, key string, version int) (PinnedLoopTemplate, error) {
	f.gotKey, f.gotVersion = key, version
	return f.value, nil
}

type fakeGateProjectionLoader struct{ value GateStateProjection }
func (f fakeGateProjectionLoader) LoadGateStates(context.Context, string, string, map[string]string) (GateStateProjection, error) {
	return f.value, nil
}

func TestPolicyProjectionRebuildsPinnedPlanAndRevisionSnapshot(t *testing.T) {
	workspace := testPGUUID(1)
	parentID := testPGUUID(2)
	backendID := testPGUUID(3)
	approvalID := testPGUUID(4)
	agentID := testPGUUID(5)

	def := looptemplate.Definition{
		SchemaVersion: 1,
		Key: "feature-development",
		Name: "Feature Development",
		Roles: map[string]looptemplate.RoleDefinition{
			"backend": {Required: true, AllowedAssigneeTypes: []string{"agent"}},
		},
		Nodes: []looptemplate.Node{
			{Key: "backend", Type: looptemplate.NodeTypeAgent, Stage: 10, Role: "backend", Title: "Backend"},
			{Key: "approval", Type: looptemplate.NodeTypeApproval, Stage: 20, Title: "Approve", Approval: &looptemplate.ApprovalConfig{RequiredRole: "member", MinApprovals: 1}},
		},
	}
	parentMeta := mustJSON(t, map[string]any{
		"manifold.loop.template_key": "feature-development",
		"manifold.loop.template_version": 7,
		"manifold.loop.policy_version": "policy-v3",
		"manifold.loop.state": "running",
	})
	backendMeta := mustJSON(t, map[string]any{
		"manifold.loop.node_key": "backend",
		"manifold.loop.role": "backend",
		"manifold.loop.retry_count": 2,
	})
	approvalMeta := mustJSON(t, map[string]any{"manifold.loop.node_key": "approval", "manifold.loop.retry_count": 0})

	reader := fakePolicyIssueReader{
		workspace: workspace,
		parent: db.Issue{ID: parentID, WorkspaceID: workspace, Metadata: parentMeta, Revision: 11},
		children: []db.Issue{
			{ID: backendID, WorkspaceID: workspace, ParentIssueID: parentID, Status: "done", AssigneeType: pgtype.Text{String: "agent", Valid: true}, AssigneeID: agentID, Metadata: backendMeta, Revision: 21},
			{ID: approvalID, WorkspaceID: workspace, ParentIssueID: parentID, Status: "backlog", Metadata: approvalMeta, Revision: 22},
		},
	}
	templates := &fakePinnedTemplateLoader{value: PinnedLoopTemplate{Definition: def, PolicyVersion: "policy-v3"}}
	gates := fakeGateProjectionLoader{value: GateStateProjection{Approvals: map[string]looppolicy.ApprovalState{}}}
	service := NewMulticaPolicyProjectionReader(reader, templates, gates)

	projection, err := service.LoadPolicyProjection(context.Background(), uuidString(parentID))
	if err != nil {
		t.Fatalf("LoadPolicyProjection() error = %v", err)
	}
	if templates.gotKey != "feature-development" || templates.gotVersion != 7 {
		t.Fatalf("template lookup = %s@%d", templates.gotKey, templates.gotVersion)
	}
	if projection.PolicyVersion != "policy-v3" || projection.Runtime.ParentState != "running" {
		t.Fatalf("projection = %#v", projection)
	}
	if projection.ExpectedParentRevision == nil || *projection.ExpectedParentRevision != 11 {
		t.Fatalf("parent revision = %#v", projection.ExpectedParentRevision)
	}
	if projection.ExpectedNodeRevisions["backend"] != 21 || projection.ExpectedNodeRevisions["approval"] != 22 {
		t.Fatalf("node revisions = %#v", projection.ExpectedNodeRevisions)
	}
	if projection.Runtime.Nodes["backend"].RetryCount != 2 {
		t.Fatalf("backend runtime = %#v", projection.Runtime.Nodes["backend"])
	}
	if len(projection.Plan.IncludedNodes) != 2 || projection.Plan.IncludedNodes[0].Key != "backend" {
		t.Fatalf("compiled plan = %#v", projection.Plan)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil { t.Fatal(err) }
	return encoded
}

func testPGUUID(seed byte) pgtype.UUID {
	var b [16]byte
	b[15] = seed
	return pgtype.UUID{Bytes: b, Valid: true}
}

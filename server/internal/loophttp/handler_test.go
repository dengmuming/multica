package loophttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/loopevaluation"
	"github.com/multica-ai/multica/server/internal/looppolicy"
	"github.com/multica-ai/multica/server/internal/loopservice"
	"github.com/multica-ai/multica/server/internal/looptemplate"
)

type fakeInstantiator struct {
	calls int
	last  loopservice.InstantiateRequest
	value loopservice.InstantiateResult
	err   error
}

func (f *fakeInstantiator) Instantiate(_ context.Context, req loopservice.InstantiateRequest) (loopservice.InstantiateResult, error) {
	f.calls++
	f.last = req
	return f.value, f.err
}

type fakeEvaluationSubmitter struct {
	calls int
	last  loopservice.SubmitEvaluationRequest
	value loopservice.SubmitEvaluationResult
	err   error
}

func (f *fakeEvaluationSubmitter) Submit(_ context.Context, req loopservice.SubmitEvaluationRequest) (loopservice.SubmitEvaluationResult, error) {
	f.calls++
	f.last = req
	return f.value, f.err
}

type fakeApprovalDecider struct {
	calls int
	last  loopservice.DecideApprovalRequest
	value loopservice.DecideApprovalResult
	err   error
}

func (f *fakeApprovalDecider) Decide(_ context.Context, req loopservice.DecideApprovalRequest) (loopservice.DecideApprovalResult, error) {
	f.calls++
	f.last = req
	return f.value, f.err
}

func testRouter(h *Handler) http.Handler {
	r := chi.NewRouter()
	h.Register(r)
	return r
}

func requestJSON(t *testing.T, method, path string, body any) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestCreateLoopUsesAuthenticatedWorkspaceContext(t *testing.T) {
	inst := &fakeInstantiator{value: loopservice.InstantiateResult{
		Template: loopservice.PublishedTemplate{Key: "feature-development", Version: 3},
		Instance: loopservice.PersistedLoopInstance{
			ParentIssueID: "parent-1",
			NodeIssueIDs: map[string]string{"backend": "issue-backend"},
		},
	}}
	h := New(inst, nil, nil)
	req := requestJSON(t, http.MethodPost, "/api/loops", map[string]any{
		"project_id":   "project-1",
		"template_key": "feature-development",
		"version":      3,
		"instance_key": "feature-123",
		"title":        "Token licensing",
		"bindings": map[string]any{
			"backend": map[string]any{"type": "agent", "id": "agent-1"},
		},
	})
	req.Header.Set("X-Workspace-ID", "workspace-server-stamped")
	rr := httptest.NewRecorder()

	testRouter(h).ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if inst.calls != 1 || inst.last.WorkspaceID != "workspace-server-stamped" {
		t.Fatalf("instantiator = calls=%d req=%#v", inst.calls, inst.last)
	}
	if inst.last.Bindings["backend"] != (looptemplate.RoleBinding{Type: "agent", ID: "agent-1"}) {
		t.Fatalf("bindings = %#v", inst.last.Bindings)
	}
}

func TestCreateLoopDuplicateReturnsConflict(t *testing.T) {
	inst := &fakeInstantiator{err: loopservice.ErrLoopInstanceExists}
	h := New(inst, nil, nil)
	req := requestJSON(t, http.MethodPost, "/api/loops", map[string]any{
		"project_id": "project-1", "template_key": "feature-development", "version": 1,
		"instance_key": "duplicate", "title": "Duplicate", "bindings": map[string]any{},
	})
	req.Header.Set("X-Workspace-ID", "workspace-1")
	rr := httptest.NewRecorder()

	testRouter(h).ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestEvaluationTaskTokenIdentityCannotCrossTask(t *testing.T) {
	evals := &fakeEvaluationSubmitter{}
	h := New(nil, evals, nil)
	req := requestJSON(t, http.MethodPost, "/api/tasks/task-url/loop-evaluation", map[string]any{
		"evaluation_kind": "test", "verdict": "pass",
	})
	req.Header.Set("X-Actor-Source", "task_token")
	req.Header.Set("X-Agent-ID", "agent-1")
	req.Header.Set("X-Task-ID", "task-other")
	rr := httptest.NewRecorder()

	testRouter(h).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if evals.calls != 0 {
		t.Fatalf("evaluation submit calls = %d, want 0", evals.calls)
	}
}

func TestEvaluationUsesServerStampedAgentIdentity(t *testing.T) {
	evals := &fakeEvaluationSubmitter{value: loopservice.SubmitEvaluationResult{
		Evaluation: loopservice.PersistedEvaluation{EvaluationID: "eval-1", Created: true},
		Normalized: loopevaluation.Normalized{Submission: loopevaluation.Submission{EvaluationKind: "test", Verdict: "pass"}},
		Policy: loopservice.PolicyTickResult{
			Applied: true,
			Decision: looppolicy.Decision{Reason: "stage_60_active"},
		},
	}}
	h := New(nil, evals, nil)
	req := requestJSON(t, http.MethodPost, "/api/tasks/task-1/loop-evaluation", map[string]any{
		"evaluation_kind": "test", "verdict": "pass",
	})
	req.Header.Set("X-Actor-Source", "task_token")
	req.Header.Set("X-Agent-ID", "agent-test")
	req.Header.Set("X-Task-ID", "task-1")
	// A raw client could try to supply X-User-ID too, but evaluator identity for
	// a task token must remain the server-stamped agent id.
	req.Header.Set("X-User-ID", "owner-user")
	rr := httptest.NewRecorder()

	testRouter(h).ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if evals.last.EvaluatorType != "agent" || evals.last.EvaluatorID != "agent-test" || evals.last.TaskID != "task-1" {
		t.Fatalf("request = %#v", evals.last)
	}
}

func TestEvaluationPersistedButPolicyFailedReturnsAccepted(t *testing.T) {
	evals := &fakeEvaluationSubmitter{
		value: loopservice.SubmitEvaluationResult{
			Evaluation: loopservice.PersistedEvaluation{EvaluationID: "eval-1", Created: true},
			Normalized: loopevaluation.Normalized{Submission: loopevaluation.Submission{EvaluationKind: "test", Verdict: "fail"}, RecoveryTarget: "backend"},
		},
		err: errors.New("evaluation persisted but policy tick failed: database unavailable"),
	}
	h := New(nil, evals, nil)
	req := requestJSON(t, http.MethodPost, "/api/tasks/task-1/loop-evaluation", map[string]any{
		"evaluation_kind": "test", "verdict": "fail",
	})
	req.Header.Set("X-User-ID", "member-1")
	rr := httptest.NewRecorder()

	testRouter(h).ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"evaluation_id":"eval-1"`)) {
		t.Fatalf("body = %s", rr.Body.String())
	}
}

func TestApprovalRejectsMachineCredentialBeforeService(t *testing.T) {
	approvals := &fakeApprovalDecider{}
	h := New(nil, nil, approvals)
	req := requestJSON(t, http.MethodPost, "/api/loop-approvals/approval-1/decision", map[string]any{
		"decision": "approved", "rationale": "ship",
	})
	req.Header.Set("X-Actor-Source", "task_token")
	req.Header.Set("X-User-ID", "owner-user")
	rr := httptest.NewRecorder()

	testRouter(h).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if approvals.calls != 0 {
		t.Fatalf("approval calls = %d, want 0", approvals.calls)
	}
}

func TestApprovalUsesAuthenticatedMemberNotBodyIdentity(t *testing.T) {
	approvals := &fakeApprovalDecider{value: loopservice.DecideApprovalResult{
		Approval: loopservice.PersistedApprovalDecision{ApprovalID: "approval-1", State: "approved", Changed: true},
		Policy: loopservice.PolicyTickResult{Applied: true, Decision: looppolicy.Decision{Reason: "stage_70_active"}},
	}}
	h := New(nil, nil, approvals)
	// Unknown fields are rejected, so a decided_by spoof cannot even be silently
	// ignored. The only identity source is X-User-ID stamped by Auth middleware.
	req := requestJSON(t, http.MethodPost, "/api/loop-approvals/approval-1/decision", map[string]any{
		"decision": "approved", "rationale": "ship", "decided_by": "attacker",
	})
	req.Header.Set("X-User-ID", "member-real")
	rr := httptest.NewRecorder()
	testRouter(h).ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("spoof status = %d body=%s", rr.Code, rr.Body.String())
	}
	if approvals.calls != 0 {
		t.Fatalf("approval calls = %d, want 0", approvals.calls)
	}

	req = requestJSON(t, http.MethodPost, "/api/loop-approvals/approval-1/decision", map[string]any{
		"decision": "approved", "rationale": "ship",
	})
	req.Header.Set("X-User-ID", "member-real")
	rr = httptest.NewRecorder()
	testRouter(h).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if approvals.last.MemberID != "member-real" {
		t.Fatalf("member id = %q, want authenticated member", approvals.last.MemberID)
	}
}

func TestApprovalPersistedButPolicyFailedReturnsAccepted(t *testing.T) {
	approvals := &fakeApprovalDecider{
		value: loopservice.DecideApprovalResult{
			Approval: loopservice.PersistedApprovalDecision{ApprovalID: "approval-1", State: "approved", Changed: true},
		},
		err: errors.New("approval persisted but policy tick failed: database unavailable"),
	}
	h := New(nil, nil, approvals)
	req := requestJSON(t, http.MethodPost, "/api/loop-approvals/approval-1/decision", map[string]any{
		"decision": "approved",
	})
	req.Header.Set("X-User-ID", "member-1")
	rr := httptest.NewRecorder()

	testRouter(h).ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"approval_id":"approval-1"`)) {
		t.Fatalf("body = %s", rr.Body.String())
	}
}

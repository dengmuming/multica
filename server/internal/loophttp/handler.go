package loophttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/loopevaluation"
	"github.com/multica-ai/multica/server/internal/loopservice"
	"github.com/multica-ai/multica/server/internal/looptemplate"
)

const maxBodyBytes = 1 << 20

// Instantiator is deliberately narrower than *loopservice.Instantiator so the
// HTTP contract can be tested and wired before sqlc-backed repositories exist.
type Instantiator interface {
	Instantiate(context.Context, loopservice.InstantiateRequest) (loopservice.InstantiateResult, error)
}

type EvaluationSubmitter interface {
	Submit(context.Context, loopservice.SubmitEvaluationRequest) (loopservice.SubmitEvaluationResult, error)
}

type ApprovalDecider interface {
	Decide(context.Context, loopservice.DecideApprovalRequest) (loopservice.DecideApprovalResult, error)
}

type Handler struct {
	Instantiate Instantiator
	Evaluations EvaluationSubmitter
	Approvals   ApprovalDecider
}

func New(instantiate Instantiator, evaluations EvaluationSubmitter, approvals ApprovalDecider) *Handler {
	return &Handler{Instantiate: instantiate, Evaluations: evaluations, Approvals: approvals}
}

// Register mounts the Manifold Agent Native Loop application API. It assumes
// the caller already installed Multica's Auth middleware. The adapter trusts
// X-User-ID / X-Agent-ID / X-Task-ID / X-Actor-Source only because Auth strips
// caller-supplied actor headers before stamping authoritative values.
func (h *Handler) Register(r chi.Router) {
	r.Post("/api/loops", h.CreateLoop)
	r.Post("/api/tasks/{taskId}/loop-evaluation", h.SubmitEvaluation)
	r.Post("/api/loop-approvals/{approvalId}/decision", h.DecideApproval)
}

type createLoopBody struct {
	ProjectID   string                              `json:"project_id"`
	TemplateKey string                              `json:"template_key"`
	Version     int                                 `json:"version"`
	InstanceKey string                              `json:"instance_key"`
	Title       string                              `json:"title"`
	Description string                              `json:"description,omitempty"`
	Bindings    map[string]looptemplate.RoleBinding `json:"bindings"`
}

type createLoopResponse struct {
	ParentIssueID string            `json:"parent_issue_id"`
	NodeIssueIDs  map[string]string `json:"node_issue_ids"`
	TemplateKey   string            `json:"template_key"`
	Version       int               `json:"version"`
}

func (h *Handler) CreateLoop(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Instantiate == nil {
		writeError(w, http.StatusServiceUnavailable, "loop instantiation is unavailable")
		return
	}
	workspaceID := strings.TrimSpace(r.Header.Get("X-Workspace-ID"))
	if workspaceID == "" {
		writeError(w, http.StatusBadRequest, "workspace context is required")
		return
	}

	var body createLoopBody
	if !decodeBody(w, r, &body) {
		return
	}
	bindings := make(looptemplate.RoleBindings, len(body.Bindings))
	for key, binding := range body.Bindings {
		bindings[key] = binding
	}

	result, err := h.Instantiate.Instantiate(r.Context(), loopservice.InstantiateRequest{
		WorkspaceID: workspaceID,
		ProjectID:   body.ProjectID,
		TemplateKey: body.TemplateKey,
		Version:     body.Version,
		InstanceKey: body.InstanceKey,
		Title:       body.Title,
		Description: body.Description,
		Bindings:    bindings,
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, loopservice.ErrLoopInstanceExists) {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, createLoopResponse{
		ParentIssueID: result.Instance.ParentIssueID,
		NodeIssueIDs:  result.Instance.NodeIssueIDs,
		TemplateKey:   result.Template.Key,
		Version:       result.Template.Version,
	})
}

type evaluationBody struct {
	EvaluationKind string                   `json:"evaluation_kind"`
	Verdict        string                   `json:"verdict"`
	Score          *float64                 `json:"score,omitempty"`
	Findings       []loopevaluation.Finding `json:"findings,omitempty"`
	Evidence       []string                 `json:"evidence,omitempty"`
}

type evaluationResponse struct {
	EvaluationID  string                    `json:"evaluation_id"`
	Created       bool                      `json:"created"`
	Normalized    loopevaluation.Normalized `json:"normalized"`
	PolicyApplied bool                      `json:"policy_applied"`
	PolicyReason  string                    `json:"policy_reason,omitempty"`
	Warning       string                    `json:"warning,omitempty"`
}

func (h *Handler) SubmitEvaluation(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Evaluations == nil {
		writeError(w, http.StatusServiceUnavailable, "loop evaluation is unavailable")
		return
	}
	taskID := strings.TrimSpace(chi.URLParam(r, "taskId"))
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "task id is required")
		return
	}

	evaluatorType, evaluatorID, ok := evaluatorIdentity(w, r, taskID)
	if !ok {
		return
	}
	var body evaluationBody
	if !decodeBody(w, r, &body) {
		return
	}

	result, err := h.Evaluations.Submit(r.Context(), loopservice.SubmitEvaluationRequest{
		TaskID:        taskID,
		EvaluatorType: evaluatorType,
		EvaluatorID:   evaluatorID,
		Submission: loopevaluation.Submission{
			EvaluationKind: body.EvaluationKind,
			Verdict:        body.Verdict,
			Score:          body.Score,
			Findings:       body.Findings,
			Evidence:       body.Evidence,
		},
	})
	if err != nil {
		// Persistence is authoritative even if the follow-up policy tick fails.
		// Surface the durable result as 202 so clients do not mistake a retryable
		// control-plane failure for a rejected evaluation and submit a new result.
		if result.Evaluation.EvaluationID != "" {
			writeJSON(w, http.StatusAccepted, evaluationResponse{
				EvaluationID: result.Evaluation.EvaluationID,
				Created:      result.Evaluation.Created,
				Normalized:   result.Normalized,
				Warning:      err.Error(),
			})
			return
		}
		writeError(w, classifyApplicationError(err), err.Error())
		return
	}

	status := http.StatusOK
	if result.Evaluation.Created {
		status = http.StatusCreated
	}
	writeJSON(w, status, evaluationResponse{
		EvaluationID:  result.Evaluation.EvaluationID,
		Created:       result.Evaluation.Created,
		Normalized:    result.Normalized,
		PolicyApplied: result.Policy.Applied,
		PolicyReason:  result.Policy.Decision.Reason,
	})
}

type approvalBody struct {
	Decision  string `json:"decision"`
	Rationale string `json:"rationale,omitempty"`
}

type approvalResponse struct {
	ApprovalID    string `json:"approval_id"`
	State         string `json:"state"`
	Changed       bool   `json:"changed"`
	PolicyApplied bool   `json:"policy_applied"`
	PolicyReason  string `json:"policy_reason,omitempty"`
	Warning       string `json:"warning,omitempty"`
}

func (h *Handler) DecideApproval(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Approvals == nil {
		writeError(w, http.StatusServiceUnavailable, "loop approval is unavailable")
		return
	}
	if source := strings.TrimSpace(r.Header.Get("X-Actor-Source")); source == "task_token" || source == "cloud_pat" {
		writeError(w, http.StatusForbidden, "this endpoint is only available to human actors")
		return
	}
	memberID := strings.TrimSpace(r.Header.Get("X-User-ID"))
	if memberID == "" {
		writeError(w, http.StatusUnauthorized, "authenticated member is required")
		return
	}
	approvalID := strings.TrimSpace(chi.URLParam(r, "approvalId"))
	if approvalID == "" {
		writeError(w, http.StatusBadRequest, "approval id is required")
		return
	}
	var body approvalBody
	if !decodeBody(w, r, &body) {
		return
	}

	result, err := h.Approvals.Decide(r.Context(), loopservice.DecideApprovalRequest{
		ApprovalID: approvalID,
		MemberID:   memberID,
		Decision:   body.Decision,
		Rationale:  body.Rationale,
	})
	if err != nil {
		if result.Approval.ApprovalID != "" {
			writeJSON(w, http.StatusAccepted, approvalResponse{
				ApprovalID: result.Approval.ApprovalID,
				State:      result.Approval.State,
				Changed:    result.Approval.Changed,
				Warning:    err.Error(),
			})
			return
		}
		status := classifyApplicationError(err)
		if strings.Contains(strings.ToLower(err.Error()), "not the requested approver") {
			status = http.StatusForbidden
		}
		writeError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, approvalResponse{
		ApprovalID:    result.Approval.ApprovalID,
		State:         result.Approval.State,
		Changed:       result.Approval.Changed,
		PolicyApplied: result.Policy.Applied,
		PolicyReason:  result.Policy.Decision.Reason,
	})
}

func evaluatorIdentity(w http.ResponseWriter, r *http.Request, taskID string) (string, string, bool) {
	if strings.TrimSpace(r.Header.Get("X-Actor-Source")) == "task_token" {
		authTaskID := strings.TrimSpace(r.Header.Get("X-Task-ID"))
		agentID := strings.TrimSpace(r.Header.Get("X-Agent-ID"))
		if authTaskID == "" || agentID == "" {
			writeError(w, http.StatusUnauthorized, "task-token identity is incomplete")
			return "", "", false
		}
		if authTaskID != taskID {
			writeError(w, http.StatusForbidden, "task token is not authorized for this task")
			return "", "", false
		}
		return "agent", agentID, true
	}
	memberID := strings.TrimSpace(r.Header.Get("X-User-ID"))
	if memberID == "" {
		writeError(w, http.StatusUnauthorized, "authenticated evaluator is required")
		return "", "", false
	}
	return "member", memberID, true
}

func classifyApplicationError(err error) int {
	if err == nil {
		return http.StatusInternalServerError
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "not found"):
		return http.StatusNotFound
	case strings.Contains(message, "conflict"), strings.Contains(message, "not pending"), strings.Contains(message, "already"):
		return http.StatusConflict
	case strings.Contains(message, "forbidden"), strings.Contains(message, "not authorized"):
		return http.StatusForbidden
	default:
		return http.StatusBadRequest
	}
}

func decodeBody(w http.ResponseWriter, r *http.Request, out any) bool {
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "request body must contain one JSON object")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		// Headers may already be committed; there is no second safe response.
		return
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": strings.TrimSpace(message)})
}

var _ = fmt.Sprintf

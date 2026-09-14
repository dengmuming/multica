package loophttp

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/loopservice"
)

type ArtifactService interface {
	Register(context.Context, loopservice.RegisterArtifactRequest) (loopservice.RegisteredArtifact, error)
	List(context.Context, string, string) ([]loopservice.RegisteredArtifact, error)
}

type ArtifactHandler struct {
	Artifacts ArtifactService
}

func NewArtifactHandler(artifacts ArtifactService) *ArtifactHandler { return &ArtifactHandler{Artifacts: artifacts} }

func (h *ArtifactHandler) Register(r chi.Router) {
	if h == nil || r == nil { return }
	r.Get("/api/loops/{parentIssueId}/artifacts", h.ListArtifacts)
	r.Post("/api/loops/{parentIssueId}/artifacts", h.RegisterArtifact)
}

func (h *ArtifactHandler) ListArtifacts(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Artifacts == nil {
		writeError(w, http.StatusServiceUnavailable, "loop artifact service is unavailable")
		return
	}
	workspaceID := strings.TrimSpace(r.Header.Get("X-Workspace-ID"))
	parentIssueID := strings.TrimSpace(chi.URLParam(r, "parentIssueId"))
	if workspaceID == "" || parentIssueID == "" {
		writeError(w, http.StatusBadRequest, "workspace and parent issue context are required")
		return
	}
	artifacts, err := h.Artifacts.List(r.Context(), workspaceID, parentIssueID)
	if err != nil {
		writeError(w, classifyApplicationError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"artifacts": artifacts})
}

type artifactBody struct {
	NodeIssueID  string         `json:"node_issue_id,omitempty"`
	TaskID       string         `json:"task_id,omitempty"`
	ArtifactType string         `json:"artifact_type"`
	Relation     string         `json:"relation"`
	Title        string         `json:"title,omitempty"`
	RefKind      string         `json:"ref_kind"`
	RefID        string         `json:"ref_id,omitempty"`
	RefURI       string         `json:"ref_uri,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

func (h *ArtifactHandler) RegisterArtifact(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Artifacts == nil {
		writeError(w, http.StatusServiceUnavailable, "loop artifact registration is unavailable")
		return
	}
	workspaceID := strings.TrimSpace(r.Header.Get("X-Workspace-ID"))
	parentIssueID := strings.TrimSpace(chi.URLParam(r, "parentIssueId"))
	if workspaceID == "" || parentIssueID == "" {
		writeError(w, http.StatusBadRequest, "workspace and parent issue context are required")
		return
	}

	source := strings.TrimSpace(r.Header.Get("X-Actor-Source"))
	if source == "cloud_pat" {
		writeError(w, http.StatusForbidden, "cloud machine credentials cannot author loop artifacts")
		return
	}
	actorType := "member"
	actorID := strings.TrimSpace(r.Header.Get("X-User-ID"))
	authTaskID := ""
	if source == "task_token" {
		actorType = "agent"
		actorID = strings.TrimSpace(r.Header.Get("X-Agent-ID"))
		authTaskID = strings.TrimSpace(r.Header.Get("X-Task-ID"))
	}
	if actorID == "" || (actorType == "agent" && authTaskID == "") {
		writeError(w, http.StatusUnauthorized, "authenticated artifact actor is required")
		return
	}

	var body artifactBody
	if !decodeBody(w, r, &body) { return }
	artifact, err := h.Artifacts.Register(r.Context(), loopservice.RegisterArtifactRequest{
		WorkspaceID: workspaceID, ParentIssueID: parentIssueID,
		ActorType: actorType, ActorID: actorID, AuthTaskID: authTaskID,
		NodeIssueID: body.NodeIssueID, TaskID: body.TaskID,
		ArtifactType: body.ArtifactType, Relation: body.Relation, Title: body.Title,
		RefKind: body.RefKind, RefID: body.RefID, RefURI: body.RefURI, Metadata: body.Metadata,
	})
	if err != nil {
		writeError(w, classifyApplicationError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, artifact)
}

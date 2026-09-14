package loophttp

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/loopservice"
)

type ArtifactRegistrar interface {
	Register(context.Context, loopservice.RegisterArtifactRequest) (loopservice.RegisteredArtifact, error)
}

type ArtifactHandler struct {
	Artifacts ArtifactRegistrar
}

func NewArtifactHandler(artifacts ArtifactRegistrar) *ArtifactHandler { return &ArtifactHandler{Artifacts: artifacts} }

func (h *ArtifactHandler) Register(r chi.Router) {
	if h == nil || r == nil { return }
	r.Post("/api/loops/{parentIssueId}/artifacts", h.RegisterArtifact)
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

	actorType := "member"
	actorID := strings.TrimSpace(r.Header.Get("X-User-ID"))
	authTaskID := ""
	if strings.TrimSpace(r.Header.Get("X-Actor-Source")) == "task_token" {
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

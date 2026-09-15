package loophttp

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/loopservice"
)

type WorkGraphReader interface {
	GetWorkGraph(context.Context, string, string) (*loopservice.WorkGraph, error)
}

type WorkGraphHandler struct {
	Graphs WorkGraphReader
}

func NewWorkGraphHandler(graphs WorkGraphReader) *WorkGraphHandler {
	return &WorkGraphHandler{Graphs: graphs}
}

func (h *WorkGraphHandler) Register(r chi.Router) {
	if h == nil || r == nil { return }
	r.Get("/api/loops/{parentIssueId}/work-graph", h.GetWorkGraph)
}

func (h *WorkGraphHandler) GetWorkGraph(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Graphs == nil {
		writeError(w, http.StatusServiceUnavailable, "work graph service is unavailable")
		return
	}
	workspaceID := strings.TrimSpace(r.Header.Get("X-Workspace-ID"))
	parentIssueID := strings.TrimSpace(chi.URLParam(r, "parentIssueId"))
	if workspaceID == "" || parentIssueID == "" {
		writeError(w, http.StatusBadRequest, "workspace and parent issue context are required")
		return
	}
	graph, err := h.Graphs.GetWorkGraph(r.Context(), workspaceID, parentIssueID)
	if err != nil {
		status := classifyApplicationError(err)
		if errors.Is(err, loopservice.ErrLoopNotFound) { status = http.StatusNotFound }
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, graph)
}

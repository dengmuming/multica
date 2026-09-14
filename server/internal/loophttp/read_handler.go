package loophttp

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/loopservice"
)

type LoopReader interface {
	List(context.Context, loopservice.ListLoopsQuery) ([]loopservice.LoopSummary, error)
	Get(context.Context, string, string) (loopservice.LoopDetail, error)
}

type ReadHandler struct {
	Loops LoopReader
}

func NewReadHandler(loops LoopReader) *ReadHandler {
	return &ReadHandler{Loops: loops}
}

func (h *ReadHandler) Register(r chi.Router) {
	if h == nil || r == nil {
		return
	}
	r.Get("/api/loops", h.ListLoops)
	r.Get("/api/loops/{parentIssueId}", h.GetLoop)
}

func (h *ReadHandler) ListLoops(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Loops == nil {
		writeError(w, http.StatusServiceUnavailable, "loop read service is unavailable")
		return
	}
	workspaceID := strings.TrimSpace(r.Header.Get("X-Workspace-ID"))
	if workspaceID == "" {
		writeError(w, http.StatusBadRequest, "workspace context is required")
		return
	}
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = value
	}
	loops, err := h.Loops.List(r.Context(), loopservice.ListLoopsQuery{
		WorkspaceID: workspaceID,
		ProjectID:   strings.TrimSpace(r.URL.Query().Get("project_id")),
		Limit:       limit,
	})
	if err != nil {
		writeError(w, classifyApplicationError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"loops": loops})
}

func (h *ReadHandler) GetLoop(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Loops == nil {
		writeError(w, http.StatusServiceUnavailable, "loop read service is unavailable")
		return
	}
	workspaceID := strings.TrimSpace(r.Header.Get("X-Workspace-ID"))
	parentIssueID := strings.TrimSpace(chi.URLParam(r, "parentIssueId"))
	if workspaceID == "" || parentIssueID == "" {
		writeError(w, http.StatusBadRequest, "workspace and parent issue context are required")
		return
	}
	loop, err := h.Loops.Get(r.Context(), workspaceID, parentIssueID)
	if err != nil {
		status := classifyApplicationError(err)
		if errors.Is(err, loopservice.ErrLoopNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, loop)
}

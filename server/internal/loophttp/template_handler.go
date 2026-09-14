package loophttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/loopservice"
	"github.com/multica-ai/multica/server/internal/looptemplate"
)

type TemplateAdmin interface {
	CreateDraft(context.Context, loopservice.CreateTemplateDraftCommand) (loopservice.ManagedTemplate, error)
	UpdateDraft(context.Context, loopservice.UpdateTemplateDraftCommand) (loopservice.ManagedTemplate, error)
	PublishDraft(context.Context, loopservice.PublishTemplateDraftCommand) (loopservice.ManagedTemplate, error)
}

type TemplateHandler struct {
	Admin TemplateAdmin
}

func NewTemplateHandler(admin TemplateAdmin) *TemplateHandler { return &TemplateHandler{Admin: admin} }

func (h *TemplateHandler) Register(r chi.Router) {
	if h == nil || r == nil {
		return
	}
	r.Post("/api/loop-templates", h.CreateDraft)
	r.Put("/api/loop-templates/{templateKey}/versions/{version}", h.UpdateDraft)
	r.Post("/api/loop-templates/{templateKey}/versions/{version}/publish", h.PublishDraft)
}

type templateWriteBody struct {
	Definition    looptemplate.Definition `json:"definition"`
	PolicyVersion string                  `json:"policy_version"`
}

type templateResponse struct {
	ID            string                  `json:"id"`
	TemplateKey   string                  `json:"template_key"`
	Version       int                     `json:"version"`
	Status        string                  `json:"status"`
	PolicyVersion string                  `json:"policy_version"`
	Definition    looptemplate.Definition `json:"definition"`
}

func (h *TemplateHandler) CreateDraft(w http.ResponseWriter, r *http.Request) {
	workspaceID, memberID, ok := humanTemplateActor(w, r)
	if !ok {
		return
	}
	if h == nil || h.Admin == nil {
		writeError(w, http.StatusServiceUnavailable, "loop template administration is unavailable")
		return
	}
	var body templateWriteBody
	if !decodeBody(w, r, &body) {
		return
	}
	result, err := h.Admin.CreateDraft(r.Context(), loopservice.CreateTemplateDraftCommand{
		WorkspaceID: workspaceID, CreatorType: "member", CreatorID: memberID,
		Definition: body.Definition, PolicyVersion: body.PolicyVersion,
	})
	if err != nil {
		writeError(w, classifyApplicationError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, templateHTTPResponse(result))
}

func (h *TemplateHandler) UpdateDraft(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := humanTemplateActor(w, r)
	if !ok {
		return
	}
	if h == nil || h.Admin == nil {
		writeError(w, http.StatusServiceUnavailable, "loop template administration is unavailable")
		return
	}
	key, version, ok := templatePath(w, r)
	if !ok {
		return
	}
	var body templateWriteBody
	if !decodeBody(w, r, &body) {
		return
	}
	result, err := h.Admin.UpdateDraft(r.Context(), loopservice.UpdateTemplateDraftCommand{
		WorkspaceID: workspaceID, TemplateKey: key, Version: version,
		Definition: body.Definition, PolicyVersion: body.PolicyVersion,
	})
	if err != nil {
		status := classifyApplicationError(err)
		if errors.Is(err, loopservice.ErrTemplateDraftNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, templateHTTPResponse(result))
}

func (h *TemplateHandler) PublishDraft(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := humanTemplateActor(w, r)
	if !ok {
		return
	}
	if h == nil || h.Admin == nil {
		writeError(w, http.StatusServiceUnavailable, "loop template administration is unavailable")
		return
	}
	key, version, ok := templatePath(w, r)
	if !ok {
		return
	}
	result, err := h.Admin.PublishDraft(r.Context(), loopservice.PublishTemplateDraftCommand{
		WorkspaceID: workspaceID, TemplateKey: key, Version: version,
	})
	if err != nil {
		status := classifyApplicationError(err)
		if errors.Is(err, loopservice.ErrTemplateDraftNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, templateHTTPResponse(result))
}

func humanTemplateActor(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	if source := strings.TrimSpace(r.Header.Get("X-Actor-Source")); source == "task_token" || source == "cloud_pat" {
		writeError(w, http.StatusForbidden, "loop templates can only be managed by human actors")
		return "", "", false
	}
	workspaceID := strings.TrimSpace(r.Header.Get("X-Workspace-ID"))
	memberID := strings.TrimSpace(r.Header.Get("X-User-ID"))
	if workspaceID == "" {
		writeError(w, http.StatusBadRequest, "workspace context is required")
		return "", "", false
	}
	if memberID == "" {
		writeError(w, http.StatusUnauthorized, "authenticated member is required")
		return "", "", false
	}
	return workspaceID, memberID, true
}

func templatePath(w http.ResponseWriter, r *http.Request) (string, int, bool) {
	key := strings.TrimSpace(chi.URLParam(r, "templateKey"))
	version, err := strconv.Atoi(strings.TrimSpace(chi.URLParam(r, "version")))
	if key == "" || err != nil || version <= 0 {
		writeError(w, http.StatusBadRequest, "valid template key and positive version are required")
		return "", 0, false
	}
	return key, version, true
}

func templateHTTPResponse(value loopservice.ManagedTemplate) templateResponse {
	return templateResponse{
		ID: value.ID, TemplateKey: value.Key, Version: value.Version, Status: value.Status,
		PolicyVersion: value.PolicyVersion, Definition: value.Definition,
	}
}

var _ = json.RawMessage{}

package loophttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/loopservice"
)

type fakeWorkGraphHTTPReader struct{}
func (fakeWorkGraphHTTPReader) GetWorkGraph(context.Context, string, string) (*loopservice.WorkGraph, error) {
	return &loopservice.WorkGraph{ID: "parent", Status: "running", Nodes: []loopservice.WorkGraphNode{}, Edges: []loopservice.WorkGraphEdge{}, Gates: []loopservice.WorkGraphGate{}}, nil
}

func TestWorkGraphHandlerRoute(t *testing.T) {
	r := chi.NewRouter()
	NewWorkGraphHandler(fakeWorkGraphHTTPReader{}).Register(r)
	req := httptest.NewRequest(http.MethodGet, "/api/loops/parent/work-graph", nil)
	req.Header.Set("X-Workspace-ID", "workspace")
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	if res.Code != http.StatusOK { t.Fatalf("status=%d body=%s", res.Code, res.Body.String()) }
}

func TestWorkGraphHandlerRequiresWorkspace(t *testing.T) {
	r := chi.NewRouter()
	NewWorkGraphHandler(fakeWorkGraphHTTPReader{}).Register(r)
	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/loops/parent/work-graph", nil))
	if res.Code != http.StatusBadRequest { t.Fatalf("status=%d", res.Code) }
}

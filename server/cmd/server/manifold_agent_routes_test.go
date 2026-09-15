package main

import (
	"net/http"
	"sort"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/loophttp"
)

func TestRegisterManifoldAgentRoutesPinsP0Surface(t *testing.T) {
	r := chi.NewRouter()
	registerManifoldAgentRoutes(r, &manifoldAgentRouteBundle{
		Loop: &loophttp.Handler{}, Read: &loophttp.ReadHandler{}, WorkGraph: &loophttp.WorkGraphHandler{}, Templates: &loophttp.TemplateHandler{}, Artifacts: &loophttp.ArtifactHandler{},
	})
	var got []string
	if err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error { got = append(got, method+" "+route); return nil }); err != nil { t.Fatalf("walk routes: %v", err) }
	sort.Strings(got)
	want := []string{
		"GET /api/loops", "GET /api/loops/{parentIssueId}", "GET /api/loops/{parentIssueId}/artifacts", "GET /api/loops/{parentIssueId}/work-graph",
		"POST /api/loop-approvals/{approvalId}/decision", "POST /api/loop-templates", "POST /api/loop-templates/{templateKey}/versions/{version}/publish", "POST /api/loops", "POST /api/loops/{parentIssueId}/artifacts", "POST /api/tasks/{taskId}/loop-evaluation", "PUT /api/loop-templates/{templateKey}/versions/{version}",
	}
	sort.Strings(want)
	if len(got) != len(want) { t.Fatalf("registered routes = %#v, want %#v", got, want) }
	for i := range want { if got[i] != want[i] { t.Fatalf("registered routes = %#v, want %#v", got, want) } }
}

func TestRegisterManifoldAgentRoutesNilBundleIsNoop(t *testing.T) {
	r := chi.NewRouter(); registerManifoldAgentRoutes(r, nil); count := 0
	if err := chi.Walk(r, func(_ string, _ string, _ http.Handler, _ ...func(http.Handler) http.Handler) error { count++; return nil }); err != nil { t.Fatalf("walk routes: %v", err) }
	if count != 0 { t.Fatalf("route count = %d, want 0", count) }
}

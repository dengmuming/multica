package loophttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/loopservice"
)

type fakeLoopReader struct {
	listCalls int
	listQuery loopservice.ListLoopsQuery
	listValue []loopservice.LoopSummary
	listErr   error
	getCalls  int
	getWorkspace string
	getParent string
	getValue loopservice.LoopDetail
	getErr error
}

func (f *fakeLoopReader) List(_ context.Context, query loopservice.ListLoopsQuery) ([]loopservice.LoopSummary, error) {
	f.listCalls++
	f.listQuery = query
	return f.listValue, f.listErr
}

func (f *fakeLoopReader) Get(_ context.Context, workspaceID, parentIssueID string) (loopservice.LoopDetail, error) {
	f.getCalls++
	f.getWorkspace = workspaceID
	f.getParent = parentIssueID
	return f.getValue, f.getErr
}

func readTestRouter(h *ReadHandler) http.Handler {
	r := chi.NewRouter()
	h.Register(r)
	return r
}

func TestListLoopsUsesWorkspaceAndProjectFilter(t *testing.T) {
	reader := &fakeLoopReader{listValue: []loopservice.LoopSummary{{ParentIssueID: "parent-1", Title: "Feature"}}}
	h := NewReadHandler(reader)
	req := httptest.NewRequest(http.MethodGet, "/api/loops?project_id=project-1&limit=25", nil)
	req.Header.Set("X-Workspace-ID", "workspace-server")
	rr := httptest.NewRecorder()

	readTestRouter(h).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if reader.listCalls != 1 || reader.listQuery.WorkspaceID != "workspace-server" || reader.listQuery.ProjectID != "project-1" || reader.listQuery.Limit != 25 {
		t.Fatalf("list query = %#v", reader.listQuery)
	}
}

func TestListLoopsRequiresWorkspace(t *testing.T) {
	reader := &fakeLoopReader{}
	h := NewReadHandler(reader)
	req := httptest.NewRequest(http.MethodGet, "/api/loops", nil)
	rr := httptest.NewRecorder()

	readTestRouter(h).ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if reader.listCalls != 0 {
		t.Fatalf("list calls = %d, want 0", reader.listCalls)
	}
}

func TestListLoopsRejectsInvalidLimit(t *testing.T) {
	reader := &fakeLoopReader{}
	h := NewReadHandler(reader)
	req := httptest.NewRequest(http.MethodGet, "/api/loops?limit=zero", nil)
	req.Header.Set("X-Workspace-ID", "workspace-1")
	rr := httptest.NewRecorder()

	readTestRouter(h).ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if reader.listCalls != 0 {
		t.Fatalf("list calls = %d, want 0", reader.listCalls)
	}
}

func TestGetLoopUsesWorkspaceAndParent(t *testing.T) {
	reader := &fakeLoopReader{getValue: loopservice.LoopDetail{LoopSummary: loopservice.LoopSummary{ParentIssueID: "parent-1", Title: "Feature"}}}
	h := NewReadHandler(reader)
	req := httptest.NewRequest(http.MethodGet, "/api/loops/parent-1", nil)
	req.Header.Set("X-Workspace-ID", "workspace-server")
	rr := httptest.NewRecorder()

	readTestRouter(h).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if reader.getCalls != 1 || reader.getWorkspace != "workspace-server" || reader.getParent != "parent-1" {
		t.Fatalf("get = calls=%d workspace=%q parent=%q", reader.getCalls, reader.getWorkspace, reader.getParent)
	}
}

func TestGetLoopNotFound(t *testing.T) {
	reader := &fakeLoopReader{getErr: loopservice.ErrLoopNotFound}
	h := NewReadHandler(reader)
	req := httptest.NewRequest(http.MethodGet, "/api/loops/missing", nil)
	req.Header.Set("X-Workspace-ID", "workspace-1")
	rr := httptest.NewRecorder()

	readTestRouter(h).ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestGetLoopServiceError(t *testing.T) {
	reader := &fakeLoopReader{getErr: errors.New("database unavailable")}
	h := NewReadHandler(reader)
	req := httptest.NewRequest(http.MethodGet, "/api/loops/parent-1", nil)
	req.Header.Set("X-Workspace-ID", "workspace-1")
	rr := httptest.NewRecorder()

	readTestRouter(h).ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
}

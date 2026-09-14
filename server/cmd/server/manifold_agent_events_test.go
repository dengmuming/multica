package main

import (
	"testing"

	"github.com/multica-ai/multica/server/internal/handler"
)

func TestManifoldIssueUpdatedPayloadMapShape(t *testing.T) {
	issueID, status, ok := manifoldIssueUpdatedPayload(map[string]any{
		"issue": map[string]any{
			"id":     "issue-1",
			"status": "done",
		},
		"status_changed": true,
	})
	if !ok || issueID != "issue-1" || status != "done" {
		t.Fatalf("payload = id=%q status=%q ok=%v", issueID, status, ok)
	}
}

func TestManifoldIssueUpdatedPayloadTypedShape(t *testing.T) {
	issueID, status, ok := manifoldIssueUpdatedPayload(map[string]any{
		"issue": handler.IssueResponse{ID: "issue-typed", Status: "done"},
	})
	if !ok || issueID != "issue-typed" || status != "done" {
		t.Fatalf("payload = id=%q status=%q ok=%v", issueID, status, ok)
	}
}

func TestManifoldIssueUpdatedPayloadRejectsMalformedPayload(t *testing.T) {
	cases := []any{
		nil,
		"not-a-map",
		map[string]any{},
		map[string]any{"issue": map[string]any{"status": "done"}},
		map[string]any{"issue": map[string]any{"id": "issue-1", "status": 123}},
	}
	for i, payload := range cases {
		_, _, ok := manifoldIssueUpdatedPayload(payload)
		if ok {
			t.Fatalf("case %d unexpectedly accepted payload %#v", i, payload)
		}
	}
}

func TestManifoldTaskIssueIDUsesCanonicalTaskEventField(t *testing.T) {
	if got := manifoldTaskIssueID(map[string]any{
		"task_id":  "task-1",
		"agent_id": "agent-1",
		"issue_id": " issue-1 ",
		"status":   "completed",
	}); got != "issue-1" {
		t.Fatalf("issue id = %q, want issue-1", got)
	}
}

func TestManifoldTaskIssueIDRejectsMalformedPayload(t *testing.T) {
	for i, payload := range []any{
		nil,
		"not-a-map",
		map[string]any{},
		map[string]any{"issue_id": 123},
		map[string]any{"issue_id": "   "},
	} {
		if got := manifoldTaskIssueID(payload); got != "" {
			t.Fatalf("case %d issue id = %q, want empty", i, got)
		}
	}
}

func TestIsManifoldLoopNodeRequiresNodeKey(t *testing.T) {
	cases := []struct {
		name string
		raw  []byte
		want bool
	}{
		{name: "valid", raw: []byte(`{"manifold.loop.node_key":"backend"}`), want: true},
		{name: "blank", raw: []byte(`{"manifold.loop.node_key":"  "}`), want: false},
		{name: "parent-only", raw: []byte(`{"manifold.loop.instance_key":"feature-1"}`), want: false},
		{name: "invalid-json", raw: []byte(`{`), want: false},
		{name: "empty", raw: nil, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isManifoldLoopNode(tc.raw); got != tc.want {
				t.Fatalf("isManifoldLoopNode() = %v, want %v", got, tc.want)
			}
		})
	}
}

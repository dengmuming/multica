package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/handler"
	"github.com/multica-ai/multica/server/internal/loopservice"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// registerManifoldAgentPolicyEvents closes the normal-node advancement gap.
// Multica's Task completion does not itself change Issue status: agents update
// their Issue through the CLI. We therefore listen to BOTH committed Issue
// updates and Task completions. Policy Tick is idempotent, so whichever arrives
// first may no-op and the later event safely retries the same deterministic
// transition.
//
// events.Bus invokes listeners synchronously after the originating write has
// committed. This keeps P0 progression deterministic and avoids introducing a
// second queue/reconciler before the first vertical slice proves a need for it.
func registerManifoldAgentPolicyEvents(bus *events.Bus, queries *db.Queries, policy *loopservice.PolicyApplicationService) {
	if bus == nil || queries == nil || policy == nil {
		return
	}

	advance := func(workspaceID, issueID string) {
		workspaceID = strings.TrimSpace(workspaceID)
		issueID = strings.TrimSpace(issueID)
		if workspaceID == "" || issueID == "" {
			return
		}
		wid, err := util.ParseUUID(workspaceID)
		if err != nil {
			return
		}
		iid, err := util.ParseUUID(issueID)
		if err != nil {
			return
		}
		issue, err := queries.GetIssueInWorkspace(context.Background(), db.GetIssueInWorkspaceParams{ID: iid, WorkspaceID: wid})
		if err != nil || !issue.ParentIssueID.Valid || !isManifoldLoopNode(issue.Metadata) {
			return
		}
		if _, err := policy.Tick(context.Background(), util.UUIDToString(issue.ParentIssueID)); err != nil {
			slog.Warn("Manifold Agent policy tick from lifecycle event failed",
				"workspace_id", workspaceID,
				"issue_id", issueID,
				"parent_issue_id", util.UUIDToString(issue.ParentIssueID),
				"error", err,
			)
		}
	}

	bus.Subscribe(protocol.EventIssueUpdated, func(event events.Event) {
		issueID, status, ok := manifoldIssueUpdatedPayload(event.Payload)
		if !ok || status != "done" {
			return
		}
		advance(event.WorkspaceID, issueID)
	})

	bus.Subscribe(protocol.EventTaskCompleted, func(event events.Event) {
		issueID := manifoldTaskIssueID(event.Payload)
		if issueID == "" {
			return
		}
		advance(event.WorkspaceID, issueID)
	})
}

func isManifoldLoopNode(raw []byte) bool {
	if len(raw) == 0 {
		return false
	}
	var metadata map[string]any
	if json.Unmarshal(raw, &metadata) != nil {
		return false
	}
	value, _ := metadata["manifold.loop.node_key"].(string)
	return strings.TrimSpace(value) != ""
}

func manifoldTaskIssueID(payload any) string {
	values, ok := payload.(map[string]any)
	if !ok {
		return ""
	}
	value, _ := values["issue_id"].(string)
	return strings.TrimSpace(value)
}

func manifoldIssueUpdatedPayload(payload any) (issueID, status string, ok bool) {
	values, ok := payload.(map[string]any)
	if !ok {
		return "", "", false
	}
	// Handler publishes its typed IssueResponse in-process. Keep a map fallback
	// so the listener remains compatible if a future publisher normalizes the
	// payload before Bus.Publish.
	switch issue := values["issue"].(type) {
	case handler.IssueResponse:
		return strings.TrimSpace(issue.ID), strings.TrimSpace(issue.Status), issue.ID != ""
	case map[string]any:
		id, _ := issue["id"].(string)
		state, _ := issue["status"].(string)
		return strings.TrimSpace(id), strings.TrimSpace(state), strings.TrimSpace(id) != ""
	default:
		return "", "", false
	}
}

package loopservice

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// CanonicalIssueGraphStore owns the database transaction that persists a full
// Parent/Child Issue graph. The concrete Multica adapter is responsible for
// using the same Issue numbering, position, metadata and workspace/project
// validation rules as ordinary Issue creation.
//
// The operation MUST be atomic: either the parent and every child are durable,
// or none are. It MUST also enforce (workspace_id, instance_key) idempotency at
// write time and return ErrLoopInstanceExists for a concurrent duplicate.
//
// This store deliberately does NOT create Task/Run rows. Agent execution is
// triggered only after the complete Issue graph has committed.
type CanonicalIssueGraphStore interface {
	CreateGraphAtomically(ctx context.Context, cmd CreateLoopIssueGraphCommand) (CreatedIssueGraph, error)
}

// InitialIssueDispatcher bridges a committed runnable child Issue into the
// existing Multica assignment/dispatch lifecycle. Implementations must be
// idempotent for the same Issue: retries after a timeout must not create a
// second canonical Task for the same activation.
type InitialIssueDispatcher interface {
	DispatchAssignedIssue(ctx context.Context, req InitialDispatchRequest) error
}

type InitialDispatchRequest struct {
	WorkspaceID  string
	IssueID      string
	NodeKey      string
	AssigneeType string
	AssigneeID   string
	Reason       string
}

// InitialDispatchFailure identifies one committed child whose ordinary Multica
// dispatch could not be started after graph persistence completed.
type InitialDispatchFailure struct {
	NodeKey string
	IssueID string
	Err     error
}

// InitialDispatchError means the Issue graph is fully durable but one or more
// first-stage agent/squad activations failed. This is intentionally different
// from a persistence failure: callers/reconcilers should retry dispatch against
// the existing instance rather than create another graph.
type InitialDispatchError struct {
	ParentIssueID string
	Failures      []InitialDispatchFailure
}

func (e *InitialDispatchError) Error() string {
	if e == nil {
		return "initial loop dispatch failed"
	}
	return fmt.Sprintf("loop graph %s committed but %d initial dispatch(es) failed", e.ParentIssueID, len(e.Failures))
}

func (e *InitialDispatchError) Unwrap() error {
	if e == nil || len(e.Failures) == 0 {
		return nil
	}
	return e.Failures[0].Err
}

// DurableIssueGraphGateway separates two concerns that must not be collapsed:
//
//  1. atomically persist the complete Parent/Child Issue graph;
//  2. after commit, enter runnable agent/squad Issues through Multica's normal
//     assignment/dispatch path so Task/Run ownership remains canonical.
//
// A dispatch failure never attempts to roll back the already-committed graph.
// Retrying/reconciling dispatch is safe because InitialIssueDispatcher is
// required to be idempotent.
type DurableIssueGraphGateway struct {
	store      CanonicalIssueGraphStore
	dispatcher InitialIssueDispatcher
}

func NewDurableIssueGraphGateway(store CanonicalIssueGraphStore, dispatcher InitialIssueDispatcher) *DurableIssueGraphGateway {
	return &DurableIssueGraphGateway{store: store, dispatcher: dispatcher}
}

func (g *DurableIssueGraphGateway) CreateLoopIssueGraph(ctx context.Context, cmd CreateLoopIssueGraphCommand) (CreatedIssueGraph, error) {
	if g == nil || g.store == nil {
		return CreatedIssueGraph{}, fmt.Errorf("canonical issue graph store is required")
	}
	if strings.TrimSpace(cmd.WorkspaceID) == "" || strings.TrimSpace(cmd.InstanceKey) == "" {
		return CreatedIssueGraph{}, fmt.Errorf("workspace id and instance key are required")
	}

	created, err := g.store.CreateGraphAtomically(ctx, cmd)
	if err != nil {
		return CreatedIssueGraph{}, err
	}
	if strings.TrimSpace(created.ParentIssueID) == "" {
		return CreatedIssueGraph{}, fmt.Errorf("canonical issue graph store returned empty parent issue id")
	}
	if err := validateCreatedGraph(cmd, created); err != nil {
		return created, err
	}

	requests := initialDispatchRequests(cmd, created)
	if len(requests) == 0 {
		return created, nil
	}
	if g.dispatcher == nil {
		return created, &InitialDispatchError{
			ParentIssueID: created.ParentIssueID,
			Failures: []InitialDispatchFailure{{
				NodeKey: requests[0].NodeKey,
				IssueID: requests[0].IssueID,
				Err:     errors.New("initial issue dispatcher is required for runnable agent/squad nodes"),
			}},
		}
	}

	failures := make([]InitialDispatchFailure, 0)
	for _, req := range requests {
		if err := g.dispatcher.DispatchAssignedIssue(ctx, req); err != nil {
			failures = append(failures, InitialDispatchFailure{
				NodeKey: req.NodeKey,
				IssueID: req.IssueID,
				Err:     err,
			})
		}
	}
	if len(failures) > 0 {
		return created, &InitialDispatchError{
			ParentIssueID: created.ParentIssueID,
			Failures:      failures,
		}
	}
	return created, nil
}

func validateCreatedGraph(cmd CreateLoopIssueGraphCommand, created CreatedIssueGraph) error {
	if len(created.NodeIssueIDs) != len(cmd.Children) {
		return fmt.Errorf("canonical issue graph store returned %d child ids for %d children", len(created.NodeIssueIDs), len(cmd.Children))
	}
	for _, child := range cmd.Children {
		id := strings.TrimSpace(created.NodeIssueIDs[child.NodeKey])
		if id == "" {
			return fmt.Errorf("canonical issue graph store returned no issue id for node %q", child.NodeKey)
		}
	}
	return nil
}

func initialDispatchRequests(cmd CreateLoopIssueGraphCommand, created CreatedIssueGraph) []InitialDispatchRequest {
	requests := make([]InitialDispatchRequest, 0)
	for _, child := range cmd.Children {
		if child.Status != "todo" {
			continue
		}
		if child.AssigneeType != "agent" && child.AssigneeType != "squad" {
			// A member-bound node is human work. An approval node is unassigned.
			// Neither should create an Agent Task merely because it is runnable.
			continue
		}
		requests = append(requests, InitialDispatchRequest{
			WorkspaceID:  cmd.WorkspaceID,
			IssueID:      created.NodeIssueIDs[child.NodeKey],
			NodeKey:      child.NodeKey,
			AssigneeType: child.AssigneeType,
			AssigneeID:   child.AssigneeID,
			Reason:       "manifold_loop_initial_stage",
		})
	}
	return requests
}

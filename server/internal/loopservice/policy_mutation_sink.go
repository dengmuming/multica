package loopservice

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/multica-ai/multica/server/internal/looppolicy"
)

// PolicyMutationStore is the single transactional boundary for deterministic
// workflow state changes. A concrete Multica implementation locks/reloads the
// parent and referenced children, verifies the pinned policy version/revisions,
// applies Issue status + metadata changes, and creates pending approvals/Inbox
// entries before commit.
//
// It MUST NOT create Agent Tasks inside the transaction. Runnable Issues are
// dispatched only after commit through InitialIssueDispatcher so the existing
// TaskService/AgentReadiness execution plane remains canonical.
type PolicyMutationStore interface {
	ApplyMutationBatchAtomically(ctx context.Context, batch PolicyMutationBatch) (PolicyMutationCommit, error)
}

type PolicyMutationBatch struct {
	WorkspaceID   string
	ParentIssueID string
	PolicyVersion string
	Reason        string
	Mutations     []PolicyMutation
}

type PolicyMutationType string

const (
	MutationActivateNode    PolicyMutationType = "activate_node"
	MutationReopenNode      PolicyMutationType = "reopen_node"
	MutationParkNode        PolicyMutationType = "park_node"
	MutationRequestApproval PolicyMutationType = "request_approval"
	MutationSetParentState  PolicyMutationType = "set_parent_state"
	MutationBlockLoop       PolicyMutationType = "block_loop"
	MutationCompleteLoop    PolicyMutationType = "complete_loop"
)

type PolicyMutation struct {
	Type          PolicyMutationType
	NodeKey       string
	IssueID       string
	ParentState   string
	Reason        string
	IncrementRetry bool
}

// PolicyMutationCommit describes only post-commit work. A store returns one
// dispatch request for every executable Issue that became runnable due to the
// committed batch. Human/member nodes and approval gates never appear here.
type PolicyMutationCommit struct {
	Dispatch []InitialDispatchRequest
}

// DurablePolicyMutationSink translates pure policy actions into one atomic
// mutation batch, then dispatches newly-runnable Agent/Squad Issues after the
// transaction commits. A post-commit dispatch failure is surfaced distinctly;
// callers must reconcile the existing state rather than retry the whole policy
// transition blindly.
type DurablePolicyMutationSink struct {
	store      PolicyMutationStore
	dispatcher InitialIssueDispatcher
}

func NewDurablePolicyMutationSink(store PolicyMutationStore, dispatcher InitialIssueDispatcher) *DurablePolicyMutationSink {
	return &DurablePolicyMutationSink{store: store, dispatcher: dispatcher}
}

type PolicyDispatchFailure struct {
	NodeKey string
	IssueID string
	Err     error
}

type PolicyDispatchError struct {
	ParentIssueID string
	Failures      []PolicyDispatchFailure
}

func (e *PolicyDispatchError) Error() string {
	if e == nil {
		return "loop policy dispatch failed"
	}
	return fmt.Sprintf("loop policy for %s committed but %d dispatch(es) failed", e.ParentIssueID, len(e.Failures))
}

func (e *PolicyDispatchError) Unwrap() error {
	if e == nil || len(e.Failures) == 0 {
		return nil
	}
	return e.Failures[0].Err
}

func (s *DurablePolicyMutationSink) ApplyPolicyDecision(ctx context.Context, cmd ApplyPolicyDecisionCommand) error {
	if s == nil || s.store == nil {
		return errors.New("policy mutation store is required")
	}
	batch, err := BuildPolicyMutationBatch(cmd)
	if err != nil {
		return err
	}
	commit, err := s.store.ApplyMutationBatchAtomically(ctx, batch)
	if err != nil {
		return err
	}
	if len(commit.Dispatch) == 0 {
		return nil
	}
	if s.dispatcher == nil {
		return &PolicyDispatchError{ParentIssueID: cmd.ParentIssueID, Failures: []PolicyDispatchFailure{{
			NodeKey: commit.Dispatch[0].NodeKey,
			IssueID: commit.Dispatch[0].IssueID,
			Err:     errors.New("issue dispatcher is required for committed runnable nodes"),
		}}}
	}

	failures := make([]PolicyDispatchFailure, 0)
	for _, req := range commit.Dispatch {
		if err := s.dispatcher.DispatchAssignedIssue(ctx, req); err != nil {
			failures = append(failures, PolicyDispatchFailure{NodeKey: req.NodeKey, IssueID: req.IssueID, Err: err})
		}
	}
	if len(failures) > 0 {
		return &PolicyDispatchError{ParentIssueID: cmd.ParentIssueID, Failures: failures}
	}
	return nil
}

// BuildPolicyMutationBatch is deliberately pure. It preserves action order,
// resolves node keys to durable Issue IDs, and records exactly which workflow
// retry transitions must increment the node retry counter.
func BuildPolicyMutationBatch(cmd ApplyPolicyDecisionCommand) (PolicyMutationBatch, error) {
	if strings.TrimSpace(cmd.WorkspaceID) == "" || strings.TrimSpace(cmd.ParentIssueID) == "" {
		return PolicyMutationBatch{}, errors.New("workspace and parent issue ids are required")
	}
	if strings.TrimSpace(cmd.PolicyVersion) == "" {
		return PolicyMutationBatch{}, errors.New("policy version is required")
	}
	if len(cmd.Actions) == 0 {
		return PolicyMutationBatch{}, errors.New("policy decision contains no actions")
	}

	mutations := make([]PolicyMutation, 0, len(cmd.Actions))
	for _, action := range cmd.Actions {
		mutation := PolicyMutation{NodeKey: action.NodeKey, ParentState: action.ParentState, Reason: action.Reason}
		switch action.Type {
		case looppolicy.ActionActivateNode:
			mutation.Type = MutationActivateNode
		case looppolicy.ActionReopenNode:
			mutation.Type = MutationReopenNode
			mutation.IncrementRetry = true
		case looppolicy.ActionParkNode:
			mutation.Type = MutationParkNode
		case looppolicy.ActionRequestApproval:
			mutation.Type = MutationRequestApproval
		case looppolicy.ActionSetParentState:
			mutation.Type = MutationSetParentState
		case looppolicy.ActionBlockLoop:
			mutation.Type = MutationBlockLoop
		case looppolicy.ActionCompleteLoop:
			mutation.Type = MutationCompleteLoop
		default:
			return PolicyMutationBatch{}, fmt.Errorf("unsupported policy action type %q", action.Type)
		}

		if action.NodeKey != "" {
			mutation.IssueID = strings.TrimSpace(cmd.NodeIssueIDs[action.NodeKey])
			if mutation.IssueID == "" {
				return PolicyMutationBatch{}, fmt.Errorf("policy action %q has no issue id for node %q", action.Type, action.NodeKey)
			}
		}
		mutations = append(mutations, mutation)
	}

	return PolicyMutationBatch{
		WorkspaceID: cmd.WorkspaceID, ParentIssueID: cmd.ParentIssueID,
		PolicyVersion: cmd.PolicyVersion, Reason: cmd.Reason, Mutations: mutations,
	}, nil
}

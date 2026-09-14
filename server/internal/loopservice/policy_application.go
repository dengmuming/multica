package loopservice

import (
	"context"
	"fmt"
	"strings"

	"github.com/multica-ai/multica/server/internal/looppolicy"
	"github.com/multica-ai/multica/server/internal/looptemplate"
)

// PolicyProjection is the complete authoritative snapshot needed for one policy
// tick. The reader must build RuntimeState from child Issues plus persisted
// Evaluation/Approval rows; comments or free-form Agent prose are not policy
// inputs.
type PolicyProjection struct {
	WorkspaceID   string
	ParentIssueID string
	PolicyVersion string
	Plan          looptemplate.CompiledPlan
	Runtime       looppolicy.RuntimeState
	NodeIssueIDs  map[string]string

	// Revision snapshots protect the write phase from a stale projection. A
	// concrete reader should populate these from the same Issue rows used to
	// build Runtime. The mutation store locks rows and verifies the snapshots
	// before applying any state change.
	ExpectedParentRevision *int64
	ExpectedNodeRevisions  map[string]int64
}

// PolicyProjectionReader loads the pinned template projection for one loop
// instance. A concrete repository should derive the template version from the
// parent Issue metadata rather than from a caller-supplied version.
type PolicyProjectionReader interface {
	LoadPolicyProjection(ctx context.Context, parentIssueID string) (PolicyProjection, error)
}

// PolicyMutationSink owns the write transaction for one deterministic policy
// decision. It translates actions into existing Multica Issue transitions,
// loop_approval/Inbox writes and ordinary assignment dispatch.
//
// Implementations must be idempotent: re-applying the same action set after a
// timeout must not create duplicate Tasks or duplicate pending approvals.
// Implementations should also protect against stale projections (for example by
// locking/reloading the parent/children or using revision predicates) before
// applying actions.
type PolicyMutationSink interface {
	ApplyPolicyDecision(ctx context.Context, cmd ApplyPolicyDecisionCommand) error
}

type ApplyPolicyDecisionCommand struct {
	WorkspaceID   string
	ParentIssueID string
	PolicyVersion string
	NodeIssueIDs  map[string]string
	Actions       []looppolicy.Action
	Reason        string

	ExpectedParentRevision *int64
	ExpectedNodeRevisions  map[string]int64
}

type PolicyTickResult struct {
	Decision looppolicy.Decision
	Applied  bool
}

// PolicyApplicationService is the server-owned control loop around the pure
// deterministic policy core. It deliberately has no LLM/Agent dependency.
type PolicyApplicationService struct {
	reader PolicyProjectionReader
	sink   PolicyMutationSink
}

func NewPolicyApplicationService(reader PolicyProjectionReader, sink PolicyMutationSink) *PolicyApplicationService {
	return &PolicyApplicationService{reader: reader, sink: sink}
}

// Tick evaluates one loop instance and atomically applies the legal next
// actions. A no-op decision is returned without opening a write transaction.
func (s *PolicyApplicationService) Tick(ctx context.Context, parentIssueID string) (PolicyTickResult, error) {
	if s == nil || s.reader == nil || s.sink == nil {
		return PolicyTickResult{}, fmt.Errorf("policy application dependencies are incomplete")
	}
	parentIssueID = strings.TrimSpace(parentIssueID)
	if parentIssueID == "" {
		return PolicyTickResult{}, fmt.Errorf("parent issue id is required")
	}

	projection, err := s.reader.LoadPolicyProjection(ctx, parentIssueID)
	if err != nil {
		return PolicyTickResult{}, fmt.Errorf("load loop policy projection: %w", err)
	}
	if err := validatePolicyProjection(projection, parentIssueID); err != nil {
		return PolicyTickResult{}, err
	}

	decision, err := looppolicy.Evaluate(projection.Plan, projection.Runtime)
	if err != nil {
		return PolicyTickResult{}, fmt.Errorf("evaluate loop policy: %w", err)
	}
	if len(decision.Actions) == 0 {
		return PolicyTickResult{Decision: decision, Applied: false}, nil
	}

	cmd := ApplyPolicyDecisionCommand{
		WorkspaceID:             projection.WorkspaceID,
		ParentIssueID:           projection.ParentIssueID,
		PolicyVersion:           projection.PolicyVersion,
		NodeIssueIDs:            cloneStringMap(projection.NodeIssueIDs),
		Actions:                 clonePolicyActions(decision.Actions),
		Reason:                  decision.Reason,
		ExpectedParentRevision:  cloneInt64Ptr(projection.ExpectedParentRevision),
		ExpectedNodeRevisions:   cloneInt64Map(projection.ExpectedNodeRevisions),
	}
	if err := validatePolicyActions(projection.Plan, cmd); err != nil {
		return PolicyTickResult{}, err
	}
	if err := s.sink.ApplyPolicyDecision(ctx, cmd); err != nil {
		return PolicyTickResult{Decision: decision, Applied: false}, fmt.Errorf("apply loop policy decision: %w", err)
	}
	return PolicyTickResult{Decision: decision, Applied: true}, nil
}

func validatePolicyProjection(projection PolicyProjection, requestedParentID string) error {
	if strings.TrimSpace(projection.WorkspaceID) == "" {
		return fmt.Errorf("policy projection workspace id is required")
	}
	if strings.TrimSpace(projection.ParentIssueID) == "" {
		return fmt.Errorf("policy projection parent issue id is required")
	}
	if projection.ParentIssueID != requestedParentID {
		return fmt.Errorf("policy projection parent issue mismatch: got %q want %q", projection.ParentIssueID, requestedParentID)
	}
	if strings.TrimSpace(projection.PolicyVersion) == "" {
		return fmt.Errorf("policy projection must pin policy version")
	}
	if len(projection.Plan.IncludedNodes) == 0 {
		return fmt.Errorf("policy projection compiled plan contains no nodes")
	}
	if len(projection.NodeIssueIDs) != len(projection.Plan.IncludedNodes) {
		return fmt.Errorf("policy projection has %d node issue ids for %d compiled nodes", len(projection.NodeIssueIDs), len(projection.Plan.IncludedNodes))
	}
	for _, node := range projection.Plan.IncludedNodes {
		if strings.TrimSpace(projection.NodeIssueIDs[node.Key]) == "" {
			return fmt.Errorf("policy projection missing issue id for node %q", node.Key)
		}
	}
	return nil
}

func validatePolicyActions(plan looptemplate.CompiledPlan, cmd ApplyPolicyDecisionCommand) error {
	for _, action := range cmd.Actions {
		switch action.Type {
		case looppolicy.ActionActivateNode, looppolicy.ActionReopenNode, looppolicy.ActionParkNode, looppolicy.ActionRequestApproval:
			if strings.TrimSpace(action.NodeKey) == "" {
				return fmt.Errorf("policy action %q requires node key", action.Type)
			}
			if _, ok := compiledNodeByKey(plan, action.NodeKey); !ok {
				return fmt.Errorf("policy action %q references unknown node %q", action.Type, action.NodeKey)
			}
			if strings.TrimSpace(cmd.NodeIssueIDs[action.NodeKey]) == "" {
				return fmt.Errorf("policy action %q has no persisted issue id for node %q", action.Type, action.NodeKey)
			}
		case looppolicy.ActionSetParentState, looppolicy.ActionBlockLoop, looppolicy.ActionCompleteLoop:
			if strings.TrimSpace(action.ParentState) == "" {
				return fmt.Errorf("policy action %q requires parent state", action.Type)
			}
		default:
			return fmt.Errorf("unsupported policy action type %q", action.Type)
		}
	}
	return nil
}

func compiledNodeByKey(plan looptemplate.CompiledPlan, key string) (looptemplate.CompiledNode, bool) {
	for _, node := range plan.IncludedNodes {
		if node.Key == key {
			return node, true
		}
	}
	return looptemplate.CompiledNode{}, false
}

func clonePolicyActions(in []looppolicy.Action) []looppolicy.Action {
	if in == nil {
		return nil
	}
	out := make([]looppolicy.Action, len(in))
	copy(out, in)
	return out
}

func cloneInt64Map(in map[string]int64) map[string]int64 {
	if in == nil {
		return nil
	}
	out := make(map[string]int64, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneInt64Ptr(in *int64) *int64 {
	if in == nil {
		return nil
	}
	value := *in
	return &value
}

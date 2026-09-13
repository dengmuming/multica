package looptemplate

// Definition is the canonical persisted shape of loop_template.definition.
// P0 deliberately models a small staged workflow: ordering comes from Stage,
// and parallelism is represented by multiple nodes sharing the same Stage.
type Definition struct {
	SchemaVersion int                       `json:"schema_version"`
	Key           string                    `json:"key"`
	Name          string                    `json:"name"`
	Description   string                    `json:"description,omitempty"`
	Labels        []string                  `json:"labels,omitempty"`
	Defaults      map[string]any            `json:"defaults,omitempty"`
	Roles         map[string]RoleDefinition `json:"roles"`
	Nodes         []Node                    `json:"nodes"`
}

type RoleDefinition struct {
	Required             bool     `json:"required"`
	Description          string   `json:"description,omitempty"`
	AllowedAssigneeTypes []string `json:"allowed_assignee_types,omitempty"`
}

type Node struct {
	Key         string            `json:"key"`
	Type        string            `json:"type"`
	Stage       int               `json:"stage"`
	Role        string            `json:"role,omitempty"`
	Title       string            `json:"title"`
	Description string            `json:"description,omitempty"`
	Required    *bool             `json:"required,omitempty"`
	Inputs      []string          `json:"inputs,omitempty"`
	Outputs     []string          `json:"outputs,omitempty"`
	MaxRetries *int              `json:"max_retries,omitempty"`
	Evaluation *EvaluationConfig `json:"evaluation,omitempty"`
	Approval    *ApprovalConfig   `json:"approval,omitempty"`
}

func (n Node) IsRequired() bool {
	return n.Required == nil || *n.Required
}

type EvaluationConfig struct {
	Kind             string               `json:"kind"`
	AllowedVerdicts  []string             `json:"allowed_verdicts"`
	OnFail           EvaluationFailConfig `json:"on_fail,omitempty"`
	OnInconclusive   string               `json:"on_inconclusive,omitempty"`
}

type EvaluationFailConfig struct {
	Strategy               string `json:"strategy"`
	TargetNode             string `json:"target_node,omitempty"`
	FallbackNode           string `json:"fallback_node,omitempty"`
	MaxWorkflowRetries     int    `json:"max_workflow_retries,omitempty"`
}

type ApprovalConfig struct {
	RequiredRole string               `json:"required_role"`
	MinApprovals int                  `json:"min_approvals"`
	OnReject     ApprovalRejectConfig `json:"on_reject,omitempty"`
}

type ApprovalRejectConfig struct {
	TargetNode         string `json:"target_node,omitempty"`
	MaxWorkflowRetries int    `json:"max_workflow_retries,omitempty"`
}

// RoleBinding is supplied when a template is instantiated. IDs stay strings at
// this layer so the validator is independent of pgx/sqlc; the compiler service
// performs workspace-scoped UUID/entity lookups before writing Issues.
type RoleBinding struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type RoleBindings map[string]RoleBinding

const (
	NodeTypeAgent      = "agent"
	NodeTypeEvaluation = "evaluation"
	NodeTypeApproval   = "approval"
)

package looptemplate

import (
	"fmt"
	"sort"
)

// CompiledPlan is the deterministic, side-effect-free result of compiling a
// template plus concrete role bindings. Persistence code consumes this plan to
// create one child Issue per IncludedNode. No DB identity is allocated here.
type CompiledPlan struct {
	TemplateKey     string         `json:"template_key"`
	SchemaVersion   int            `json:"schema_version"`
	InitialStage    int            `json:"initial_stage"`
	IncludedNodes   []CompiledNode `json:"included_nodes"`
	OmittedNodeKeys []string       `json:"omitted_node_keys,omitempty"`
}

type CompiledNode struct {
	Key         string       `json:"key"`
	Type        string       `json:"type"`
	Stage       int          `json:"stage"`
	Title       string       `json:"title"`
	Description string       `json:"description,omitempty"`
	Role        string       `json:"role,omitempty"`
	Binding     *RoleBinding `json:"binding,omitempty"`
	Required    bool         `json:"required"`
	Inputs      []string     `json:"inputs,omitempty"`
	Outputs     []string     `json:"outputs,omitempty"`
	MaxRetries int          `json:"max_retries"`
	InitialStatus string     `json:"initial_status"`
	Evaluation *EvaluationConfig `json:"evaluation,omitempty"`
	Approval    *ApprovalConfig   `json:"approval,omitempty"`
}

// Compile validates the template/bindings and turns symbolic roles into a
// deterministic list of Issue-shaped nodes. Optional executable nodes without
// a binding are omitted. Approval nodes are always included because they are
// server-owned human gates, not role-bound workers.
func Compile(def Definition, bindings RoleBindings) (CompiledPlan, error) {
	if err := ValidateBindings(def, bindings); err != nil {
		return CompiledPlan{}, err
	}

	plan := CompiledPlan{
		TemplateKey:   def.Key,
		SchemaVersion: def.SchemaVersion,
	}

	for _, node := range def.Nodes {
		compiled, include, err := compileNode(node, bindings)
		if err != nil {
			return CompiledPlan{}, err
		}
		if !include {
			plan.OmittedNodeKeys = append(plan.OmittedNodeKeys, node.Key)
			continue
		}
		plan.IncludedNodes = append(plan.IncludedNodes, compiled)
	}

	if len(plan.IncludedNodes) == 0 {
		return CompiledPlan{}, fmt.Errorf("compiled loop contains no nodes")
	}

	// Template authoring order is not workflow truth. Normalize output so the
	// same semantic template always yields the same plan: stage first, then key.
	sort.SliceStable(plan.IncludedNodes, func(i, j int) bool {
		if plan.IncludedNodes[i].Stage == plan.IncludedNodes[j].Stage {
			return plan.IncludedNodes[i].Key < plan.IncludedNodes[j].Key
		}
		return plan.IncludedNodes[i].Stage < plan.IncludedNodes[j].Stage
	})
	sort.Strings(plan.OmittedNodeKeys)

	plan.InitialStage = plan.IncludedNodes[0].Stage
	for i := range plan.IncludedNodes {
		if plan.IncludedNodes[i].Stage == plan.InitialStage {
			plan.IncludedNodes[i].InitialStatus = "todo"
		} else {
			plan.IncludedNodes[i].InitialStatus = "backlog"
		}
	}

	return plan, nil
}

func compileNode(node Node, bindings RoleBindings) (CompiledNode, bool, error) {
	compiled := CompiledNode{
		Key:         node.Key,
		Type:        node.Type,
		Stage:       node.Stage,
		Title:       node.Title,
		Description: node.Description,
		Role:        node.Role,
		Required:    node.IsRequired(),
		Inputs:      append([]string(nil), node.Inputs...),
		Outputs:     append([]string(nil), node.Outputs...),
		Evaluation:  cloneEvaluation(node.Evaluation),
		Approval:    cloneApproval(node.Approval),
	}
	if node.MaxRetries != nil {
		compiled.MaxRetries = *node.MaxRetries
	}

	if node.Type == NodeTypeApproval {
		return compiled, true, nil
	}

	binding, bound := bindings[node.Role]
	if !bound {
		if node.IsRequired() {
			// ValidateBindings should have rejected this already; keep the guard so
			// compileNode stays safe if it is reused independently later.
			return CompiledNode{}, false, fmt.Errorf("required node %q has no binding for role %q", node.Key, node.Role)
		}
		return CompiledNode{}, false, nil
	}
	bindingCopy := binding
	compiled.Binding = &bindingCopy
	return compiled, true, nil
}

func cloneEvaluation(in *EvaluationConfig) *EvaluationConfig {
	if in == nil {
		return nil
	}
	out := *in
	out.AllowedVerdicts = append([]string(nil), in.AllowedVerdicts...)
	return &out
}

func cloneApproval(in *ApprovalConfig) *ApprovalConfig {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

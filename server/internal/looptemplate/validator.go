package looptemplate

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const maxP0Nodes = 64

var (
	templateKeyRE = regexp.MustCompile(`^[a-z][a-z0-9-]{1,62}$`)
	symbolKeyRE   = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)
	artifactKeyRE = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,62}$`)
)

type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string {
	return "invalid loop template: " + strings.Join(e.Problems, "; ")
}

func Validate(def Definition) error {
	problems := make([]string, 0)
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	if def.SchemaVersion != 1 {
		add("schema_version must be 1")
	}
	if !templateKeyRE.MatchString(def.Key) {
		add("key must match %s", templateKeyRE.String())
	}
	if strings.TrimSpace(def.Name) == "" {
		add("name is required")
	}
	if len(def.Nodes) == 0 {
		add("nodes must contain at least one node")
	}
	if len(def.Nodes) > maxP0Nodes {
		add("nodes cannot exceed %d entries", maxP0Nodes)
	}

	for roleKey, role := range def.Roles {
		if !symbolKeyRE.MatchString(roleKey) {
			add("role %q has an invalid key", roleKey)
		}
		seenTypes := map[string]bool{}
		for _, assigneeType := range role.AllowedAssigneeTypes {
			if !isAssigneeType(assigneeType) {
				add("role %q has unsupported assignee type %q", roleKey, assigneeType)
			}
			if seenTypes[assigneeType] {
				add("role %q repeats assignee type %q", roleKey, assigneeType)
			}
			seenTypes[assigneeType] = true
		}
	}

	nodeByKey := make(map[string]Node, len(def.Nodes))
	for i, node := range def.Nodes {
		prefix := fmt.Sprintf("nodes[%d]", i)
		if !symbolKeyRE.MatchString(node.Key) {
			add("%s.key is invalid", prefix)
		} else if _, exists := nodeByKey[node.Key]; exists {
			add("node key %q is duplicated", node.Key)
		} else {
			nodeByKey[node.Key] = node
		}
		if node.Stage <= 0 {
			add("%s.stage must be positive", prefix)
		}
		if strings.TrimSpace(node.Title) == "" {
			add("%s.title is required", prefix)
		}
		if node.MaxRetries != nil && *node.MaxRetries < 0 {
			add("%s.max_retries cannot be negative", prefix)
		}
		for _, artifact := range append(append([]string{}, node.Inputs...), node.Outputs...) {
			if !artifactKeyRE.MatchString(artifact) {
				add("%s has invalid artifact kind %q", prefix, artifact)
			}
		}

		switch node.Type {
		case NodeTypeAgent:
			validateExecutableNode(prefix, node, def.Roles, add)
			if node.Evaluation != nil {
				add("%s agent node cannot define evaluation", prefix)
			}
			if node.Approval != nil {
				add("%s agent node cannot define approval", prefix)
			}
		case NodeTypeEvaluation:
			validateExecutableNode(prefix, node, def.Roles, add)
			if node.Evaluation == nil {
				add("%s evaluation config is required", prefix)
			}
			if node.Approval != nil {
				add("%s evaluation node cannot define approval", prefix)
			}
		case NodeTypeApproval:
			if node.Role != "" {
				add("%s approval node cannot define role", prefix)
			}
			if node.Approval == nil {
				add("%s approval config is required", prefix)
			}
			if node.Evaluation != nil {
				add("%s approval node cannot define evaluation", prefix)
			}
		default:
			add("%s.type must be agent, evaluation, or approval", prefix)
		}
	}

	for i, node := range def.Nodes {
		prefix := fmt.Sprintf("nodes[%d]", i)
		if node.Evaluation != nil {
			validateEvaluation(prefix, node, *node.Evaluation, nodeByKey, add)
		}
		if node.Approval != nil {
			validateApproval(prefix, node, *node.Approval, nodeByKey, add)
		}
	}

	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return &ValidationError{Problems: problems}
}

func ValidateBindings(def Definition, bindings RoleBindings) error {
	if err := Validate(def); err != nil {
		return err
	}
	problems := make([]string, 0)
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	for roleKey, binding := range bindings {
		role, exists := def.Roles[roleKey]
		if !exists {
			add("binding references unknown role %q", roleKey)
			continue
		}
		if !isAssigneeType(binding.Type) {
			add("binding for role %q has unsupported type %q", roleKey, binding.Type)
		}
		if strings.TrimSpace(binding.ID) == "" {
			add("binding for role %q requires id", roleKey)
		}
		if len(role.AllowedAssigneeTypes) > 0 && !contains(role.AllowedAssigneeTypes, binding.Type) {
			add("binding type %q is not allowed for role %q", binding.Type, roleKey)
		}
	}

	for roleKey, role := range def.Roles {
		_, bound := bindings[roleKey]
		requiredByNode := false
		usedByNode := false
		for _, node := range def.Nodes {
			if node.Role != roleKey {
				continue
			}
			usedByNode = true
			if node.IsRequired() {
				requiredByNode = true
			}
		}
		if !bound && (role.Required || requiredByNode) {
			add("required role %q has no binding", roleKey)
		}
		if role.Required && !usedByNode {
			add("required role %q is not used by any node", roleKey)
		}
	}

	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return &ValidationError{Problems: problems}
}

func validateExecutableNode(prefix string, node Node, roles map[string]RoleDefinition, add func(string, ...any)) {
	if node.Role == "" {
		add("%s.role is required", prefix)
		return
	}
	if _, exists := roles[node.Role]; !exists {
		add("%s references unknown role %q", prefix, node.Role)
	}
}

func validateEvaluation(prefix string, node Node, cfg EvaluationConfig, nodeByKey map[string]Node, add func(string, ...any)) {
	if strings.TrimSpace(cfg.Kind) == "" {
		add("%s.evaluation.kind is required", prefix)
	}
	if len(cfg.AllowedVerdicts) == 0 {
		add("%s.evaluation.allowed_verdicts cannot be empty", prefix)
	}
	seen := map[string]bool{}
	for _, verdict := range cfg.AllowedVerdicts {
		if !isVerdict(verdict) {
			add("%s.evaluation has unsupported verdict %q", prefix, verdict)
		}
		if seen[verdict] {
			add("%s.evaluation repeats verdict %q", prefix, verdict)
		}
		seen[verdict] = true
	}
	if !seen["pass"] {
		add("%s.evaluation must allow pass so a forward terminal path exists", prefix)
	}
	if cfg.OnInconclusive != "" && cfg.OnInconclusive != "block" && cfg.OnInconclusive != "retry_evaluator" {
		add("%s.evaluation.on_inconclusive must be block or retry_evaluator", prefix)
	}
	if cfg.OnFail.MaxWorkflowRetries < 0 {
		add("%s.evaluation.on_fail.max_workflow_retries cannot be negative", prefix)
	}
	if !seen["fail"] {
		if cfg.OnFail.Strategy != "" || cfg.OnFail.TargetNode != "" || cfg.OnFail.FallbackNode != "" || cfg.OnFail.MaxWorkflowRetries != 0 {
			add("%s.evaluation.on_fail is configured but fail is not an allowed verdict", prefix)
		}
		return
	}

	switch cfg.OnFail.Strategy {
	case "fixed_target":
		if cfg.OnFail.TargetNode == "" {
			add("%s.evaluation.on_fail.target_node is required for fixed_target", prefix)
		} else {
			validateRecoveryTarget(prefix+".evaluation.on_fail.target_node", node, cfg.OnFail.TargetNode, nodeByKey, add)
		}
	case "route_by_finding_owner":
		if cfg.OnFail.FallbackNode == "" {
			add("%s.evaluation.on_fail.fallback_node is required for route_by_finding_owner", prefix)
		} else {
			validateRecoveryTarget(prefix+".evaluation.on_fail.fallback_node", node, cfg.OnFail.FallbackNode, nodeByKey, add)
		}
	case "block":
		if cfg.OnFail.TargetNode != "" || cfg.OnFail.FallbackNode != "" {
			add("%s.evaluation.on_fail block strategy cannot define a target", prefix)
		}
	default:
		add("%s.evaluation.on_fail.strategy must be fixed_target, route_by_finding_owner, or block", prefix)
	}
}

func validateApproval(prefix string, node Node, cfg ApprovalConfig, nodeByKey map[string]Node, add func(string, ...any)) {
	if cfg.RequiredRole != "member" {
		add("%s.approval.required_role must be member in P0", prefix)
	}
	if cfg.MinApprovals != 1 {
		add("%s.approval.min_approvals must be 1 in P0", prefix)
	}
	if cfg.OnReject.MaxWorkflowRetries < 0 {
		add("%s.approval.on_reject.max_workflow_retries cannot be negative", prefix)
	}
	if cfg.OnReject.TargetNode != "" {
		validateRecoveryTarget(prefix+".approval.on_reject.target_node", node, cfg.OnReject.TargetNode, nodeByKey, add)
	}
}

func validateRecoveryTarget(field string, source Node, targetKey string, nodeByKey map[string]Node, add func(string, ...any)) {
	target, exists := nodeByKey[targetKey]
	if !exists {
		add("%s references unknown node %q", field, targetKey)
		return
	}
	if target.Stage > source.Stage {
		add("%s cannot route forward from stage %d to stage %d", field, source.Stage, target.Stage)
	}
	if target.Type == NodeTypeApproval {
		add("%s cannot route recovery to approval node %q", field, targetKey)
	}
}

func isAssigneeType(value string) bool {
	return value == "agent" || value == "squad" || value == "member"
}

func isVerdict(value string) bool {
	return value == "pass" || value == "fail" || value == "warn" || value == "inconclusive"
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

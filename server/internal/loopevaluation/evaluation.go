package loopevaluation

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/multica-ai/multica/server/internal/looptemplate"
)

var findingCodeRE = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)

type Submission struct {
	EvaluationKind string    `json:"evaluation_kind"`
	Verdict        string    `json:"verdict"`
	Score          *float64  `json:"score,omitempty"`
	Findings       []Finding `json:"findings,omitempty"`
	Evidence       []string  `json:"evidence,omitempty"`
}

type Finding struct {
	Code          string   `json:"code"`
	Severity      string   `json:"severity"`
	OwnerRole     string   `json:"owner_role,omitempty"`
	OwnerNodeKey  string   `json:"owner_node_key,omitempty"`
	Summary       string   `json:"summary"`
	ArtifactRefs  []string `json:"artifact_refs,omitempty"`
}

// Normalized is safe for persistence/policy after workspace-level artifact and
// submitting Task/Issue authorization checks have also passed in the service
// layer. RecoveryTarget is deterministic and never derived from prose.
type Normalized struct {
	Submission
	RecoveryTarget string `json:"recovery_target,omitempty"`
}

type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string {
	return "invalid loop evaluation: " + strings.Join(e.Problems, "; ")
}

// Validate checks a structured evaluator payload against the exact compiled
// evaluation-node contract. It does not perform DB authorization/visibility
// checks; callers must separately prove Task/Issue/artifact ownership.
func Validate(plan looptemplate.CompiledPlan, nodeKey string, in Submission) (Normalized, error) {
	node, ok := nodeByKey(plan, nodeKey)
	if !ok {
		return Normalized{}, &ValidationError{Problems: []string{fmt.Sprintf("unknown evaluation node %q", nodeKey)}}
	}
	if node.Type != looptemplate.NodeTypeEvaluation || node.Evaluation == nil {
		return Normalized{}, &ValidationError{Problems: []string{fmt.Sprintf("node %q is not an evaluation node", nodeKey)}}
	}

	problems := make([]string, 0)
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	cfg := node.Evaluation
	if in.EvaluationKind != cfg.Kind {
		add("evaluation_kind %q does not match contract %q", in.EvaluationKind, cfg.Kind)
	}
	if !contains(cfg.AllowedVerdicts, in.Verdict) {
		add("verdict %q is not allowed by node %q", in.Verdict, nodeKey)
	}
	if in.Score != nil && (*in.Score < 0 || *in.Score > 1) {
		add("score must be between 0 and 1")
	}

	resolvedTargets := map[string]bool{}
	for i, finding := range in.Findings {
		prefix := fmt.Sprintf("findings[%d]", i)
		if !findingCodeRE.MatchString(finding.Code) {
			add("%s.code is invalid", prefix)
		}
		if !isSeverity(finding.Severity) {
			add("%s.severity must be info, low, medium, high, or critical", prefix)
		}
		if strings.TrimSpace(finding.Summary) == "" {
			add("%s.summary is required", prefix)
		}
		for j, ref := range finding.ArtifactRefs {
			if strings.TrimSpace(ref) == "" {
				add("%s.artifact_refs[%d] cannot be empty", prefix, j)
			}
		}

		if finding.OwnerNodeKey != "" {
			target, exists := nodeByKey(plan, finding.OwnerNodeKey)
			if !exists {
				add("%s.owner_node_key references unknown node %q", prefix, finding.OwnerNodeKey)
			} else if err := validateRecoveryTarget(node, target); err != nil {
				add("%s.owner_node_key %s", prefix, err.Error())
			} else {
				resolvedTargets[target.Key] = true
			}
		}
		if finding.OwnerRole != "" {
			matches := nodesByRole(plan, finding.OwnerRole)
			if len(matches) == 0 {
				add("%s.owner_role references unknown/uncompiled role %q", prefix, finding.OwnerRole)
			} else if len(matches) > 1 && finding.OwnerNodeKey == "" {
				add("%s.owner_role %q is ambiguous; owner_node_key is required", prefix, finding.OwnerRole)
			} else if finding.OwnerNodeKey == "" {
				if err := validateRecoveryTarget(node, matches[0]); err != nil {
					add("%s.owner_role %s", prefix, err.Error())
				} else {
					resolvedTargets[matches[0].Key] = true
				}
			} else {
				target, exists := nodeByKey(plan, finding.OwnerNodeKey)
				if exists && target.Role != finding.OwnerRole {
					add("%s owner_role %q does not match owner_node_key %q role %q", prefix, finding.OwnerRole, finding.OwnerNodeKey, target.Role)
				}
			}
		}
	}
	for i, ref := range in.Evidence {
		if strings.TrimSpace(ref) == "" {
			add("evidence[%d] cannot be empty", i)
		}
	}

	normalized := Normalized{Submission: in}
	if in.Verdict == "fail" {
		target, err := resolveFailureTarget(plan, node, resolvedTargets)
		if err != nil {
			add("%s", err.Error())
		} else {
			normalized.RecoveryTarget = target
		}
	}

	if len(problems) > 0 {
		sort.Strings(problems)
		return Normalized{}, &ValidationError{Problems: problems}
	}
	return normalized, nil
}

func resolveFailureTarget(plan looptemplate.CompiledPlan, gate looptemplate.CompiledNode, findingTargets map[string]bool) (string, error) {
	cfg := gate.Evaluation.OnFail
	switch cfg.Strategy {
	case "block":
		return "", nil
	case "fixed_target":
		target, ok := nodeByKey(plan, cfg.TargetNode)
		if !ok {
			return "", fmt.Errorf("fixed recovery target %q is not compiled", cfg.TargetNode)
		}
		if err := validateRecoveryTarget(gate, target); err != nil {
			return "", fmt.Errorf("fixed recovery target %s", err.Error())
		}
		return target.Key, nil
	case "route_by_finding_owner":
		keys := make([]string, 0, len(findingTargets))
		for key := range findingTargets {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if len(keys) == 1 {
			return keys[0], nil
		}
		if len(keys) > 1 {
			return "", fmt.Errorf("multiple recovery targets %v are not supported in P0; route findings to one node or use the fallback", keys)
		}
		if cfg.FallbackNode == "" {
			return "", fmt.Errorf("failed evaluation has no finding owner and no fallback_node")
		}
		target, ok := nodeByKey(plan, cfg.FallbackNode)
		if !ok {
			return "", fmt.Errorf("fallback recovery target %q is not compiled", cfg.FallbackNode)
		}
		if err := validateRecoveryTarget(gate, target); err != nil {
			return "", fmt.Errorf("fallback recovery target %s", err.Error())
		}
		return target.Key, nil
	default:
		return "", fmt.Errorf("unsupported on_fail strategy %q", cfg.Strategy)
	}
}

func validateRecoveryTarget(gate, target looptemplate.CompiledNode) error {
	if target.Stage > gate.Stage {
		return fmt.Errorf("cannot route forward from stage %d to node %q at stage %d", gate.Stage, target.Key, target.Stage)
	}
	if target.Type == looptemplate.NodeTypeApproval {
		return fmt.Errorf("cannot target approval node %q", target.Key)
	}
	return nil
}

func nodeByKey(plan looptemplate.CompiledPlan, key string) (looptemplate.CompiledNode, bool) {
	for _, node := range plan.IncludedNodes {
		if node.Key == key {
			return node, true
		}
	}
	return looptemplate.CompiledNode{}, false
}

func nodesByRole(plan looptemplate.CompiledPlan, role string) []looptemplate.CompiledNode {
	var nodes []looptemplate.CompiledNode
	for _, node := range plan.IncludedNodes {
		if node.Role == role {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func isSeverity(value string) bool {
	switch value {
	case "info", "low", "medium", "high", "critical":
		return true
	default:
		return false
	}
}

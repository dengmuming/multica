package looptemplate

import (
	"strings"
	"testing"
)

func boolPtr(v bool) *bool { return &v }

func validFeatureDefinition() Definition {
	return Definition{
		SchemaVersion: 1,
		Key:           "feature-development",
		Name:          "Feature Development",
		Roles: map[string]RoleDefinition{
			"product":      {Required: true, AllowedAssigneeTypes: []string{"agent"}},
			"architect":    {Required: true, AllowedAssigneeTypes: []string{"agent"}},
			"backend":      {AllowedAssigneeTypes: []string{"agent", "squad"}},
			"frontend":     {AllowedAssigneeTypes: []string{"agent", "squad"}},
			"review":       {Required: true, AllowedAssigneeTypes: []string{"agent"}},
			"test":         {Required: true, AllowedAssigneeTypes: []string{"agent"}},
			"release":      {Required: true, AllowedAssigneeTypes: []string{"agent"}},
		},
		Nodes: []Node{
			{Key: "product", Type: NodeTypeAgent, Stage: 10, Role: "product", Title: "Product specification", Outputs: []string{"product_spec"}},
			{Key: "architecture", Type: NodeTypeAgent, Stage: 20, Role: "architect", Title: "Architecture", Inputs: []string{"product_spec"}, Outputs: []string{"architecture_plan"}},
			{Key: "backend", Type: NodeTypeAgent, Stage: 30, Role: "backend", Title: "Backend", Required: boolPtr(false)},
			{Key: "frontend", Type: NodeTypeAgent, Stage: 30, Role: "frontend", Title: "Frontend", Required: boolPtr(false)},
			{
				Key: "review", Type: NodeTypeEvaluation, Stage: 40, Role: "review", Title: "Review",
				Evaluation: &EvaluationConfig{
					Kind: "engineering_review", AllowedVerdicts: []string{"pass", "fail", "warn"},
					OnFail: EvaluationFailConfig{Strategy: "route_by_finding_owner", FallbackNode: "architecture", MaxWorkflowRetries: 2},
				},
			},
			{
				Key: "test", Type: NodeTypeEvaluation, Stage: 50, Role: "test", Title: "Test",
				Evaluation: &EvaluationConfig{
					Kind: "test", AllowedVerdicts: []string{"pass", "fail", "inconclusive"}, OnInconclusive: "block",
					OnFail: EvaluationFailConfig{Strategy: "route_by_finding_owner", FallbackNode: "backend", MaxWorkflowRetries: 3},
				},
			},
			{
				Key: "release_approval", Type: NodeTypeApproval, Stage: 60, Title: "Release approval",
				Approval: &ApprovalConfig{RequiredRole: "member", MinApprovals: 1, OnReject: ApprovalRejectConfig{TargetNode: "architecture", MaxWorkflowRetries: 1}},
			},
			{Key: "release", Type: NodeTypeAgent, Stage: 70, Role: "release", Title: "Release"},
		},
	}
}

func TestValidateAcceptsFeatureDevelopmentTemplate(t *testing.T) {
	if err := Validate(validFeatureDefinition()); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsDuplicateNodeKey(t *testing.T) {
	def := validFeatureDefinition()
	def.Nodes = append(def.Nodes, Node{Key: "backend", Type: NodeTypeAgent, Stage: 35, Role: "backend", Title: "Duplicate"})
	assertValidationContains(t, Validate(def), `node key "backend" is duplicated`)
}

func TestValidateRejectsForwardRecoveryRoute(t *testing.T) {
	def := validFeatureDefinition()
	for i := range def.Nodes {
		if def.Nodes[i].Key == "review" {
			def.Nodes[i].Evaluation.OnFail.FallbackNode = "release"
		}
	}
	assertValidationContains(t, Validate(def), "cannot route forward")
}

func TestValidateRejectsInvalidApprovalQuorum(t *testing.T) {
	def := validFeatureDefinition()
	for i := range def.Nodes {
		if def.Nodes[i].Key == "release_approval" {
			def.Nodes[i].Approval.MinApprovals = 2
		}
	}
	assertValidationContains(t, Validate(def), "min_approvals must be 1 in P0")
}

func TestValidateBindingsAllowsMissingOptionalRole(t *testing.T) {
	def := validFeatureDefinition()
	bindings := RoleBindings{
		"product":   {Type: "agent", ID: "product-agent"},
		"architect": {Type: "agent", ID: "architect-agent"},
		"review":    {Type: "agent", ID: "review-agent"},
		"test":      {Type: "agent", ID: "test-agent"},
		"release":   {Type: "agent", ID: "release-agent"},
	}
	if err := ValidateBindings(def, bindings); err != nil {
		t.Fatalf("ValidateBindings() error = %v", err)
	}
}

func TestValidateBindingsRejectsMissingRequiredRole(t *testing.T) {
	def := validFeatureDefinition()
	bindings := RoleBindings{
		"product": {Type: "agent", ID: "product-agent"},
		"review":  {Type: "agent", ID: "review-agent"},
		"test":    {Type: "agent", ID: "test-agent"},
		"release": {Type: "agent", ID: "release-agent"},
	}
	assertValidationContains(t, ValidateBindings(def, bindings), `required role "architect" has no binding`)
}

func TestValidateBindingsRejectsDisallowedAssigneeType(t *testing.T) {
	def := validFeatureDefinition()
	bindings := RoleBindings{
		"product":   {Type: "member", ID: "human"},
		"architect": {Type: "agent", ID: "architect-agent"},
		"review":    {Type: "agent", ID: "review-agent"},
		"test":      {Type: "agent", ID: "test-agent"},
		"release":   {Type: "agent", ID: "release-agent"},
	}
	assertValidationContains(t, ValidateBindings(def, bindings), `binding type "member" is not allowed for role "product"`)
}

func assertValidationContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected validation error containing %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want substring %q", err.Error(), want)
	}
}

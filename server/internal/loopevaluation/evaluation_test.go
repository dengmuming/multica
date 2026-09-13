package loopevaluation

import (
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/looptemplate"
)

func evaluationPlan() looptemplate.CompiledPlan {
	return looptemplate.CompiledPlan{
		TemplateKey:   "feature-development",
		SchemaVersion: 1,
		IncludedNodes: []looptemplate.CompiledNode{
			{Key: "architecture", Type: looptemplate.NodeTypeAgent, Stage: 20, Role: "architect"},
			{Key: "backend", Type: looptemplate.NodeTypeAgent, Stage: 30, Role: "backend"},
			{Key: "frontend", Type: looptemplate.NodeTypeAgent, Stage: 30, Role: "frontend"},
			{
				Key: "test", Type: looptemplate.NodeTypeEvaluation, Stage: 50, Role: "test",
				Evaluation: &looptemplate.EvaluationConfig{
					Kind: "test",
					AllowedVerdicts: []string{"pass", "fail", "inconclusive"},
					OnFail: looptemplate.EvaluationFailConfig{
						Strategy: "route_by_finding_owner", FallbackNode: "backend", MaxWorkflowRetries: 3,
					},
				},
			},
		},
	}
}

func TestValidatePassEvaluation(t *testing.T) {
	got, err := Validate(evaluationPlan(), "test", Submission{
		EvaluationKind: "test",
		Verdict:        "pass",
		Evidence:       []string{"artifact-test-report"},
	})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if got.RecoveryTarget != "" {
		t.Fatalf("RecoveryTarget = %q, want empty", got.RecoveryTarget)
	}
}

func TestValidateResolvesFindingOwnerNode(t *testing.T) {
	got, err := Validate(evaluationPlan(), "test", Submission{
		EvaluationKind: "test",
		Verdict:        "fail",
		Findings: []Finding{{
			Code: "api_contract_failure", Severity: "high", OwnerRole: "backend",
			OwnerNodeKey: "backend", Summary: "POST /token returns 500",
		}},
	})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if got.RecoveryTarget != "backend" {
		t.Fatalf("RecoveryTarget = %q, want backend", got.RecoveryTarget)
	}
}

func TestValidateResolvesFindingOwnerRole(t *testing.T) {
	got, err := Validate(evaluationPlan(), "test", Submission{
		EvaluationKind: "test",
		Verdict:        "fail",
		Findings: []Finding{{
			Code: "ui_failure", Severity: "medium", OwnerRole: "frontend", Summary: "CTA is disabled",
		}},
	})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if got.RecoveryTarget != "frontend" {
		t.Fatalf("RecoveryTarget = %q, want frontend", got.RecoveryTarget)
	}
}

func TestValidateUsesFallbackWhenFindingHasNoOwner(t *testing.T) {
	got, err := Validate(evaluationPlan(), "test", Submission{
		EvaluationKind: "test",
		Verdict:        "fail",
		Findings: []Finding{{
			Code: "unknown_regression", Severity: "high", Summary: "Regression owner is unclear",
		}},
	})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if got.RecoveryTarget != "backend" {
		t.Fatalf("RecoveryTarget = %q, want fallback backend", got.RecoveryTarget)
	}
}

func TestValidateRejectsMultipleRecoveryTargetsInP0(t *testing.T) {
	_, err := Validate(evaluationPlan(), "test", Submission{
		EvaluationKind: "test",
		Verdict:        "fail",
		Findings: []Finding{
			{Code: "backend_failure", Severity: "high", OwnerNodeKey: "backend", Summary: "backend failed"},
			{Code: "frontend_failure", Severity: "medium", OwnerNodeKey: "frontend", Summary: "frontend failed"},
		},
	})
	assertErrorContains(t, err, "multiple recovery targets")
}

func TestValidateRejectsWrongEvaluationKind(t *testing.T) {
	_, err := Validate(evaluationPlan(), "test", Submission{EvaluationKind: "security", Verdict: "pass"})
	assertErrorContains(t, err, "does not match contract")
}

func TestValidateRejectsOwnerRoleMismatch(t *testing.T) {
	_, err := Validate(evaluationPlan(), "test", Submission{
		EvaluationKind: "test",
		Verdict:        "fail",
		Findings: []Finding{{
			Code: "api_failure", Severity: "high", OwnerRole: "frontend", OwnerNodeKey: "backend", Summary: "mismatch",
		}},
	})
	assertErrorContains(t, err, "does not match owner_node_key")
}

func TestValidateRejectsScoreOutsideUnitInterval(t *testing.T) {
	score := 1.2
	_, err := Validate(evaluationPlan(), "test", Submission{EvaluationKind: "test", Verdict: "pass", Score: &score})
	assertErrorContains(t, err, "score must be between 0 and 1")
}

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want substring %q", err.Error(), want)
	}
}

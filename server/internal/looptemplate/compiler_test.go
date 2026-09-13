package looptemplate

import "testing"

func requiredBindings() RoleBindings {
	return RoleBindings{
		"product":   {Type: "agent", ID: "product-agent"},
		"architect": {Type: "agent", ID: "architect-agent"},
		"review":    {Type: "agent", ID: "review-agent"},
		"test":      {Type: "agent", ID: "test-agent"},
		"release":   {Type: "agent", ID: "release-agent"},
	}
}

func TestCompileOmitsUnboundOptionalNodes(t *testing.T) {
	plan, err := Compile(validFeatureDefinition(), requiredBindings())
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	if plan.InitialStage != 10 {
		t.Fatalf("InitialStage = %d, want 10", plan.InitialStage)
	}
	if len(plan.OmittedNodeKeys) != 2 || plan.OmittedNodeKeys[0] != "backend" || plan.OmittedNodeKeys[1] != "frontend" {
		t.Fatalf("OmittedNodeKeys = %v, want [backend frontend]", plan.OmittedNodeKeys)
	}

	for _, node := range plan.IncludedNodes {
		wantStatus := "backlog"
		if node.Stage == 10 {
			wantStatus = "todo"
		}
		if node.InitialStatus != wantStatus {
			t.Errorf("node %s status = %q, want %q", node.Key, node.InitialStatus, wantStatus)
		}
	}
}

func TestCompileIncludesBoundParallelNodes(t *testing.T) {
	bindings := requiredBindings()
	bindings["backend"] = RoleBinding{Type: "agent", ID: "backend-agent"}
	bindings["frontend"] = RoleBinding{Type: "squad", ID: "frontend-squad"}

	plan, err := Compile(validFeatureDefinition(), bindings)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	backend := compiledNodeByKey(t, plan, "backend")
	frontend := compiledNodeByKey(t, plan, "frontend")
	if backend.Stage != 30 || frontend.Stage != 30 {
		t.Fatalf("parallel stages = backend:%d frontend:%d, want 30/30", backend.Stage, frontend.Stage)
	}
	if backend.Binding == nil || backend.Binding.Type != "agent" || backend.Binding.ID != "backend-agent" {
		t.Fatalf("backend binding = %#v", backend.Binding)
	}
	if frontend.Binding == nil || frontend.Binding.Type != "squad" || frontend.Binding.ID != "frontend-squad" {
		t.Fatalf("frontend binding = %#v", frontend.Binding)
	}
}

func TestCompileAlwaysIncludesApprovalWithoutBinding(t *testing.T) {
	plan, err := Compile(validFeatureDefinition(), requiredBindings())
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	approval := compiledNodeByKey(t, plan, "release_approval")
	if approval.Binding != nil {
		t.Fatalf("approval Binding = %#v, want nil", approval.Binding)
	}
	if approval.Type != NodeTypeApproval {
		t.Fatalf("approval Type = %q, want %q", approval.Type, NodeTypeApproval)
	}
}

func TestCompileNormalizesNodeOrderByStageThenKey(t *testing.T) {
	def := validFeatureDefinition()
	for i, j := 0, len(def.Nodes)-1; i < j; i, j = i+1, j-1 {
		def.Nodes[i], def.Nodes[j] = def.Nodes[j], def.Nodes[i]
	}

	plan, err := Compile(def, requiredBindings())
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	for i := 1; i < len(plan.IncludedNodes); i++ {
		prev := plan.IncludedNodes[i-1]
		cur := plan.IncludedNodes[i]
		if prev.Stage > cur.Stage || (prev.Stage == cur.Stage && prev.Key > cur.Key) {
			t.Fatalf("compiled order not normalized at %d: %#v then %#v", i, prev, cur)
		}
	}
}

func compiledNodeByKey(t *testing.T, plan CompiledPlan, key string) CompiledNode {
	t.Helper()
	for _, node := range plan.IncludedNodes {
		if node.Key == key {
			return node
		}
	}
	t.Fatalf("compiled node %q not found", key)
	return CompiledNode{}
}

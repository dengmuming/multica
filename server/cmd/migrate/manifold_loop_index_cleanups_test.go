package main

import "testing"

func TestManifoldLoopConcurrentIndexesHaveCleanupHooks(t *testing.T) {
	want := map[string]string{
		"469_loop_template_version_uq":    "idx_loop_template_workspace_key_version",
		"470_loop_template_active_uq":     "idx_loop_template_one_active",
		"471_loop_template_status_idx":    "idx_loop_template_workspace_status_key",
		"472_loop_evaluation_parent_idx":  "idx_loop_evaluation_parent_created",
		"473_loop_evaluation_node_idx":    "idx_loop_evaluation_node_created",
		"474_loop_evaluation_task_idx":    "idx_loop_evaluation_task",
		"475_loop_approval_pending_uq":    "idx_loop_approval_one_pending",
		"476_loop_approval_parent_idx":    "idx_loop_approval_parent_created",
		"477_loop_approval_requested_idx": "idx_loop_approval_requested_state",
		"478_loop_artifact_parent_idx":    "idx_loop_artifact_parent_created",
		"479_loop_artifact_node_idx":      "idx_loop_artifact_node_created",
		"480_loop_artifact_task_idx":      "idx_loop_artifact_task",
		"481_loop_instance_key_uq":        "idx_issue_manifold_loop_instance_key",
	}

	for version, index := range want {
		if got := concurrentIndexCleanups[version]; got != index {
			t.Errorf("concurrentIndexCleanups[%q] = %q, want %q", version, got, index)
		}
		if preMigrationHooks[version] == nil {
			t.Errorf("preMigrationHooks[%q] is not registered", version)
		}
	}
}

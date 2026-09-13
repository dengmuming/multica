package main

// MNL-005A registers every Manifold concurrent-index migration with the same
// invalid-index recovery path used by the rest of Multica. CREATE INDEX
// CONCURRENTLY can leave an INVALID relation when interrupted; without this
// hook, a retry could treat the leftover name as success and permanently keep
// an unusable index.
//
// This lives in a separate file to keep the Manifold extension isolated from
// upstream migration bookkeeping. preMigrationHooks is initialized from
// concurrentIndexCleanups in main.go before init functions run, so we update
// both maps here.
func init() {
	indexes := map[string]string{
		"469_loop_template_version_uq":     "idx_loop_template_workspace_key_version",
		"470_loop_template_active_uq":      "idx_loop_template_one_active",
		"471_loop_template_status_idx":     "idx_loop_template_workspace_status_key",
		"472_loop_evaluation_parent_idx":   "idx_loop_evaluation_parent_created",
		"473_loop_evaluation_node_idx":     "idx_loop_evaluation_node_created",
		"474_loop_evaluation_task_idx":     "idx_loop_evaluation_task",
		"475_loop_approval_pending_uq":     "idx_loop_approval_one_pending",
		"476_loop_approval_parent_idx":     "idx_loop_approval_parent_created",
		"477_loop_approval_requested_idx":  "idx_loop_approval_requested_state",
		"478_loop_artifact_parent_idx":     "idx_loop_artifact_parent_created",
		"479_loop_artifact_node_idx":       "idx_loop_artifact_node_created",
		"480_loop_artifact_task_idx":       "idx_loop_artifact_task",
	}

	for version, index := range indexes {
		concurrentIndexCleanups[version] = index
		preMigrationHooks[version] = cleanupInvalidConcurrentIndexHook(index)
	}
}

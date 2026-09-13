CREATE INDEX CONCURRENTLY idx_loop_artifact_node_created ON loop_artifact(node_issue_id, created_at) WHERE node_issue_id IS NOT NULL;

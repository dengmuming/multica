CREATE INDEX CONCURRENTLY idx_loop_evaluation_node_created ON loop_evaluation(node_issue_id, created_at DESC);

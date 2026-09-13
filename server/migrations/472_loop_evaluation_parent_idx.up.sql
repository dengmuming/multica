CREATE INDEX CONCURRENTLY idx_loop_evaluation_parent_created ON loop_evaluation(parent_issue_id, created_at DESC);

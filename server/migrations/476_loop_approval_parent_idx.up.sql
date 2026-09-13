CREATE INDEX CONCURRENTLY idx_loop_approval_parent_created ON loop_approval(parent_issue_id, created_at DESC);

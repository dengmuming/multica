CREATE UNIQUE INDEX CONCURRENTLY idx_loop_approval_one_pending ON loop_approval(node_issue_id, approval_key) WHERE state = 'pending';

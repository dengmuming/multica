CREATE INDEX CONCURRENTLY idx_loop_approval_requested_state ON loop_approval(requested_from_id, state) WHERE requested_from_id IS NOT NULL;

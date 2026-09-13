CREATE INDEX CONCURRENTLY idx_loop_artifact_parent_created ON loop_artifact(parent_issue_id, created_at);

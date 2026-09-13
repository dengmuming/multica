CREATE INDEX CONCURRENTLY idx_loop_artifact_task ON loop_artifact(task_id) WHERE task_id IS NOT NULL;

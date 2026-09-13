CREATE INDEX CONCURRENTLY idx_loop_evaluation_task ON loop_evaluation(task_id) WHERE task_id IS NOT NULL;

CREATE UNIQUE INDEX CONCURRENTLY idx_loop_evaluation_one_per_task
ON loop_evaluation (task_id)
WHERE task_id IS NOT NULL;

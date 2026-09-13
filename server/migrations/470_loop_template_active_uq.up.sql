CREATE UNIQUE INDEX CONCURRENTLY idx_loop_template_one_active ON loop_template(workspace_id, template_key) WHERE status = 'active';

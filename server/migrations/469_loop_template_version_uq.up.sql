CREATE UNIQUE INDEX CONCURRENTLY idx_loop_template_workspace_key_version ON loop_template(workspace_id, template_key, version);

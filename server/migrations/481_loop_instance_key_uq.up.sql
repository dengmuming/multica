CREATE UNIQUE INDEX CONCURRENTLY idx_issue_manifold_loop_instance_key
ON issue (workspace_id, project_id, (metadata ->> 'manifold.loop.instance_key'))
WHERE metadata ? 'manifold.loop.instance_key';

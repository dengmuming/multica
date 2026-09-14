-- Rollback is intended only for pre-production MNL development data. It
-- requires stored policy_version values to be numeric if non-NULL.

ALTER TABLE loop_approval
    ALTER COLUMN policy_version TYPE INT
    USING NULLIF(policy_version, '')::int;

ALTER TABLE loop_approval
    ADD CHECK (policy_version IS NULL OR policy_version > 0);

ALTER TABLE loop_evaluation
    ALTER COLUMN policy_version TYPE INT
    USING NULLIF(policy_version, '')::int;

ALTER TABLE loop_evaluation
    ADD CHECK (policy_version IS NULL OR policy_version > 0);

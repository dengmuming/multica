-- MNL policy_version is an opaque pinned policy identifier, not an arithmetic
-- counter. Application contracts intentionally allow values such as "policy-v1".
-- Keep persisted Evaluation/Approval evidence byte-for-byte aligned with the
-- parent Issue metadata rather than coercing it to a template integer.
--
-- Migration 468 created numeric CHECK constraints automatically named by
-- PostgreSQL. Drop them before changing the column type; otherwise the server
-- cannot re-plan `policy_version > 0` against TEXT and this migration fails.

ALTER TABLE loop_evaluation
    DROP CONSTRAINT IF EXISTS loop_evaluation_policy_version_check;

ALTER TABLE loop_approval
    DROP CONSTRAINT IF EXISTS loop_approval_policy_version_check;

ALTER TABLE loop_evaluation
    ALTER COLUMN policy_version TYPE TEXT
    USING policy_version::text;

ALTER TABLE loop_approval
    ALTER COLUMN policy_version TYPE TEXT
    USING policy_version::text;

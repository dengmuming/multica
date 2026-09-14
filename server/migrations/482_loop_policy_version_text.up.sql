-- MNL policy_version is an opaque pinned policy identifier, not an arithmetic
-- counter. Application contracts intentionally allow values such as "policy-v1".
-- Keep persisted Evaluation/Approval evidence byte-for-byte aligned with the
-- parent Issue metadata rather than coercing it to a template integer.

ALTER TABLE loop_evaluation
    ALTER COLUMN policy_version TYPE TEXT
    USING policy_version::text;

ALTER TABLE loop_approval
    ALTER COLUMN policy_version TYPE TEXT
    USING policy_version::text;

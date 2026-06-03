ALTER TABLE audit_log
    ADD COLUMN IF NOT EXISTS occurred_at timestamptz;

UPDATE audit_log
SET occurred_at = created_at
WHERE occurred_at IS NULL;

ALTER TABLE audit_log
    ALTER COLUMN occurred_at SET DEFAULT now();

ALTER TABLE audit_log
    ALTER COLUMN occurred_at SET NOT NULL;

CREATE INDEX IF NOT EXISTS audit_log_environment_occurred_at_idx
    ON audit_log(environment_id, occurred_at DESC);

CREATE INDEX IF NOT EXISTS audit_log_actor_user_occurred_at_idx
    ON audit_log(actor_user_id, occurred_at DESC);

CREATE TABLE audit_logs (
    id uuid PRIMARY KEY,
    environment_id uuid NULL REFERENCES environments(id) ON DELETE SET NULL,
    actor_user_id uuid NOT NULL REFERENCES users(id),
    action text NOT NULL,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    occurred_at timestamptz DEFAULT now()
);

CREATE INDEX audit_logs_environment_id_occurred_at_idx
    ON audit_logs(environment_id, occurred_at DESC);

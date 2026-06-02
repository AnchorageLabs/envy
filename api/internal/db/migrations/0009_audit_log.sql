CREATE TABLE IF NOT EXISTS audit_log (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_user_id uuid NOT NULL REFERENCES users(id),
    project_id uuid NULL REFERENCES projects(id) ON DELETE CASCADE,
    environment_id uuid NULL REFERENCES environments(id) ON DELETE CASCADE,
    action text NOT NULL,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS audit_log_project_id_idx ON audit_log(project_id);
CREATE INDEX IF NOT EXISTS audit_log_environment_id_idx ON audit_log(environment_id);
CREATE INDEX IF NOT EXISTS audit_log_actor_user_id_idx ON audit_log(actor_user_id);

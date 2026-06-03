CREATE TABLE IF NOT EXISTS proposals (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    environment_id uuid NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    message text NOT NULL,
    changes jsonb NOT NULL,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    created_by_user_id uuid NOT NULL REFERENCES users(id),
    resolved_by_user_id uuid NULL REFERENCES users(id),
    resolved_at timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (jsonb_typeof(changes) = 'array' AND jsonb_array_length(changes) > 0)
);

CREATE INDEX IF NOT EXISTS proposals_project_environment_status_idx ON proposals(project_id, environment_id, status);
CREATE INDEX IF NOT EXISTS proposals_environment_id_idx ON proposals(environment_id);
CREATE INDEX IF NOT EXISTS proposals_created_by_user_id_idx ON proposals(created_by_user_id);

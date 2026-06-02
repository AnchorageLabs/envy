CREATE TABLE IF NOT EXISTS variables (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    environment_id uuid NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    key text NOT NULL,
    type text NOT NULL CHECK (type IN ('string', 'enum', 'boolean', 'number')),
    required boolean NOT NULL DEFAULT false,
    secret boolean NOT NULL DEFAULT false,
    default_value text NULL,
    description text NOT NULL DEFAULT '',
    owner_user_id uuid NULL REFERENCES users(id),
    deprecated boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (environment_id, key)
);

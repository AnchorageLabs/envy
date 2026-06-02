CREATE TABLE variable_values (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    environment_id uuid NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    key text NOT NULL,
    value_plain text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (environment_id, key)
);

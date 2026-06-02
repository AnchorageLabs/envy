-- +goose Up
CREATE TABLE IF NOT EXISTS api_tokens (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash text NOT NULL UNIQUE,
    label text NOT NULL DEFAULT 'cli',
    last_used_at timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz NULL
);

CREATE INDEX IF NOT EXISTS api_tokens_user_id_idx ON api_tokens(user_id);

-- +goose Down
DROP TABLE IF EXISTS api_tokens;

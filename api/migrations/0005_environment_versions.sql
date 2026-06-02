CREATE TABLE environment_versions (
    id uuid PRIMARY KEY,
    environment_id uuid NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    number bigint NOT NULL,
    schema_snapshot jsonb NOT NULL,
    values_snapshot bytea NOT NULL,
    published_at timestamptz DEFAULT now(),
    published_by_user_id uuid NOT NULL REFERENCES users(id),
    CONSTRAINT environment_versions_environment_id_number_key UNIQUE (environment_id, number)
);

ALTER TABLE environments
    ADD COLUMN IF NOT EXISTS stable_version_id uuid;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'environments_stable_version_id_fkey'
          AND conrelid = 'environments'::regclass
    ) THEN
        ALTER TABLE environments
            ADD CONSTRAINT environments_stable_version_id_fkey
            FOREIGN KEY (stable_version_id)
            REFERENCES environment_versions(id)
            DEFERRABLE INITIALLY DEFERRED;
    END IF;
END $$;

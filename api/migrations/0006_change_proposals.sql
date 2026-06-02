CREATE TABLE change_proposals (
    id uuid PRIMARY KEY,
    environment_id uuid NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    proposed_by_user_id uuid NOT NULL REFERENCES users(id),
    status text NOT NULL DEFAULT 'pending',
    changes jsonb NOT NULL,
    message text NOT NULL DEFAULT '',
    created_at timestamptz DEFAULT now(),
    resolved_at timestamptz NULL,
    resolved_by_user_id uuid NULL REFERENCES users(id),
    CONSTRAINT change_proposals_status_check CHECK (status IN ('pending', 'approved', 'rejected'))
);

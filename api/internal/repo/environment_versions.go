package repo

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EnvironmentVersion is the JSON/API representation of a published environment version.
type EnvironmentVersion struct {
	ID                string          `json:"id"`
	EnvironmentID     string          `json:"environment_id"`
	Number            int64           `json:"number"`
	SchemaSnapshot    json.RawMessage `json:"schema_snapshot"`
	ValuesSnapshot    []byte          `json:"values_snapshot"`
	PublishedAt       string          `json:"published_at"`
	PublishedByUserID string          `json:"published_by_user_id"`
}

// EnvironmentVersionStore provides pgx-backed environment version queries.
type EnvironmentVersionStore struct {
	pool *pgxpool.Pool
}

func NewEnvironmentVersionStore(pool *pgxpool.Pool) *EnvironmentVersionStore {
	return &EnvironmentVersionStore{pool: pool}
}

// PublishDraftVersion snapshots the current environment draft into an immutable version.
func (s *EnvironmentVersionStore) PublishDraftVersion(ctx context.Context, projectID, environmentID, actorUserID string, setStable bool) (*EnvironmentVersion, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.Serializable,
		AccessMode: pgx.ReadWrite,
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var lockedEnvironmentID string
	if err := tx.QueryRow(ctx, `
		select id::text
		from environments
		where id = $1::uuid and project_id = $2::uuid
		for update
	`, environmentID, projectID).Scan(&lockedEnvironmentID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	var number int64
	if err := tx.QueryRow(ctx, `
		select coalesce(max(number), 0) + 1
		from environment_versions
		where environment_id = $1::uuid
	`, environmentID).Scan(&number); err != nil {
		return nil, err
	}

	var schemaSnapshot []byte
	if err := tx.QueryRow(ctx, `
		select coalesce(
			jsonb_agg(
				jsonb_build_object(
					'key', key,
					'type', type,
					'required', required,
					'secret', secret,
					'default', default_value,
					'description', description,
					'owner', owner_user_id::text,
					'deprecated', deprecated
				)
				order by key
			),
			'[]'::jsonb
		)
		from variables
		where environment_id = $1::uuid
	`, environmentID).Scan(&schemaSnapshot); err != nil {
		return nil, err
	}

	valuesSnapshot := []byte("{}")

	version := &EnvironmentVersion{}
	if err := tx.QueryRow(ctx, `
		insert into environment_versions (
			id,
			environment_id,
			number,
			schema_snapshot,
			values_snapshot,
			published_by_user_id
		) values (
			gen_random_uuid(),
			$1::uuid,
			$2,
			$3::jsonb,
			$4::bytea,
			$5::uuid
		)
		returning id::text, environment_id::text, number, schema_snapshot, values_snapshot, published_at::text, published_by_user_id::text
	`, environmentID, number, schemaSnapshot, valuesSnapshot, actorUserID).Scan(
		&version.ID,
		&version.EnvironmentID,
		&version.Number,
		&version.SchemaSnapshot,
		&version.ValuesSnapshot,
		&version.PublishedAt,
		&version.PublishedByUserID,
	); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, err
	}

	if setStable {
		if _, err := tx.Exec(ctx, `
			update environments
			set stable_version_id = $1::uuid
			where id = $2::uuid
		`, version.ID, environmentID); err != nil {
			return nil, err
		}
	}

	payload, err := json.Marshal(map[string]any{
		"number":     version.Number,
		"set_stable": setStable,
	})
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		insert into audit_log (actor_user_id, project_id, environment_id, action, payload)
		values ($1::uuid, $2::uuid, $3::uuid, 'version.published', $4::jsonb)
	`, actorUserID, projectID, environmentID, payload); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return version, nil
}

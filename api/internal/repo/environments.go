package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Environment is the JSON/API representation of an environment row.
type Environment struct {
	ID                  string  `json:"id"`
	ProjectID           string  `json:"project_id"`
	Name                string  `json:"name"`
	StableVersionID     *string `json:"stable_version_id"`
	StableVersionNumber *int64  `json:"stable_version_number"`
}

// EnvironmentStore provides pgx-backed environment queries.
type EnvironmentStore struct {
	pool *pgxpool.Pool
}

func NewEnvironmentStore(pool *pgxpool.Pool) *EnvironmentStore {
	return &EnvironmentStore{pool: pool}
}

func (s *EnvironmentStore) CreateEnvironment(ctx context.Context, projectID, name string) (*Environment, error) {
	environment, err := scanEnvironment(s.pool.QueryRow(ctx, `
		insert into environments (id, project_id, name)
		values (gen_random_uuid(), $1::uuid, $2)
		returning id::text, project_id::text, name, stable_version_id::text, null::bigint
	`, projectID, name))
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, err
	}
	return environment, nil
}

func (s *EnvironmentStore) ListEnvironmentsByProject(ctx context.Context, projectID string) ([]Environment, error) {
	rows, err := s.pool.Query(ctx, `
		select e.id::text, e.project_id::text, e.name, e.stable_version_id::text, ev.number
		from environments e
		left join environment_versions ev on ev.id = e.stable_version_id
		where e.project_id = $1::uuid
		order by e.name
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	environments := make([]Environment, 0)
	for rows.Next() {
		environment, err := scanEnvironment(rows)
		if err != nil {
			return nil, err
		}
		environments = append(environments, *environment)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return environments, nil
}

func (s *EnvironmentStore) GetEnvironmentByProjectAndName(ctx context.Context, projectID, name string) (*Environment, error) {
	environment, err := scanEnvironment(s.pool.QueryRow(ctx, `
		select e.id::text, e.project_id::text, e.name, e.stable_version_id::text, ev.number
		from environments e
		left join environment_versions ev on ev.id = e.stable_version_id
		where e.project_id = $1::uuid and e.name = $2
	`, projectID, name))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return environment, nil
}

func (s *EnvironmentStore) DeleteEnvironmentByProjectAndName(ctx context.Context, projectID, name string) error {
	tag, err := s.pool.Exec(ctx, `
		delete from environments
		where project_id = $1::uuid and name = $2 and stable_version_id is null
	`, projectID, name)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		return nil
	}

	var hasStableVersion bool
	err = s.pool.QueryRow(ctx, `
		select stable_version_id is not null
		from environments
		where project_id = $1::uuid and name = $2
	`, projectID, name).Scan(&hasStableVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if hasStableVersion {
		return ErrConflict
	}
	return ErrNotFound
}

type environmentScanner interface {
	Scan(dest ...any) error
}

func scanEnvironment(row environmentScanner) (*Environment, error) {
	environment := &Environment{}
	var stableVersionID sql.NullString
	var stableVersionNumber sql.NullInt64

	if err := row.Scan(
		&environment.ID,
		&environment.ProjectID,
		&environment.Name,
		&stableVersionID,
		&stableVersionNumber,
	); err != nil {
		return nil, err
	}

	if stableVersionID.Valid {
		environment.StableVersionID = &stableVersionID.String
	}
	if stableVersionNumber.Valid {
		environment.StableVersionNumber = &stableVersionNumber.Int64
	}

	return environment, nil
}

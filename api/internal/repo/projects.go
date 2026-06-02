package repo

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Project is the JSON/API representation of a project row.
type Project struct {
	ID      string `json:"id"`
	OwnerID string `json:"owner_id"`
	Slug    string `json:"slug"`
	Name    string `json:"name"`
}

// ProjectStore provides pgx-backed project queries.
type ProjectStore struct {
	pool *pgxpool.Pool
}

func NewProjectStore(pool *pgxpool.Pool) *ProjectStore {
	return &ProjectStore{pool: pool}
}

func (s *ProjectStore) CreateProject(ctx context.Context, ownerID, slug, name string) (*Project, error) {
	project := &Project{}
	err := s.pool.QueryRow(ctx, `
		insert into projects (id, owner_id, slug, name)
		values (gen_random_uuid(), $1::uuid, $2, $3)
		returning id::text, owner_id::text, slug, name
	`, ownerID, slug, name).Scan(&project.ID, &project.OwnerID, &project.Slug, &project.Name)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, err
	}
	return project, nil
}

func (s *ProjectStore) ListProjectsByOwner(ctx context.Context, ownerID string) ([]Project, error) {
	rows, err := s.pool.Query(ctx, `
		select id::text, owner_id::text, slug, name
		from projects
		where owner_id = $1::uuid
		order by slug
	`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	projects := make([]Project, 0)
	for rows.Next() {
		var project Project
		if err := rows.Scan(&project.ID, &project.OwnerID, &project.Slug, &project.Name); err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return projects, nil
}

func (s *ProjectStore) GetProjectBySlugForOwner(ctx context.Context, ownerID, slug string) (*Project, error) {
	project := &Project{}
	err := s.pool.QueryRow(ctx, `
		select id::text, owner_id::text, slug, name
		from projects
		where owner_id = $1::uuid and slug = $2
	`, ownerID, slug).Scan(&project.ID, &project.OwnerID, &project.Slug, &project.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return project, nil
}

func (s *ProjectStore) UpdateProjectNameForOwner(ctx context.Context, ownerID, slug, name string) (*Project, error) {
	project := &Project{}
	err := s.pool.QueryRow(ctx, `
		update projects
		set name = $3
		where owner_id = $1::uuid and slug = $2
		returning id::text, owner_id::text, slug, name
	`, ownerID, slug, name).Scan(&project.ID, &project.OwnerID, &project.Slug, &project.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return project, nil
}

func (s *ProjectStore) DeleteProjectForOwner(ctx context.Context, ownerID, slug string) error {
	tag, err := s.pool.Exec(ctx, `
		delete from projects
		where owner_id = $1::uuid and slug = $2
	`, ownerID, slug)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

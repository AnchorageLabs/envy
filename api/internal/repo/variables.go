package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// VariableDefinition is the JSON/API representation of a draft schema variable.
// It intentionally excludes any environment value fields.
type VariableDefinition struct {
	Key         string    `json:"key"`
	Type        string    `json:"type"`
	Required    bool      `json:"required"`
	Secret      bool      `json:"secret"`
	Default     *string   `json:"default"`
	Description string    `json:"description"`
	Owner       *string   `json:"owner"`
	Deprecated  bool      `json:"deprecated"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// VariableStore provides pgx-backed draft variable schema queries.
type VariableStore struct {
	pool *pgxpool.Pool
}

func NewVariableStore(pool *pgxpool.Pool) *VariableStore {
	return &VariableStore{pool: pool}
}

func IsAllowedVariableType(variableType string) bool {
	switch variableType {
	case "string", "enum", "boolean", "number":
		return true
	default:
		return false
	}
}

func (s *VariableStore) ListDraftSchema(ctx context.Context, environmentID string) ([]VariableDefinition, error) {
	return listDraftSchema(ctx, s.pool, environmentID, false)
}

func (s *VariableStore) ReplaceDraftSchema(ctx context.Context, actorUserID, projectID, environmentID string, schema []VariableDefinition) ([]VariableDefinition, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	existing, err := listDraftSchema(ctx, tx, environmentID, true)
	if err != nil {
		return nil, err
	}

	diff := buildSchemaDiff(existing, schema)

	submittedByKey := make(map[string]VariableDefinition, len(schema))
	for _, variable := range schema {
		submittedByKey[variable.Key] = variable
		_, err := tx.Exec(ctx, `
			insert into variables (
				environment_id,
				key,
				type,
				required,
				secret,
				default_value,
				description,
				owner_user_id,
				deprecated,
				updated_at
			) values (
				$1::uuid,
				$2,
				$3,
				$4,
				$5,
				$6,
				$7,
				$8::uuid,
				$9,
				now()
			)
			on conflict (environment_id, key) do update set
				type = excluded.type,
				required = excluded.required,
				secret = excluded.secret,
				default_value = excluded.default_value,
				description = excluded.description,
				owner_user_id = excluded.owner_user_id,
				deprecated = excluded.deprecated,
				updated_at = now()
		`, environmentID, variable.Key, variable.Type, variable.Required, variable.Secret, nullableString(variable.Default), variable.Description, nullableString(variable.Owner), variable.Deprecated)
		if err != nil {
			return nil, err
		}
	}

	for _, variable := range existing {
		if _, keep := submittedByKey[variable.Key]; keep {
			continue
		}
		if _, err := tx.Exec(ctx, `
			delete from variables
			where environment_id = $1::uuid and key = $2
		`, environmentID, variable.Key); err != nil {
			return nil, err
		}
	}

	diffJSON, err := json.Marshal(diff)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		insert into audit_log (id, actor_user_id, project_id, environment_id, action, payload)
		values (gen_random_uuid(), $1::uuid, $2::uuid, $3::uuid, 'schema.replaced', $4::jsonb)
	`, actorUserID, projectID, environmentID, diffJSON); err != nil {
		return nil, err
	}

	persisted, err := listDraftSchema(ctx, tx, environmentID, false)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return persisted, nil
}

type variableQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func listDraftSchema(ctx context.Context, q variableQuerier, environmentID string, forUpdate bool) ([]VariableDefinition, error) {
	query := `
		select key, type, required, secret, default_value, description, owner_user_id::text, deprecated, updated_at
		from variables
		where environment_id = $1::uuid
		order by key
	`
	if forUpdate {
		query += ` for update`
	}

	rows, err := q.Query(ctx, query, environmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	variables := make([]VariableDefinition, 0)
	for rows.Next() {
		variable, err := scanVariableDefinition(rows)
		if err != nil {
			return nil, err
		}
		variables = append(variables, *variable)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return variables, nil
}

type variableScanner interface {
	Scan(dest ...any) error
}

func scanVariableDefinition(row variableScanner) (*VariableDefinition, error) {
	variable := &VariableDefinition{}
	var defaultValue sql.NullString
	var owner sql.NullString

	if err := row.Scan(
		&variable.Key,
		&variable.Type,
		&variable.Required,
		&variable.Secret,
		&defaultValue,
		&variable.Description,
		&owner,
		&variable.Deprecated,
		&variable.UpdatedAt,
	); err != nil {
		return nil, err
	}

	if defaultValue.Valid {
		variable.Default = &defaultValue.String
	}
	if owner.Valid {
		variable.Owner = &owner.String
	}

	return variable, nil
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

type schemaDiff struct {
	Added   []schemaAuditVariable       `json:"added"`
	Updated []schemaAuditVariableUpdate `json:"updated"`
	Removed []schemaAuditVariable       `json:"removed"`
}

type schemaAuditVariableUpdate struct {
	Key    string              `json:"key"`
	Before schemaAuditVariable `json:"before"`
	After  schemaAuditVariable `json:"after"`
}

type schemaAuditVariable struct {
	Key         string  `json:"key"`
	Type        string  `json:"type"`
	Required    bool    `json:"required"`
	Secret      bool    `json:"secret"`
	Default     *string `json:"default"`
	Description string  `json:"description"`
	Owner       *string `json:"owner"`
	Deprecated  bool    `json:"deprecated"`
}

func buildSchemaDiff(existing, submitted []VariableDefinition) schemaDiff {
	diff := schemaDiff{
		Added:   make([]schemaAuditVariable, 0),
		Updated: make([]schemaAuditVariableUpdate, 0),
		Removed: make([]schemaAuditVariable, 0),
	}

	existingByKey := make(map[string]VariableDefinition, len(existing))
	submittedByKey := make(map[string]VariableDefinition, len(submitted))

	for _, variable := range existing {
		existingByKey[variable.Key] = variable
	}
	for _, variable := range submitted {
		submittedByKey[variable.Key] = variable
		before, found := existingByKey[variable.Key]
		if !found {
			diff.Added = append(diff.Added, auditVariable(variable))
			continue
		}
		if !sameVariableDefinition(before, variable) {
			diff.Updated = append(diff.Updated, schemaAuditVariableUpdate{
				Key:    variable.Key,
				Before: auditVariable(before),
				After:  auditVariable(variable),
			})
		}
	}

	for _, variable := range existing {
		if _, keep := submittedByKey[variable.Key]; !keep {
			diff.Removed = append(diff.Removed, auditVariable(variable))
		}
	}

	return diff
}

func sameVariableDefinition(a, b VariableDefinition) bool {
	return a.Key == b.Key &&
		a.Type == b.Type &&
		a.Required == b.Required &&
		a.Secret == b.Secret &&
		sameStringPointer(a.Default, b.Default) &&
		a.Description == b.Description &&
		sameStringPointer(a.Owner, b.Owner) &&
		a.Deprecated == b.Deprecated
}

func sameStringPointer(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func auditVariable(variable VariableDefinition) schemaAuditVariable {
	return schemaAuditVariable{
		Key:         variable.Key,
		Type:        variable.Type,
		Required:    variable.Required,
		Secret:      variable.Secret,
		Default:     variable.Default,
		Description: variable.Description,
		Owner:       variable.Owner,
		Deprecated:  variable.Deprecated,
	}
}

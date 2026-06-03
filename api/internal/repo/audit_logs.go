package repo

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultAuditLogLimit = 50
	maxAuditLogLimit     = 200
)

// AuditLogActor is the actor representation embedded in audit log API responses.
type AuditLogActor struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// AuditLogEntry is the JSON/API representation of an audit log row.
type AuditLogEntry struct {
	ID         string          `json:"id"`
	Actor      AuditLogActor   `json:"actor"`
	Action     string          `json:"action"`
	Payload    json.RawMessage `json:"payload"`
	OccurredAt string          `json:"occurred_at"`
}

// AuditLogPage contains normalized audit log pagination options.
type AuditLogPage struct {
	Limit  int
	Before *time.Time
}

// AuditLogStore provides pgx-backed audit log queries.
type AuditLogStore struct {
	pool *pgxpool.Pool
}

func NewAuditLogStore(pool *pgxpool.Pool) *AuditLogStore {
	return &AuditLogStore{pool: pool}
}

// ClampAuditLogLimit applies audit log endpoint limit semantics.
func ClampAuditLogLimit(limit int) int {
	if limit < 1 {
		return 1
	}
	if limit > maxAuditLogLimit {
		return maxAuditLogLimit
	}
	return limit
}

// ParseAuditLogPagination parses shared audit log pagination query values.
func ParseAuditLogPagination(limitValue, beforeValue string) (AuditLogPage, error) {
	page := AuditLogPage{Limit: defaultAuditLogLimit}

	if limitValue != "" {
		limit, err := strconv.Atoi(limitValue)
		if err != nil {
			return page, err
		}
		page.Limit = ClampAuditLogLimit(limit)
	}

	if beforeValue != "" {
		before, err := time.Parse(time.RFC3339Nano, beforeValue)
		if err != nil {
			return page, err
		}
		page.Before = &before
	}

	return page, nil
}

// ListEnvironmentAuditLogsForOwner returns audit logs for one environment in a project owned by ownerID.
func (s *AuditLogStore) ListEnvironmentAuditLogsForOwner(ctx context.Context, ownerID, projectSlug, environmentName string, before *time.Time, limit int) ([]AuditLogEntry, error) {
	limit = ClampAuditLogLimit(limit)

	var environmentID string
	if err := s.pool.QueryRow(ctx, `
		select e.id::text
		from projects p
		join environments e on e.project_id = p.id
		where p.owner_id = $1::uuid
		  and p.slug = $2
		  and e.name = $3
	`, ownerID, projectSlug, environmentName).Scan(&environmentID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		select al.id::text,
		       u.id::text,
		       u.email::text,
		       u.name,
		       al.action,
		       al.payload,
		       al.occurred_at::text
		from audit_log al
		join users u on u.id = al.actor_user_id
		where al.environment_id = $1::uuid
		  and ($2::timestamptz is null or al.occurred_at < $2::timestamptz)
		order by al.occurred_at desc, al.id desc
		limit $3
	`, environmentID, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanAuditLogEntries(rows)
}

// ListVisibleAuditLogsForActor returns audit logs created by actorUserID in projects owned by actorUserID.
func (s *AuditLogStore) ListVisibleAuditLogsForActor(ctx context.Context, actorUserID string, before *time.Time, limit int) ([]AuditLogEntry, error) {
	limit = ClampAuditLogLimit(limit)

	rows, err := s.pool.Query(ctx, `
		select al.id::text,
		       u.id::text,
		       u.email::text,
		       u.name,
		       al.action,
		       al.payload,
		       al.occurred_at::text
		from audit_log al
		join users u on u.id = al.actor_user_id
		join environments e on e.id = al.environment_id
		join projects p on p.id = e.project_id
		where al.actor_user_id = $1::uuid
		  and p.owner_id = $1::uuid
		  and ($2::timestamptz is null or al.occurred_at < $2::timestamptz)
		order by al.occurred_at desc, al.id desc
		limit $3
	`, actorUserID, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanAuditLogEntries(rows)
}

type auditLogRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanAuditLogEntries(rows auditLogRows) ([]AuditLogEntry, error) {
	entries := make([]AuditLogEntry, 0)
	for rows.Next() {
		var entry AuditLogEntry
		var payload []byte
		if err := rows.Scan(
			&entry.ID,
			&entry.Actor.ID,
			&entry.Actor.Email,
			&entry.Actor.Name,
			&entry.Action,
			&payload,
			&entry.OccurredAt,
		); err != nil {
			return nil, err
		}
		entry.Payload = append(json.RawMessage(nil), payload...)
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

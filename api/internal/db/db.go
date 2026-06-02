package db

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Open creates a PostgreSQL connection pool and verifies connectivity.
func Open(ctx context.Context, dbURL string) (*pgxpool.Pool, error) {
	dbURL = strings.TrimSpace(dbURL)
	if dbURL == "" {
		return nil, errors.New("ENVY_DB_URL is required")
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}

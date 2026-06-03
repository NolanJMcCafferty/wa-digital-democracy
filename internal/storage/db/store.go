// Package db wraps pgx with project-specific helpers.
package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is the application-level handle on the database.
type Store struct {
	Pool *pgxpool.Pool
}

// Open establishes a pool and verifies the database is reachable. Caller is
// responsible for Close.
func Open(ctx context.Context, dsn string) (*Store, error) {
	store, err := OpenLazy(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := store.Pool.Ping(ctx); err != nil {
		store.Close()
		return nil, fmt.Errorf("pgxpool ping: %w", err)
	}
	return store, nil
}

// OpenLazy establishes a pool without forcing an immediate connection. This is
// useful for HTTP processes that need to bind a liveness endpoint before
// dependent services have finished booting.
func OpenLazy(ctx context.Context, dsn string) (*Store, error) {
	if dsn == "" {
		return nil, errors.New("db: dsn required")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool new: %w", err)
	}
	return &Store{Pool: pool}, nil
}

func (s *Store) Close() {
	if s.Pool != nil {
		s.Pool.Close()
	}
}

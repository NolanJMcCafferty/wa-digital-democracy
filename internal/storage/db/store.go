// Package db wraps pgx with project-specific helpers.
//
// For Phase 1 we expose just enough to bootstrap connectors: a pool, an
// InsertSourceRecord helper, and a RawSink adapter that bridges
// httpx.RawFetch -> object storage + source_record row.
//
// Once schemas stabilize we will introduce sqlc-generated query bindings
// under this same package.
package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/objectstore"
)

// Store is the application-level handle on the database.
type Store struct {
	Pool *pgxpool.Pool
}

// Open establishes a pool. Caller is responsible for Close.
func Open(ctx context.Context, dsn string) (*Store, error) {
	if dsn == "" {
		return nil, errors.New("db: dsn required")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool new: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pgxpool ping: %w", err)
	}
	return &Store{Pool: pool}, nil
}

func (s *Store) Close() {
	if s.Pool != nil {
		s.Pool.Close()
	}
}

// SourceRecordParams is the input to InsertSourceRecord.
type SourceRecordParams struct {
	System           string
	Endpoint         string
	URL              string
	SourceID         string
	FetchedAt        any // time.Time
	ContentHash      string
	RawPath          string
	ContentType      string
	TransformVersion string
}

// InsertSourceRecord writes a row and returns its id. Idempotent for the same
// logical request and response bytes; distinct endpoints/URLs are preserved as
// distinct provenance rows even when they return identical content.
func (s *Store) InsertSourceRecord(ctx context.Context, p SourceRecordParams) (int64, error) {
	const q = `
INSERT INTO source_record
  (source_system, source_endpoint, source_url, source_id, fetched_at,
   content_hash, raw_path, content_type, transform_version)
VALUES ($1,$2,$3,NULLIF($4,''),$5,$6,$7,NULLIF($8,''),COALESCE(NULLIF($9,''),'v0'))
ON CONFLICT (source_system, source_endpoint, source_url, content_hash, transform_version)
  DO UPDATE SET fetched_at = EXCLUDED.fetched_at
RETURNING id;
`
	var id int64
	err := s.Pool.QueryRow(ctx, q,
		p.System, p.Endpoint, p.URL, p.SourceID, p.FetchedAt,
		p.ContentHash, p.RawPath, p.ContentType, p.TransformVersion,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert source_record: %w", err)
	}
	return id, nil
}

// RawSink implements httpx.RawSink by writing the body to objectstore and
// inserting a source_record row.
type RawSink struct {
	Store            *Store
	Objects          objectstore.Store
	TransformVersion string
}

func (s RawSink) Record(ctx context.Context, f *httpx.RawFetch) error {
	rel, err := s.Objects.Put(ctx, f.System, f.Hash, f.ContentType, f.Body)
	if err != nil {
		return fmt.Errorf("objectstore put: %w", err)
	}
	id, err := s.Store.InsertSourceRecord(ctx, SourceRecordParams{
		System:           f.System,
		Endpoint:         f.Endpoint,
		URL:              f.URL,
		FetchedAt:        f.FetchedAt,
		ContentHash:      f.Hash,
		RawPath:          rel,
		ContentType:      f.ContentType,
		TransformVersion: s.TransformVersion,
	})
	if err != nil {
		return err
	}
	f.SourceRecordID = id
	return nil
}

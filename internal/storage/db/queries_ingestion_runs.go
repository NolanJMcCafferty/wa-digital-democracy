package db

import (
	"context"
	"fmt"
)

func (s *Store) StartIngestionRun(ctx context.Context, job string, args map[string]any) (int64, error) {
	const q = `INSERT INTO ingestion_run (job, args) VALUES ($1, $2) RETURNING id;`
	var id int64
	err := s.Pool.QueryRow(ctx, q, job, args).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("start ingestion_run: %w", err)
	}
	return id, nil
}

func (s *Store) FinishIngestionRun(ctx context.Context, id int64, status string, fetched, upserted int, runErr error) error {
	const q = `UPDATE ingestion_run SET finished_at = NOW(), status = $2,
                                rows_fetched = $3, rows_upserted = $4, error = $5
              WHERE id = $1;`
	var errStr *string
	if runErr != nil {
		s := runErr.Error()
		errStr = &s
	}
	_, err := s.Pool.Exec(ctx, q, id, status, fetched, upserted, errStr)
	return err
}

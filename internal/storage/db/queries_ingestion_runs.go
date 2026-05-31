package db

import (
	"context"
	"fmt"
	"time"
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

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// SourceSummaryRow is the row shape ListSourceSummaries returns.
type SourceSummaryRow struct {
	System          string
	Calls           int
	LatestFetchedAt time.Time
	Endpoints       []string
}

// ListSourceSummaries groups source_record rows by source_system and
// returns counts + most-recent fetch + distinct endpoints.
func (s *Store) ListSourceSummaries(ctx context.Context) ([]SourceSummaryRow, error) {
	const q = `
SELECT source_system, COUNT(*), MAX(fetched_at), ARRAY_AGG(DISTINCT source_endpoint)
  FROM source_record
 GROUP BY source_system
 ORDER BY source_system;`
	rows, err := s.Pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list source summaries: %w", err)
	}
	defer rows.Close()
	out := []SourceSummaryRow{}
	for rows.Next() {
		var r SourceSummaryRow
		if err := rows.Scan(&r.System, &r.Calls, &r.LatestFetchedAt, &r.Endpoints); err != nil {
			return nil, fmt.Errorf("scan source summary: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

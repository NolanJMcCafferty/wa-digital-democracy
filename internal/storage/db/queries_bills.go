package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type UpsertBillParams struct {
	Biennium       string
	Prefix         string
	Number         int
	Title          string
	Description    string
	ChamberOrigin  string
	CurrentStatus  string
	StatusDate     time.Time
	OfficialURL    string
	SourceRecordID int64
}

// UpsertBill inserts/updates a bill keyed on (biennium, prefix, number).
func (s *Store) UpsertBill(ctx context.Context, p UpsertBillParams) (int64, error) {
	const q = `
INSERT INTO bill (biennium, prefix, number, title, description, chamber_origin,
                  current_status, status_date, official_url, source_record_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT (biennium, prefix, number) DO UPDATE SET
  title          = EXCLUDED.title,
  description    = EXCLUDED.description,
  chamber_origin = EXCLUDED.chamber_origin,
  current_status = EXCLUDED.current_status,
  status_date    = EXCLUDED.status_date,
  official_url   = EXCLUDED.official_url,
  source_record_id = EXCLUDED.source_record_id,
  updated_at     = NOW()
RETURNING id;`
	var id int64
	err := s.Pool.QueryRow(ctx, q,
		p.Biennium, p.Prefix, p.Number,
		strOrNull(p.Title), strOrNull(p.Description), strOrNull(p.ChamberOrigin),
		strOrNull(p.CurrentStatus), dateOrNull(p.StatusDate), strOrNull(p.OfficialURL),
		p.SourceRecordID,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert bill: %w", err)
	}
	return id, nil
}

// ---------------------------------------------------------------------------
// legislator + bill_sponsor
// ---------------------------------------------------------------------------

func (s *Store) UpsertBillSponsor(ctx context.Context, billID, legislatorID int64, sponsorType string) error {
	const q = `
INSERT INTO bill_sponsor (bill_id, legislator_id, sponsor_type)
VALUES ($1, $2, $3)
ON CONFLICT (bill_id, legislator_id, sponsor_type) DO NOTHING;`
	_, err := s.Pool.Exec(ctx, q, billID, legislatorID, sponsorType)
	return err
}

// ---------------------------------------------------------------------------
// hearing + agenda_item + testifier
// ---------------------------------------------------------------------------

type InsertStatusChangeParams struct {
	BillID         int64
	ActionDate     time.Time
	HistoryLine    string
	Actor          string
	SourceRecordID int64
}

// ReplaceStatusChangesForBill clears and reloads the status timeline. LWS
// returns the canonical sequence each time we ask, so a snapshot replace
// keeps the table consistent without diffing.
func (s *Store) ReplaceStatusChangesForBill(ctx context.Context, billID int64, rows []InsertStatusChangeParams) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM bill_status_change WHERE bill_id = $1`, billID); err != nil {
		return fmt.Errorf("delete status: %w", err)
	}
	const insQ = `
INSERT INTO bill_status_change (bill_id, action_date, history_line, actor, source_record_id)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT DO NOTHING;`
	for _, r := range rows {
		if _, err := tx.Exec(ctx, insQ,
			r.BillID, dateOrNull(r.ActionDate), r.HistoryLine, strOrNull(r.Actor), r.SourceRecordID,
		); err != nil {
			return fmt.Errorf("insert status: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// ---------------------------------------------------------------------------
// ingestion_run — bookkeeping
// ---------------------------------------------------------------------------

// ListedBill is the row shape the bills-index API endpoint surfaces.
// Lead-sponsor and status fields can be empty when the bill_sponsor /
// bill_status_change joins miss (rare for ingested bills, but the
// `Primary` sponsor isn't always populated).
type ListedBill struct {
	Biennium      string
	Prefix        string
	Number        int
	BillID        string // generated column, e.g. "HB 1501"
	Title         string
	ChamberOrigin string // "House" | "Senate"
	CurrentStatus string
	StatusDate    time.Time
	LeadSponsor   string // legislator.name e.g. "Senator Reed" — empty when no Primary sponsor
	LeadFirstName string
	LeadLastName  string
	LeadParty     string // "D" | "R"
	LeadSlug      string // computed in API handler; left empty here
}

// BillSearchParams are the filter knobs SearchBills accepts. Empty
// strings are no-ops. The handler is responsible for clamping limit
// and offset to safe ranges.
type BillSearchParams struct {
	Query         string   // matches title or bill_number (ILIKE)
	Prefix        string   // exact match on bill.prefix (HB, SB, HJR, …)
	Chamber       string   // "House" | "Senate"
	Party         string   // "D" | "R" — filters on lead sponsor's party
	Status        string   // "in_progress" | "passed" | "failed" | "" — bucketed from current_status
	Sponsor       string   // legislator slug; matches any sponsor row
	LeadSponsor   string   // legislator slug; matches the Primary sponsor only
	BillIDs       []string // restrict to this set of bill_ids (used by issue pages); empty = no filter
	TopicKeywords []string // OR-set of ILIKE keywords matched against bill title/desc, agenda label, committee name
	Limit         int
	Offset        int
}

// BillSearchFacets carries the distinct values we render in the
// sidebar so the UI doesn't hard-code lists. Counts here are over the
// full unfiltered set; the API handler attaches them once per query.
type BillSearchFacets struct {
	Prefixes []string // ordered: HB, SB, HJR, SJR, HCR, SCR, HJM, SJM, then alphabetical
	Chambers []string // House, Senate
	Parties  []string // D, R, …
	Statuses []string // in_progress, passed, failed
}

// ListIngestedBills returns every bill row in stable display order
// (newest biennium first, then prefix, then number). Filters out the
// occasional placeholder row with number=0 from broken upserts.
//
// Returns the same shape as SearchBills with no filters but no total
// count — kept for back-compat with callers that don't paginate.
func (s *Store) ListIngestedBills(ctx context.Context) ([]ListedBill, error) {
	hits, _, err := s.SearchBills(ctx, BillSearchParams{Limit: 100000, Offset: 0})
	return hits, err
}

// SearchBills is the paginated, filtered query backing /api/v1/bills.
// Returns (hits, total, err). total is the count of matches across all
// pages (not just the page returned), so the frontend can render
// numbered pagination without a second query.
func (s *Store) SearchBills(ctx context.Context, p BillSearchParams) ([]ListedBill, int, error) {
	if p.Limit <= 0 {
		p.Limit = 50
	}
	if p.Offset < 0 {
		p.Offset = 0
	}

	// Build the WHERE fragment dynamically. Push a value once, take its
	// $N index, use it any number of times in the matching fragment.
	args := []any{}
	where := []string{"b.number > 0"}
	push := func(v any) int {
		args = append(args, v)
		return len(args)
	}
	if q := strings.TrimSpace(p.Query); q != "" {
		idx := push(q)
		where = append(where, fmt.Sprintf(
			"(b.title ILIKE '%%' || $%d || '%%' OR b.bill_number ILIKE '%%' || $%d || '%%')", idx, idx))
	}
	if p.Prefix != "" {
		idx := push(p.Prefix)
		where = append(where, fmt.Sprintf("b.prefix = $%d", idx))
	}
	if p.Chamber != "" {
		idx := push(p.Chamber)
		where = append(where, fmt.Sprintf("b.chamber_origin = $%d", idx))
	}
	if p.Party != "" {
		idx := push(p.Party)
		where = append(where, fmt.Sprintf("primary_sponsor.party = $%d", idx))
	}
	if p.Sponsor != "" {
		idx := push(p.Sponsor)
		where = append(where, fmt.Sprintf(`
EXISTS (
  SELECT 1
    FROM bill_sponsor sponsor_filter_bs
    JOIN legislator sponsor_filter_l ON sponsor_filter_l.id = sponsor_filter_bs.legislator_id
   WHERE sponsor_filter_bs.bill_id = b.id
     AND trim(both '-' from regexp_replace(replace(lower(sponsor_filter_l.name), '&', ' and '), '[^a-z0-9]+', '-', 'g')) = $%d
)`, idx))
	}
	if p.LeadSponsor != "" {
		idx := push(p.LeadSponsor)
		where = append(where, fmt.Sprintf("trim(both '-' from regexp_replace(replace(lower(primary_sponsor.name), '&', ' and '), '[^a-z0-9]+', '-', 'g')) = $%d", idx))
	}
	if len(p.BillIDs) > 0 {
		idx := push(p.BillIDs)
		where = append(where, fmt.Sprintf("b.bill_number = ANY($%d)", idx))
	}
	if keywords := nonEmptyStrings(p.TopicKeywords); len(keywords) > 0 {
		ors := make([]string, 0, len(keywords))
		for _, kw := range keywords {
			idx := push(kw)
			ors = append(ors, fmt.Sprintf(`(
	b.bill_number ILIKE '%%' || $%d || '%%'
	OR b.title ILIKE '%%' || $%d || '%%'
	OR COALESCE(b.description, '') ILIKE '%%' || $%d || '%%'
	OR EXISTS (
	  SELECT 1
	    FROM agenda_item a
	    JOIN hearing h ON h.id = a.hearing_id
	   WHERE a.bill_id = b.id
	     AND (a.label ILIKE '%%' || $%d || '%%' OR h.committee_name ILIKE '%%' || $%d || '%%')
	)
)`, idx, idx, idx, idx, idx))
		}
		where = append(where, "("+strings.Join(ors, " OR ")+")")
	}
	if p.Status != "" {
		// Status accepts either a coarse bucket ("passed", "failed",
		// "in_progress") or one of the fine-grained stage labels mirrored
		// in apps/web/src/lib/billStatus.ts. Stages map to the same SQL
		// CASE expression used elsewhere; keep the two in sync.
		switch strings.ToLower(p.Status) {
		case "passed":
			where = append(where, "(b.current_status ILIKE '%effective date%' OR b.current_status ILIKE '%governor signed%' OR b.current_status ILIKE '%chapter %2026 laws%')")
		case "failed":
			where = append(where, "(b.current_status ILIKE '%died%' OR b.current_status ILIKE '%vetoed%' OR b.current_status ILIKE '%not passed%')")
		case "in_progress":
			where = append(where,
				"NOT (b.current_status ILIKE '%effective date%' OR b.current_status ILIKE '%governor signed%' OR b.current_status ILIKE '%chapter %2026 laws%' OR b.current_status ILIKE '%died%' OR b.current_status ILIKE '%vetoed%' OR b.current_status ILIKE '%not passed%')")
		default:
			idx := push(p.Status)
			where = append(where, fmt.Sprintf("(%s) = $%d", billStageCaseSQL("b.current_status"), idx))
		}
	}

	whereSQL := strings.Join(where, " AND ")

	// One join to the legislator that's the Primary sponsor (DISTINCT
	// ON keeps it to one row per bill if there are duplicate Primary
	// rows from re-ingestion).
	const baseFROM = `
FROM bill b
LEFT JOIN LATERAL (
  SELECT l.name, COALESCE(l.first_name, '') AS first_name,
         COALESCE(l.last_name, '') AS last_name,
         COALESCE(l.party, '') AS party,
         COALESCE(l.lws_sponsor_id, '') AS lws_sponsor_id
    FROM bill_sponsor bs
    JOIN legislator l ON l.id = bs.legislator_id
   WHERE bs.bill_id = b.id AND bs.sponsor_type = 'Primary'
   ORDER BY l.id
   LIMIT 1
) primary_sponsor ON TRUE`

	// Count total matches once (not per page).
	countQ := "SELECT COUNT(*) " + baseFROM + " WHERE " + whereSQL
	var total int
	if err := s.Pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count bills: %w", err)
	}

	// Selection page.
	args = append(args, p.Limit, p.Offset)
	q := `
SELECT b.biennium, b.prefix, b.number, b.bill_number,
       COALESCE(b.title, ''),
       COALESCE(b.chamber_origin, ''),
       COALESCE(b.current_status, ''),
       b.status_date,
       COALESCE(primary_sponsor.name, ''),
       COALESCE(primary_sponsor.first_name, ''),
       COALESCE(primary_sponsor.last_name, ''),
       COALESCE(primary_sponsor.party, '')
` + baseFROM + ` WHERE ` + whereSQL + `
 ORDER BY b.biennium DESC, b.prefix, b.number
 LIMIT $` + fmt.Sprintf("%d", len(args)-1) + ` OFFSET $` + fmt.Sprintf("%d", len(args))
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("search bills: %w", err)
	}
	defer rows.Close()
	out := []ListedBill{}
	for rows.Next() {
		var b ListedBill
		var statusDate pgtype.Date
		if err := rows.Scan(&b.Biennium, &b.Prefix, &b.Number, &b.BillID, &b.Title,
			&b.ChamberOrigin, &b.CurrentStatus, &statusDate,
			&b.LeadSponsor, &b.LeadFirstName, &b.LeadLastName, &b.LeadParty); err != nil {
			return nil, 0, fmt.Errorf("scan bill: %w", err)
		}
		if statusDate.Valid {
			b.StatusDate = statusDate.Time
		}
		out = append(out, b)
	}
	return out, total, rows.Err()
}

// ListBillSearchFacets returns the distinct prefix/chamber/party/status
// values present in the bill table, for the sidebar facet list.
func (s *Store) ListBillSearchFacets(ctx context.Context) (BillSearchFacets, error) {
	var f BillSearchFacets

	// Prefixes — keep only the first 12 to avoid degenerate noise; the
	// schema allows arbitrary text but in practice WA has ~10 distinct.
	rows, err := s.Pool.Query(ctx, `SELECT DISTINCT prefix FROM bill WHERE number > 0 AND prefix <> '' ORDER BY prefix`)
	if err != nil {
		return f, fmt.Errorf("facets prefix: %w", err)
	}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return f, err
		}
		f.Prefixes = append(f.Prefixes, p)
	}
	rows.Close()

	rows, err = s.Pool.Query(ctx, `SELECT DISTINCT chamber_origin FROM bill WHERE chamber_origin IS NOT NULL AND chamber_origin <> '' ORDER BY chamber_origin`)
	if err != nil {
		return f, fmt.Errorf("facets chamber: %w", err)
	}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			rows.Close()
			return f, err
		}
		f.Chambers = append(f.Chambers, c)
	}
	rows.Close()

	rows, err = s.Pool.Query(ctx, `SELECT DISTINCT party FROM legislator WHERE party IS NOT NULL AND party <> '' ORDER BY party`)
	if err != nil {
		return f, fmt.Errorf("facets party: %w", err)
	}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return f, err
		}
		f.Parties = append(f.Parties, p)
	}
	rows.Close()

	// Statuses come from the lifecycle-stage classifier. Return only
	// stages that actually have rows backing them, so the sidebar
	// doesn't show stages that match nothing in the current dataset.
	stageQ := `SELECT DISTINCT stage FROM (
  SELECT ` + billStageCaseSQL("current_status") + ` AS stage
    FROM bill WHERE number > 0 AND current_status IS NOT NULL AND current_status <> ''
) s WHERE stage <> '' ORDER BY stage`
	rows, err = s.Pool.Query(ctx, stageQ)
	if err != nil {
		return f, fmt.Errorf("facets status: %w", err)
	}
	for rows.Next() {
		var st string
		if err := rows.Scan(&st); err != nil {
			rows.Close()
			return f, err
		}
		f.Statuses = append(f.Statuses, st)
	}
	rows.Close()
	return f, nil
}

// ListFreshBillKeys returns the set of "PREFIX|NUMBER" keys for bills in
// the given biennium whose row was upserted within the last `since`
// duration. Used by `wa-dd ingest-session --skip-fresh` to resume a
// killed run without re-fetching bills already pulled this cycle.
func (s *Store) ListFreshBillKeys(ctx context.Context, biennium string, since time.Duration) (map[string]struct{}, error) {
	const q = `
SELECT prefix, number
  FROM bill
 WHERE biennium = $1
   AND updated_at >= NOW() - $2::interval;`
	rows, err := s.Pool.Query(ctx, q, biennium, fmt.Sprintf("%d seconds", int64(since.Seconds())))
	if err != nil {
		return nil, fmt.Errorf("list fresh bill keys: %w", err)
	}
	defer rows.Close()
	out := map[string]struct{}{}
	for rows.Next() {
		var prefix string
		var number int
		if err := rows.Scan(&prefix, &number); err != nil {
			return nil, fmt.Errorf("scan fresh bill: %w", err)
		}
		out[fmt.Sprintf("%s|%d", prefix, number)] = struct{}{}
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Auto-discovery queries — back the `wa-dd discover-hearings` and
// `wa-dd ingest-hearings` commands.
// ---------------------------------------------------------------------------

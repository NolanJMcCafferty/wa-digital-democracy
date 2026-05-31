package db

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// nonEmptyStrings returns trimmed, non-empty entries from in. Used by
// query builders that accept multi-value filters.
func nonEmptyStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if t := strings.TrimSpace(v); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// timeOrNull returns t for non-zero times, or pgtype Null otherwise. We
// use pgtype.Timestamptz because nullable TIMESTAMPTZ fields don't accept
// Go's zero time.Time gracefully.
func timeOrNull(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// dateOrNull returns d as a pg date or null when zero.
func dateOrNull(t time.Time) pgtype.Date {
	if t.IsZero() {
		return pgtype.Date{Valid: false}
	}
	return pgtype.Date{Time: t, Valid: true}
}

// strOrNull returns the string when non-empty or NULL otherwise.
func strOrNull(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ---------------------------------------------------------------------------
// bill
// ---------------------------------------------------------------------------

func nonNilStrings(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}

func fileSizeOrNull(n int64) any {
	if n == 0 {
		return nil
	}
	return n
}

func marshalJSONDefault(v any, def any) ([]byte, error) {
	if v == nil {
		v = def
	}
	return json.Marshal(v)
}

func defaultStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// billStageCaseSQL returns a SQL expression that classifies a
// current_status string into one of the lifecycle-stage labels mirrored
// in apps/web/src/lib/billStatus.ts. Rules are evaluated end-of-life
// first, matching the JS classifyBillStage rule order.
func billStageCaseSQL(col string) string {
	return `CASE
  WHEN ` + col + ` ILIKE '%effective date%' OR ` + col + ` ~* 'chapter [0-9]+,' OR ` + col + ` ILIKE '%filed with secretary of state%' THEN 'Session law'
  WHEN ` + col + ` ILIKE '%governor%vetoed%' THEN 'Vetoed'
  WHEN ` + col + ` ILIKE '%governor signed%' THEN 'Signed by Governor'
  WHEN ` + col + ` ILIKE '%delivered to governor%' THEN 'On Governor''s desk'
  WHEN ` + col + ` ILIKE '%speaker signed%' OR ` + col + ` ILIKE '%president signed%' THEN 'Passed Legislature'
  WHEN ` + col + ` ILIKE '%"x" file%' THEN 'Shelved'
  WHEN ` + col + ` ILIKE '%by resolution, reintroduced%' THEN 'Reintroduced'
  WHEN ` + col + ` ~* '\bdied\b' THEN 'Died'
  WHEN ` + col + ` ILIKE '%third reading, passed%' THEN 'Passed chamber'
  WHEN ` + col + ` ILIKE '%placed on second reading%' OR ` + col + ` ILIKE '%placed on third reading%' OR ` + col + ` ILIKE '%second reading%' THEN 'On floor calendar'
  WHEN ` + col + ` ILIKE '%referred to%' OR ` + col + ` ILIKE '%public hearing%' OR ` + col + ` ILIKE '%executive action%' OR ` + col + ` ILIKE '%executive session%' OR ` + col + ` ILIKE '%majority report%' OR ` + col + ` ILIKE '%minority report%' OR ` + col + ` ILIKE '%passed to rules%' THEN 'In committee'
  WHEN ` + col + ` ILIKE '%first reading%' OR ` + col + ` ILIKE '%prefiled%' OR ` + col + ` ILIKE '%introduced%' THEN 'Introduced'
  ELSE 'In progress'
END`
}

func nullStringArray(a []string) any {
	if a == nil {
		return []string{}
	}
	return a
}

func lowerTrim(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		out = append(out, r)
	}
	// Trim leading/trailing spaces.
	start, end := 0, len(out)
	for start < end && (out[start] == ' ' || out[start] == '\t') {
		start++
	}
	for end > start && (out[end-1] == ' ' || out[end-1] == '\t') {
		end--
	}
	return string(out[start:end])
}

// ---------------------------------------------------------------------------
// Aggregation queries — back the /api/v1/{legislators,organizations,hearings,sources}
// endpoints. All read-only; safe to run any time.
// ---------------------------------------------------------------------------

func datePtrOrNull(t *time.Time) pgtype.Date {
	if t == nil || t.IsZero() {
		return pgtype.Date{Valid: false}
	}
	return pgtype.Date{Time: *t, Valid: true}
}

func boolPtrOrNull(b *bool) any {
	if b == nil {
		return nil
	}
	return *b
}

func floatPtrOrNull(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func intPtrOrNull(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

// ---------------------------------------------------------------------------
// speaker identity evidence + review
// ---------------------------------------------------------------------------

func hasMinLetterRatio(s string, ratio float64) bool {
	var letters, total int
	for _, r := range s {
		if r == ' ' {
			continue
		}
		total++
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			letters++
		}
	}
	if total == 0 {
		return false
	}
	return float64(letters)/float64(total) >= ratio
}

package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	if key == "WADD_DSN" {
		if v := os.Getenv("DATABASE_URL"); v != "" {
			return v
		}
	}
	return def
}

func defaultAddr() string {
	if port := os.Getenv("PORT"); port != "" {
		return ":" + port
	}
	return ":8080"
}

// splitCSV parses a "HB1006,SB5001" query-string value into trimmed,
// non-empty parts. Returns nil for empty input so SearchBills's
// len-check skips the filter.
func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func upper(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		out[i] = c
	}
	return string(out)
}

// ---------------------------------------------------------------------------
// Aggregation handlers — back the home/index pages so the frontend avoids
// fanning out one page-detail request per bill.
// ---------------------------------------------------------------------------

// slugify mirrors apps/web/src/lib/api.ts:slugify exactly:
//
//	s.toLowerCase()
//	  .replace(/&/g, " and ")
//	  .replace(/[^a-z0-9]+/g, "-")
//	  .replace(/^-+|-+$/g, "")
//
// Identifiers produced server-side must match what the frontend emits in
// <Link> hrefs and route slugs, otherwise /api/v1/legislators/{slug}
// lookups silently miss.
func slugify(s string) string {
	// 1. lowercase, expand `&` → " and ".
	step1 := make([]byte, 0, len(s)+8)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c == '&' {
			step1 = append(step1, ' ', 'a', 'n', 'd', ' ')
			continue
		}
		step1 = append(step1, c)
	}
	// 2. collapse runs of [^a-z0-9] to single '-'.
	step2 := make([]byte, 0, len(step1))
	prevDash := false
	for _, c := range step1 {
		ok := (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
		if ok {
			step2 = append(step2, c)
			prevDash = false
			continue
		}
		if !prevDash {
			step2 = append(step2, '-')
			prevDash = true
		}
	}
	// 3. trim leading/trailing dashes.
	start, end := 0, len(step2)
	for start < end && step2[start] == '-' {
		start++
	}
	for end > start && step2[end-1] == '-' {
		end--
	}
	return string(step2[start:end])
}

// ---------------------------------------------------------------------------
// Transcript full-text search.
// ---------------------------------------------------------------------------

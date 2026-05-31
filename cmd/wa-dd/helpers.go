package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/candidate"
)

func runStub(cmd string, args []string, phase string) {
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "wa-dd %s: not yet implemented (planned for %s)\n", cmd, phase)
	}
	_ = fs.Parse(args)
	fmt.Fprintf(os.Stderr, "wa-dd %s: not yet implemented (planned for %s)\n", cmd, phase)
	os.Exit(64)
}
func printTop(cs []candidate.Candidate, n int) {
	fmt.Fprintln(os.Stderr, "Top candidates (highest score, most recent first):")
	fmt.Fprintln(os.Stderr, "  score  bill          chamber  meeting             testifiers (Pro/Con/Other)  orgs  agenda_item")
	for i, c := range cs {
		if i >= n {
			break
		}
		bill := c.BillID
		if bill == "" {
			bill = "(no bill)"
		}
		meeting := "-"
		if !c.MeetingDateTime.IsZero() {
			meeting = c.MeetingDateTime.Format("2006-01-02 15:04")
		}
		fmt.Fprintf(os.Stderr, "  %5d  %-13s %-7s  %-19s   %2d (%d/%d/%d)              %4d  %s\n",
			c.Score, bill, c.Chamber, meeting,
			c.TestifierCount, c.ProCount, c.ConCount, c.OtherCount,
			c.UniqueOrganizations, c.AgendaItemID)
	}
	fmt.Fprintln(os.Stderr, "\nUse discover-hearings and ingest-hearings to enrich selected agenda items from the database.")
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

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
func bienniumYears(b string) ([]int, error) {
	parts := strings.SplitN(b, "-", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid biennium %q (expected e.g. 2025-26)", b)
	}
	startYear, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid biennium start year: %w", err)
	}
	endYear := startYear + 1
	if len(parts[1]) == 2 {
		// "26" → 2026
		yy, err := strconv.Atoi(parts[1])
		if err == nil {
			endYear = (startYear/100)*100 + yy
		}
	} else if len(parts[1]) == 4 {
		yy, err := strconv.Atoi(parts[1])
		if err == nil {
			endYear = yy
		}
	}
	return []int{startYear, endYear}, nil
}

func parseTypeFilter(s string) map[string]struct{} {
	if s == "" {
		return nil
	}
	out := map[string]struct{}{}
	for _, t := range strings.Split(s, ",") {
		t = strings.TrimSpace(strings.ToUpper(t))
		if t != "" {
			out[t] = struct{}{}
		}
	}
	return out
}

// discoveryDeps adds CSI + TVW to the metadata-only set. TVW is
// constructed without an embedder key — only FetchWPVideoArchive is
// used during discovery, and that endpoint doesn't require it.
func normalizeMoney(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(s), "$", ""), ",", "")
}

func mediaExt(rawURL, contentType string) string {
	lowCT := strings.ToLower(contentType)
	if strings.Contains(lowCT, "mpeg") || strings.Contains(lowCT, "mp3") {
		return ".mp3"
	}
	if strings.Contains(lowCT, "mp4") {
		return ".mp4"
	}
	base := strings.ToLower(rawURL)
	for _, ext := range []string{".mp3", ".mp4", ".m4a", ".wav", ".aac"} {
		if strings.Contains(base, ext) {
			return ext
		}
	}
	return ".bin"
}

func safePathPart(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}
func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

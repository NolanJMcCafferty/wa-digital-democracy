package db_test

import (
	"context"
	"testing"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

func TestJunkOrganizationName(t *testing.T) {
	cases := []struct {
		name            string
		raw, normalized string
		junk            bool
	}{
		{"empty raw", "", "anything", true},
		{"empty normalized", "Foo", "", true},
		{"too short normalized", "Hi", "HI", true},
		{"exact self-descriptor", "self", "SELF", true},
		{"exact none", "N/A", "N/A", true},
		{"hashtag", "#NotABot", "NOTABOT", true},
		{"wrapped parens", "(Retired)", "RETIRED", true},
		{"wrapped brackets", "[Self]", "SELF", true},
		{"wrapped quotes", `"Home"`, "HOME", true},
		{"placeholder please select", "Please select", "PLEASE SELECT", true},
		{"placeholder enter your", "enter your name", "ENTER YOUR NAME", true},
		{"all symbols", "###!", "###!", true},
		{"mostly digits", "1234567", "1234567", true},
		{"real org", "Sierra Club", "SIERRA CLUB", false},
		{"real org with letters", "ACLU of Washington", "ACLU OF WASHINGTON", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := db.JunkOrganizationName(c.raw, c.normalized); got != c.junk {
				t.Errorf("db.JunkOrganizationName(%q, %q) = %v, want %v", c.raw, c.normalized, got, c.junk)
			}
		})
	}
}

func TestGetOrganizationTranscriptQuotes(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	// Test with non-existent organization ID (should not error, just return empty)
	quotes, err := store.GetOrganizationTranscriptQuotes(ctx, 99999)
	if err != nil {
		t.Fatalf("GetOrganizationTranscriptQuotes: %v", err)
	}

	// Should return empty slice, not nil
	if quotes == nil {
		t.Error("GetOrganizationTranscriptQuotes should return empty slice, not nil")
	}

	// With test fixtures, there may or may not be organizations with quotes.
	// The important thing is that the query runs without error and returns
	// proper structure.
	orgs, err := store.ListOrganizations(ctx, nil)
	if err != nil {
		t.Fatalf("ListOrganizations: %v", err)
	}

	for _, org := range orgs {
		quotes, err := store.GetOrganizationTranscriptQuotes(ctx, org.ID)
		if err != nil {
			t.Errorf("GetOrganizationTranscriptQuotes for org %d (%s): %v", org.ID, org.CanonicalName, err)
			continue
		}

		// Verify that any returned quotes have proper structure
		for i, quote := range quotes {
			if quote.Text == "" {
				t.Errorf("org %d quote[%d].Text is empty", org.ID, i)
			}
			if quote.EndMS <= quote.StartMS {
				t.Errorf("org %d quote[%d] has invalid time range: %d-%d", org.ID, i, quote.StartMS, quote.EndMS)
			}
			if quote.ReviewStatus != "accepted" {
				t.Errorf("org %d quote[%d] should only include accepted reviews, got %q", org.ID, i, quote.ReviewStatus)
			}
		}
	}
}

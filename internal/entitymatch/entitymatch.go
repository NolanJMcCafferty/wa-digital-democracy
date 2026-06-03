// Package entitymatch provides deterministic, reviewable entity-resolution
// helpers for linking procurement vendors and customers to existing
// organization records. It only generates candidates with evidence; callers
// must keep uncertain matches reviewable instead of treating them as truth.
package entitymatch

import (
	"regexp"
	"sort"
	"strings"
)

const (
	ConfidenceConfirmed = "confirmed"
	ConfidenceProbable  = "probable"
	ConfidencePossible  = "possible"
)

// NormalizedName is the deterministic join key used for first-pass entity
// matching. It is intentionally conservative: punctuation, case, common legal
// suffixes, and leading articles are ignored, but word order is preserved.
func NormalizedName(name string) string {
	upper := strings.ToUpper(strings.TrimSpace(name))
	upper = ampersandRE.ReplaceAllString(upper, " AND ")
	upper = punctuationRE.ReplaceAllString(upper, " ")
	words := strings.Fields(upper)
	out := make([]string, 0, len(words))
	for _, w := range words {
		w = strings.Trim(w, ".,;:'\"()[]{}")
		if w == "" || ignoredToken[w] {
			continue
		}
		out = append(out, w)
	}
	return strings.Join(out, " ")
}

// Candidate describes a possible source-name → organization link with visible
// confidence and evidence. SourceRecordIDs point at the raw rows that produced
// the candidate; they are not required to be exhaustive.
type Candidate struct {
	SourceKind     string   `json:"source_kind"`
	SourceName     string   `json:"source_name"`
	NormalizedName string   `json:"normalized_name"`
	OrganizationID int64    `json:"organization_id"`
	CanonicalName  string   `json:"canonical_name"`
	Confidence     string   `json:"confidence"`
	Evidence       []string `json:"evidence"`
}

// Decision stores an explicit human/system review decision for a candidate.
type Decision struct {
	CandidateID    int64  `json:"candidate_id"`
	OrganizationID int64  `json:"organization_id"`
	Decision       string `json:"decision"`
	Confidence     string `json:"confidence"`
	ReviewedBy     string `json:"reviewed_by,omitempty"`
	ReviewNotes    string `json:"review_notes,omitempty"`
}

// ConfidenceFor returns a conservative confidence label for two names.
// Exact normalized-name equality can be auto-candidate "probable"; alias or
// raw-canonical exact evidence upgrades to "confirmed" because the existing
// organization record already encodes a reviewed alias relationship.
func ConfidenceFor(sourceName, canonicalName string, aliases []string) (string, []string) {
	sourceNorm := NormalizedName(sourceName)
	canonicalNorm := NormalizedName(canonicalName)
	if sourceNorm == "" || canonicalNorm == "" {
		return "", nil
	}

	evidence := []string{"normalized_name:" + sourceNorm}
	if strings.EqualFold(strings.TrimSpace(sourceName), strings.TrimSpace(canonicalName)) {
		return ConfidenceConfirmed, append(evidence, "raw_name_exact_canonical")
	}
	for _, alias := range aliases {
		if strings.EqualFold(strings.TrimSpace(sourceName), strings.TrimSpace(alias)) {
			return ConfidenceConfirmed, append(evidence, "raw_name_exact_alias")
		}
		if sourceNorm == NormalizedName(alias) {
			return ConfidenceConfirmed, append(evidence, "normalized_name_exact_alias")
		}
	}
	if sourceNorm == canonicalNorm {
		return ConfidenceProbable, append(evidence, "normalized_name_exact_canonical")
	}
	if tokenContained(sourceNorm, canonicalNorm) {
		return ConfidencePossible, append(evidence, "token_subset")
	}
	return "", nil
}

// FalsePositiveRisk flags candidates that should not be emitted by purely
// deterministic name matching because they are too short or generic.
func FalsePositiveRisk(normalized string) bool {
	words := strings.Fields(normalized)
	if len(words) == 0 {
		return true
	}
	if len(words) == 1 {
		return len(words[0]) < 5 || genericToken[words[0]]
	}
	meaningful := 0
	for _, w := range words {
		if !genericToken[w] {
			meaningful++
		}
	}
	return meaningful == 0
}

func tokenContained(a, b string) bool {
	aw, bw := strings.Fields(a), strings.Fields(b)
	if len(aw) < 2 || len(bw) < 2 {
		return false
	}
	as, bs := wordSet(aw), wordSet(bw)
	return setContains(as, bs) || setContains(bs, as)
}

func wordSet(words []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, w := range words {
		if !genericToken[w] {
			out[w] = struct{}{}
		}
	}
	return out
}

func setContains(a, b map[string]struct{}) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	if len(a) < len(b) {
		return false
	}
	for w := range b {
		if _, ok := a[w]; !ok {
			return false
		}
	}
	return true
}

// UniqueStrings returns stable sorted non-empty values. It is shared by CLI
// output paths that aggregate evidence from multiple source tables.
func UniqueStrings(xs []string) []string {
	seen := map[string]struct{}{}
	for _, x := range xs {
		x = strings.TrimSpace(x)
		if x != "" {
			seen[x] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for x := range seen {
		out = append(out, x)
	}
	sort.Strings(out)
	return out
}

var (
	ampersandRE   = regexp.MustCompile(`&`)
	punctuationRE = regexp.MustCompile(`[^A-Z0-9]+`)
	ignoredToken  = map[string]bool{"THE": true, "A": true, "AN": true, "INC": true, "INCORPORATED": true, "LLC": true, "L": true, "LTD": true, "LIMITED": true, "CORP": true, "CORPORATION": true, "CO": true, "COMPANY": true, "PLC": true, "PC": true, "PLLC": true, "LP": true, "LLP": true, "ASSN": true, "ASSOCIATION": true}
	genericToken  = map[string]bool{"WASHINGTON": true, "STATE": true, "DEPARTMENT": true, "OFFICE": true, "CITY": true, "COUNTY": true, "PUBLIC": true, "SERVICES": true, "SERVICE": true, "GROUP": true, "SYSTEMS": true, "SOLUTIONS": true, "TECHNOLOGIES": true, "ENTERPRISES": true, "INTERNATIONAL": true, "NATIONAL": true, "NORTHWEST": true}
)

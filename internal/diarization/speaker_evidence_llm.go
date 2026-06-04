package diarization

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const LLMSpeakerEvidenceType = "llm_candidate"

// SpeakerEvidenceLLMClient is the provider boundary used by the LLM evidence
// extractor. OpenRouter is the production implementation; tests use fakes.
type SpeakerEvidenceLLMClient interface {
	ExtractSpeakerEvidence(ctx context.Context, req SpeakerEvidenceLLMRequest) (SpeakerEvidenceLLMResponse, error)
}

type SpeakerEvidenceLLMRequest struct {
	Agenda   SpeakerEvidenceAgenda
	Segments []SpeakerEvidenceLLMSegment
}

type SpeakerEvidenceLLMResponse struct {
	Candidates []SpeakerEvidenceLLMResult
	Usage      SpeakerEvidenceLLMUsage
}

type SpeakerEvidenceLLMUsage struct {
	InputTokens              int
	OutputTokens             int
	CacheCreationInputTokens int
	CacheReadInputTokens     int
}

type SpeakerEvidenceLLMSegment struct {
	ID               int64  `json:"segment_id"`
	SpeakerClusterID int64  `json:"speaker_cluster_id,omitempty"`
	ClusterLabel     string `json:"cluster_label"`
	StartMS          int    `json:"start_ms"`
	EndMS            int    `json:"end_ms"`
	Text             string `json:"text"`
}

type SpeakerEvidenceAgenda struct {
	TVWEventID   string                   `json:"tvw_event_id"`
	Hearings     []SpeakerEvidenceHearing `json:"hearings,omitempty"`
	Legislators  []SpeakerEvidenceRef     `json:"legislators"`
	Testifiers   []SpeakerEvidenceRef     `json:"testifiers"`
	Bills        []SpeakerEvidenceBill    `json:"bills,omitempty"`
	BillSponsors []SpeakerEvidenceSponsor `json:"bill_sponsors,omitempty"`
}

type SpeakerEvidenceHearing struct {
	HearingID     int64  `json:"hearing_id"`
	CommitteeName string `json:"committee_name"`
	Chamber       string `json:"chamber"`
}

type SpeakerEvidenceRef struct {
	ID           int64    `json:"id"`
	Label        string   `json:"label"`
	Aliases      []string `json:"aliases,omitempty"`
	Role         string   `json:"role,omitempty"`
	Chamber      string   `json:"chamber,omitempty"`
	District     string   `json:"district,omitempty"`
	Party        string   `json:"party,omitempty"`
	Organization string   `json:"organization,omitempty"`
	Position     string   `json:"position,omitempty"`
	Testified    bool     `json:"testified,omitempty"`
	AgendaItemID int64    `json:"agenda_item_id,omitempty"`
	AgendaLabel  string   `json:"agenda_label,omitempty"`
}

type SpeakerEvidenceBill struct {
	ID     int64  `json:"id"`
	Number string `json:"number"`
	Title  string `json:"title,omitempty"`
}

type SpeakerEvidenceSponsor struct {
	BillID       int64  `json:"bill_id"`
	BillNumber   string `json:"bill_number"`
	LegislatorID int64  `json:"legislator_id"`
	Label        string `json:"label"`
	SponsorType  string `json:"sponsor_type"`
}

type SpeakerEvidenceLLMResult struct {
	SegmentID           int64   `json:"segment_id"`
	ClusterLabel        string  `json:"cluster_label,omitempty"`
	CandidateKind       string  `json:"candidate_kind"`
	CandidateLabel      string  `json:"candidate_label"`
	CandidateID         string  `json:"candidate_id,omitempty"`
	EvidenceType        string  `json:"evidence_type,omitempty"`
	EvidenceTextExcerpt string  `json:"evidence_text_excerpt"`
	Confidence          float64 `json:"confidence"`
	Reasoning           string  `json:"reasoning,omitempty"`
}

type SpeakerEvidenceLLMMatch struct {
	Segment   SpeakerEvidenceLLMSegment
	Candidate SpeakerEvidenceCandidate
	Raw       SpeakerEvidenceLLMResult
}

func ExtractSpeakerEvidenceLLM(ctx context.Context, client SpeakerEvidenceLLMClient, segments []SpeakerEvidenceLLMSegment, agenda SpeakerEvidenceAgenda) ([]SpeakerEvidenceLLMMatch, SpeakerEvidenceLLMUsage, error) {
	if client == nil {
		return nil, SpeakerEvidenceLLMUsage{}, fmt.Errorf("speaker evidence llm: client is nil")
	}
	cleanSegments := evidenceLLMSegmentsWithText(segments)
	if len(cleanSegments) == 0 {
		return nil, SpeakerEvidenceLLMUsage{}, nil
	}
	resp, err := client.ExtractSpeakerEvidence(ctx, SpeakerEvidenceLLMRequest{
		Agenda:   agenda,
		Segments: cleanSegments,
	})
	if err != nil {
		return nil, resp.Usage, err
	}
	return normalizeSpeakerEvidenceLLMResults(resp.Candidates, cleanSegments, agenda), resp.Usage, nil
}

func evidenceLLMSegmentsWithText(segments []SpeakerEvidenceLLMSegment) []SpeakerEvidenceLLMSegment {
	out := make([]SpeakerEvidenceLLMSegment, 0, len(segments))
	for _, s := range segments {
		s.Text = strings.TrimSpace(s.Text)
		if s.ID == 0 || s.Text == "" || !hasUsefulText(s.Text) {
			continue
		}
		out = append(out, s)
	}
	return out
}

func hasUsefulText(s string) bool {
	letters := 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			letters++
			if letters >= 3 {
				return true
			}
		}
	}
	return false
}

func normalizeSpeakerEvidenceLLMResults(results []SpeakerEvidenceLLMResult, segments []SpeakerEvidenceLLMSegment, agenda SpeakerEvidenceAgenda) []SpeakerEvidenceLLMMatch {
	segmentsByID := map[int64]SpeakerEvidenceLLMSegment{}
	for _, s := range segments {
		segmentsByID[s.ID] = s
	}

	legislators := speakerEvidenceRefsByID(agenda.Legislators)
	testifiers := speakerEvidenceRefsByID(agenda.Testifiers)

	type dedupValue struct {
		match SpeakerEvidenceLLMMatch
		order int
	}
	seen := map[string]dedupValue{}
	for i, r := range results {
		segment, ok := segmentsByID[r.SegmentID]
		if !ok {
			continue
		}
		conf := clampConfidence(r.Confidence)
		if conf < 0.5 {
			continue
		}
		label := cleanLLMCandidateLabel(r.CandidateLabel)
		if label == "" {
			continue
		}
		kind, candidateID, canonicalLabel, ok := groundLLMCandidate(r, label, legislators, testifiers)
		if !ok {
			continue
		}
		excerpt := strings.TrimSpace(r.EvidenceTextExcerpt)
		if excerpt == "" {
			excerpt = segment.Text
		}
		candidate := SpeakerEvidenceCandidate{
			Kind:           kind,
			CandidateID:    candidateID,
			Label:          canonicalLabel,
			EvidenceType:   LLMSpeakerEvidenceType,
			EvidenceText:   excerpt,
			Confidence:     conf,
			MatchedPattern: "llm_structured_output",
			Reasoning:      strings.TrimSpace(r.Reasoning),
		}
		key := fmt.Sprintf("%d:%s:%d:%s", segment.ID, candidate.Kind, candidate.CandidateID, strings.ToLower(candidate.Label))
		next := SpeakerEvidenceLLMMatch{Segment: segment, Candidate: candidate, Raw: r}
		if prev, exists := seen[key]; !exists || next.Candidate.Confidence > prev.match.Candidate.Confidence {
			seen[key] = dedupValue{match: next, order: i}
		}
	}
	out := make([]dedupValue, 0, len(seen))
	for _, v := range seen {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].order < out[j].order })
	matches := make([]SpeakerEvidenceLLMMatch, 0, len(out))
	for _, v := range out {
		matches = append(matches, v.match)
	}
	return matches
}

func groundLLMCandidate(r SpeakerEvidenceLLMResult, label string, legislators, testifiers map[int64]SpeakerEvidenceRef) (string, int64, string, bool) {
	kind := normalizeLLMCandidateKind(r.CandidateKind)
	id := parseLLMCandidateID(r.CandidateID)
	switch kind {
	case "legislator":
		if ref, ok := legislators[id]; ok {
			return "legislator", ref.ID, ref.Label, true
		}
		if ref, ok := findSpeakerEvidenceRefByLabel(label, legislators); ok {
			return "legislator", ref.ID, ref.Label, true
		}
		return "person", 0, label, true
	case "testifier":
		if ref, ok := testifiers[id]; ok {
			return "testifier", ref.ID, ref.Label, true
		}
		if ref, ok := findSpeakerEvidenceRefByLabel(label, testifiers); ok {
			return "testifier", ref.ID, ref.Label, true
		}
		return "person", 0, label, true
	case "person":
		if ref, ok := findSpeakerEvidenceRefByLabel(label, legislators); ok {
			return "legislator", ref.ID, ref.Label, true
		}
		if ref, ok := findSpeakerEvidenceRefByLabel(label, testifiers); ok {
			return "testifier", ref.ID, ref.Label, true
		}
		return "person", 0, label, true
	default:
		return "", 0, "", false
	}
}

func speakerEvidenceRefsByID(refs []SpeakerEvidenceRef) map[int64]SpeakerEvidenceRef {
	out := map[int64]SpeakerEvidenceRef{}
	for _, ref := range refs {
		if ref.ID != 0 && strings.TrimSpace(ref.Label) != "" {
			out[ref.ID] = ref
		}
	}
	return out
}

func findSpeakerEvidenceRefByLabel(label string, refs map[int64]SpeakerEvidenceRef) (SpeakerEvidenceRef, bool) {
	want := normalizeEvidenceName(label)
	if want == "" {
		return SpeakerEvidenceRef{}, false
	}
	lastNameMatches := []SpeakerEvidenceRef{}
	for _, ref := range refs {
		names := append([]string{ref.Label}, ref.Aliases...)
		for _, name := range names {
			if normalizeEvidenceName(name) == want {
				return ref, true
			}
		}
		if lastNameOnly(label) != "" && lastNameOnly(label) == lastNameOnly(ref.Label) {
			lastNameMatches = append(lastNameMatches, ref)
		}
	}
	if len(lastNameMatches) == 1 {
		return lastNameMatches[0], true
	}
	return SpeakerEvidenceRef{}, false
}

func normalizeLLMCandidateKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "legislator", "testifier", "person":
		return strings.ToLower(strings.TrimSpace(kind))
	default:
		return "unknown"
	}
}

func parseLLMCandidateID(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if idx := strings.LastIndex(raw, ":"); idx >= 0 {
		raw = raw[idx+1:]
	}
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id < 0 {
		return 0
	}
	return id
}

func cleanLLMCandidateLabel(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, " .,;:!?()[]{}\"'")
	return strings.Join(strings.Fields(s), " ")
}

func normalizeEvidenceName(s string) string {
	s = strings.ToLower(cleanLLMCandidateLabel(s))
	replacer := strings.NewReplacer(
		"senator ", "",
		"sen. ", "",
		"representative ", "",
		"rep. ", "",
	)
	s = replacer.Replace(s)
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r))
	})
	return strings.Join(fields, " ")
}

func lastNameOnly(s string) string {
	fields := strings.Fields(normalizeEvidenceName(s))
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

func clampConfidence(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

package diarization

import (
	"context"
	"testing"
)

type fakeSpeakerEvidenceLLMClient struct {
	resp SpeakerEvidenceLLMResponse
	err  error
}

func (f fakeSpeakerEvidenceLLMClient) ExtractSpeakerEvidence(context.Context, SpeakerEvidenceLLMRequest) (SpeakerEvidenceLLMResponse, error) {
	return f.resp, f.err
}

func TestExtractSpeakerEvidenceLLM(t *testing.T) {
	agenda := SpeakerEvidenceAgenda{
		Legislators: []SpeakerEvidenceRef{{
			ID: 11, Label: "Senator Jane Smith", Aliases: []string{"Jane Smith", "Senator Smith"}, District: "32",
		}},
		Testifiers: []SpeakerEvidenceRef{{
			ID: 22, Label: "Jane Doe", Aliases: []string{"Jane Doe"}, Organization: "Attorney General's Office",
		}},
	}
	segments := []SpeakerEvidenceLLMSegment{
		{ID: 101, SpeakerClusterID: 501, ClusterLabel: "SPEAKER_01", StartMS: 1000, EndMS: 3000, Text: "Senator Smith from the 32nd, thank you."},
		{ID: 102, SpeakerClusterID: 502, ClusterLabel: "SPEAKER_02", StartMS: 4000, EndMS: 7000, Text: "Mr. Chair, Jane Doe with the AG's office."},
		{ID: 103, SpeakerClusterID: 503, ClusterLabel: "SPEAKER_03", StartMS: 8000, EndMS: 9500, Text: "Good morning. My name is Alex Lee."},
	}
	resp := SpeakerEvidenceLLMResponse{Candidates: []SpeakerEvidenceLLMResult{
		{SegmentID: 101, CandidateKind: "legislator", CandidateLabel: "Senator Smith", CandidateID: "11", EvidenceType: "llm_candidate", EvidenceTextExcerpt: "Senator Smith from the 32nd", Confidence: 0.86},
		{SegmentID: 102, CandidateKind: "testifier", CandidateLabel: "Jane Doe", EvidenceType: "llm_candidate", EvidenceTextExcerpt: "Jane Doe with the AG's office", Confidence: 0.79},
		{SegmentID: 103, CandidateKind: "person", CandidateLabel: "Alex Lee", EvidenceType: "llm_candidate", EvidenceTextExcerpt: "My name is Alex Lee", Confidence: 0.64},
		{SegmentID: 103, CandidateKind: "person", CandidateLabel: "Alex Lee", EvidenceType: "llm_candidate", EvidenceTextExcerpt: "My name is Alex Lee", Confidence: 0.61},
		{SegmentID: 101, CandidateKind: "person", CandidateLabel: "Low Confidence", EvidenceType: "llm_candidate", EvidenceTextExcerpt: "low", Confidence: 0.49},
	}}

	got, _, err := ExtractSpeakerEvidenceLLM(context.Background(), fakeSpeakerEvidenceLLMClient{resp: resp}, segments, agenda)
	if err != nil {
		t.Fatalf("ExtractSpeakerEvidenceLLM: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("matches = %d, want 3: %#v", len(got), got)
	}
	assertCandidate(t, got[0], 101, "legislator", 11, "Senator Jane Smith", 0.86)
	assertCandidate(t, got[1], 102, "testifier", 22, "Jane Doe", 0.79)
	assertCandidate(t, got[2], 103, "person", 0, "Alex Lee", 0.64)
	for _, m := range got {
		if m.Candidate.EvidenceType != LLMSpeakerEvidenceType {
			t.Fatalf("evidence type = %q, want %q", m.Candidate.EvidenceType, LLMSpeakerEvidenceType)
		}
	}
}

func TestExtractSpeakerEvidenceLLMEmptyOrGarbledTranscript(t *testing.T) {
	got, _, err := ExtractSpeakerEvidenceLLM(
		context.Background(),
		fakeSpeakerEvidenceLLMClient{resp: SpeakerEvidenceLLMResponse{Candidates: []SpeakerEvidenceLLMResult{{SegmentID: 1, CandidateKind: "person", CandidateLabel: "Jane Doe", Confidence: 0.9}}}},
		[]SpeakerEvidenceLLMSegment{{ID: 1, Text: "..."}, {ID: 2, Text: ""}},
		SpeakerEvidenceAgenda{},
	)
	if err != nil {
		t.Fatalf("ExtractSpeakerEvidenceLLM: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("matches = %#v, want none", got)
	}
}

func assertCandidate(t *testing.T, got SpeakerEvidenceLLMMatch, segmentID int64, kind string, id int64, label string, confidence float64) {
	t.Helper()
	if got.Segment.ID != segmentID ||
		got.Candidate.Kind != kind ||
		got.Candidate.CandidateID != id ||
		got.Candidate.Label != label ||
		got.Candidate.Confidence != confidence {
		t.Fatalf("candidate = %#v", got)
	}
}

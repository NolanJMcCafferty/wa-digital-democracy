package diarization

import "testing"

func TestExtractSpeakerEvidenceLegislator(t *testing.T) {
	got := ExtractSpeakerEvidence("For the record, Senator Noel Frame from the thirty sixth Legislative District bringing forward this bill.")
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1: %#v", len(got), got)
	}
	if got[0].Kind != "legislator" || got[0].Label != "Noel Frame" || got[0].EvidenceType != "self_introduction" {
		t.Fatalf("candidate = %#v", got[0])
	}
}

func TestExtractSpeakerEvidencePerson(t *testing.T) {
	got := ExtractSpeakerEvidence("Good morning. For the record, my name is Jane Doe with the Housing Alliance.")
	if len(got) != 1 || got[0].Kind != "person" || got[0].Label != "Jane Doe" {
		t.Fatalf("got = %#v", got)
	}
}

func TestExtractSpeakerEvidenceIAmName(t *testing.T) {
	got := ExtractSpeakerEvidence("Good morning, Mr. Chair and Mr. Ranking Member. I'm Mark Mattson. I'm staff to this committee.")
	if len(got) != 1 || got[0].Kind != "person" || got[0].Label != "Mark Mattson" {
		t.Fatalf("got = %#v", got)
	}
	if got[0].MatchedPattern != "i_am_name" {
		t.Fatalf("pattern = %q", got[0].MatchedPattern)
	}
}

func TestExtractSpeakerEvidenceAvoidsSingleName(t *testing.T) {
	if got := ExtractSpeakerEvidence("My name is Jane and I support the bill."); len(got) != 0 {
		t.Fatalf("got = %#v, want none", got)
	}
}

func TestExtractSpeakerEvidenceAvoidsGenericIAmPhrase(t *testing.T) {
	if got := ExtractSpeakerEvidence("I'm staff to this committee and I will describe the bill."); len(got) != 0 {
		t.Fatalf("got = %#v, want none", got)
	}
	if got := ExtractSpeakerEvidence("I'm not going to go into those details."); len(got) != 0 {
		t.Fatalf("got = %#v, want none", got)
	}
}

func TestRegexAndLLMEvidenceTypesDoNotOverlap(t *testing.T) {
	got := ExtractSpeakerEvidence("For the record, my name is Jane Doe with the Housing Alliance.")
	if len(got) != 1 {
		t.Fatalf("got = %#v, want one regex candidate", got)
	}
	if got[0].EvidenceType == LLMSpeakerEvidenceType {
		t.Fatalf("regex evidence type overlaps LLM evidence type %q", LLMSpeakerEvidenceType)
	}
}

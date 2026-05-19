package diarization

import (
	"regexp"
	"strings"
)

// SpeakerEvidenceCandidate is one high-precision identity clue extracted from
// diarized transcript text.
type SpeakerEvidenceCandidate struct {
	Kind           string
	Label          string
	EvidenceType   string
	EvidenceText   string
	Confidence     float64
	MatchedPattern string
}

var selfIntroPatterns = []struct {
	name string
	re   *regexp.Regexp
	kind string
	conf float64
}{
	{
		name: "for_the_record_legislator",
		re:   regexp.MustCompile(`(?i)\bfor the record,?\s+(senator|representative|rep\.?|sen\.?)\s+([A-Z][A-Za-z'’-]+(?:\s+[A-Z][A-Za-z'’-]+){0,3})`),
		kind: "legislator",
		conf: 0.95,
	},
	{
		name: "legislator_from_district",
		re:   regexp.MustCompile(`(?i)\b(senator|representative|rep\.?|sen\.?)\s+([A-Z][A-Za-z'’-]+(?:\s+[A-Z][A-Za-z'’-]+){0,3})\s+from\s+(?:the\s+)?(?:[\w\s-]+district|district)`),
		kind: "legislator",
		conf: 0.90,
	},
	{
		name: "for_the_record_my_name_is",
		re:   regexp.MustCompile(`(?i)\bfor the record,?\s+my name is\s+([A-Z][A-Za-z'’-]+(?:\s+[A-Z][A-Za-z'’-]+){1,3})`),
		kind: "person",
		conf: 0.82,
	},
	{
		name: "my_name_is",
		re:   regexp.MustCompile(`(?i)\bmy name is\s+([A-Z][A-Za-z'’-]+(?:\s+[A-Z][A-Za-z'’-]+){1,3})`),
		kind: "person",
		conf: 0.72,
	},
}

// ExtractSpeakerEvidence finds conservative self-introduction patterns in one
// diarized segment. It intentionally emits few candidates; false positives are
// worse than misses because candidates become review tasks.
func ExtractSpeakerEvidence(text string) []SpeakerEvidenceCandidate {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	out := []SpeakerEvidenceCandidate{}
	seen := map[string]bool{}
	for _, p := range selfIntroPatterns {
		matches := p.re.FindAllStringSubmatch(text, -1)
		for _, m := range matches {
			if len(m) == 0 {
				continue
			}
			label := ""
			if p.kind == "legislator" && len(m) >= 3 {
				label = cleanCandidateName(m[2])
			} else if len(m) >= 2 {
				label = cleanCandidateName(m[1])
			}
			if label == "" || badCandidateLabel(label) {
				continue
			}
			key := p.kind + ":" + strings.ToLower(label)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, SpeakerEvidenceCandidate{
				Kind:           p.kind,
				Label:          label,
				EvidenceType:   "self_introduction",
				EvidenceText:   text,
				Confidence:     p.conf,
				MatchedPattern: p.name,
			})
		}
	}
	return out
}

func cleanCandidateName(s string) string {
	s = strings.TrimSpace(s)
	stopWords := []string{" from ", " with ", " representing ", " on behalf of ", " bringing ", " and ", " but ", " if ", " because "}
	low := strings.ToLower(s)
	cut := len(s)
	for _, stop := range stopWords {
		if i := strings.Index(low, stop); i >= 0 && i < cut {
			cut = i
		}
	}
	s = strings.TrimSpace(s[:cut])
	s = strings.Trim(s, " .,;:!?()[]{}\"'")
	return s
}

func badCandidateLabel(s string) bool {
	words := strings.Fields(s)
	badTail := map[string]bool{"and": true, "but": true, "if": true, "because": true, "with": true, "from": true}
	if len(words) > 0 && badTail[strings.ToLower(words[len(words)-1])] {
		return true
	}
	if len(words) < 2 {
		return true
	}
	if len(words) > 4 {
		return true
	}
	return false
}

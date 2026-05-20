package jobs

// Bill-discussion segmentation lives here as a pure function so the
// segmenter can be unit-tested without a database. The single source
// of truth for "this cue mentions bill X" is DetectBillDiscussionWindows
// + the regex it uses (billDiscussionMentionPattern below). Downstream
// consumers — page assemblers, search-result deep-links, future
// transcript-highlighting UI — should:
//
//   1. Read agenda_item_window rows that SegmentTranscript persisted, or
//   2. Call DetectBillDiscussionWindows with the same inputs,
//
// rather than running their own regex against transcript_segment.text.
// Two implementations of "matches the bill" inevitably drift, and
// rendered windows that disagree with the segmenter's persisted windows
// have already bitten this codebase once (the old in-SQL window-detect
// old page-assembly query used a 120s gap threshold, while the segmenter
// used 180s — bills with mention gaps in that band rendered with a
// different window count than they were tagged with). Don't repeat it.

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const defaultBillSegmentPaddingMS = 60_000

// SegmentWindow is one detected bill-discussion window in a transcript.
// A single agenda item can have multiple windows when a bill is revisited
// after intervening agenda items.
type SegmentWindow struct {
	StartMS  int
	EndMS    int
	Mentions int
}

type segmentCue struct {
	StartMS int
	EndMS   int
	Text    string
}

// DetectBillDiscussionWindows groups bill-number mentions into one or more
// discussion windows. Close cues and far-apart mentions split windows so a
// repeated bill discussion in one TVW event does not become one giant range.
func DetectBillDiscussionWindows(cues []segmentCue, billPrefix string, billNumber int) []SegmentWindow {
	if len(cues) == 0 {
		return nil
	}
	sorted := append([]segmentCue(nil), cues...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].StartMS < sorted[j].StartMS })

	mentionRe := regexp.MustCompile(billDiscussionMentionPattern(billPrefix, billNumber))
	var windows []SegmentWindow
	var current *SegmentWindow
	lastMentionMS := 0
	closedSinceLastMention := false

	for _, cue := range sorted {
		text := normalizeCueText(cue.Text)
		if !mentionRe.MatchString(text) {
			if current != nil && isBillDiscussionCloseCue(text) {
				current.EndMS = cue.EndMS
				closedSinceLastMention = true
			}
			continue
		}

		startNew := current == nil
		if current != nil {
			// Split when another bill opened/closed in between, or when mentions
			// are far enough apart that they are likely separate agenda moments.
			if closedSinceLastMention || cue.StartMS-lastMentionMS > defaultBillSegmentPaddingMS*3 {
				windows = append(windows, *current)
				startNew = true
			}
		}
		if startNew {
			current = &SegmentWindow{StartMS: clampNonNegative(cue.StartMS - defaultBillSegmentPaddingMS)}
			closedSinceLastMention = false
		}
		current.Mentions++
		current.EndMS = cue.EndMS + defaultBillSegmentPaddingMS
		lastMentionMS = cue.StartMS
	}
	if current != nil {
		windows = append(windows, *current)
	}
	return mergeOverlappingWindows(windows)
}

// billDiscussionMentionPattern returns a case-insensitive regex that
// matches every common spoken or abbreviated form of a bill mention.
//
// Coverage:
//   - All bare prefixes WA legislators use: HB, SB, HJR, SJR, HCR, SCR,
//     HJM, SJM (the same set our prefix normalizer recognizes).
//   - The substituted/engrossed chrome an LWS bill carries as it moves
//     through the legislature: optional "E" (engrossed), optional "2"
//     or "3" (Nth substitute), optional "S" (substitute). So "ESHB",
//     "2SSB", "E2SHB" all match the same bare prefix.
//   - The spoken English long form ("house bill", "senate concurrent
//     resolution", etc.) including the "engrossed substitute …" chrome.
//
// Whitespace between the prefix and the digits is optional so
// "HB1234" (no space) matches alongside "HB 1234".
//
// SegmentTranscript is the canonical source for "this cue mentions the
// bill" — search-result highlighting and other downstream consumers
// should read agenda_item_window or call DetectBillDiscussionWindows
// rather than running their own regex, which would otherwise drift.
func billDiscussionMentionPattern(prefix string, number int) string {
	chamberWord := map[string]string{
		"HB":  "house bill",
		"SB":  "senate bill",
		"HJR": "house joint resolution",
		"SJR": "senate joint resolution",
		"HCR": "house concurrent resolution",
		"SCR": "senate concurrent resolution",
		"HJM": "house joint memorial",
		"SJM": "senate joint memorial",
	}
	num := fmt.Sprintf("%d", number)
	cw, known := chamberWord[prefix]

	// Prefix half: optional engrossment / Nth-substitute / substitute
	// chrome before the bare prefix. Same shape as
	// internal/sources/lws/normalize.go:baseBillPrefix's input grammar.
	abbrev := fmt.Sprintf(`E?[23]?S?%s`, prefix)

	if !known {
		return fmt.Sprintf(`(?i)\b%s\s*%s\b`, abbrev, num)
	}
	// Spoken half: optional "engrossed (substitute|second substitute|…)
	// (substitute)?" prefix before the long chamber form.
	spoken := fmt.Sprintf(`(?:engrossed\s+)?(?:(?:second|third|2nd|3rd)\s+)?(?:substitute\s+)?%s`, cw)
	return fmt.Sprintf(`(?i)\b(?:%s|%s)\s*%s\b`, abbrev, spoken, num)
}

func normalizeCueText(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

func isBillDiscussionCloseCue(text string) bool {
	closePatterns := []string{
		"that closes", "this closes", "that concludes", "this concludes",
		"we are done", "we're done", "no further questions", "moving on",
		"move on", "next bill", "next item", "next agenda", "we will now move",
		"that ends", "this ends", "executive session", "public hearing is closed",
	}
	for _, p := range closePatterns {
		if strings.Contains(text, p) {
			return true
		}
	}
	return false
}

func mergeOverlappingWindows(windows []SegmentWindow) []SegmentWindow {
	if len(windows) < 2 {
		return windows
	}
	sort.Slice(windows, func(i, j int) bool { return windows[i].StartMS < windows[j].StartMS })
	out := []SegmentWindow{windows[0]}
	for _, w := range windows[1:] {
		last := &out[len(out)-1]
		if w.StartMS <= last.EndMS {
			if w.EndMS > last.EndMS {
				last.EndMS = w.EndMS
			}
			last.Mentions += w.Mentions
			continue
		}
		out = append(out, w)
	}
	return out
}

func clampNonNegative(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

func describeWindows(windows []SegmentWindow) string {
	parts := make([]string, 0, len(windows))
	for _, w := range windows {
		parts = append(parts, fmt.Sprintf("[%d, %d]", w.StartMS, w.EndMS))
	}
	return strings.Join(parts, ", ")
}

package jobs

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/config"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/lws"
)

// stderrSink is a no-op io.Writer for warning-style messages from inside
// jobs. We can't log via the user-supplied callback from helpers, so this
// flushes to OS stderr.
var stderrSink io.Writer = os.Stderr

// biennialBounds returns ("YYYY-01-01", "YYYY+1-12-31") for a biennium
// like "2025-26". Used as date bounds for LWS history calls that require
// beginDate/endDate.
func biennialBounds(biennium string) (string, string) {
	parts := strings.SplitN(biennium, "-", 2)
	if len(parts) != 2 {
		return "1900-01-01", "2099-12-31"
	}
	return parts[0] + "-01-01", "20" + parts[1] + "-12-31"
}

// parseLWSDate is the same parser as lws/normalize.go but available here
// so jobs can convert ActionDate strings without importing internals.
//
// LWS values are Pacific wall-clock; see lws/normalize.go for context.
func parseLWSDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		loc = time.UTC
	}
	for _, layout := range []string{
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05",
	} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, nil
}

// pickHearingForDemo picks the LWS hearing that most likely corresponds to
// the demo's committee. Match priority:
//
//   1. Exact LWS AgendaId match against demo's CSI meeting family ID.
//      (LWS AgendaId and CSI meetingFamilyId are different namespaces, so
//       this rarely fires — but cheap to check.)
//   2. Same chamber AND committee acronym matches (case-insensitive).
//   3. Same chamber AND committee LongName contains "housing".
//   4. First hearing whose chamber matches the demo.
//
// We deliberately do NOT fall back to the first arbitrary hearing — that
// led to a real bug where a House-origin bill's first hearing in the
// House Housing committee shadowed the demo's Senate Housing hearing.
func pickHearingForDemo(in []lws.Hearing, demo *config.SelectedDemo) *lws.Hearing {
	if len(in) == 0 {
		return nil
	}
	chamber := strings.ToLower(strings.TrimSpace(demo.Chamber))
	wantAcronym := strings.ToLower(strings.TrimSpace(demo.Committee.Acronym))

	// Priority 1: AgendaId match.
	for i, h := range in {
		if h.CommitteeMeeting.AgendaID != "" && h.CommitteeMeeting.AgendaID == demo.Agenda.CSIMeetingFamilyID {
			return &in[i]
		}
	}
	// Priority 2: chamber + acronym.
	for i, h := range in {
		if !sameChamber(h, chamber) || len(h.CommitteeMeeting.Committees) == 0 {
			continue
		}
		c := h.CommitteeMeeting.Committees[0]
		if wantAcronym != "" && strings.EqualFold(c.Acronym, wantAcronym) {
			return &in[i]
		}
	}
	// Priority 3: chamber + "housing" in LongName.
	for i, h := range in {
		if !sameChamber(h, chamber) || len(h.CommitteeMeeting.Committees) == 0 {
			continue
		}
		c := h.CommitteeMeeting.Committees[0]
		if strings.Contains(strings.ToLower(c.LongName), "housing") {
			return &in[i]
		}
	}
	// Priority 4: first hearing in the right chamber.
	for i, h := range in {
		if sameChamber(h, chamber) {
			return &in[i]
		}
	}
	return nil
}

func sameChamber(h lws.Hearing, chamber string) bool {
	return strings.EqualFold(strings.TrimSpace(h.CommitteeMeeting.Agency), chamber)
}

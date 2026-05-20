// Package candidate implements `wa-dd find-candidates` — the Phase 3
// operator-facing tool that scores recent committee hearings for the
// first-page demo.
//
// Per Blueprint Step 0 (lines 528–550), we look for hearings where:
//
//   - CSI testifiers exist;
//   - TVW captions are available (Phase 0 confirmed: 100% coverage on
//     legislative-committee events — so we don't bother scoring this);
//   - identifiable organizations appear among testifiers.
//
// Strategy: for issue=housing we go directly to the House Housing and
// Senate Housing committees in CSI rather than scanning the entire bill
// list. Every agenda item in those committees is on-issue by definition,
// and CSI is the source of testifier truth anyway.
package candidate

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/csi"
)

// Candidate is one scored bill/hearing pair the operator can pick from.
type Candidate struct {
	Issue                  string    `json:"issue"`
	Score                  int       `json:"score"`

	// Bill identifier extracted from the agenda label.
	BillPrefix             string    `json:"bill_prefix,omitempty"`           // e.g. "HB"
	BillNumber             int       `json:"bill_number,omitempty"`           // e.g. 1234
	BillID                 string    `json:"bill_id,omitempty"`               // "HB 1234"
	BillTitle              string    `json:"bill_title,omitempty"`            // free-text from agenda label

	// Committee + meeting context.
	Chamber                string    `json:"chamber"`
	CommitteeID            string    `json:"csi_committee_id"`
	CommitteeName          string    `json:"committee_name"`
	MeetingFamilyID        string    `json:"csi_meeting_family_id"`
	MeetingDateTime        time.Time `json:"meeting_datetime,omitempty"`
	MeetingLabel           string    `json:"meeting_label"`

	// Agenda item identifiers used by discovery and ingest-hearings.
	AgendaItemFamilyID     string    `json:"csi_agenda_item_family_id"`
	AgendaItemID           string    `json:"csi_agenda_item_id"`
	AgendaItemLabel        string    `json:"agenda_item_label"`

	// Testifier signal.
	TestifierCount         int       `json:"testifier_count"`
	UniqueOrganizations    int       `json:"unique_organizations"`
	ProCount               int       `json:"pro_count"`
	ConCount               int       `json:"con_count"`
	OtherCount             int       `json:"other_count"`
	SampleOrganizations    []string  `json:"sample_organizations,omitempty"`

	// Source URLs for operator inspection/debugging.
	OfficialBillURL        string    `json:"official_bill_url,omitempty"`

	// Diagnostics — empty on success.
	FetchError             string    `json:"fetch_error,omitempty"`
}

// CommitteeTarget is a (chamber, csi_committee_id) pair the finder should
// scan. For housing we hardcode House Housing and Senate Housing.
type CommitteeTarget struct {
	Chamber       string
	CommitteeID   string
	CommitteeName string
}

// IssueTargets returns the CSI committees we scan for a given issue.
//
// Phase 0 confirmed both housing committees are active across the full
// 2025-26 biennium with substantial meeting history. The wiki recommends
// housing as the first demo issue.
func IssueTargets(issue string) ([]CommitteeTarget, error) {
	switch strings.ToLower(issue) {
	case "housing":
		return []CommitteeTarget{
			{Chamber: "House", CommitteeID: "31633", CommitteeName: "House Housing"},
			{Chamber: "Senate", CommitteeID: "34078", CommitteeName: "Senate Housing"},
		}, nil
	default:
		return nil, fmt.Errorf("candidate: no committee targets configured for issue %q (try 'housing')", issue)
	}
}

// ScoreSignals captures the raw signals scoring uses; kept separate from
// the scoring rule so the rule is unit-testable in isolation.
type ScoreSignals struct {
	TestifierCount      int
	UniqueOrganizations int
}

// Score computes the candidate score per Phase 3 rule:
//
//   +2 if CSI testifiers exist
//   +1 if ≥1 testifier carries an organization
//   +1 if ≥2 distinct organizations
//
// (TVW caption coverage is dropped from the score — Phase 0 measured 100%
// coverage on legislative-committee events, so it carries no signal.)
//
// Max score: 4.
func Score(s ScoreSignals) int {
	score := 0
	if s.TestifierCount > 0 {
		score += 2
	}
	if s.UniqueOrganizations >= 1 {
		score += 1
	}
	if s.UniqueOrganizations >= 2 {
		score += 1
	}
	return score
}

// FromTestifiers populates score-relevant counts from a CSI testifier list.
func FromTestifiers(rows []csi.Testifier) (ScoreSignals, OrgSummary) {
	uniqOrgs := map[string]struct{}{}
	var sample []string
	var pro, con, other int
	for _, t := range rows {
		switch csi.CanonicalPosition(t.Position) {
		case "Pro":
			pro++
		case "Con":
			con++
		case "Other":
			other++
		}
		org := strings.TrimSpace(t.Organization)
		if org == "" {
			continue
		}
		key := strings.ToLower(org)
		if _, ok := uniqOrgs[key]; !ok {
			uniqOrgs[key] = struct{}{}
			if len(sample) < 5 {
				sample = append(sample, org)
			}
		}
	}
	return ScoreSignals{
			TestifierCount:      len(rows),
			UniqueOrganizations: len(uniqOrgs),
		}, OrgSummary{
			Pro:                 pro,
			Con:                 con,
			Other:               other,
			UniqueOrganizations: len(uniqOrgs),
			SampleOrgs:          sample,
		}
}

// OrgSummary is the testifier breakdown stored on each Candidate.
type OrgSummary struct {
	Pro                 int
	Con                 int
	Other               int
	UniqueOrganizations int
	SampleOrgs          []string
}

// ParseBillFromLabel extracts the bill identifier from a CSI agenda label
// like "HB 2747 Budget sustainability". Returns ("", 0) if the label does
// not start with a recognizable bill prefix.
func ParseBillFromLabel(label string) (prefix string, number int, title string) {
	m := billLabelRe.FindStringSubmatch(strings.TrimSpace(label))
	if m == nil {
		return "", 0, strings.TrimSpace(label)
	}
	prefix = strings.ToUpper(m[1])
	if n, err := parseInt(m[2]); err == nil {
		number = n
	}
	title = strings.TrimSpace(m[3])
	return
}

// Recognized prefixes from leg.wa.gov: HB/SB (bills), HJR/SJR (resolutions),
// HCR/SCR (concurrent), HJM/SJM (memorials), HR/SR (chamber resolutions).
//
// CSI labels also occasionally show "SHB" (substitute), "ESHB" (engrossed
// substitute), etc. — we strip those modifiers and key on the trailing
// (HB|SB|...) prefix so number lookups work.
var billLabelRe = regexp.MustCompile(`^(?:E?2?S?)?(HB|SB|HJR|SJR|HCR|SCR|HJM|SJM|HR|SR)\s+(\d+)\s*(.*)$`)

func parseInt(s string) (int, error) {
	n := 0
	if s == "" {
		return 0, errors.New("empty")
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("not a number: %q", s)
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

// SortCandidatesDesc sorts in place: highest score first, ties broken by
// most-recent meeting date.
func SortCandidatesDesc(cs []Candidate) {
	sort.Slice(cs, func(i, j int) bool {
		if cs[i].Score != cs[j].Score {
			return cs[i].Score > cs[j].Score
		}
		return cs[i].MeetingDateTime.After(cs[j].MeetingDateTime)
	})
}

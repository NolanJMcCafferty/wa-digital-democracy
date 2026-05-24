package candidate

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/csi"
)

// FindOptions configures a Finder run.
type FindOptions struct {
	Issue              string
	MaxAgendaItems     int          // hard cap across all committees (default 50)
	MaxMeetingsPerComm int          // recent meetings to inspect per committee (default 8)
	OnProgress         func(string) // optional progress callback
}

type csiReader interface {
	ListMeetings(ctx context.Context, chamber, committeeID string) ([]csi.Meeting, error)
	ListAgendaItems(ctx context.Context, chamber, meetingFamilyID string) ([]csi.AgendaItem, error)
	GetTestifiers(ctx context.Context, agendaItemID, agendaItemDescription string) ([]csi.Testifier, error)
}

// Finder runs the CSI-driven candidate scan.
type Finder struct {
	CSI csiReader
}

// NewFinder constructs a Finder.
func NewFinder(c *csi.Client) *Finder {
	return &Finder{CSI: c}
}

// Find scans the issue's target committees and returns scored candidates.
// All errors are reported per-candidate (FetchError) so a single transient
// failure doesn't sink the whole run.
func (f *Finder) Find(ctx context.Context, opts FindOptions) ([]Candidate, error) {
	if f.CSI == nil {
		return nil, errors.New("candidate: nil CSI client")
	}
	if opts.MaxAgendaItems == 0 {
		opts.MaxAgendaItems = 50
	}
	if opts.MaxMeetingsPerComm == 0 {
		opts.MaxMeetingsPerComm = 8
	}

	targets, err := IssueTargets(opts.Issue)
	if err != nil {
		return nil, err
	}

	progress := opts.OnProgress
	if progress == nil {
		progress = func(string) {}
	}

	var out []Candidate
	for _, t := range targets {
		if len(out) >= opts.MaxAgendaItems {
			break
		}
		progress(fmt.Sprintf("listing meetings for %s/%s", t.Chamber, t.CommitteeName))

		meetings, err := f.CSI.ListMeetings(ctx, t.Chamber, t.CommitteeID)
		if err != nil {
			out = append(out, Candidate{
				Issue: opts.Issue, Chamber: t.Chamber, CommitteeID: t.CommitteeID,
				CommitteeName: t.CommitteeName,
				FetchError:    fmt.Sprintf("ListMeetings: %v", err),
			})
			continue
		}
		// CSI returns meetings most-recent first by `text` ordering; cap.
		if len(meetings) > opts.MaxMeetingsPerComm {
			meetings = meetings[:opts.MaxMeetingsPerComm]
		}

		for _, m := range meetings {
			if len(out) >= opts.MaxAgendaItems {
				break
			}
			progress(fmt.Sprintf("  meeting %s — %s", m.MeetingFamilyID, m.Label))

			items, err := f.CSI.ListAgendaItems(ctx, t.Chamber, m.MeetingFamilyID)
			if err != nil {
				out = append(out, Candidate{
					Issue: opts.Issue, Chamber: t.Chamber, CommitteeID: t.CommitteeID,
					CommitteeName: t.CommitteeName, MeetingFamilyID: m.MeetingFamilyID,
					MeetingLabel: m.Label, MeetingDateTime: m.StartDateTime,
					FetchError: fmt.Sprintf("ListAgendaItems: %v", err),
				})
				continue
			}

			for _, item := range items {
				if len(out) >= opts.MaxAgendaItems {
					break
				}
				cand := Candidate{
					Issue:              opts.Issue,
					Chamber:            t.Chamber,
					CommitteeID:        t.CommitteeID,
					CommitteeName:      t.CommitteeName,
					MeetingFamilyID:    m.MeetingFamilyID,
					MeetingDateTime:    m.StartDateTime,
					MeetingLabel:       m.Label,
					AgendaItemFamilyID: item.AgendaItemFamilyID,
					AgendaItemID:       item.AgendaItemID,
					AgendaItemLabel:    item.Label,
				}
				prefix, num, title := ParseBillFromLabel(item.Label)
				if prefix != "" {
					cand.BillPrefix = prefix
					cand.BillNumber = num
					cand.BillID = prefix + " " + strconv.Itoa(num)
					cand.BillTitle = title
					cand.OfficialBillURL = officialBillURL(prefix, num, m.StartDateTime)
				}

				rows, err := f.CSI.GetTestifiers(ctx, item.AgendaItemID, item.Label)
				if err != nil {
					cand.FetchError = fmt.Sprintf("GetTestifiers: %v", err)
					out = append(out, cand)
					continue
				}
				signals, orgs := FromTestifiers(rows)
				cand.TestifierCount = signals.TestifierCount
				cand.UniqueOrganizations = signals.UniqueOrganizations
				cand.ProCount = orgs.Pro
				cand.ConCount = orgs.Con
				cand.OtherCount = orgs.Other
				cand.SampleOrganizations = orgs.SampleOrgs
				cand.Score = Score(signals)

				out = append(out, cand)
			}
		}
	}

	SortCandidatesDesc(out)
	return out, nil
}

// officialBillURL returns the public bill-summary URL. Year is derived from
// the meeting datetime when available, otherwise the current year.
func officialBillURL(prefix string, number int, meetingTime any) string {
	year := "2025"
	if t, ok := meetingTime.(interface{ Year() int }); ok {
		if y := t.Year(); y >= 2000 {
			year = strconv.Itoa(y)
		}
	}
	return "https://app.leg.wa.gov/billsummary?BillNumber=" + strconv.Itoa(number) +
		"&Year=" + year
}

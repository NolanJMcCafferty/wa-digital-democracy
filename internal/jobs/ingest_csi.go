package jobs

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/csi"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

// IngestCSI fetches the agenda item's testifier list and upserts the
// hearing/agenda_item/testifier rows. It assumes the operator picked a
// candidate from Phase 3, so the IDs are already known and we don't have
// to walk the full meeting tree.
func (p *Pipeline) IngestCSI(ctx context.Context, ids *IDs) error {
	httpClient := p.CSI.HTTP
	base := p.CSI.BaseURL
	chamber := p.Demo.Chamber
	demo := p.Demo

	// 1. Fetch the testifier list (and capture the source_record id).
	q := url.Values{}
	q.Set("agendaItemId", demo.Agenda.CSIAgendaItemID)
	q.Set("agendaItemDescription", demo.Agenda.Label)
	tFetch, err := httpClient.Do(ctx, httpx.Request{
		System:   csi.SystemName,
		Endpoint: "csi.GetOtherTestifiers",
		URL:      base + "/Home/GetOtherTestifiers?" + q.Encode(),
		Headers:  csiAjaxHTML(),
	})
	if err != nil {
		return fmt.Errorf("GetOtherTestifiers: %w", err)
	}
	rows, err := csi.ParseTestifiers(tFetch.Body)
	if err != nil {
		return err
	}

	// 2. We may not yet have a hearing row from Step 1 if LWS returned no
	// hearings. Materialize one from the CSI meeting label as a fallback.
	if ids.HearingID == 0 {
		mFetch, err := httpClient.Do(ctx, httpx.Request{
			System:   csi.SystemName,
			Endpoint: "csi.GetMeetings",
			URL: base + "/Home/GetMeetings?" + (url.Values{
				"chamber": []string{chamber}, "committeeId": []string{demo.Committee.CSIID},
			}).Encode(),
			Headers: csiAjaxJSON(),
		})
		if err != nil {
			return fmt.Errorf("GetMeetings: %w", err)
		}
		meetings, err := csi.ParseMeetings(mFetch.Body)
		if err != nil {
			return err
		}
		startTime := pickMeetingTime(meetings, demo.Agenda.CSIMeetingFamilyID)
		hid, err := p.Store.UpsertHearing(ctx, db.UpsertHearingParams{
			BillID:           pInt64(ids.BillID),
			CommitteeName:    chamberCommitteeName(chamber, demo.Committee.Acronym),
			CommitteeAcronym: demo.Committee.Acronym,
			Chamber:          chamber,
			MeetingDateTime:  startTime,
			SourceRecordID:   mFetch.SourceRecordID,
		})
		if err != nil {
			return err
		}
		ids.HearingID = hid
	}

	// 3. Upsert the agenda_item.
	aiID, err := p.Store.UpsertAgendaItem(ctx, db.UpsertAgendaItemParams{
		HearingID:             ids.HearingID,
		BillID:                pInt64(ids.BillID),
		Label:                 demo.Agenda.Label,
		CSIMeetingFamilyID:    demo.Agenda.CSIMeetingFamilyID,
		CSIAgendaItemFamilyID: demo.Agenda.CSIAgendaItemFamilyID,
		CSIAgendaItemID:       demo.Agenda.CSIAgendaItemID,
		SourceRecordID:        tFetch.SourceRecordID,
	})
	if err != nil {
		return err
	}
	ids.AgendaItemID = aiID

	// 4. Replace testifiers.
	params := make([]db.InsertTestifierParams, 0, len(rows))
	for _, t := range rows {
		params = append(params, db.InsertTestifierParams{
			AgendaItemID:    aiID,
			RawName:         t.Name,
			RawOrganization: t.Organization,
			Position:        csi.CanonicalPosition(t.Position),
			Testified:       t.Testified,
			TimeSignedIn:    t.TimeSignedIn,
			SourceRecordID:  tFetch.SourceRecordID,
		})
	}
	if err := p.Store.ReplaceTestifiersForAgenda(ctx, aiID, params); err != nil {
		return err
	}
	return nil
}

func csiAjaxJSON() http.Header {
	h := http.Header{}
	h.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	h.Set("X-Requested-With", "XMLHttpRequest")
	return h
}

func csiAjaxHTML() http.Header {
	h := http.Header{}
	h.Set("Accept", "text/html, */*")
	h.Set("X-Requested-With", "XMLHttpRequest")
	return h
}

// chamberCommitteeName produces a human label for the hearing committee
// when LWS didn't supply one. We don't have a mapping table, so we pick
// chamber + acronym, which is at least informative.
func chamberCommitteeName(chamber, acronym string) string {
	if acronym == "" {
		return chamber + " Committee"
	}
	return chamber + " " + acronym
}

func pickMeetingTime(in []csi.Meeting, familyID string) time.Time {
	for _, m := range in {
		if m.MeetingFamilyID == familyID && !m.StartDateTime.IsZero() {
			return m.StartDateTime
		}
	}
	if len(in) > 0 {
		return in[0].StartDateTime
	}
	return time.Time{}
}

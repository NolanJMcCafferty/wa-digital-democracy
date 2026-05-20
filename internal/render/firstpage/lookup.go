package firstpage

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/config"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

// ErrBillNotFound signals the API layer that no ingested bill+hearing was
// found for the requested (biennium, prefix, number). The handler maps
// this to HTTP 404; everything else propagates as 500.
var ErrBillNotFound = errors.New("firstpage: bill not ingested")

// LookupSelectedDemo reconstructs a SelectedDemo for a bill by joining
// against the agenda_item + hearing tables Phase 4's ingest pipeline
// populated. The API path reads it from Postgres directly; the singular
// `wa-dd build-bundle` reads it from config/selected_demo.yml.
//
// When a bill has multiple hearings (e.g. House referral and Senate
// referral), the most recent hearing wins. That's the v1 default; if a
// page renders the wrong chamber's hearing, add a `?hearing=<chamber>`
// query param and route it through here.
func LookupSelectedDemo(
	ctx context.Context,
	store *db.Store,
	biennium, prefix string,
	number int,
) (*config.SelectedDemo, error) {
	const q = `
SELECT a.csi_agenda_item_id, a.csi_meeting_family_id,
       a.csi_agenda_item_family_id, a.label,
       COALESCE(h.tvw_event_id, ''),
       COALESCE(h.committee_acronym, ''),
       h.chamber
  FROM bill b
  JOIN agenda_item a ON a.bill_id = b.id
  JOIN hearing     h ON h.id = a.hearing_id
 WHERE b.biennium = $1 AND b.prefix = $2 AND b.number = $3
 ORDER BY h.meeting_datetime DESC
 LIMIT 1;`
	var (
		csiAgendaItemID, csiMeetingFamilyID, csiAgendaItemFamilyID string
		label, tvwEventID, committeeAcronym, chamber               string
	)
	err := store.Pool.QueryRow(ctx, q, biennium, prefix, number).Scan(
		&csiAgendaItemID, &csiMeetingFamilyID, &csiAgendaItemFamilyID,
		&label, &tvwEventID, &committeeAcronym, &chamber,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s %d in %s", ErrBillNotFound, prefix, number, biennium)
	}
	if err != nil {
		return nil, fmt.Errorf("lookup demo: %w", err)
	}

	demo := &config.SelectedDemo{
		Biennium:   biennium,
		BillPrefix: prefix,
		BillNumber: number,
		Chamber:    chamber,
	}
	demo.Committee.Acronym = committeeAcronym
	demo.Agenda.CSIAgendaItemID = csiAgendaItemID
	demo.Agenda.CSIMeetingFamilyID = csiMeetingFamilyID
	demo.Agenda.CSIAgendaItemFamilyID = csiAgendaItemFamilyID
	demo.Agenda.Label = label
	demo.TVW.EventID = tvwEventID
	return demo, nil
}

// LookupAllSelectedDemos returns one SelectedDemo per agenda_item attached
// to the bill, ordered most-recent-first. The bill-detail page uses this
// to render every hearing (House referral, Senate referral, work session,
// etc.) instead of just the latest one.
func LookupAllSelectedDemos(
	ctx context.Context,
	store *db.Store,
	biennium, prefix string,
	number int,
) ([]*config.SelectedDemo, error) {
	const q = `
SELECT a.csi_agenda_item_id, a.csi_meeting_family_id,
       a.csi_agenda_item_family_id, a.label,
       COALESCE(h.tvw_event_id, ''),
       COALESCE(h.committee_acronym, ''),
       h.chamber
  FROM bill b
  JOIN agenda_item a ON a.bill_id = b.id
  JOIN hearing     h ON h.id = a.hearing_id
 WHERE b.biennium = $1 AND b.prefix = $2 AND b.number = $3
 ORDER BY h.meeting_datetime DESC;`
	rows, err := store.Pool.Query(ctx, q, biennium, prefix, number)
	if err != nil {
		return nil, fmt.Errorf("lookup all demos: %w", err)
	}
	defer rows.Close()
	var out []*config.SelectedDemo
	for rows.Next() {
		var (
			csiAgendaItemID, csiMeetingFamilyID, csiAgendaItemFamilyID string
			label, tvwEventID, committeeAcronym, chamber               string
		)
		if err := rows.Scan(
			&csiAgendaItemID, &csiMeetingFamilyID, &csiAgendaItemFamilyID,
			&label, &tvwEventID, &committeeAcronym, &chamber,
		); err != nil {
			return nil, fmt.Errorf("scan demo: %w", err)
		}
		demo := &config.SelectedDemo{
			Biennium:   biennium,
			BillPrefix: prefix,
			BillNumber: number,
			Chamber:    chamber,
		}
		demo.Committee.Acronym = committeeAcronym
		demo.Agenda.CSIAgendaItemID = csiAgendaItemID
		demo.Agenda.CSIMeetingFamilyID = csiMeetingFamilyID
		demo.Agenda.CSIAgendaItemFamilyID = csiAgendaItemFamilyID
		demo.Agenda.Label = label
		demo.TVW.EventID = tvwEventID
		out = append(out, demo)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// LookupSelectedDemoByAgendaItem reconstructs a SelectedDemo by pivoting
// on the CSI agenda item ID. The hearing-detail API endpoint
// (/api/v1/hearings/{id}) needs this so it can call firstpage.Build
// without first knowing the bill identifiers.
func LookupSelectedDemoByAgendaItem(
	ctx context.Context,
	store *db.Store,
	csiAgendaItemID string,
) (*config.SelectedDemo, error) {
	const q = `
SELECT b.biennium, b.prefix, b.number,
       a.csi_agenda_item_id, a.csi_meeting_family_id,
       a.csi_agenda_item_family_id, a.label,
       COALESCE(h.tvw_event_id, ''),
       COALESCE(h.committee_acronym, ''),
       h.chamber
  FROM agenda_item a
  JOIN hearing h ON h.id = a.hearing_id
  JOIN bill    b ON b.id = a.bill_id
 WHERE a.csi_agenda_item_id = $1
 LIMIT 1;`
	var (
		biennium, prefix                                          string
		number                                                    int
		gotCSIAgendaItemID, csiMeetingFamilyID, csiAgendaItemFamilyID string
		label, tvwEventID, committeeAcronym, chamber              string
	)
	err := store.Pool.QueryRow(ctx, q, csiAgendaItemID).Scan(
		&biennium, &prefix, &number,
		&gotCSIAgendaItemID, &csiMeetingFamilyID, &csiAgendaItemFamilyID,
		&label, &tvwEventID, &committeeAcronym, &chamber,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: agenda_item %s", ErrBillNotFound, csiAgendaItemID)
	}
	if err != nil {
		return nil, fmt.Errorf("lookup demo by agenda item: %w", err)
	}

	demo := &config.SelectedDemo{
		Biennium:   biennium,
		BillPrefix: prefix,
		BillNumber: number,
		Chamber:    chamber,
	}
	demo.Committee.Acronym = committeeAcronym
	demo.Agenda.CSIAgendaItemID = gotCSIAgendaItemID
	demo.Agenda.CSIMeetingFamilyID = csiMeetingFamilyID
	demo.Agenda.CSIAgendaItemFamilyID = csiAgendaItemFamilyID
	demo.Agenda.Label = label
	demo.TVW.EventID = tvwEventID
	return demo, nil
}

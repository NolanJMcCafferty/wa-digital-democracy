package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/common"
)

// ErrBillNotFound signals that no ingested bill or bill/agenda target was
// found for the requested lookup.
var ErrBillNotFound = errors.New("db: bill not ingested")

// LookupBillAgendaTarget reconstructs a BillAgendaTarget for a bill by joining
// against the agenda_item + hearing tables the ingestion pipeline populated.
//
// When a bill has multiple hearings (e.g. House referral and Senate
// referral), the most recent hearing wins. That's the v1 default; if a
// page renders the wrong chamber's hearing, add a `?hearing=<chamber>`
// query param and route it through here.
func (s *Store) LookupBillAgendaTarget(
	ctx context.Context,
	biennium, prefix string,
	number int,
) (*common.BillAgendaTarget, error) {
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
	err := s.Pool.QueryRow(ctx, q, biennium, prefix, number).Scan(
		&csiAgendaItemID, &csiMeetingFamilyID, &csiAgendaItemFamilyID,
		&label, &tvwEventID, &committeeAcronym, &chamber,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s %d in %s", ErrBillNotFound, prefix, number, biennium)
	}
	if err != nil {
		return nil, fmt.Errorf("lookup bill agenda target: %w", err)
	}

	billAgendaTarget := &common.BillAgendaTarget{
		Bill:      common.BillKey{Biennium: biennium, Prefix: prefix, Number: number},
		Committee: common.CommitteeRef{Chamber: chamber},
	}
	billAgendaTarget.Committee.Acronym = committeeAcronym
	billAgendaTarget.AgendaItem.CSIAgendaItemID = csiAgendaItemID
	billAgendaTarget.AgendaItem.CSIMeetingFamilyID = csiMeetingFamilyID
	billAgendaTarget.AgendaItem.CSIAgendaItemFamilyID = csiAgendaItemFamilyID
	billAgendaTarget.AgendaItem.Label = label
	billAgendaTarget.TVW.EventID = tvwEventID
	return billAgendaTarget, nil
}

// LookupBillAgendaTargetsForBill returns one BillAgendaTarget per agenda_item attached
// to the bill, ordered most-recent-first. The bill-detail page uses this
// to render every hearing (House referral, Senate referral, work session,
// etc.) instead of just the latest one.
func (s *Store) LookupBillAgendaTargetsForBill(
	ctx context.Context,
	biennium, prefix string,
	number int,
) ([]*common.BillAgendaTarget, error) {
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
	rows, err := s.Pool.Query(ctx, q, biennium, prefix, number)
	if err != nil {
		return nil, fmt.Errorf("lookup bill agenda targets: %w", err)
	}
	defer rows.Close()
	var out []*common.BillAgendaTarget
	for rows.Next() {
		var (
			csiAgendaItemID, csiMeetingFamilyID, csiAgendaItemFamilyID string
			label, tvwEventID, committeeAcronym, chamber               string
		)
		if err := rows.Scan(
			&csiAgendaItemID, &csiMeetingFamilyID, &csiAgendaItemFamilyID,
			&label, &tvwEventID, &committeeAcronym, &chamber,
		); err != nil {
			return nil, fmt.Errorf("scan bill agenda target: %w", err)
		}
		billAgendaTarget := &common.BillAgendaTarget{
			Bill:      common.BillKey{Biennium: biennium, Prefix: prefix, Number: number},
			Committee: common.CommitteeRef{Chamber: chamber},
		}
		billAgendaTarget.Committee.Acronym = committeeAcronym
		billAgendaTarget.AgendaItem.CSIAgendaItemID = csiAgendaItemID
		billAgendaTarget.AgendaItem.CSIMeetingFamilyID = csiMeetingFamilyID
		billAgendaTarget.AgendaItem.CSIAgendaItemFamilyID = csiAgendaItemFamilyID
		billAgendaTarget.AgendaItem.Label = label
		billAgendaTarget.TVW.EventID = tvwEventID
		out = append(out, billAgendaTarget)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// LookupBillAgendaTargetByAgendaItem reconstructs a BillAgendaTarget by pivoting
// on the CSI agenda item ID. Hearing ingestion and the hearing-detail API use
// this to build per-agenda-item sections without first knowing bill IDs.
func (s *Store) LookupBillAgendaTargetByAgendaItem(
	ctx context.Context,
	csiAgendaItemID string,
) (*common.BillAgendaTarget, error) {
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
		biennium, prefix                                              string
		number                                                        int
		gotCSIAgendaItemID, csiMeetingFamilyID, csiAgendaItemFamilyID string
		label, tvwEventID, committeeAcronym, chamber                  string
	)
	err := s.Pool.QueryRow(ctx, q, csiAgendaItemID).Scan(
		&biennium, &prefix, &number,
		&gotCSIAgendaItemID, &csiMeetingFamilyID, &csiAgendaItemFamilyID,
		&label, &tvwEventID, &committeeAcronym, &chamber,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: agenda_item %s", ErrBillNotFound, csiAgendaItemID)
	}
	if err != nil {
		return nil, fmt.Errorf("lookup bill agenda target by agenda item: %w", err)
	}

	billAgendaTarget := &common.BillAgendaTarget{
		Bill:      common.BillKey{Biennium: biennium, Prefix: prefix, Number: number},
		Committee: common.CommitteeRef{Chamber: chamber},
	}
	billAgendaTarget.Committee.Acronym = committeeAcronym
	billAgendaTarget.AgendaItem.CSIAgendaItemID = gotCSIAgendaItemID
	billAgendaTarget.AgendaItem.CSIMeetingFamilyID = csiMeetingFamilyID
	billAgendaTarget.AgendaItem.CSIAgendaItemFamilyID = csiAgendaItemFamilyID
	billAgendaTarget.AgendaItem.Label = label
	billAgendaTarget.TVW.EventID = tvwEventID
	return billAgendaTarget, nil
}

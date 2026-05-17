package jobs

import (
	"context"
	"fmt"
	"strings"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/config"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/pdc"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

// PDCContext loads `config/reviewed_matches.yml`, upserts an organization
// row for each reviewed match, and pulls a small sample of PDC lobbying
// records for each. v1 is reviewed-match-only (Blueprint Step 7 endorses
// this); auto-fuzzy-matching against PDC is deferred.
//
// Path: config/reviewed_matches.yml (relative to cwd). Empty file → no-op.
func (p *Pipeline) PDCContext(ctx context.Context, ids *IDs) error {
	matches, err := config.LoadReviewedMatches("config/reviewed_matches.yml")
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		fmt.Fprintln(stderrSink, "  no reviewed matches; org context section will be empty")
		return nil
	}

	for _, m := range matches {
		canonical := strings.TrimSpace(m.CanonicalName)
		if canonical == "" {
			canonical = m.CSIOrganization
		}
		orgID, err := p.Store.UpsertOrganization(ctx, db.UpsertOrganizationParams{
			CanonicalName:         canonical,
			Aliases:               []string{m.CSIOrganization},
			PDCLobbyistEmployerID: m.PDCLobbyistEmployerID,
			PDCCommitteeOrFilerID: m.PDCCommitteeOrFilerID,
			MatchConfidence:       defaultStr(m.Confidence, "confirmed"),
			MatchNotes:            m.Notes,
		})
		if err != nil {
			return err
		}
		ids.OrgIDs[canonical] = orgID

		// Bind any matching testifier rows for this agenda item.
		if _, err := p.Store.LinkTestifiersToOrg(ctx, orgID, []string{m.CSIOrganization}); err != nil {
			return err
		}

		// Pull a few PDC rows if we have an employer ID. Lobbying employment
		// is the cheapest signal that an org has any PDC presence at all.
		if m.PDCLobbyistEmployerID != "" {
			where := fmt.Sprintf("employer_id='%s'", strings.ReplaceAll(m.PDCLobbyistEmployerID, "'", "''"))
			rows, err := p.PDC.FetchPage(ctx, pdc.DatasetLobbyistEmployment, pdc.Query{
				Where: where, Limit: 50, Order: ":id",
			})
			if err != nil {
				fmt.Fprintf(stderrSink, "  warn: PDC employment fetch for %s: %v\n", canonical, err)
				continue
			}
			ctxRows := make([]db.InsertOrgContextParams, 0, len(rows))
			for _, r := range rows {
				le := pdc.NormalizeLobbyistEmployment(r)
				ctxRows = append(ctxRows, db.InsertOrgContextParams{
					OrganizationID:  orgID,
					ContextType:     "lobbying_registration",
					SourceDatasetID: pdc.DatasetLobbyistEmployment,
					SourceRowID:     le.ReportNumber,
					SummaryFields: map[string]any{
						"lobbyist_name":     le.LobbyistName,
						"employer_name":    le.EmployerName,
						"employment_year":  le.EmploymentYear,
						"employment_period": le.EmploymentPeriod,
					},
					SourceURL:       le.EmploymentURL,
					MatchConfidence: defaultStr(m.Confidence, "confirmed"),
					SourceRecordID:  0, // PDC pulls go through the same RawSink, but we
					// don't capture the source_record id without re-running through Do().
					// Leaving 0 makes this fail the NOT NULL FK, so use a sentinel:
				})
			}
			if len(ctxRows) > 0 {
				// We need a real source_record id. The sink wrote one for the
				// most recent fetch — pull it back by content_hash. Cheap +
				// idempotent. (Phase 5 sqlc work could thread this through
				// natively.)
				if err := p.fillSourceRecordIDs(ctx, ctxRows, "pdc_socrata"); err != nil {
					return err
				}
				if err := p.Store.ReplaceOrgContextForOrganization(ctx, orgID, ctxRows); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// fillSourceRecordIDs sets InsertOrgContextParams.SourceRecordID to the most
// recent source_record id for the given system. This is a workaround until
// connectors return RawFetch ids through their public API.
func (p *Pipeline) fillSourceRecordIDs(ctx context.Context, rows []db.InsertOrgContextParams, system string) error {
	const q = `SELECT id FROM source_record WHERE source_system = $1 ORDER BY fetched_at DESC LIMIT 1`
	var id int64
	if err := p.Store.Pool.QueryRow(ctx, q, system).Scan(&id); err != nil {
		return fmt.Errorf("fill source_record_id: %w", err)
	}
	for i := range rows {
		rows[i].SourceRecordID = id
	}
	return nil
}

func defaultStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

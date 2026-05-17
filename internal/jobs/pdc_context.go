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
			rows, fetch, err := p.PDC.FetchPageWithSource(ctx, pdc.DatasetLobbyistEmployment, pdc.Query{
				Where: where, Limit: 50, Order: ":id",
			})
			if err != nil {
				fmt.Fprintf(stderrSink, "  warn: PDC employment fetch for %s: %v\n", canonical, err)
				continue
			}
			if fetch.SourceRecordID == 0 {
				return fmt.Errorf("PDC employment fetch for %s did not record source_record_id", canonical)
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
						"employer_name":     le.EmployerName,
						"employment_year":   le.EmploymentYear,
						"employment_period": le.EmploymentPeriod,
					},
					SourceURL:       le.EmploymentURL,
					MatchConfidence: defaultStr(m.Confidence, "confirmed"),
					SourceRecordID:  fetch.SourceRecordID,
				})
			}
			if len(ctxRows) > 0 {
				if err := p.Store.ReplaceOrgContextForOrganization(ctx, orgID, ctxRows); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func defaultStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

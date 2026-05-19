package jobs

import (
	"context"
	"fmt"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

// PopulateOrganizations seeds canonical organization rows from source-backed
// organization strings already ingested with hearings/testimony. It replaces
// the old hand-maintained reviewed_matches.yml runtime path for v1.
func (p *Pipeline) PopulateOrganizations(ctx context.Context, ids *IDs) error {
	stats, err := p.Store.PopulateOrganizationsFromCSIWithProgress(ctx, func(progress db.PopulateOrganizationsProgress) {
		switch progress.Phase {
		case "listing":
			fmt.Fprintln(stderrSink, "  scanning distinct CSI organization names")
		case "processing":
			if progress.Stats.CandidatesProcessed == 0 {
				fmt.Fprintln(stderrSink, "  processing CSI organization candidates")
				return
			}
			fmt.Fprintf(stderrSink, "  processed=%d organizations=%d mentions=%d linked=%d skipped=%d elapsed=%s\n",
				progress.Stats.CandidatesProcessed,
				progress.Stats.OrganizationsUpserted,
				progress.Stats.MentionsUpserted,
				progress.Stats.TestifiersLinked,
				progress.Stats.Skipped,
				progress.ElapsedTime,
			)
		}
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(stderrSink, "  seeded %d organizations from CSI (verified %d via cross-source) and linked %d testifier rows (skipped %d junk names, %d candidates processed)\n", stats.OrganizationsUpserted, stats.Verified, stats.TestifiersLinked, stats.Skipped, stats.CandidatesProcessed)
	return nil
}

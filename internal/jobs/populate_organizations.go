package jobs

import (
	"context"
	"fmt"
)

// PopulateOrganizations seeds canonical organization rows from source-backed
// organization strings already ingested with hearings/testimony. It replaces
// the old hand-maintained reviewed_matches.yml runtime path for v1.
func (p *Pipeline) PopulateOrganizations(ctx context.Context, ids *IDs) error {
	stats, err := p.Store.PopulateOrganizationsFromCSI(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(stderrSink, "  seeded %d organizations from CSI and linked %d testifier rows (skipped %d junk names)\n", stats.OrganizationsUpserted, stats.TestifiersLinked, stats.Skipped)
	return nil
}

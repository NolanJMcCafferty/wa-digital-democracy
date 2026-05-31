package db

import (
	"context"
	"fmt"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/entitymatch"
)

type DeepgramOrganizationEvidenceStats struct {
	Scanned          int
	MentionsUpserted int
	Candidates       int
	Skipped          int
}

type DeepgramOrganizationEvidenceProgress struct {
	Phase              string
	Stats              DeepgramOrganizationEvidenceStats
	LookupCacheSize    int
	LastNormalizedName string
	ElapsedTime        time.Duration
}

func (s *Store) GenerateDeepgramOrganizationEvidence(ctx context.Context, minConfidence float64, limit int) (DeepgramOrganizationEvidenceStats, error) {
	return s.GenerateDeepgramOrganizationEvidenceWithProgress(ctx, minConfidence, limit, nil)
}

func (s *Store) GenerateDeepgramOrganizationEvidenceWithProgress(ctx context.Context, minConfidence float64, limit int, progress func(DeepgramOrganizationEvidenceProgress)) (DeepgramOrganizationEvidenceStats, error) {
	if minConfidence <= 0 {
		minConfidence = 0.85
	}
	started := time.Now()
	emit := func(phase string, stats DeepgramOrganizationEvidenceStats, cacheSize int, normalized string) {
		if progress == nil {
			return
		}
		progress(DeepgramOrganizationEvidenceProgress{
			Phase:              phase,
			Stats:              stats,
			LookupCacheSize:    cacheSize,
			LastNormalizedName: normalized,
			ElapsedTime:        time.Since(started).Round(time.Second),
		})
	}

	const q = `
SELECT id, text, wa_dd_normalize_entity_name(text), COALESCE(confidence, 0)
  FROM entity_mention
 WHERE upper(entity_type) = 'ORGANIZATION'
   AND text IS NOT NULL
   AND wa_dd_normalize_entity_name(text) IS NOT NULL
   AND confidence IS NOT NULL
   AND confidence >= $1
 ORDER BY confidence DESC, id DESC
 LIMIT CASE WHEN $2 > 0 THEN $2 ELSE 100000 END;`
	emit("querying", DeepgramOrganizationEvidenceStats{}, 0, "")
	rows, err := s.Pool.Query(ctx, q, minConfidence, limit)
	if err != nil {
		return DeepgramOrganizationEvidenceStats{}, fmt.Errorf("list deepgram organization mentions: %w", err)
	}
	defer rows.Close()

	var stats DeepgramOrganizationEvidenceStats
	matchCache := map[string][]organizationNameMatch{}
	lastProgress := time.Now()
	emit("processing", stats, len(matchCache), "")
	for rows.Next() {
		var id int64
		var text, normalized string
		var providerConfidence float64
		if err := rows.Scan(&id, &text, &normalized, &providerConfidence); err != nil {
			return stats, fmt.Errorf("scan deepgram organization mention: %w", err)
		}
		stats.Scanned++
		if entitymatch.FalsePositiveRisk(normalized) || junkOrganizationName(text, normalized) {
			stats.Skipped++
			if stats.Scanned == 1 || stats.Scanned%1000 == 0 || time.Since(lastProgress) >= 10*time.Second {
				emit("processing", stats, len(matchCache), normalized)
				lastProgress = time.Now()
			}
			continue
		}
		matches, ok := matchCache[normalized]
		if !ok {
			var err error
			matches, err = s.organizationMatchesForNormalizedName(ctx, normalized)
			if err != nil {
				return stats, err
			}
			matchCache[normalized] = matches
		}
		mentionOrgID := int64(0)
		if len(matches) == 1 {
			mentionOrgID = matches[0].ID
		}
		if err := s.upsertOrganizationSourceMention(ctx, "deepgram_entity", "entity_mention", id, text, normalized, mentionOrgID, 1, 0, "possible"); err != nil {
			return stats, err
		}
		stats.MentionsUpserted++
		for _, match := range matches {
			candidateID, err := s.UpsertVendorEntityMatchCandidate(ctx, UpsertVendorEntityMatchCandidateParams{
				SourceKind:          "deepgram_organization_mention",
				SourceTable:         "entity_mention",
				SourcePK:            id,
				SourceDatasetID:     "deepgram",
				SourceRowID:         fmt.Sprintf("%d", id),
				SourceName:          text,
				NormalizedName:      normalized,
				OrganizationID:      match.ID,
				CandidateConfidence: "possible",
				Evidence: []string{
					"entity_type:ORGANIZATION",
					fmt.Sprintf("deepgram_confidence:%.3f", providerConfidence),
					"review_required:transcript_mention",
				},
			})
			if err != nil {
				return stats, err
			}
			if candidateID > 0 {
				stats.Candidates++
			}
		}
		if stats.Scanned == 1 || stats.Scanned%1000 == 0 || time.Since(lastProgress) >= 10*time.Second {
			emit("processing", stats, len(matchCache), normalized)
			lastProgress = time.Now()
		}
	}
	if err := rows.Err(); err != nil {
		return stats, err
	}
	emit("complete", stats, len(matchCache), "")
	return stats, nil
}

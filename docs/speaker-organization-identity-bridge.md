# Speaker-Organization Identity Bridge

Last updated: 2026-06-04.

This document defines the canonical attribution path from diarized speech segments to organizations and the confidence rules that govern public display of organization-attributed quotes.

## Canonical Attribution Path

The system uses this canonical path for attributing speech segments to organizations:

```
diarized_speech_segment → speaker_assignment → testifier → organization
```

### 1. Speech Segment → Speaker Assignment

- **Table**: `diarized_speech_segment` → `speaker_assignment`
- **Join**: `diarization_job_id` + `speaker_cluster_id`
- **Filter**: `speaker_assignment.review_status = 'accepted'`

Only accepted speaker assignments are included in public attribution. Unreviewed or rejected assignments remain anonymous.

### 2. Speaker Assignment → Testifier  

- **Table**: `speaker_assignment` → `testifier`
- **Join**: `speaker_assignment.testifier_id = testifier.id`
- **Requirements**: 
  - `speaker_assignment.speaker_kind = 'testifier'`
  - `testifier_id IS NOT NULL`

The `testifier_id` foreign key was added to strengthen the identity bridge. When a speaker is reviewed and accepted as a testifier, this field links directly to the CSI testifier record.

### 3. Testifier → Organization

- **Table**: `testifier` → `organization`  
- **Join**: `testifier.normalized_org_id = organization.id`
- **Requirements**:
  - `normalized_org_id IS NOT NULL`
  - Organization must be confirmed (`match_confidence = 'confirmed'`)

Only testifiers with confirmed organization links contribute to public attribution.

## Confidence Rules

### Speaker Assignment Confidence

Speaker assignments have these review statuses:
- `pending`: Not yet reviewed (not public)
- `accepted`: Reviewed and approved (public)
- `rejected`: Reviewed and rejected (not public) 
- `needs_more_evidence`: Requires additional review (not public)
- `superseded`: Replaced by newer assignment (not public)

**Public Rule**: Only `accepted` assignments appear in public APIs.

### Organization Match Confidence

Organizations have these match confidence levels:
- `confirmed`: High confidence, suitable for public attribution
- `probable`: Medium confidence, requires manual review
- `possible`: Low confidence, not suitable for public attribution  
- `unmatched`: No match found

**Public Rule**: Only `confirmed` organizations appear in organization-attributed speech.

## API Behavior

### Public Speech Attribution

The following APIs only return reviewed, confirmed attribution:

1. **`/hearings/{id}`** - Shows diarized transcript with speaker labels
   - Filters: `speaker_assignment.review_status = 'accepted'`
   - Uses `PublicSpeakerLabel()` helper that returns empty string for unreviewed assignments

2. **`/organizations/{slug}/speech`** - Shows speech segments attributed to organization
   - Filters: `speaker_assignment.review_status = 'accepted'` AND `organization.match_confidence = 'confirmed'`
   - Requires complete attribution chain: segment → assignment → testifier → org

3. **`/search/transcripts`** - Full-text search across transcript segments
   - Only shows speaker labels for accepted assignments
   - Organization attribution available through joined data

### Admin/Review APIs

Admin endpoints show all assignments regardless of review status to support the review workflow:

1. **`/admin/review/speakers`** - Speaker review interface
2. **`/admin/review/speakers/clusters/{id}`** - Individual cluster review

## Storage Helpers

### Organization Attribution Queries

Two new storage helpers support organization attribution:

1. **`ListOrganizationAttributedSegments(organizationID, limit, offset)`**
   - Returns speech segments for a specific organization
   - Only includes reviewed assignments and confirmed organizations
   - Used by `/organizations/{slug}/speech` endpoint

2. **`ListOrganizationAttributedSegmentsByEvent(tvwEventID)`**
   - Returns organization-attributed segments for a hearing
   - Useful for hearing pages showing which orgs spoke
   - Includes left joins for optional testifier/org data

### Evidence and Review Task Generation

The `extract-speaker-evidence` command generates review tasks with priority scoring:

```go
priority := 100
if kind == "legislator" {
    priority += 50  // Legislators get higher priority
}
if candidateID != 0 {
    priority += 25  // Resolved testifiers get higher priority
}
```

Testifier candidates are resolved by matching against the CSI testifier list for the same hearing.

## Review Workflow

### Accepting Speaker Assignments

When a review task is accepted:

1. **Update task status** to `accepted`
2. **Create/update speaker assignment** with review details
3. **Link testifier** if `candidate_kind = 'testifier'` and `candidate_id` exists
4. **Supersede competing tasks** for the same cluster

### Manual Assignments

Reviewers can manually assign speaker labels when automatic extraction fails or is insufficient. Manual assignments:

1. **Allow custom labels** not tied to automatic evidence
2. **Support any speaker kind** (person, testifier, legislator, unknown)
3. **Supersede existing tasks** for the cluster

## Safety Principles

1. **Reviewed First**: No unreviewed speaker identity appears as public fact
2. **Conservative Attribution**: Only complete attribution chains (segment → assignment → testifier → org) support organization quotes
3. **Provenance Preserved**: All assignments link back to source evidence and reviewer
4. **Fallback to Anonymous**: Incomplete chains leave speakers anonymous rather than guessing
5. **Confidence Transparency**: Review status and confidence levels are preserved in API responses

## Example Query

Here's the core query used by `ListOrganizationAttributedSegments`:

```sql
SELECT dss.id, dss.tvw_event_id, dss.start_ms, dss.end_ms, dss.text,
       dss.cluster_label, sa.speaker_label, sa.speaker_kind::text,
       t.id AS testifier_id, t.raw_name AS testifier_name,
       o.id AS organization_id, o.canonical_name AS organization_name,
       sa.review_status::text, sa.updated_at
  FROM diarized_speech_segment dss
  JOIN speaker_assignment sa ON sa.diarization_job_id = dss.diarization_job_id
                             AND sa.speaker_cluster_id = dss.speaker_cluster_id
  JOIN testifier t ON t.id = sa.testifier_id
  JOIN organization o ON o.id = t.normalized_org_id
 WHERE sa.review_status = 'accepted'
   AND o.id = $1
 ORDER BY dss.tvw_event_id, dss.start_ms;
```

This query ensures only reviewed speaker assignments with confirmed organization links contribute to public organization attribution.
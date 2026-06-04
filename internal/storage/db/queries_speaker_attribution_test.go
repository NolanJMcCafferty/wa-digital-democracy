package db_test

import (
	"context"
	"testing"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

func TestOrganizationAttributedSegments(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Setup test data
	orgID, testifierID, jobID, clusterID, segmentID := setupAttributionTestData(t, store, ctx)

	// Create unaccepted speaker assignment (should not appear in results)
	unacceptedTaskID, err := store.UpsertSpeakerReviewTask(ctx, db.SpeakerReviewTaskParams{
		DiarizationJobID: jobID,
		SpeakerClusterID: clusterID,
		Priority:         50,
		CandidateKind:    "testifier",
		CandidateID:      testifierID,
		CandidateLabel:   "Jane Doe",
		Confidence:       0.85,
		EvidenceIDs:      []int64{},
	})
	if err != nil {
		t.Fatalf("create unaccepted review task: %v", err)
	}

	// Create accepted speaker assignment with testifier link
	acceptedTaskID, err := store.UpsertSpeakerReviewTask(ctx, db.SpeakerReviewTaskParams{
		DiarizationJobID: jobID,
		SpeakerClusterID: clusterID,
		Priority:         100,
		CandidateKind:    "testifier",
		CandidateID:      testifierID,
		CandidateLabel:   "Jane Doe",
		Confidence:       0.90,
		EvidenceIDs:      []int64{},
	})
	if err != nil {
		t.Fatalf("create accepted review task: %v", err)
	}

	// Accept the second task
	err = store.AcceptSpeakerReviewTask(ctx, acceptedTaskID, "test-reviewer", "looks good")
	if err != nil {
		t.Fatalf("accept speaker review task: %v", err)
	}

	t.Run("ListOrganizationAttributedSegments", func(t *testing.T) {
		// Only accepted assignments should appear
		segments, err := store.ListOrganizationAttributedSegments(ctx, orgID, 10, 0)
		if err != nil {
			t.Fatalf("list organization attributed segments: %v", err)
		}

		if len(segments) != 1 {
			t.Fatalf("expected 1 segment, got %d", len(segments))
		}

		seg := segments[0]
		if seg.SegmentID != segmentID {
			t.Errorf("expected segment ID %d, got %d", segmentID, seg.SegmentID)
		}
		if seg.TVWEventID != "event123" {
			t.Errorf("expected event ID 'event123', got %q", seg.TVWEventID)
		}
		if seg.SpeakerLabel != "Jane Doe" {
			t.Errorf("expected speaker label 'Jane Doe', got %q", seg.SpeakerLabel)
		}
		if seg.SpeakerKind != "testifier" {
			t.Errorf("expected speaker kind 'testifier', got %q", seg.SpeakerKind)
		}
		if seg.TestifierID != testifierID {
			t.Errorf("expected testifier ID %d, got %d", testifierID, seg.TestifierID)
		}
		if seg.TestifierName != "Jane Doe" {
			t.Errorf("expected testifier name 'Jane Doe', got %q", seg.TestifierName)
		}
		if seg.OrganizationID != orgID {
			t.Errorf("expected organization ID %d, got %d", orgID, seg.OrganizationID)
		}
		if seg.OrganizationName != "Test Organization" {
			t.Errorf("expected organization name 'Test Organization', got %q", seg.OrganizationName)
		}
		if seg.ReviewStatus != "accepted" {
			t.Errorf("expected review status 'accepted', got %q", seg.ReviewStatus)
		}
	})

	t.Run("ListOrganizationAttributedSegmentsByEvent", func(t *testing.T) {
		segments, err := store.ListOrganizationAttributedSegmentsByEvent(ctx, "event123")
		if err != nil {
			t.Fatalf("list organization attributed segments by event: %v", err)
		}

		if len(segments) != 1 {
			t.Fatalf("expected 1 segment, got %d", len(segments))
		}

		seg := segments[0]
		if seg.OrganizationName != "Test Organization" {
			t.Errorf("expected organization name 'Test Organization', got %q", seg.OrganizationName)
		}
	})

	t.Run("NoUnreviewedSegments", func(t *testing.T) {
		// Create a new org and testifier without accepted assignment
		orgID2 := insertTestOrganization(t, store, ctx, "Unreviewed Org")
		testifierID2 := insertTestTestifier(t, store, ctx, orgID2, "John Smith")

		// Create unaccepted task
		_, err := store.UpsertSpeakerReviewTask(ctx, db.SpeakerReviewTaskParams{
			DiarizationJobID: jobID,
			SpeakerClusterID: clusterID,
			Priority:         80,
			CandidateKind:    "testifier",
			CandidateID:      testifierID2,
			CandidateLabel:   "John Smith",
			Confidence:       0.75,
			EvidenceIDs:      []int64{},
		})
		if err != nil {
			t.Fatalf("create unreviewed task: %v", err)
		}

		// Should not appear in attributed segments
		segments, err := store.ListOrganizationAttributedSegments(ctx, orgID2, 10, 0)
		if err != nil {
			t.Fatalf("list unreviewed segments: %v", err)
		}

		if len(segments) != 0 {
			t.Errorf("expected 0 segments for unreviewed org, got %d", len(segments))
		}
	})

	// Cleanup
	_ = unacceptedTaskID // Used for data setup
}

func TestSpeakerAssignmentTestifierLink(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Setup test data
	orgID, testifierID, jobID, clusterID, _ := setupAttributionTestData(t, store, ctx)

	// Create task with testifier candidate
	taskID, err := store.UpsertSpeakerReviewTask(ctx, db.SpeakerReviewTaskParams{
		DiarizationJobID: jobID,
		SpeakerClusterID: clusterID,
		Priority:         100,
		CandidateKind:    "testifier",
		CandidateID:      testifierID,
		CandidateLabel:   "Jane Doe",
		Confidence:       0.90,
		EvidenceIDs:      []int64{},
	})
	if err != nil {
		t.Fatalf("create review task: %v", err)
	}

	// Accept the task
	err = store.AcceptSpeakerReviewTask(ctx, taskID, "test-reviewer", "confirmed testifier")
	if err != nil {
		t.Fatalf("accept task: %v", err)
	}

	// Verify testifier_id was linked correctly
	segments, err := store.ListOrganizationAttributedSegments(ctx, orgID, 1, 0)
	if err != nil {
		t.Fatalf("list segments: %v", err)
	}

	if len(segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segments))
	}

	if segments[0].TestifierID != testifierID {
		t.Errorf("expected testifier_id %d, got %d", testifierID, segments[0].TestifierID)
	}
}

// setupAttributionTestData creates the full data chain needed for attribution testing.
// Returns (orgID, testifierID, jobID, clusterID, segmentID).
func setupAttributionTestData(t *testing.T, store *db.Store, ctx context.Context) (int64, int64, int64, int64, int64) {
	t.Helper()

	// Create source record for foreign key constraints
	insertTestSourceRecord(t, store, ctx)

	// Create organization
	orgID := insertTestOrganization(t, store, ctx, "Test Organization")

	// Create testifier linked to organization
	testifierID := insertTestTestifier(t, store, ctx, orgID, "Jane Doe")

	// Create diarization job and cluster
	jobID, clusterID := insertTestDiarizationJob(t, store, ctx)

	// Create speech segment
	segmentID := insertTestSpeechSegment(t, store, ctx, jobID, clusterID)

	return orgID, testifierID, jobID, clusterID, segmentID
}

func insertTestSourceRecord(t *testing.T, store *db.Store, ctx context.Context) {
	t.Helper()
	const q = `
INSERT INTO source_record (id, source_system, source_endpoint, source_url, fetched_at, content_hash, raw_path)
VALUES (1, 'test', 'test-endpoint', 'http://test.example', NOW(), 'test-hash', 'test.json')
ON CONFLICT (id) DO NOTHING`
	_, err := store.Pool.Exec(ctx, q)
	if err != nil {
		t.Fatalf("insert test source record: %v", err)
	}
}

func insertTestOrganization(t *testing.T, store *db.Store, ctx context.Context, name string) int64 {
	t.Helper()
	const q = `
INSERT INTO organization (canonical_name, match_confidence, created_at, updated_at)
VALUES ($1, 'confirmed', NOW(), NOW())
RETURNING id`
	var id int64
	err := store.Pool.QueryRow(ctx, q, name).Scan(&id)
	if err != nil {
		t.Fatalf("insert test organization: %v", err)
	}
	return id
}

func insertTestTestifier(t *testing.T, store *db.Store, ctx context.Context, orgID int64, name string) int64 {
	t.Helper()

	// First need agenda item
	agendaItemID := insertTestAgendaItem(t, store, ctx)

	const q = `
INSERT INTO testifier (agenda_item_id, raw_name, normalized_org_id, position, testified, source_record_id, created_at)
VALUES ($1, $2, $3, 'Pro', true, 1, NOW())
RETURNING id`
	var id int64
	err := store.Pool.QueryRow(ctx, q, agendaItemID, name, orgID).Scan(&id)
	if err != nil {
		t.Fatalf("insert test testifier: %v", err)
	}
	return id
}

func insertTestAgendaItem(t *testing.T, store *db.Store, ctx context.Context) int64 {
	t.Helper()

	// First need hearing
	hearingID := insertTestHearing(t, store, ctx)

	const q = `
INSERT INTO agenda_item (hearing_id, label, source_record_id)
VALUES ($1, 'HB 1234 Test Bill', 1)
RETURNING id`
	var id int64
	err := store.Pool.QueryRow(ctx, q, hearingID).Scan(&id)
	if err != nil {
		t.Fatalf("insert test agenda item: %v", err)
	}
	return id
}

func insertTestHearing(t *testing.T, store *db.Store, ctx context.Context) int64 {
	t.Helper()
	const q = `
INSERT INTO hearing (committee_name, chamber, meeting_datetime, tvw_event_id, source_record_id, created_at, updated_at)
VALUES ('Test Committee', 'House', NOW(), 'event123', 1, NOW(), NOW())
RETURNING id`
	var id int64
	err := store.Pool.QueryRow(ctx, q).Scan(&id)
	if err != nil {
		t.Fatalf("insert test hearing: %v", err)
	}
	return id
}

func insertTestDiarizationJob(t *testing.T, store *db.Store, ctx context.Context) (int64, int64) {
	t.Helper()

	// Insert diarization job
	const jobQ = `
INSERT INTO diarization_job (tvw_event_id, provider, model, status, submitted_at, finished_at)
VALUES ('event123', 'test', 'test-model', 'succeeded', NOW(), NOW())
RETURNING id`
	var jobID int64
	err := store.Pool.QueryRow(ctx, jobQ).Scan(&jobID)
	if err != nil {
		t.Fatalf("insert test diarization job: %v", err)
	}

	// Insert speaker cluster
	const clusterQ = `
INSERT INTO speaker_cluster (diarization_job_id, tvw_event_id, cluster_label, total_speech_ms, turn_count)
VALUES ($1, 'event123', 'SPEAKER_00', 30000, 5)
RETURNING id`
	var clusterID int64
	err = store.Pool.QueryRow(ctx, clusterQ, jobID).Scan(&clusterID)
	if err != nil {
		t.Fatalf("insert test speaker cluster: %v", err)
	}

	return jobID, clusterID
}

func insertTestSpeechSegment(t *testing.T, store *db.Store, ctx context.Context, jobID, clusterID int64) int64 {
	t.Helper()
	const q = `
INSERT INTO diarized_speech_segment (diarization_job_id, speaker_cluster_id, tvw_event_id,
                                     cluster_label, start_ms, end_ms, text)
VALUES ($1, $2, 'event123', 'SPEAKER_00', 1000, 5000, 'This is a test speech segment.')
RETURNING id`
	var id int64
	err := store.Pool.QueryRow(ctx, q, jobID, clusterID).Scan(&id)
	if err != nil {
		t.Fatalf("insert test speech segment: %v", err)
	}
	return id
}

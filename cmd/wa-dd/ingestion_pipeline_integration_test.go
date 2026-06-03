//go:build integration
// +build integration

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/common"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/diarization"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/jobs"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/csi"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/lws"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/tvw"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/objectstore"
)

// TestLiveIngestionPipelineSingleBillHearingAndDiarization is intentionally
// live except for Deepgram. It pulls the current LWS legislator roster, ingests
// one real bill, discovers one real hearing through CSI + TVW, then runs the
// hearing-level pipeline with a mocked Deepgram diarization result.
func TestLiveIngestionPipelineSingleBillHearingAndDiarization(t *testing.T) {
	dsn := os.Getenv("WADD_TEST_DSN")
	if strings.TrimSpace(dsn) == "" {
		t.Skip("WADD_TEST_DSN is required because this live integration test writes real-source rows")
	}
	embedderKey := os.Getenv("INVINTUS_EMBEDDER_KEY")
	if strings.TrimSpace(embedderKey) == "" {
		t.Skip("INVINTUS_EMBEDDER_KEY is required for the real TVW/Invintus ingest step")
	}

	ctx := context.Background()
	store := openLiveIngestionTestStore(t, dsn)
	httpClient := liveRawSinkHTTPClient(t, store)
	lwsClient := lws.New(httpClient)
	csiClient := csi.New(httpClient)
	tvwClient := tvw.New(httpClient, embedderKey)

	const biennium = "2025-26"
	legislators := ingestLiveLegislators(t, ctx, store, lwsClient, biennium)
	assertLiveLegislatorsIngested(t, ctx, store, legislators)

	target := findLiveIngestionTarget(t, ctx, store, lwsClient, csiClient, tvwClient, biennium)
	t.Cleanup(func() {
		cleanupLiveTargetRows(t, store, target)
	})

	mockDeepgram := &mockDeepgramClient{
		t:       t,
		eventID: target.Demo.TVW.EventID,
		result: &diarization.Result{
			Provider: "deepgram",
			Model:    "mock-nova",
			EventID:  target.Demo.TVW.EventID,
			Raw:      []byte(fmt.Sprintf(`{"mock":"deepgram","event_id":%q}`, target.Demo.TVW.EventID)),
			Segments: []diarization.Segment{
				{StartMS: 0, EndMS: 1800, SpeakerCluster: "SPEAKER_00", Confidence: float64Ptr(0.96), Text: "The committee will come to order."},
				{StartMS: 2000, EndMS: 6200, SpeakerCluster: "SPEAKER_01", Confidence: float64Ptr(0.94), Text: fmt.Sprintf("We will now consider %s. For the record, my name is Jane Doe with Civic Housing Alliance.", target.Demo.Bill.ID())},
				{StartMS: 6400, EndMS: 9000, SpeakerCluster: "SPEAKER_00", Confidence: float64Ptr(0.91), Text: "Thank you for your testimony."},
			},
			Entities: []diarization.EntityMention{
				{
					Text:           "Jane Doe",
					NormalizedText: "jane doe",
					Type:           "PERSON",
					StartMS:        intPtr(2000),
					EndMS:          intPtr(2800),
					StartWord:      intPtr(5),
					EndWord:        intPtr(7),
					Confidence:     float64Ptr(0.88),
				},
			},
		},
	}
	deps := &buildDeps{
		store:     store,
		csiClient: csiClient,
		tvwClient: tvwClient,
	}
	hearing := db.DiscoveredHearingRow{
		HearingID:       target.Hearing.HearingID,
		TVWEventID:      target.Demo.TVW.EventID,
		AgendaItemCount: 1,
	}
	if err := ingestHearing(ctx, deps, hearing, mockDeepgram, "deepgram", "mock-nova", t.TempDir(), true, 1, make(chan struct{}, 1), func(string) {}); err != nil {
		t.Fatalf("ingestHearing(%d / %s): %v", target.Hearing.HearingID, target.Demo.TVW.EventID, err)
	}
	assertLiveBillIngested(t, ctx, store, target.Hearing.BillID)
	assertLiveHearingIngested(t, ctx, store, target)
	assertLiveDiarizationIngested(t, ctx, store, target.Demo.TVW.EventID)
}

type liveIngestionTarget struct {
	Demo    *common.BillAgendaTarget
	Hearing db.HearingForDiscovery
}

func findLiveIngestionTarget(
	t *testing.T,
	ctx context.Context,
	store *db.Store,
	lwsClient *lws.Client,
	csiClient *csi.Client,
	tvwClient *tvw.Client,
	biennium string,
) liveIngestionTarget {
	t.Helper()
	candidates := []common.BillKey{
		{Biennium: biennium, Prefix: "HB", Number: 2747},
		{Biennium: biennium, Prefix: "HB", Number: 1501},
		{Biennium: biennium, Prefix: "HB", Number: 1234},
		{Biennium: biennium, Prefix: "SB", Number: 6054},
		{Biennium: biennium, Prefix: "HB", Number: 1859},
	}
	var misses []string
	for _, bill := range candidates {
		target, ok, why := tryLiveIngestionTarget(t, ctx, store, lwsClient, csiClient, tvwClient, bill)
		if ok {
			return target
		}
		cleanupLiveBillRows(t, store, bill)
		misses = append(misses, bill.ID()+": "+why)
	}
	t.Fatalf("no live ingest target found among candidates:\n%s", strings.Join(misses, "\n"))
	return liveIngestionTarget{}
}

func tryLiveIngestionTarget(
	t *testing.T,
	ctx context.Context,
	store *db.Store,
	lwsClient *lws.Client,
	csiClient *csi.Client,
	tvwClient *tvw.Client,
	bill common.BillKey,
) (liveIngestionTarget, bool, string) {
	t.Helper()
	metadataDemo := &common.BillAgendaTarget{Bill: bill}
	metadataPipeline := &jobs.Pipeline{Store: store, LWS: lwsClient, Demo: metadataDemo}
	if err := metadataPipeline.RunMetadataOnly(ctx, func(string) {}, jobs.NewIDs()); err != nil {
		return liveIngestionTarget{}, false, err.Error()
	}

	hearings, err := hearingsForBill(ctx, store, bill)
	if err != nil {
		return liveIngestionTarget{}, false, err.Error()
	}
	if len(hearings) == 0 {
		return liveIngestionTarget{}, false, "LWS returned no hearings"
	}

	disc := jobs.NewDiscoverer(jobs.DiscoveryDeps{Store: store, CSI: csiClient, TVW: tvwClient})
	var misses []string
	for _, h := range hearings {
		res, err := disc.DiscoverOne(ctx, h)
		if err != nil {
			misses = append(misses, fmt.Sprintf("hearing %d: %v", h.HearingID, err))
			continue
		}
		if res.CSIAgendaItemID == "" {
			misses = append(misses, fmt.Sprintf("hearing %d: missing CSI agenda item", h.HearingID))
			continue
		}
		if res.TVWEventID == "" {
			misses = append(misses, fmt.Sprintf("hearing %d: missing TVW event", h.HearingID))
			continue
		}
		if err := disc.Commit(ctx, h, res); err != nil {
			misses = append(misses, fmt.Sprintf("hearing %d commit: %v", h.HearingID, err))
			continue
		}
		return liveIngestionTarget{
			Hearing: h,
			Demo: &common.BillAgendaTarget{
				Bill: bill,
				Committee: common.CommitteeRef{
					Chamber: h.Chamber,
					Acronym: h.CommitteeAcronym,
					CSIID:   res.CSICommitteeID,
				},
				AgendaItem: common.AgendaItemRef{
					CSIMeetingFamilyID:    res.CSIMeetingFamilyID,
					CSIAgendaItemFamilyID: res.CSIAgendaItemFamilyID,
					CSIAgendaItemID:       res.CSIAgendaItemID,
					Label:                 res.AgendaItemLabel,
				},
				TVW: common.TVWRef{EventID: res.TVWEventID},
			},
		}, true, ""
	}
	return liveIngestionTarget{}, false, strings.Join(misses, "; ")
}

func ingestLiveLegislators(t *testing.T, ctx context.Context, store *db.Store, client *lws.Client, biennium string) map[string]lws.Member {
	t.Helper()
	senate, err := client.GetSenateSponsors(ctx, biennium)
	if err != nil {
		t.Fatalf("GetSenateSponsors(%s): %v", biennium, err)
	}
	house, err := client.GetHouseSponsors(ctx, biennium)
	if err != nil {
		t.Fatalf("GetHouseSponsors(%s): %v", biennium, err)
	}
	expected := map[string]lws.Member{}
	for _, m := range append(senate, house...) {
		if strings.TrimSpace(m.ID) == "" {
			t.Fatalf("LWS returned a legislator without an Id: %#v", m)
		}
		if strings.TrimSpace(m.LongName) == "" {
			t.Fatalf("LWS returned legislator %s without LongName: %#v", m.ID, m)
		}
		if _, err := store.UpsertLegislator(ctx, db.UpsertLegislatorParams{
			LWSSponsorID: m.ID,
			Name:         m.LongName,
			Chamber:      m.Agency,
			District:     m.District,
			Party:        m.Party,
			FirstName:    m.FirstName,
			LastName:     m.LastName,
			Email:        m.Email,
			Phone:        m.Phone,
			Acronym:      m.Acronym,
		}); err != nil {
			t.Fatalf("UpsertLegislator(%s): %v", m.ID, err)
		}
		expected[m.ID] = m
	}
	return expected
}

func assertLiveLegislatorsIngested(t *testing.T, ctx context.Context, store *db.Store, expected map[string]lws.Member) {
	t.Helper()
	for id, want := range expected {
		var got lws.Member
		if err := store.Pool.QueryRow(ctx, `
SELECT lws_sponsor_id, COALESCE(name, ''), COALESCE(chamber, ''),
       COALESCE(acronym, ''), COALESCE(party, ''), COALESCE(district, ''),
       COALESCE(phone, ''), COALESCE(email, ''), COALESCE(first_name, ''),
       COALESCE(last_name, '')
  FROM legislator
 WHERE lws_sponsor_id = $1`, id).Scan(
			&got.ID, &got.LongName, &got.Agency, &got.Acronym, &got.Party,
			&got.District, &got.Phone, &got.Email, &got.FirstName, &got.LastName,
		); err != nil {
			t.Fatalf("select legislator %s: %v", id, err)
		}
		if got.LongName != want.LongName ||
			got.Agency != want.Agency ||
			got.Acronym != want.Acronym ||
			got.Party != want.Party ||
			got.District != want.District ||
			got.Phone != want.Phone ||
			got.Email != want.Email ||
			got.FirstName != want.FirstName ||
			got.LastName != want.LastName {
			t.Fatalf("legislator %s mismatch:\n got  %#v\n want %#v", id, got, want)
		}
	}
}

func assertLiveBillIngested(t *testing.T, ctx context.Context, store *db.Store, billID int64) {
	t.Helper()
	var title, status, officialURL string
	if err := store.Pool.QueryRow(ctx, `
SELECT COALESCE(title, ''), COALESCE(current_status, ''), COALESCE(official_url, '')
  FROM bill WHERE id = $1`, billID).Scan(&title, &status, &officialURL); err != nil {
		t.Fatalf("select bill: %v", err)
	}
	if title == "" || status == "" || officialURL == "" {
		t.Fatalf("bill fields missing: title=%q status=%q officialURL=%q", title, status, officialURL)
	}

	var sponsorCount, statusCount int
	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM bill_sponsor WHERE bill_id = $1`, billID).Scan(&sponsorCount); err != nil {
		t.Fatalf("count sponsors: %v", err)
	}
	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM bill_status_change WHERE bill_id = $1`, billID).Scan(&statusCount); err != nil {
		t.Fatalf("count status changes: %v", err)
	}
	if sponsorCount == 0 || statusCount == 0 {
		t.Fatalf("bill sponsor/status rows missing: sponsors=%d statuses=%d", sponsorCount, statusCount)
	}
}

func assertLiveHearingIngested(t *testing.T, ctx context.Context, store *db.Store, target liveIngestionTarget) {
	t.Helper()
	var hearingEventID, agendaLabel string
	var agendaItemID int64
	if err := store.Pool.QueryRow(ctx, `
SELECT a.id, COALESCE(h.tvw_event_id, ''), COALESCE(a.label, '')
  FROM hearing h
  JOIN agenda_item a ON a.hearing_id = h.id
 WHERE a.csi_agenda_item_id = $1`, target.Demo.AgendaItem.CSIAgendaItemID).Scan(&agendaItemID, &hearingEventID, &agendaLabel); err != nil {
		t.Fatalf("select hearing/agenda: %v", err)
	}
	if hearingEventID != target.Demo.TVW.EventID || agendaLabel == "" {
		t.Fatalf("hearing agenda mismatch: event=%q label=%q", hearingEventID, agendaLabel)
	}

	var testifierCount, orgLinkCount int
	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM testifier WHERE agenda_item_id = $1`, agendaItemID).Scan(&testifierCount); err != nil {
		t.Fatalf("count testifiers: %v", err)
	}
	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM testifier WHERE agenda_item_id = $1 AND normalized_org_id IS NOT NULL`, agendaItemID).Scan(&orgLinkCount); err != nil {
		t.Fatalf("count org links: %v", err)
	}
	if testifierCount == 0 || orgLinkCount == 0 {
		t.Fatalf("hearing ingest incomplete: testifiers=%d orgLinks=%d", testifierCount, orgLinkCount)
	}

	windows, err := store.ListAgendaItemWindowsByAgendaItem(ctx, target.Demo.AgendaItem.CSIAgendaItemID)
	if err != nil {
		t.Fatalf("ListAgendaItemWindowsByAgendaItem: %v", err)
	}
	if len(windows) == 0 {
		t.Fatalf("no transcript windows for agenda item %s", target.Demo.AgendaItem.CSIAgendaItemID)
	}
}

func assertLiveDiarizationIngested(t *testing.T, ctx context.Context, store *db.Store, tvwEventID string) {
	t.Helper()
	var jobStatus string
	var clusterCount, segmentCount, entityCount int
	if err := store.Pool.QueryRow(ctx, `
SELECT j.status::text,
       (SELECT count(*) FROM speaker_cluster WHERE tvw_event_id = $1),
       (SELECT count(*) FROM diarized_speech_segment WHERE tvw_event_id = $1),
       (SELECT count(*) FROM entity_mention WHERE tvw_event_id = $1)
  FROM diarization_job j
 WHERE j.tvw_event_id = $1
 ORDER BY j.id DESC
 LIMIT 1`, tvwEventID).Scan(&jobStatus, &clusterCount, &segmentCount, &entityCount); err != nil {
		t.Fatalf("select diarization summary: %v", err)
	}
	if jobStatus != "succeeded" || clusterCount != 2 || segmentCount != 3 || entityCount != 1 {
		t.Fatalf("diarization = status %q clusters %d segments %d entities %d", jobStatus, clusterCount, segmentCount, entityCount)
	}

	segs, err := store.ListDiarizedSegmentsByTVWEvent(ctx, tvwEventID)
	if err != nil {
		t.Fatalf("ListDiarizedSegmentsByTVWEvent: %v", err)
	}
	if len(segs) != 3 || segs[1].ClusterLabel != "SPEAKER_01" || !strings.Contains(segs[1].Text, "Jane Doe") {
		t.Fatalf("diarized segments = %#v", segs)
	}
}

func hearingsForBill(ctx context.Context, store *db.Store, bill common.BillKey) ([]db.HearingForDiscovery, error) {
	const q = `
SELECT h.id, b.id, b.prefix, b.number,
       h.committee_name, COALESCE(h.committee_acronym, ''), h.chamber,
       h.meeting_datetime, h.source_record_id
  FROM hearing h
  JOIN bill b ON b.id = h.bill_id
 WHERE b.biennium = $1 AND b.prefix = $2 AND b.number = $3
 ORDER BY h.meeting_datetime DESC`
	rows, err := store.Pool.Query(ctx, q, bill.Biennium, bill.Prefix, bill.Number)
	if err != nil {
		return nil, fmt.Errorf("list hearings for %s: %w", bill.ID(), err)
	}
	defer rows.Close()
	var out []db.HearingForDiscovery
	for rows.Next() {
		var h db.HearingForDiscovery
		if err := rows.Scan(
			&h.HearingID, &h.BillID, &h.BillPrefix, &h.BillNumber,
			&h.CommitteeName, &h.CommitteeAcronym, &h.Chamber,
			&h.MeetingDateTime, &h.SourceRecordID,
		); err != nil {
			return nil, fmt.Errorf("scan hearing for %s: %w", bill.ID(), err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func cleanupLiveTargetRows(t *testing.T, store *db.Store, target liveIngestionTarget) {
	t.Helper()
	ctx := context.Background()
	bill := target.Demo.Bill
	agendaID := target.Demo.AgendaItem.CSIAgendaItemID
	eventID := target.Demo.TVW.EventID
	stmts := []struct {
		sql  string
		args []any
	}{
		{`DELETE FROM entity_mention WHERE tvw_event_id = $1`, []any{eventID}},
		{`DELETE FROM diarized_speech_segment WHERE tvw_event_id = $1`, []any{eventID}},
		{`DELETE FROM speaker_cluster WHERE tvw_event_id = $1`, []any{eventID}},
		{`DELETE FROM diarization_job WHERE tvw_event_id = $1`, []any{eventID}},
		{`DELETE FROM tvw_audio_asset WHERE tvw_event_id = $1`, []any{eventID}},
		{`DELETE FROM tvw_media_asset WHERE tvw_event_id = $1`, []any{eventID}},
		{`DELETE FROM agenda_item_window WHERE agenda_item_id IN (SELECT id FROM agenda_item WHERE csi_agenda_item_id = $1)`, []any{agendaID}},
		{`DELETE FROM testifier WHERE agenda_item_id IN (SELECT id FROM agenda_item WHERE csi_agenda_item_id = $1)`, []any{agendaID}},
		{`DELETE FROM agenda_item WHERE csi_agenda_item_id = $1`, []any{agendaID}},
		{`UPDATE hearing SET tvw_event_id = NULL, tvw_url = NULL, committee_schedule_agenda_id = NULL WHERE id = $1`, []any{target.Hearing.HearingID}},
		{`DELETE FROM bill_status_change WHERE bill_id IN (SELECT id FROM bill WHERE biennium = $1 AND prefix = $2 AND number = $3)`, []any{bill.Biennium, bill.Prefix, bill.Number}},
		{`DELETE FROM bill_sponsor WHERE bill_id IN (SELECT id FROM bill WHERE biennium = $1 AND prefix = $2 AND number = $3)`, []any{bill.Biennium, bill.Prefix, bill.Number}},
		{`DELETE FROM hearing WHERE bill_id IN (SELECT id FROM bill WHERE biennium = $1 AND prefix = $2 AND number = $3)`, []any{bill.Biennium, bill.Prefix, bill.Number}},
		{`DELETE FROM bill WHERE biennium = $1 AND prefix = $2 AND number = $3`, []any{bill.Biennium, bill.Prefix, bill.Number}},
		{`DELETE FROM tvw_event WHERE tvw_event_id = $1`, []any{eventID}},
		{`DELETE FROM ingestion_run WHERE args->>'bill' = $1 OR args->>'hearing_id' = $2`, []any{bill.ID(), fmt.Sprint(target.Hearing.HearingID)}},
	}
	for _, stmt := range stmts {
		if _, err := store.Pool.Exec(ctx, stmt.sql, stmt.args...); err != nil {
			t.Errorf("cleanup %q: %v", stmt.sql, err)
		}
	}
}

func cleanupLiveBillRows(t *testing.T, store *db.Store, bill common.BillKey) {
	t.Helper()
	ctx := context.Background()
	stmts := []struct {
		sql  string
		args []any
	}{
		{`DELETE FROM agenda_item_window WHERE agenda_item_id IN (
			SELECT a.id FROM agenda_item a
			JOIN bill b ON b.id = a.bill_id
			WHERE b.biennium = $1 AND b.prefix = $2 AND b.number = $3
		)`, []any{bill.Biennium, bill.Prefix, bill.Number}},
		{`DELETE FROM testifier WHERE agenda_item_id IN (
			SELECT a.id FROM agenda_item a
			JOIN bill b ON b.id = a.bill_id
			WHERE b.biennium = $1 AND b.prefix = $2 AND b.number = $3
		)`, []any{bill.Biennium, bill.Prefix, bill.Number}},
		{`DELETE FROM agenda_item WHERE bill_id IN (SELECT id FROM bill WHERE biennium = $1 AND prefix = $2 AND number = $3)`, []any{bill.Biennium, bill.Prefix, bill.Number}},
		{`DELETE FROM bill_status_change WHERE bill_id IN (SELECT id FROM bill WHERE biennium = $1 AND prefix = $2 AND number = $3)`, []any{bill.Biennium, bill.Prefix, bill.Number}},
		{`DELETE FROM bill_sponsor WHERE bill_id IN (SELECT id FROM bill WHERE biennium = $1 AND prefix = $2 AND number = $3)`, []any{bill.Biennium, bill.Prefix, bill.Number}},
		{`DELETE FROM hearing WHERE bill_id IN (SELECT id FROM bill WHERE biennium = $1 AND prefix = $2 AND number = $3)`, []any{bill.Biennium, bill.Prefix, bill.Number}},
		{`DELETE FROM bill WHERE biennium = $1 AND prefix = $2 AND number = $3`, []any{bill.Biennium, bill.Prefix, bill.Number}},
		{`DELETE FROM ingestion_run WHERE args->>'bill' = $1`, []any{bill.ID()}},
	}
	for _, stmt := range stmts {
		if _, err := store.Pool.Exec(ctx, stmt.sql, stmt.args...); err != nil {
			t.Errorf("cleanup %q: %v", stmt.sql, err)
		}
	}
}

func liveRawSinkHTTPClient(t *testing.T, store *db.Store) *httpx.Client {
	t.Helper()
	objs, err := objectstore.NewFS(filepath.Join(t.TempDir(), "raw"))
	if err != nil {
		t.Fatalf("objectstore: %v", err)
	}
	return httpx.New(httpx.Config{
		UserAgent:    userAgent,
		Sink:         db.RawSink{Store: store, Objects: objs, TransformVersion: "v0"},
		Timeout:      45 * time.Second,
		MaxRetries:   2,
		RetryBackoff: 750 * time.Millisecond,
		HostRateLimit: map[string]float64{
			"wslwebservices.leg.wa.gov": 10,
			"app.leg.wa.gov":            10,
			"tvw.org":                   10,
			"api.v3.invintus.com":       10,
		},
	})
}

func openLiveIngestionTestStore(t *testing.T, dsn string) *db.Store {
	t.Helper()
	store, err := db.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(store.Close)
	return store
}

type mockDeepgramClient struct {
	t        *testing.T
	eventID  string
	audioURL string
	result   *diarization.Result
}

func (m *mockDeepgramClient) Diarize(ctx context.Context, in diarization.AudioInput) (*diarization.Result, error) {
	m.t.Helper()
	if in.EventID != m.eventID {
		m.t.Fatalf("mock Deepgram EventID = %q, want %q", in.EventID, m.eventID)
	}
	if m.audioURL != "" && in.URL != m.audioURL {
		m.t.Fatalf("mock Deepgram URL = %q, want %q", in.URL, m.audioURL)
	}
	if m.audioURL == "" && in.URL == "" {
		m.t.Fatalf("mock Deepgram received empty URL")
	}
	if len(in.Bytes) != 0 {
		m.t.Fatalf("mock Deepgram received bytes with useURL=true")
	}
	return m.result, nil
}

func intPtr(v int) *int {
	return &v
}

func float64Ptr(v float64) *float64 {
	return &v
}

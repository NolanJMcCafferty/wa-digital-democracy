package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/tvw"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

// IngestTVW fetches Invintus event detail and the WebVTT caption file (if
// present), upserts tvw_event, and replaces the transcript_segment rows.
func (p *Pipeline) IngestTVW(ctx context.Context, ids *IDs) error {
	eventID := p.Demo.TVW.EventID
	ids.TVWEventID = eventID

	httpClient := p.TVW.HTTP

	// 1. Invintus Event/getDetailed.
	body, _ := json.Marshal(map[string]string{
		"eventID":  eventID,
		"clientID": p.TVW.ClientID,
	})
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("authorization", p.TVW.EmbedderAuthName)
	headers.Set("wsc-api-key", p.TVW.EmbedderKey)
	headers.Set("Accept", "application/json")

	evFetch, err := httpClient.Do(ctx, httpx.Request{
		System:   tvw.InvintusSystemName,
		Endpoint: "Event.getDetailed",
		Method:   http.MethodPost,
		URL:      p.TVW.InvintusBaseURL + "/Event/getDetailed",
		Headers:  headers,
		Body:     body,
	})
	if err != nil {
		return fmt.Errorf("Event/getDetailed: %w", err)
	}
	ev, err := tvw.ParseEventDetail(evFetch.Body)
	if err != nil {
		return err
	}
	norm := tvw.NormalizeFromInvintus(ev, nil)
	if _, err := p.Store.UpsertTVWEvent(ctx, db.UpsertTVWEventParams{
		TVWEventID:     norm.TVWEventID,
		Title:          norm.Title,
		Description:    norm.Description,
		StartDateTime:  norm.StartDatetime,
		CaptionURL:     norm.CaptionURL,
		ThumbnailURL:   norm.ThumbnailURL,
		RawCategories:  norm.RawCategories,
		SourceRecordID: evFetch.SourceRecordID,
	}); err != nil {
		return err
	}

	// Bind the TVW event ID + URL onto the demo's hearing so the bundle's
	// hearing→tvw_event join works. ingest-bill picks the hearing row
	// before ingest-tvw runs, so we update it here.
	if ids.HearingID != 0 {
		const updQ = `
UPDATE hearing
   SET tvw_event_id = $2,
       tvw_url = $3,
       updated_at = NOW()
 WHERE id = $1;`
		tvwURL := "https://tvw.org/watch?eventID=" + eventID
		if _, err := p.Store.Pool.Exec(ctx, updQ, ids.HearingID, eventID, tvwURL); err != nil {
			return fmt.Errorf("bind tvw_event_id to hearing: %w", err)
		}
	}

	// 2. Caption fetch + parse, if available.
	if norm.CaptionURL == "" {
		fmt.Fprintf(stderrSink, "  warn: no captionPath for event %s; transcript section will be empty\n", eventID)
		return nil
	}
	capFetch, err := httpClient.Do(ctx, httpx.Request{
		System:   tvw.InvintusSystemName,
		Endpoint: "captionPath",
		URL:      norm.CaptionURL,
	})
	if err != nil {
		return fmt.Errorf("fetch captions: %w", err)
	}
	segs, err := tvw.ParseVTT(capFetch.Body)
	if err != nil {
		return fmt.Errorf("parse vtt: %w", err)
	}
	rows := make([]db.InsertTranscriptSegmentParams, 0, len(segs))
	for _, s := range segs {
		rows = append(rows, db.InsertTranscriptSegmentParams{
			TVWEventID:        norm.TVWEventID,
			StartMS:           s.StartMS,
			EndMS:             s.EndMS,
			Text:              s.Text,
			SpeakerConfidence: "unknown_speaker",
			SourceCaptionURL:  norm.CaptionURL,
			SourceRecordID:    capFetch.SourceRecordID,
		})
	}
	if err := p.Store.ReplaceTranscriptSegments(ctx, norm.TVWEventID, rows); err != nil {
		return err
	}
	return nil
}

var _ = bytes.Buffer{} // import retained for potential future XML rendering

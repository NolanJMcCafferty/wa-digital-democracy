package jobs

import (
	"bytes"
	"context"
	"fmt"

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

	// 1. TVW WordPress metadata + rich Invintus Event/getDetailed.
	wp, _, err := p.TVW.FetchWPVideoByEventID(ctx, eventID)
	if err != nil {
		return fmt.Errorf("wp video metadata: %w", err)
	}
	playerReferer := ""
	if wp != nil {
		playerReferer = wp.Link
	}
	ev, evFetch, err := p.TVW.FetchEventDetailWithSource(ctx, eventID, playerReferer)
	if err != nil {
		return fmt.Errorf("Event/getDetailed: %w", err)
	}
	norm := tvw.NormalizeFromInvintusAndWP(ev, wp)
	if _, err := p.Store.UpsertTVWEvent(ctx, db.UpsertTVWEventParams{
		TVWEventID:          norm.TVWEventID,
		WPPostID:            norm.WPPostID,
		WPSlug:              norm.WPSlug,
		WPLink:              norm.WPLink,
		Title:               norm.Title,
		Description:         norm.Description,
		StartDateTime:       norm.StartDatetime,
		CaptionURL:          norm.CaptionURL,
		ThumbnailURL:        norm.ThumbnailURL,
		CustomID:            norm.CustomID,
		LocationName:        norm.LocationName,
		TotalRuntime:        norm.TotalRuntime,
		TotalRuntimeSeconds: norm.TotalRuntimeSeconds,
		PublishedAudioURL:   norm.PublishedAudioURL,
		AudioDownloadURL:    norm.AudioDownloadURL,
		VideoDownloadURL:    norm.VideoDownloadURL,
		StreamingURIs:       norm.StreamingURIs,
		RawCategories:       norm.RawCategories,
		RawKeywords:         norm.RawKeywords,
		RawWPTags:           norm.RawWPTags,
		RawWPCategories:     norm.RawWPCategories,
		SourceRecordID:      evFetch.SourceRecordID,
	}); err != nil {
		return err
	}
	assetRows := make([]db.UpsertTVWMediaAssetParams, 0, len(ev.MediaAssets))
	for _, a := range tvw.NormalizeMediaAssets(ev) {
		assetRows = append(assetRows, db.UpsertTVWMediaAssetParams{
			TVWEventID:          norm.TVWEventID,
			AssetID:             a.AssetID,
			AssetType:           a.AssetType,
			Name:                a.Name,
			FileURL:             a.FileURL,
			ThumbnailURL:        a.ThumbnailURL,
			SpriteURL:           a.SpriteURL,
			PreviewURL:          a.PreviewURL,
			FileSizeBytes:       a.FileSizeBytes,
			TotalRuntime:        a.TotalRuntime,
			TotalRuntimeSeconds: a.TotalRuntimeSeconds,
			CurrentStatus:       a.CurrentStatus,
			DateCreated:         a.DateCreated,
			AdvancedDetails:     a.AdvancedDetails,
			SourceRecordID:      evFetch.SourceRecordID,
		})
	}
	if err := p.Store.ReplaceTVWMediaAssets(ctx, norm.TVWEventID, assetRows); err != nil {
		return err
	}

	// Bind the TVW event ID + URL onto the selected hearing so page
	// assemblers can join hearing→tvw_event. ingest-bill picks the hearing row
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

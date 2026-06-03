package jobs

import (
	"context"
	"fmt"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/tvw"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

// IngestTVW fetches Invintus event detail and upserts tvw_event +
// tvw_media_asset. The WebVTT caption URL is recorded on tvw_event for
// archival and frontend deep-linking; transcript text and speaker
// attribution come from the diarization pipeline, not from VTT.
func (p *Pipeline) IngestTVW(ctx context.Context, ids *IDs) error {
	eventID := p.BillAgendaTarget.TVW.EventID
	ids.TVWEventID = eventID

	// Discovery already resolved the TVW event ID. Fetch the authoritative
	// Invintus event detail directly; WordPress post metadata is optional
	// enrichment and should not block hearing ingest.
	ev, evFetch, err := p.TVW.FetchEventDetailWithSource(ctx, eventID)
	if err != nil {
		return fmt.Errorf("Event/getDetailed: %w", err)
	}
	norm := tvw.NormalizeFromInvintus(ev, nil)
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

	if norm.CaptionURL == "" {
		fmt.Fprintf(stderrSink, "  warn: no captionPath for event %s\n", eventID)
	}
	return nil
}

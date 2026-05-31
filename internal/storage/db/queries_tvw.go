package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type UpsertTVWEventParams struct {
	TVWEventID          string
	WPPostID            *int64
	WPSlug              string
	WPLink              string
	Title               string
	Description         string
	StartDateTime       time.Time
	CaptionURL          string
	ThumbnailURL        string
	CustomID            string
	LocationName        string
	TotalRuntime        string
	TotalRuntimeSeconds int
	PublishedAudioURL   string
	AudioDownloadURL    string
	VideoDownloadURL    string
	StreamingURIs       any
	RawCategories       []string
	RawKeywords         []string
	RawWPTags           []int
	RawWPCategories     []int
	SourceRecordID      int64
}

func (s *Store) UpsertTVWEvent(ctx context.Context, p UpsertTVWEventParams) (int64, error) {
	streaming, err := marshalJSONDefault(p.StreamingURIs, map[string]any{})
	if err != nil {
		return 0, fmt.Errorf("marshal streaming uris: %w", err)
	}
	wpTags, err := json.Marshal(p.RawWPTags)
	if err != nil {
		return 0, fmt.Errorf("marshal wp tags: %w", err)
	}
	wpCategories, err := json.Marshal(p.RawWPCategories)
	if err != nil {
		return 0, fmt.Errorf("marshal wp categories: %w", err)
	}
	const q = `
INSERT INTO tvw_event (tvw_event_id, wp_post_id, wp_slug, wp_link, title,
                       description, start_datetime, caption_url, thumbnail_url,
                       custom_id, location_name, total_runtime,
                       total_runtime_seconds, published_audio_url,
                       audio_download_url, video_download_url, streaming_uris,
                       raw_categories, raw_keywords, raw_wp_tags,
                       raw_wp_categories, source_record_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,NULLIF($13,0),$14,$15,$16,$17::jsonb,$18,$19,$20::jsonb,$21::jsonb,$22)
ON CONFLICT (tvw_event_id) DO UPDATE SET
  wp_post_id     = COALESCE(EXCLUDED.wp_post_id, tvw_event.wp_post_id),
  wp_slug        = COALESCE(EXCLUDED.wp_slug, tvw_event.wp_slug),
  wp_link        = COALESCE(EXCLUDED.wp_link, tvw_event.wp_link),
  title          = EXCLUDED.title,
  description    = EXCLUDED.description,
  start_datetime = EXCLUDED.start_datetime,
  caption_url    = EXCLUDED.caption_url,
  thumbnail_url  = EXCLUDED.thumbnail_url,
  custom_id      = COALESCE(EXCLUDED.custom_id, tvw_event.custom_id),
  location_name  = COALESCE(EXCLUDED.location_name, tvw_event.location_name),
  total_runtime  = COALESCE(EXCLUDED.total_runtime, tvw_event.total_runtime),
  total_runtime_seconds = COALESCE(EXCLUDED.total_runtime_seconds, tvw_event.total_runtime_seconds),
  published_audio_url = COALESCE(EXCLUDED.published_audio_url, tvw_event.published_audio_url),
  audio_download_url = COALESCE(EXCLUDED.audio_download_url, tvw_event.audio_download_url),
  video_download_url = COALESCE(EXCLUDED.video_download_url, tvw_event.video_download_url),
  streaming_uris = COALESCE(EXCLUDED.streaming_uris, tvw_event.streaming_uris),
  raw_categories = EXCLUDED.raw_categories,
  raw_keywords = EXCLUDED.raw_keywords,
  raw_wp_tags = EXCLUDED.raw_wp_tags,
  raw_wp_categories = EXCLUDED.raw_wp_categories,
  source_record_id = EXCLUDED.source_record_id,
  updated_at     = NOW()
RETURNING id;`
	var id int64
	err = s.Pool.QueryRow(ctx, q,
		p.TVWEventID, p.WPPostID, strOrNull(p.WPSlug), strOrNull(p.WPLink), strOrNull(p.Title),
		strOrNull(p.Description), timeOrNull(p.StartDateTime), strOrNull(p.CaptionURL), strOrNull(p.ThumbnailURL),
		strOrNull(p.CustomID), strOrNull(p.LocationName), strOrNull(p.TotalRuntime), p.TotalRuntimeSeconds,
		strOrNull(p.PublishedAudioURL), strOrNull(p.AudioDownloadURL), strOrNull(p.VideoDownloadURL), string(streaming),
		nonNilStrings(p.RawCategories), nonNilStrings(p.RawKeywords), string(wpTags), string(wpCategories), p.SourceRecordID,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert tvw_event: %w", err)
	}
	return id, nil
}

type UpsertTVWMediaAssetParams struct {
	TVWEventID          string
	AssetID             string
	AssetType           string
	Name                string
	FileURL             string
	ThumbnailURL        string
	SpriteURL           string
	PreviewURL          string
	FileSizeBytes       int64
	TotalRuntime        string
	TotalRuntimeSeconds int
	CurrentStatus       string
	DateCreated         time.Time
	AdvancedDetails     any
	SourceRecordID      int64
}

func (s *Store) ReplaceTVWMediaAssets(ctx context.Context, tvwEventID string, rows []UpsertTVWMediaAssetParams) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM tvw_media_asset WHERE tvw_event_id = $1`, tvwEventID); err != nil {
		return fmt.Errorf("delete tvw media assets: %w", err)
	}
	const q = `
INSERT INTO tvw_media_asset (tvw_event_id, asset_id, asset_type, name, file_url,
                             thumbnail_url, sprite_url, preview_url,
                             file_size_bytes, total_runtime,
                             total_runtime_seconds, current_status,
                             date_created, advanced_details, source_record_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9::bigint,0),$10,NULLIF($11,0),$12,$13,$14::jsonb,$15);`
	for _, r := range rows {
		advanced, err := marshalJSONDefault(r.AdvancedDetails, map[string]any{})
		if err != nil {
			return fmt.Errorf("marshal advanced details: %w", err)
		}
		if _, err := tx.Exec(ctx, q,
			r.TVWEventID, r.AssetID, r.AssetType, strOrNull(r.Name), strOrNull(r.FileURL),
			strOrNull(r.ThumbnailURL), strOrNull(r.SpriteURL), strOrNull(r.PreviewURL),
			fileSizeOrNull(r.FileSizeBytes), strOrNull(r.TotalRuntime), r.TotalRuntimeSeconds,
			strOrNull(r.CurrentStatus), timeOrNull(r.DateCreated), string(advanced), r.SourceRecordID,
		); err != nil {
			return fmt.Errorf("insert tvw media asset %s: %w", r.AssetID, err)
		}
	}
	return tx.Commit(ctx)
}

type TVWAudioSource struct {
	TVWEventID string
	URL        string
	Kind       string
	Priority   int
}

const tvwAudioSourceCandidatesSQL = `
WITH candidates AS (
  SELECT tvw_event_id, audio_download_url AS url, 'audio_download_url' AS kind, 1 AS priority
    FROM tvw_event WHERE tvw_event_id = $1 AND NULLIF(audio_download_url, '') IS NOT NULL
  UNION ALL
  SELECT tvw_event_id, published_audio_url AS url, 'published_audio_url' AS kind, 2 AS priority
    FROM tvw_event WHERE tvw_event_id = $1 AND NULLIF(published_audio_url, '') IS NOT NULL
  UNION ALL
  SELECT tvw_event_id, file_url AS url, 'media_asset_audio' AS kind, 3 AS priority
    FROM tvw_media_asset WHERE tvw_event_id = $1 AND asset_type ILIKE 'audio' AND NULLIF(file_url, '') IS NOT NULL
  UNION ALL
  SELECT tvw_event_id, video_download_url AS url, 'video_download_url' AS kind, 4 AS priority
    FROM tvw_event WHERE tvw_event_id = $1 AND NULLIF(video_download_url, '') IS NOT NULL
  UNION ALL
  SELECT tvw_event_id, file_url AS url, 'media_asset_video' AS kind, 5 AS priority
    FROM tvw_media_asset WHERE tvw_event_id = $1 AND asset_type ILIKE 'video' AND NULLIF(file_url, '') IS NOT NULL
)
SELECT tvw_event_id, url, kind, priority FROM candidates`

// BestTVWAudioSource returns the preferred downloadable source for an event:
// direct audio first, then audio media assets, then video fallback for ffmpeg
// extraction.
func (s *Store) BestTVWAudioSource(ctx context.Context, eventID string) (TVWAudioSource, error) {
	var out TVWAudioSource
	if err := s.Pool.QueryRow(ctx, tvwAudioSourceCandidatesSQL+` ORDER BY priority LIMIT 1;`, eventID).Scan(&out.TVWEventID, &out.URL, &out.Kind, &out.Priority); err != nil {
		return TVWAudioSource{}, fmt.Errorf("best tvw audio source: %w", err)
	}
	return out, nil
}

// ListTVWAudioSourceCandidates returns every usable TVW/Invintus media URL
// audio-cache considered, in the same priority order BestTVWAudioSource uses.
func (s *Store) ListTVWAudioSourceCandidates(ctx context.Context, eventID string) ([]TVWAudioSource, error) {
	rows, err := s.Pool.Query(ctx, tvwAudioSourceCandidatesSQL+` ORDER BY priority, url;`, eventID)
	if err != nil {
		return nil, fmt.Errorf("list tvw audio source candidates: %w", err)
	}
	defer rows.Close()

	out := []TVWAudioSource{}
	for rows.Next() {
		var c TVWAudioSource
		if err := rows.Scan(&c.TVWEventID, &c.URL, &c.Kind, &c.Priority); err != nil {
			return nil, fmt.Errorf("scan tvw audio source candidate: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type UpsertTVWAudioAssetParams struct {
	TVWEventID     string
	SourceURL      string
	SourceKind     string
	OriginalPath   string
	NormalizedPath string
	ContentHash    string
	DurationMS     int
	SampleRate     int
	Channels       int
	Codec          string
}

func (s *Store) UpsertTVWAudioAsset(ctx context.Context, p UpsertTVWAudioAssetParams) (int64, error) {
	const q = `
INSERT INTO tvw_audio_asset (tvw_event_id, source_url, source_kind, original_path,
                             normalized_path, content_hash, duration_ms,
                             sample_rate, channels, codec)
VALUES ($1,$2,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,0),NULLIF($8,0),NULLIF($9,0),NULLIF($10,''))
ON CONFLICT (tvw_event_id, source_url) DO UPDATE SET
  source_kind = EXCLUDED.source_kind,
  original_path = EXCLUDED.original_path,
  normalized_path = EXCLUDED.normalized_path,
  duration_ms = EXCLUDED.duration_ms,
  sample_rate = EXCLUDED.sample_rate,
  channels = EXCLUDED.channels,
  codec = EXCLUDED.codec
RETURNING id;`
	var id int64
	err := s.Pool.QueryRow(ctx, q, p.TVWEventID, p.SourceURL, p.SourceKind,
		p.OriginalPath, p.NormalizedPath, p.ContentHash, p.DurationMS,
		p.SampleRate, p.Channels, p.Codec).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert tvw_audio_asset: %w", err)
	}
	return id, nil
}

type UpsertTVWAudioSourceParams struct {
	TVWEventID string
	SourceURL  string
	SourceKind string
}

func (s *Store) UpsertTVWAudioSource(ctx context.Context, p UpsertTVWAudioSourceParams) (int64, error) {
	const q = `
INSERT INTO tvw_audio_asset (tvw_event_id, source_url, source_kind)
VALUES ($1,$2,$3)
ON CONFLICT (tvw_event_id, source_url) DO UPDATE SET
  source_kind = EXCLUDED.source_kind
RETURNING id;`
	var id int64
	if err := s.Pool.QueryRow(ctx, q, p.TVWEventID, p.SourceURL, p.SourceKind).Scan(&id); err != nil {
		return 0, fmt.Errorf("upsert tvw_audio_asset source: %w", err)
	}
	return id, nil
}

type LatestTVWAudioAsset struct {
	ID             int64
	TVWEventID     string
	SourceURL      string
	SourceKind     string
	OriginalPath   string
	NormalizedPath string
	ContentHash    string
	DurationMS     int
}

func (s *Store) LatestTVWAudioAsset(ctx context.Context, eventID string) (LatestTVWAudioAsset, error) {
	const q = `
SELECT id, tvw_event_id, source_url, source_kind,
       COALESCE(original_path, ''), COALESCE(normalized_path, ''),
       COALESCE(content_hash, ''), COALESCE(duration_ms, 0)
  FROM tvw_audio_asset
 WHERE tvw_event_id = $1
 ORDER BY created_at DESC, id DESC
 LIMIT 1;`
	var out LatestTVWAudioAsset
	if err := s.Pool.QueryRow(ctx, q, eventID).Scan(&out.ID, &out.TVWEventID, &out.SourceURL,
		&out.SourceKind, &out.OriginalPath, &out.NormalizedPath, &out.ContentHash, &out.DurationMS); err != nil {
		return LatestTVWAudioAsset{}, fmt.Errorf("latest tvw_audio_asset: %w", err)
	}
	return out, nil
}

func (s *Store) EnsureTVWAudioAsset(ctx context.Context, eventID string) (LatestTVWAudioAsset, bool, error) {
	audio, err := s.LatestTVWAudioAsset(ctx, eventID)
	if err == nil {
		return audio, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return LatestTVWAudioAsset{}, false, err
	}

	src, err := s.BestTVWAudioSource(ctx, eventID)
	if err != nil {
		return LatestTVWAudioAsset{}, false, fmt.Errorf("resolve tvw audio source: %w", err)
	}
	id, err := s.UpsertTVWAudioSource(ctx, UpsertTVWAudioSourceParams{
		TVWEventID: eventID,
		SourceURL:  src.URL,
		SourceKind: src.Kind,
	})
	if err != nil {
		return LatestTVWAudioAsset{}, false, err
	}
	return LatestTVWAudioAsset{
		ID:         id,
		TVWEventID: src.TVWEventID,
		SourceURL:  src.URL,
		SourceKind: src.Kind,
	}, true, nil
}

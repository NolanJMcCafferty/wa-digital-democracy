package tvw

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// NormalizedTVWEvent maps onto the `tvw_event` table.
type NormalizedTVWEvent struct {
	TVWEventID          string
	WPPostID            *int64
	WPSlug              string
	WPLink              string
	Title               string
	Description         string
	StartDatetime       time.Time
	CaptionURL          string
	ThumbnailURL        string
	CustomID            string
	LocationName        string
	TotalRuntime        string
	TotalRuntimeSeconds int
	PublishedAudioURL   string
	AudioDownloadURL    string
	VideoDownloadURL    string
	StreamingURIs       json.RawMessage
	RawCategories       []string
	RawKeywords         []string
	RawWPTags           []int
	RawWPCategories     []int
}

// NormalizedMediaAsset maps one Invintus media/document/link asset to storage.
type NormalizedMediaAsset struct {
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
	AdvancedDetails     json.RawMessage
}

// NormalizeFromInvintus converts an InvintusEvent into the canonical shape.
// wpPostID may be nil if discovery happened via the schedule API.
func NormalizeFromInvintus(e *InvintusEvent, wpPostID *int64) NormalizedTVWEvent {
	return NormalizedTVWEvent{
		TVWEventID:          e.EventID,
		WPPostID:            wpPostID,
		Title:               CleanTitle(e.Title),
		Description:         e.Description,
		StartDatetime:       ParseInvintusStartDateTime(e.StartDateTime),
		CaptionURL:          e.CaptionPath,
		ThumbnailURL:        e.VideoThumbnail,
		CustomID:            e.CustomID,
		LocationName:        e.LocationName,
		TotalRuntime:        e.TotalRunTime,
		TotalRuntimeSeconds: runtimeToSeconds(e.TotalRunTime),
		PublishedAudioURL:   e.PublishedAudio,
		AudioDownloadURL:    e.AudioDownloadURI,
		VideoDownloadURL:    e.VideoDownloadURI,
		StreamingURIs:       normalizeRawJSON(e.StreamingURIs, "{}"),
		RawCategories:       append([]string(nil), e.Categories...),
		RawKeywords:         append([]string(nil), e.Keywords...),
	}
}

// NormalizeFromInvintusAndWP enriches Invintus detail with WordPress archive
// metadata such as slug/link and TVW's bill/category taxonomy IDs.
func NormalizeFromInvintusAndWP(e *InvintusEvent, wp *WPVideoPost) NormalizedTVWEvent {
	var wpID *int64
	out := NormalizeFromInvintus(e, nil)
	if wp != nil {
		id := int64(wp.ID)
		wpID = &id
		out.WPPostID = wpID
		out.WPSlug = wp.Slug
		out.WPLink = wp.Link
		out.RawWPTags = append([]int(nil), wp.InvintusTag...)
		out.RawWPCategories = append([]int(nil), wp.InvintusCategory...)
	}
	return out
}

func NormalizeMediaAssets(e *InvintusEvent) []NormalizedMediaAsset {
	out := make([]NormalizedMediaAsset, 0, len(e.MediaAssets))
	for _, a := range e.MediaAssets {
		out = append(out, NormalizedMediaAsset{
			AssetID:             a.AssetID,
			AssetType:           a.Type,
			Name:                a.Name,
			FileURL:             a.FileURL,
			ThumbnailURL:        a.Thumbnail,
			SpriteURL:           a.Sprite,
			PreviewURL:          a.PreviewURI,
			FileSizeBytes:       parseInt64(a.FileSize),
			TotalRuntime:        a.TotalRunTime,
			TotalRuntimeSeconds: firstNonZero(a.TotalRunTimeS, runtimeToSeconds(a.TotalRunTime)),
			CurrentStatus:       a.CurrentStatus,
			DateCreated:         ParseInvintusStartDateTime(a.DateCreated),
			AdvancedDetails:     normalizeRawJSON(a.AdvancedDetails, "{}"),
		})
	}
	return out
}

func runtimeToSeconds(raw string) int {
	parts := strings.Split(strings.TrimSpace(raw), ":")
	if len(parts) != 3 {
		return 0
	}
	h, errH := strconv.Atoi(parts[0])
	m, errM := strconv.Atoi(parts[1])
	s, errS := strconv.Atoi(parts[2])
	if errH != nil || errM != nil || errS != nil {
		return 0
	}
	return h*3600 + m*60 + s
}

func parseInt64(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	n, _ := strconv.ParseInt(raw, 10, 64)
	return n
}

func firstNonZero(xs ...int) int {
	for _, x := range xs {
		if x != 0 {
			return x
		}
	}
	return 0
}

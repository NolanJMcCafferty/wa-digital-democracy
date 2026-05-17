package tvw

import "time"

// NormalizedTVWEvent maps onto the `tvw_event` table.
type NormalizedTVWEvent struct {
	TVWEventID    string
	WPPostID      *int64
	Title         string
	Description   string
	StartDatetime time.Time
	CaptionURL    string
	ThumbnailURL  string
	RawCategories []string
}

// NormalizeFromInvintus converts an InvintusEvent into the canonical shape.
// wpPostID may be nil if discovery happened via the schedule API.
func NormalizeFromInvintus(e *InvintusEvent, wpPostID *int64) NormalizedTVWEvent {
	return NormalizedTVWEvent{
		TVWEventID:    e.EventID,
		WPPostID:      wpPostID,
		Title:         CleanTitle(e.Title),
		Description:   e.Description,
		StartDatetime: ParseInvintusStartDateTime(e.StartDateTime),
		CaptionURL:    e.CaptionPath,
		ThumbnailURL:  e.VideoThumbnail,
		RawCategories: append([]string(nil), e.Categories...),
	}
}

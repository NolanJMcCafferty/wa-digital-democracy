package committeeschedules

import (
	"regexp"
	"sort"
)

// ScheduleRow is one meeting parsed from /Home/Search/. We collect the IDs
// the wiki cares about (per spec 03 line 95–101): the internal video modal
// ID and the TVW/Invintus event ID.
type ScheduleRow struct {
	AgendaID   string // arg of showAgendaDetailModal(<id>)
	VideoID    string // arg #1 of showVideoModal(<videoID>, <eventID>)
	TVWEventID string // arg #2 of showVideoModal
}

// VideoModalRef is a single (committee_schedule_video_id, tvw_event_id) pair
// extracted from the page. Same shape as ScheduleRow but without the agenda
// wrapper — useful when the caller only needs the TVW mapping.
type VideoModalRef struct {
	VideoID    string
	TVWEventID string
}

// ParseSearchResults walks the page and returns the agenda meetings it
// finds. Each meeting may or may not carry a paired showVideoModal(...). We
// stitch the two by document order: a showVideoModal that follows a
// showAgendaDetailModal closely is paired with it.
//
// This is intentionally lenient: the upstream HTML structure is fragile, so
// we look at the inline JS calls rather than DOM nesting.
func ParseSearchResults(body []byte) ([]ScheduleRow, error) {
	src := string(body)

	// Find every modal-call in document order along with its kind.
	type tok struct {
		kind  string // "agenda" | "video"
		index int
		args  []string
	}
	var toks []tok

	for _, m := range agendaRe.FindAllStringSubmatchIndex(src, -1) {
		toks = append(toks, tok{
			kind:  "agenda",
			index: m[0],
			args:  []string{src[m[2]:m[3]]},
		})
	}
	for _, m := range videoRe.FindAllStringSubmatchIndex(src, -1) {
		toks = append(toks, tok{
			kind:  "video",
			index: m[0],
			args:  []string{src[m[2]:m[3]], src[m[4]:m[5]]},
		})
	}
	sort.Slice(toks, func(i, j int) bool { return toks[i].index < toks[j].index })

	var rows []ScheduleRow
	var cur *ScheduleRow
	for _, t := range toks {
		switch t.kind {
		case "agenda":
			if cur != nil {
				rows = append(rows, *cur)
			}
			cur = &ScheduleRow{AgendaID: t.args[0]}
		case "video":
			if cur == nil {
				// Video without preceding agenda — emit a row anyway so the
				// caller doesn't lose the (videoID, eventID) mapping.
				rows = append(rows, ScheduleRow{VideoID: t.args[0], TVWEventID: t.args[1]})
				continue
			}
			cur.VideoID = t.args[0]
			cur.TVWEventID = t.args[1]
		}
	}
	if cur != nil {
		rows = append(rows, *cur)
	}
	return rows, nil
}

// ParseVideoMappings returns just the (videoID, tvwEventID) pairs from a
// page's showVideoModal calls.
func ParseVideoMappings(body []byte) []VideoModalRef {
	src := string(body)
	matches := videoRe.FindAllStringSubmatch(src, -1)
	out := make([]VideoModalRef, 0, len(matches))
	for _, m := range matches {
		out = append(out, VideoModalRef{VideoID: m[1], TVWEventID: m[2]})
	}
	return out
}

// Regexes
var (
	agendaRe = regexp.MustCompile(`showAgendaDetailModal\(\s*['"]?(\d+)['"]?\s*\)`)
	videoRe  = regexp.MustCompile(`showVideoModal\(\s*['"]?(\d+)['"]?\s*,\s*['"]?(\d+)['"]?\s*\)`)
)

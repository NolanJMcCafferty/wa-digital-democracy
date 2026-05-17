package tvw

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ScheduleEvent is one row from /tvw/v1/schedule.
type ScheduleEvent struct {
	EventID       string `json:"eventID"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	AiringTime    string `json:"airingTime"`
	EventDateTime string `json:"eventDateTime"`
	Duration      string `json:"duration"`
	EventStatus   string `json:"eventStatus"`
	VideoThumb    string `json:"videoThumbnail"`
	URL           string `json:"url"`
}

// WPVideoPost is one row from /wp/v2/invintus_video.
type WPVideoPost struct {
	ID    int    `json:"id"`
	Date  string `json:"date"`
	Link  string `json:"link"`
	Title struct {
		Rendered string `json:"rendered"`
	} `json:"title"`
	Content struct {
		Rendered string `json:"rendered"`
	} `json:"content"`
	Excerpt struct {
		Rendered string `json:"rendered"`
	} `json:"excerpt"`
}

// InvintusEvent is the parsed Invintus Event/getDetailed payload.
type InvintusEvent struct {
	EventID        string   `json:"eventID"`
	ClientID       string   `json:"clientID"`
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	StartDateTime  string   `json:"startDateTime"`
	EventStatus    string   `json:"eventStatus"`
	Categories     []string `json:"categories"`
	LocationName   string   `json:"locationName"`
	CaptionPath    string   `json:"captionPath"`
	StreamingURIs  []any    `json:"streamingURIs"`
	EstRuntime     int      `json:"estRuntime"`
	VideoThumbnail string   `json:"videoThumbnail"`
	EventNotes     string   `json:"eventNotes"`
}

// TranscriptSegment is one cue parsed from a WebVTT caption file.
type TranscriptSegment struct {
	StartMS          int    `json:"start_ms"`
	EndMS            int    `json:"end_ms"`
	Text             string `json:"text"`
	SourceCaptionURL string `json:"source_caption_url"`
	TVWEventID       string `json:"tvw_event_id"`
}

// ParseSchedule decodes the {success, data:[...]} envelope.
func ParseSchedule(body []byte) ([]ScheduleEvent, error) {
	var env struct {
		Success bool            `json:"success"`
		Data    []ScheduleEvent `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("schedule decode: %w", err)
	}
	return env.Data, nil
}

// ParseWPVideoList decodes the bare WP REST array.
func ParseWPVideoList(body []byte) ([]WPVideoPost, error) {
	var posts []WPVideoPost
	if err := json.Unmarshal(body, &posts); err != nil {
		return nil, fmt.Errorf("wp video list decode: %w", err)
	}
	return posts, nil
}

// ParseEventDetail decodes the {errors, data, meta} envelope; data.captionPath
// may be JSON null (hence we read into *string then flatten).
func ParseEventDetail(body []byte) (*InvintusEvent, error) {
	var env struct {
		Errors any `json:"errors"`
		Data   struct {
			InvintusEvent
			CaptionPath *string `json:"captionPath"`
		} `json:"data"`
		Meta any `json:"meta"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("event detail decode: %w", err)
	}
	out := env.Data.InvintusEvent
	if env.Data.CaptionPath != nil {
		out.CaptionPath = *env.Data.CaptionPath
	}
	return &out, nil
}

// ParseVTT parses a WebVTT body into ordered segments.
//
// The WebVTT format we see in TVW captions is minimal: an optional WEBVTT
// header, blank-line-separated cues with a "HH:MM:SS.mmm --> HH:MM:SS.mmm"
// timing line followed by one or more text lines. No cue identifiers, no
// styling, no NOTE blocks expected — but we tolerate them defensively.
func ParseVTT(body []byte) ([]TranscriptSegment, error) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var (
		out           []TranscriptSegment
		curStart      = -1
		curEnd        = -1
		curText       []string
	)
	flush := func() {
		if curStart < 0 {
			return
		}
		out = append(out, TranscriptSegment{
			StartMS: curStart,
			EndMS:   curEnd,
			Text:    strings.Join(curText, " "),
		})
		curStart, curEnd = -1, -1
		curText = nil
	}

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		trim := strings.TrimSpace(line)

		// Blank line ends the current cue.
		if trim == "" {
			flush()
			continue
		}
		// Header / NOTE / STYLE / REGION sections — skip until next blank line.
		if curStart < 0 && (strings.HasPrefix(trim, "WEBVTT") || strings.HasPrefix(trim, "NOTE") ||
			strings.HasPrefix(trim, "STYLE") || strings.HasPrefix(trim, "REGION")) {
			continue
		}
		// Timing line?
		if strings.Contains(trim, "-->") {
			startMS, endMS, ok := parseTiming(trim)
			if !ok {
				continue // malformed; skip cue
			}
			curStart, curEnd = startMS, endMS
			continue
		}
		// Otherwise it's text content for the current cue.
		if curStart >= 0 {
			curText = append(curText, trim)
		}
	}
	flush()

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan vtt: %w", err)
	}
	return out, nil
}

// parseTiming parses "HH:MM:SS.mmm --> HH:MM:SS.mmm[ ...settings]" or
// "MM:SS.mmm --> MM:SS.mmm" lines. Returns ms-since-zero ints.
func parseTiming(line string) (int, int, bool) {
	parts := strings.SplitN(line, "-->", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	start, ok1 := timeToMS(strings.TrimSpace(parts[0]))
	endRaw := strings.TrimSpace(parts[1])
	if i := strings.IndexAny(endRaw, " \t"); i >= 0 {
		endRaw = endRaw[:i]
	}
	end, ok2 := timeToMS(endRaw)
	return start, end, ok1 && ok2
}

func timeToMS(s string) (int, bool) {
	// Forms: HH:MM:SS.mmm  |  MM:SS.mmm
	bits := strings.Split(s, ":")
	if len(bits) < 2 || len(bits) > 3 {
		return 0, false
	}
	var hours, minutes int
	var secsField string
	switch len(bits) {
	case 2:
		minutes = atoiOr(bits[0], -1)
		secsField = bits[1]
	case 3:
		hours = atoiOr(bits[0], -1)
		minutes = atoiOr(bits[1], -1)
		secsField = bits[2]
	}
	if hours < 0 || minutes < 0 {
		return 0, false
	}
	dot := strings.IndexByte(secsField, '.')
	var seconds, millis int
	if dot < 0 {
		seconds = atoiOr(secsField, -1)
	} else {
		seconds = atoiOr(secsField[:dot], -1)
		ms := secsField[dot+1:]
		// pad/truncate fractional seconds to 3 digits.
		switch len(ms) {
		case 0:
			millis = 0
		case 1:
			millis = atoiOr(ms, -1) * 100
		case 2:
			millis = atoiOr(ms, -1) * 10
		case 3:
			millis = atoiOr(ms, -1)
		default:
			millis = atoiOr(ms[:3], -1)
		}
	}
	if seconds < 0 || millis < 0 {
		return 0, false
	}
	return ((hours*60+minutes)*60+seconds)*1000 + millis, true
}

func atoiOr(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// ParseInvintusStartDateTime turns Invintus's "2025-03-14 11:00:00" string
// into a time.Time in UTC. Returns the zero time on failure.
func ParseInvintusStartDateTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	// Invintus uses "2006-01-02 15:04:05" without TZ; treat as UTC.
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

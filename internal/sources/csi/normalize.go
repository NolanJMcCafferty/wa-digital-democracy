package csi

import "strings"

// NormalizedTestifier maps onto the `testifier` table.
type NormalizedTestifier struct {
	AgendaItemSourceKey string // CSI agendaItemId — used as a join key
	RawName             string
	RawOrganization     string
	Position            string // canonical "Pro" | "Con" | "Other" | "Unknown"
	Testified           bool
	TimeSignedIn        any // time.Time or zero
}

// Normalize maps a Testifier into the DB's NormalizedTestifier shape. The
// agendaItemSourceKey is the CSI agendaItemId for the testifier's agenda item.
func Normalize(t Testifier, agendaItemSourceKey string) NormalizedTestifier {
	return NormalizedTestifier{
		AgendaItemSourceKey: agendaItemSourceKey,
		RawName:             strings.TrimSpace(t.Name),
		RawOrganization:     strings.TrimSpace(t.Organization),
		Position:            CanonicalPosition(t.Position),
		Testified:           t.Testified,
		TimeSignedIn:        t.TimeSignedIn,
	}
}

// CanonicalPosition normalizes free-text positions to the testifier_position
// enum values defined in the schema.
func CanonicalPosition(p string) string {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "pro":
		return "Pro"
	case "con":
		return "Con"
	case "other":
		return "Other"
	default:
		return "Unknown"
	}
}

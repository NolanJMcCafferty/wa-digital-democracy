package candidate

import (
	"context"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/csi"
)

type fakeCSI struct {
	meetings     map[string][]csi.Meeting
	meetingErr   map[string]error
	items        map[string][]csi.AgendaItem
	agendaErr    map[string]error
	testifiers   map[string][]csi.Testifier
	testifierErr map[string]error
}

func (f *fakeCSI) ListMeetings(_ context.Context, chamber, committeeID string) ([]csi.Meeting, error) {
	key := chamber + ":" + committeeID
	if err := f.meetingErr[key]; err != nil {
		return nil, err
	}
	return f.meetings[key], nil
}

func (f *fakeCSI) ListAgendaItems(_ context.Context, chamber, meetingFamilyID string) ([]csi.AgendaItem, error) {
	key := chamber + ":" + meetingFamilyID
	if err := f.agendaErr[key]; err != nil {
		return nil, err
	}
	return f.items[key], nil
}

func (f *fakeCSI) GetTestifiers(_ context.Context, agendaItemID, _ string) ([]csi.Testifier, error) {
	if err := f.testifierErr[agendaItemID]; err != nil {
		return nil, err
	}
	return f.testifiers[agendaItemID], nil
}

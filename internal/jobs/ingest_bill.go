package jobs

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/lws"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

// IngestBill fetches the LWS bill metadata package for the selected bill, persists raw
// XML via the RawSink, and upserts bill / bill_sponsor /
// bill_status_change. Legislator rows are owned by ingest-legislators;
// sponsor joins resolve against that roster and skip missing members.
func (p *Pipeline) IngestBill(ctx context.Context, ids *IDs) error {
	biennium := p.Demo.Biennium
	billNumber := strconv.Itoa(p.Demo.BillNumber)
	billID := p.Demo.BillID()

	// We bypass the lws.Client wrappers and hit Do() directly so we can
	// capture the source_record_id from the RawFetch each call returns.
	httpClient := p.LWS.HTTP

	// 1. GetLegislation → bill metadata + current status snapshot.
	body, srBill, err := lwsCall(ctx, httpClient, p.LWS.BaseURL, "GetLegislation", map[string]string{
		"biennium":   biennium,
		"billNumber": billNumber,
	})
	if err != nil {
		return fmt.Errorf("GetLegislation: %w", err)
	}
	leg, err := lws.ParseGetLegislation(body)
	if err != nil {
		return err
	}
	norm := lws.NormalizeBill(leg)
	id, err := p.Store.UpsertBill(ctx, db.UpsertBillParams{
		Biennium:       norm.Biennium,
		Prefix:         norm.Prefix,
		Number:         norm.Number,
		Title:          norm.Title,
		Description:    norm.Description,
		ChamberOrigin:  norm.ChamberOrigin,
		CurrentStatus:  norm.CurrentStatus,
		StatusDate:     norm.StatusDate,
		OfficialURL:    norm.OfficialURL,
		SourceRecordID: srBill,
	})
	if err != nil {
		return err
	}
	ids.BillID = id

	// 2. GetSponsors → bill_sponsor. The legislator table is owned by
	// ingest-legislators, so missing sponsor IDs are warning-only skips.
	body, _, err = lwsCall(ctx, httpClient, p.LWS.BaseURL, "GetSponsors", map[string]string{
		"biennium": biennium,
		"billId":   billID,
	})
	if err != nil {
		return fmt.Errorf("GetSponsors: %w", err)
	}
	sps, err := lws.ParseSponsors(body)
	if err != nil {
		return err
	}
	missingSponsors := 0
	for _, s := range lws.NormalizeSponsors(sps) {
		legID, ok, err := p.Store.FindLegislatorIDByLWSSponsorID(ctx, s.LWSSponsorID)
		if err != nil {
			return err
		}
		if !ok {
			missingSponsors++
			fmt.Fprintf(stderrSink, "  warn: %s sponsor %s %q missing from legislator roster; skipping bill_sponsor\n",
				billID, s.LWSSponsorID, s.LongName)
			continue
		}
		if err := p.Store.UpsertBillSponsor(ctx, ids.BillID, legID, s.SponsorType); err != nil {
			return err
		}
	}
	if missingSponsors > 0 {
		fmt.Fprintf(stderrSink, "  warn: %s skipped %d sponsor(s) missing from legislator roster\n", billID, missingSponsors)
	}

	// 3. GetLegislativeStatusChangesByBillNumber → status timeline.
	// LWS expects beginDate/endDate; cover the whole biennium generously.
	begin, end := biennialBounds(biennium)
	body, srStatus, err := lwsCall(ctx, httpClient, p.LWS.BaseURL, "GetLegislativeStatusChangesByBillNumber", map[string]string{
		"biennium":   biennium,
		"billNumber": billNumber,
		"beginDate":  begin,
		"endDate":    end,
	})
	if err != nil {
		// Some bill numbers return faults here when no status changes yet;
		// don't sink the whole job over that — log and continue.
		fmt.Fprintf(stderrSink, "  warn: GetLegislativeStatusChangesByBillNumber: %v\n", err)
	} else {
		changes, err := lws.ParseStatusChanges(body)
		if err != nil {
			return err
		}
		params := make([]db.InsertStatusChangeParams, 0, len(changes))
		for _, ch := range changes {
			actDate, _ := parseLWSDate(ch.ActionDate)
			params = append(params, db.InsertStatusChangeParams{
				BillID:         ids.BillID,
				ActionDate:     actDate,
				HistoryLine:    ch.HistoryLine,
				SourceRecordID: srStatus,
			})
		}
		if len(params) > 0 {
			if err := p.Store.ReplaceStatusChangesForBill(ctx, ids.BillID, params); err != nil {
				return err
			}
		}
	}

	// 4. GetHearings — used both to seed the hearing row (Step 3 will
	// re-upsert with CSI/TVW IDs) and to give us the LWS AgendaId for
	// the matching meeting.
	body, srHr, err := lwsCall(ctx, httpClient, p.LWS.BaseURL, "GetHearings", map[string]string{
		"biennium":   biennium,
		"billNumber": billNumber,
	})
	if err != nil {
		return fmt.Errorf("GetHearings: %w", err)
	}
	hearings, err := lws.ParseHearings(body)
	if err != nil {
		return err
	}
	// Hearing-row policy: when the caller is the curated path (demo
	// pins a chamber + committee), narrow to that one hearing so the
	// rest of the pipeline binds correctly. When the caller is the
	// metadata-only path (no chamber, no committee — ingest-session
	// driver), upsert every hearing LWS reports so the auto-discovery
	// pass downstream has rows to enrich.
	if p.Demo.Chamber != "" {
		matched := pickHearingForDemo(hearings, p.Demo)
		if matched != nil {
			nh := lws.NormalizeHearings([]lws.Hearing{*matched})[0]
			hid, err := p.Store.UpsertHearing(ctx, db.UpsertHearingParams{
				BillID:           pInt64(ids.BillID),
				CommitteeName:    nh.CommitteeName,
				CommitteeAcronym: nh.CommitteeAcronym,
				Chamber:          nh.Chamber,
				MeetingDateTime:  nh.MeetingDateTime,
				Location:         nh.Location,
				LWSMeetingID:     nh.LWSMeetingID,
				SourceRecordID:   srHr,
			})
			if err != nil {
				return err
			}
			ids.HearingID = hid
		}
		return nil
	}

	// Metadata-only path: upsert all hearings.
	for _, nh := range lws.NormalizeHearings(hearings) {
		if nh.CommitteeName == "" || nh.MeetingDateTime.IsZero() {
			continue
		}
		if _, err := p.Store.UpsertHearing(ctx, db.UpsertHearingParams{
			BillID:           pInt64(ids.BillID),
			CommitteeName:    nh.CommitteeName,
			CommitteeAcronym: nh.CommitteeAcronym,
			Chamber:          nh.Chamber,
			MeetingDateTime:  nh.MeetingDateTime,
			Location:         nh.Location,
			LWSMeetingID:     nh.LWSMeetingID,
			SourceRecordID:   srHr,
		}); err != nil {
			return err
		}
	}
	return nil
}

// lwsCall is a thin wrapper that builds the SOAP envelope, posts it via
// the httpx client, and returns the response body plus the source_record id.
func lwsCall(ctx context.Context, h *httpx.Client, baseURL, op string, params map[string]string) ([]byte, int64, error) {
	// Render param XML preserving whatever order Go iterates the map in —
	// LWS doesn't care about order.
	pairs := make([][2]string, 0, len(params))
	for k, v := range params {
		pairs = append(pairs, [2]string{k, v})
	}
	inner := lwsRenderOp(op, pairs)
	envelope := `<?xml version="1.0" encoding="utf-8"?>` +
		`<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/">` +
		`<soap:Body>` + inner + `</soap:Body></soap:Envelope>`

	headers := http.Header{}
	headers.Set("Content-Type", "text/xml; charset=utf-8")
	headers.Set("SOAPAction", lws.SOAPNamespace+op)
	headers.Set("Accept", "text/xml")

	fetch, err := h.Do(ctx, httpx.Request{
		System:   lws.SystemName,
		Endpoint: "LegislationService." + op,
		Method:   http.MethodPost,
		URL:      baseURL + "/LegislationService.asmx",
		Headers:  headers,
		Body:     []byte(envelope),
	})
	if err != nil {
		return nil, 0, err
	}
	return fetch.Body, fetch.SourceRecordID, nil
}

func lwsRenderOp(op string, params [][2]string) string {
	var b []byte
	b = append(b, '<')
	b = append(b, op...)
	b = append(b, ' ')
	b = append(b, []byte(`xmlns="`+lws.SOAPNamespace+`"`)...)
	b = append(b, '>')
	for _, kv := range params {
		b = append(b, '<')
		b = append(b, kv[0]...)
		b = append(b, '>')
		for _, r := range kv[1] {
			switch r {
			case '&':
				b = append(b, []byte("&amp;")...)
			case '<':
				b = append(b, []byte("&lt;")...)
			case '>':
				b = append(b, []byte("&gt;")...)
			default:
				b = append(b, []byte(string(r))...)
			}
		}
		b = append(b, []byte("</"+kv[0]+">")...)
	}
	b = append(b, []byte("</"+op+">")...)
	return string(b)
}

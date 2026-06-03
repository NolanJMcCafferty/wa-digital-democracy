// Package csi is the Washington Legislature Committee Sign In connector.
//
// CSI is the public source for testifier sign-ins, agenda item IDs, and
// pro/con/other position records. Per data-sources/02:
//
//  1. GET /csi/<chamber>          — parse <select id="SelectedCommitteeId">
//  2. GET /csi/Home/GetMeetings    — JSON option list (meetingFamilyId)
//  3. GET /csi/Home/GetAgendaItems — HTML; parse onclick of class="agendaItem"
//  4. GET /csi/Home/GetOtherTestifiers — HTML; parse data-json on the two tables
//
// Ethical/safety constraint (spec 02 line 53): READ-ONLY. Never call
// /Testimony/Add, /Testifier/GetPanelTestifier, or any submit endpoint.
package csi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/connector"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

const (
	SystemName     = "csi"
	DefaultBaseURL = "https://app.leg.wa.gov/csi"
)

var descriptor = connector.Descriptor{
	System:      SystemName,
	BaseURL:     DefaultBaseURL,
	Description: "Washington Legislature Committee Sign In (read-only HTML scrape)",
}

func init() { connector.Register(descriptor) }

// Client wraps an httpx.Client with CSI configuration.
type Client struct {
	HTTP    *httpx.Client
	BaseURL string
}

// New returns a Client.
func New(h *httpx.Client) *Client {
	return &Client{HTTP: h, BaseURL: DefaultBaseURL}
}

// Descriptor implements connector.Source.
func (c *Client) Descriptor() connector.Descriptor { return descriptor }

// Chamber values valid in CSI URLs.
var validChambers = map[string]bool{
	"House":  true,
	"Senate": true,
	"Joint":  true,
	"Agency": true,
}

func validateChamber(chamber string) error {
	if !validChambers[chamber] {
		return fmt.Errorf("csi: invalid chamber %q (want House|Senate|Joint|Agency)", chamber)
	}
	return nil
}

// ListCommittees fetches the chamber index page and parses committee IDs
// from <select id="SelectedCommitteeId">.
func (c *Client) ListCommittees(ctx context.Context, chamber string) ([]Committee, error) {
	if err := validateChamber(chamber); err != nil {
		return nil, err
	}
	u := c.BaseURL + "/" + chamber
	fetch, err := c.HTTP.Do(ctx, httpx.Request{
		System:   SystemName,
		Endpoint: "csi.chamber",
		URL:      u,
		Headers:  htmlAccept(),
	})
	if err != nil {
		return nil, err
	}
	committees, err := ParseChamberCommittees(fetch.Body)
	if err != nil {
		return nil, err
	}
	for i := range committees {
		committees[i].Chamber = chamber
	}
	return committees, nil
}

// ListMeetings hits /Home/GetMeetings and returns parsed meeting options.
func (c *Client) ListMeetings(ctx context.Context, chamber, committeeID string) ([]Meeting, error) {
	if err := validateChamber(chamber); err != nil {
		return nil, err
	}
	if committeeID == "" {
		return nil, errors.New("csi: committeeID required")
	}
	q := url.Values{}
	q.Set("chamber", chamber)
	q.Set("committeeId", committeeID)
	u := c.BaseURL + "/Home/GetMeetings?" + q.Encode()

	fetch, err := c.HTTP.Do(ctx, httpx.Request{
		System:   SystemName,
		Endpoint: "csi.GetMeetings",
		URL:      u,
		Headers:  ajaxJSON(),
	})
	if err != nil {
		return nil, err
	}
	return ParseMeetings(fetch.Body)
}

// ListAgendaItems hits /Home/GetAgendaItems and returns parsed buttons.
func (c *Client) ListAgendaItems(ctx context.Context, chamber, meetingFamilyID string) ([]AgendaItem, error) {
	if err := validateChamber(chamber); err != nil {
		return nil, err
	}
	if meetingFamilyID == "" {
		return nil, errors.New("csi: meetingFamilyID required")
	}
	q := url.Values{}
	q.Set("chamber", chamber)
	q.Set("meetingFamilyId", meetingFamilyID)
	u := c.BaseURL + "/Home/GetAgendaItems?" + q.Encode()

	fetch, err := c.HTTP.Do(ctx, httpx.Request{
		System:   SystemName,
		Endpoint: "csi.GetAgendaItems",
		URL:      u,
		Headers:  ajaxHTML(),
	})
	if err != nil {
		return nil, err
	}
	items, err := ParseAgendaItems(fetch.Body)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Chamber = chamber
	}
	return items, nil
}

// GetTestifiers hits /Home/GetOtherTestifiers and returns the two testifier
// tables flattened into one slice (with Testified true/false set per table).
func (c *Client) GetTestifiers(ctx context.Context, agendaItemID, agendaItemDescription string) ([]Testifier, error) {
	if agendaItemID == "" {
		return nil, errors.New("csi: agendaItemID required")
	}
	q := url.Values{}
	q.Set("agendaItemId", agendaItemID)
	q.Set("agendaItemDescription", agendaItemDescription)
	u := c.BaseURL + "/Home/GetOtherTestifiers?" + q.Encode()

	fetch, err := c.HTTP.Do(ctx, httpx.Request{
		System:   SystemName,
		Endpoint: "csi.GetOtherTestifiers",
		URL:      u,
		Headers:  ajaxHTML(),
	})
	if err != nil {
		return nil, err
	}
	return ParseTestifiers(fetch.Body)
}

func htmlAccept() http.Header {
	h := http.Header{}
	h.Set("Accept", "text/html,application/xhtml+xml")
	return h
}

func ajaxJSON() http.Header {
	h := http.Header{}
	h.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	h.Set("X-Requested-With", "XMLHttpRequest")
	return h
}

func ajaxHTML() http.Header {
	h := http.Header{}
	h.Set("Accept", "text/html, */*")
	h.Set("X-Requested-With", "XMLHttpRequest")
	return h
}

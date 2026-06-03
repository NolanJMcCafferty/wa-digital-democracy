//go:build integration
// +build integration

package lws_test

import (
	"context"
	"testing"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/lws"
)

// Hits live LWS endpoints. Run with:
//
//	go test -tags=integration ./internal/sources/lws/...
func TestLive_GetLegislation_HB1234(t *testing.T) {
	c := lws.New(httpx.New(httpx.Config{}))
	leg, err := c.GetLegislation(context.Background(), "2025-26", "1234")
	if err != nil {
		t.Fatalf("GetLegislation: %v", err)
	}
	if leg.BillID != "HB 1234" {
		t.Errorf("BillID = %q", leg.BillID)
	}
	if leg.LongDescription == "" {
		t.Errorf("LongDescription empty")
	}
}

func TestLive_GetSponsors_HB1234(t *testing.T) {
	c := lws.New(httpx.New(httpx.Config{}))
	sps, err := c.GetSponsors(context.Background(), "2025-26", "HB 1234")
	if err != nil {
		t.Fatalf("GetSponsors: %v", err)
	}
	if len(sps) == 0 {
		t.Fatal("no sponsors returned")
	}
}

func TestLive_GetHearings_HB1234(t *testing.T) {
	c := lws.New(httpx.New(httpx.Config{}))
	hs, err := c.GetHearings(context.Background(), "2025-26", "1234")
	if err != nil {
		t.Fatalf("GetHearings: %v", err)
	}
	if len(hs) == 0 {
		t.Fatal("no hearings returned")
	}
}

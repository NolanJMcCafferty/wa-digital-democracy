package main

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

func init() {
	registerRoute(route{Method: "GET", Path: "/addresses/suggest", Scope: scopeAPI, Plain: suggestAddressesHandler})
	registerRoute(route{Method: "GET", Path: "/legislators", Scope: scopeAPI, Store: listLegislatorsHandler})
	registerRoute(route{Method: "GET", Path: "/legislators/lookup", Scope: scopeAPI, Store: lookupLegislatorsByAddressHandler})
	registerRoute(route{Method: "GET", Path: "/legislators/{slug}", Scope: scopeAPI, Store: getLegislatorHandler})
}

func listLegislatorsHandler(store *db.Store) http.HandlerFunc {
	type item struct {
		Slug         string `json:"slug"`
		Name         string `json:"name"`         // "Senator Alvarado"
		DisplayName  string `json:"display_name"` // "Emily Alvarado"
		FirstName    string `json:"first_name,omitempty"`
		LastName     string `json:"last_name,omitempty"`
		Role         string `json:"role,omitempty"` // "State Senator" | "State Representative"
		Chamber      string `json:"chamber,omitempty"`
		District     string `json:"district,omitempty"`
		Party        string `json:"party,omitempty"`
		Email        string `json:"email,omitempty"`
		Phone        string `json:"phone,omitempty"`
		OfficialURL  string `json:"official_url,omitempty"`
		PhotoURL     string `json:"photo_url,omitempty"`
		ThumbnailURL string `json:"thumbnail_url,omitempty"`
		BillCount    int    `json:"bill_count"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		legs, err := store.ListLegislators(req.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		out := make([]item, 0, len(legs))
		for _, l := range legs {
			photoURL, thumbnailURL := legislatorPhotoURLs(l.LWSSponsorID)
			out = append(out, item{
				Slug:         slugify(l.Name), // unchanged so existing /legislators/{slug} routes still resolve
				Name:         l.Name,
				DisplayName:  strings.TrimSpace(l.FirstName + " " + l.LastName),
				FirstName:    l.FirstName,
				LastName:     l.LastName,
				Role:         legislatorRole(l.Chamber),
				Chamber:      l.Chamber,
				District:     l.District,
				Party:        l.Party,
				Email:        l.Email,
				Phone:        l.Phone,
				OfficialURL:  l.OfficialURL,
				PhotoURL:     photoURL,
				ThumbnailURL: thumbnailURL,
				BillCount:    l.BillCount,
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func suggestAddressesHandler() http.HandlerFunc {
	type suggestion struct {
		Text     string `json:"text"`
		MagicKey string `json:"magic_key,omitempty"`
	}
	type body struct {
		Query       string       `json:"query"`
		Suggestions []suggestion `json:"suggestions"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		query := strings.TrimSpace(req.URL.Query().Get("query"))
		if len(query) < 4 {
			writeJSON(w, http.StatusOK, body{Query: query, Suggestions: []suggestion{}})
			return
		}

		suggestions, err := suggestWashingtonAddresses(req.Context(), query, 6)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		out := make([]suggestion, 0, len(suggestions))
		for _, s := range suggestions {
			out = append(out, suggestion{Text: s.Text, MagicKey: s.MagicKey})
		}
		writeJSON(w, http.StatusOK, body{Query: query, Suggestions: out})
	}
}

func lookupLegislatorsByAddressHandler(store *db.Store) http.HandlerFunc {
	type legislatorItem struct {
		Slug         string `json:"slug"`
		Name         string `json:"name"`
		DisplayName  string `json:"display_name,omitempty"`
		Role         string `json:"role,omitempty"`
		Chamber      string `json:"chamber,omitempty"`
		District     string `json:"district,omitempty"`
		Party        string `json:"party,omitempty"`
		PhotoURL     string `json:"photo_url,omitempty"`
		ThumbnailURL string `json:"thumbnail_url,omitempty"`
		BillCount    int    `json:"bill_count"`
	}
	type body struct {
		QueryAddress   string           `json:"query_address"`
		MatchedAddress string           `json:"matched_address,omitempty"`
		District       string           `json:"district"`
		Legislators    []legislatorItem `json:"legislators"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		address := strings.TrimSpace(req.URL.Query().Get("address"))
		magicKey := strings.TrimSpace(req.URL.Query().Get("magic_key"))
		if len(address) < 5 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "address is required"})
			return
		}

		lookup, err := lookupLegislativeDistrict(req.Context(), address, magicKey)
		if err != nil {
			switch {
			case errors.Is(err, errAddressNotFound):
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "address not found"})
			case errors.Is(err, errDistrictNotFound):
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "legislative district not found"})
			default:
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			}
			return
		}

		legs, err := store.ListLegislatorsByDistrict(req.Context(), lookup.District)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		out := make([]legislatorItem, 0, len(legs))
		senators := 0
		representatives := 0
		for _, l := range legs {
			switch l.Chamber {
			case "Senate":
				if senators >= 1 {
					continue
				}
				senators++
			case "House":
				if representatives >= 2 {
					continue
				}
				representatives++
			}
			photoURL, thumbnailURL := legislatorPhotoURLs(l.LWSSponsorID)
			out = append(out, legislatorItem{
				Slug:         slugify(l.Name),
				Name:         l.Name,
				DisplayName:  strings.TrimSpace(l.FirstName + " " + l.LastName),
				Role:         legislatorRole(l.Chamber),
				Chamber:      l.Chamber,
				District:     l.District,
				Party:        l.Party,
				PhotoURL:     photoURL,
				ThumbnailURL: thumbnailURL,
				BillCount:    l.BillCount,
			})
		}
		writeJSON(w, http.StatusOK, body{
			QueryAddress:   address,
			MatchedAddress: lookup.MatchedAddress,
			District:       lookup.District,
			Legislators:    out,
		})
	}
}

func getLegislatorHandler(store *db.Store) http.HandlerFunc {
	type appearance struct {
		Biennium      string `json:"biennium"`
		BillID        string `json:"bill_id"`
		BillPrefix    string `json:"bill_prefix"`
		BillNumber    int    `json:"bill_number"`
		BillTitle     string `json:"bill_title,omitempty"`
		SponsorType   string `json:"sponsor_type,omitempty"`
		ChamberOrigin string `json:"chamber_origin,omitempty"`
		CurrentStatus string `json:"current_status,omitempty"`
		StatusBucket  string `json:"status_bucket,omitempty"`
		LeadSponsor   string `json:"lead_sponsor,omitempty"`
		LeadDisplay   string `json:"lead_display,omitempty"`
		LeadParty     string `json:"lead_party,omitempty"`
		LeadSlug      string `json:"lead_slug,omitempty"`
	}
	type body struct {
		Slug         string       `json:"slug"`
		Name         string       `json:"name"`
		DisplayName  string       `json:"display_name,omitempty"`
		FirstName    string       `json:"first_name,omitempty"`
		LastName     string       `json:"last_name,omitempty"`
		Chamber      string       `json:"chamber,omitempty"`
		District     string       `json:"district,omitempty"`
		Party        string       `json:"party,omitempty"`
		PhotoURL     string       `json:"photo_url,omitempty"`
		ThumbnailURL string       `json:"thumbnail_url,omitempty"`
		Appearances  []appearance `json:"appearances"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		slug := chi.URLParam(req, "slug")
		legs, err := store.ListLegislators(req.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		var match *db.LegislatorAggregate
		for i := range legs {
			if slugify(legs[i].Name) == slug {
				match = &legs[i]
				break
			}
		}
		if match == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "legislator not found"})
			return
		}
		bills, err := store.GetLegislatorBills(req.Context(), match.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		apps := make([]appearance, 0, len(bills))
		for _, b := range bills {
			display := strings.TrimSpace(b.LeadFirstName + " " + b.LeadLastName)
			apps = append(apps, appearance{
				Biennium:      b.Biennium,
				BillID:        b.BillID,
				BillPrefix:    b.BillPrefix,
				BillNumber:    b.BillNumber,
				BillTitle:     b.BillTitle,
				SponsorType:   b.SponsorType,
				ChamberOrigin: b.ChamberOrigin,
				CurrentStatus: b.CurrentStatus,
				StatusBucket:  bucketStatus(b.CurrentStatus),
				LeadSponsor:   b.LeadSponsor,
				LeadDisplay:   display,
				LeadParty:     b.LeadParty,
				LeadSlug:      slugify(b.LeadSponsor),
			})
		}
		display := strings.TrimSpace(match.FirstName + " " + match.LastName)
		photoURL, thumbnailURL := legislatorPhotoURLs(match.LWSSponsorID)
		writeJSON(w, http.StatusOK, body{
			Slug:         slug,
			Name:         match.Name,
			DisplayName:  display,
			FirstName:    match.FirstName,
			LastName:     match.LastName,
			Chamber:      match.Chamber,
			District:     match.District,
			Party:        match.Party,
			PhotoURL:     photoURL,
			ThumbnailURL: thumbnailURL,
			Appearances:  apps,
		})
	}
}

func normalizeDistrict(s string) string {
	var digits strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	out := strings.TrimLeft(digits.String(), "0")
	if out == "" && digits.Len() > 0 {
		return "0"
	}
	return out
}

func legislatorRole(chamber string) string {
	switch chamber {
	case "Senate":
		return "State Senator"
	case "House":
		return "State Representative"
	default:
		return "Legislator"
	}
}

func legislatorPhotoURLs(lwsSponsorID string) (string, string) {
	id := strings.TrimSpace(lwsSponsorID)
	if id == "" {
		return "", ""
	}
	escaped := url.PathEscape(id)
	return "https://leg.wa.gov/memberphoto/" + escaped + ".jpg",
		"https://leg.wa.gov/memberthumbnail/" + escaped + ".jpg"
}

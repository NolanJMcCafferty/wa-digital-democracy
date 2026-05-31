package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	errAddressNotFound  = errors.New("address not found")
	errDistrictNotFound = errors.New("legislative district not found")
	districtHTTPClient  = &http.Client{Timeout: 12 * time.Second}
)

const (
	censusGeocoderURL = "https://geocoding.geo.census.gov/geocoder/locations/onelineaddress"
	esriSuggestURL    = "https://geocode.arcgis.com/arcgis/rest/services/World/GeocodeServer/suggest"
	esriCandidateURL  = "https://geocode.arcgis.com/arcgis/rest/services/World/GeocodeServer/findAddressCandidates"
	legDistrictURL    = "https://services7.arcgis.com/zJ5hF9SNB8WMMiGf/ArcGIS/rest/services/Legislative_Districts/FeatureServer/0/query"
	washingtonExtent  = "-124.85,45.54,-116.91,49.01"
)

type districtLookupResult struct {
	District       string
	MatchedAddress string
}

func lookupLegislativeDistrict(ctx context.Context, address, magicKey string) (districtLookupResult, error) {
	if result, ok := e2eDistrictLookup(address); ok {
		return result, nil
	}
	point, err := geocodeAddress(ctx, address, magicKey)
	if err != nil {
		return districtLookupResult{}, err
	}
	district, err := legislativeDistrictForPoint(ctx, point.lon, point.lat)
	if err != nil {
		return districtLookupResult{}, err
	}
	return districtLookupResult{District: district, MatchedAddress: point.matchedAddress}, nil
}

type geocodedPoint struct {
	lon            float64
	lat            float64
	matchedAddress string
}

func e2eDistrictLookup(address string) (districtLookupResult, bool) {
	if os.Getenv("WADD_E2E_ADDRESS_LOOKUP") != "1" {
		return districtLookupResult{}, false
	}
	if !strings.Contains(strings.ToLower(address), "600 4th ave") {
		return districtLookupResult{}, false
	}
	return districtLookupResult{
		District:       "99",
		MatchedAddress: "600 4th Ave, Seattle, WA 98104",
	}, true
}

type addressSuggestion struct {
	Text     string
	MagicKey string
}

func suggestWashingtonAddresses(ctx context.Context, query string, limit int) ([]addressSuggestion, error) {
	var suggestions []addressSuggestion
	seen := map[string]bool{}
	for _, variant := range washingtonAddressQueryVariants(query) {
		next, err := suggestWashingtonAddressVariant(ctx, variant, limit)
		if err != nil {
			return nil, err
		}
		for _, suggestion := range next {
			if !strings.Contains(suggestion.Text, ", WA,") || seen[suggestion.Text] {
				continue
			}
			seen[suggestion.Text] = true
			suggestions = append(suggestions, suggestion)
			if len(suggestions) >= limit {
				return suggestions, nil
			}
		}
	}
	if suggestions == nil {
		suggestions = []addressSuggestion{}
	}
	return suggestions, nil
}

func suggestWashingtonAddressVariant(ctx context.Context, query string, limit int) ([]addressSuggestion, error) {
	u, err := url.Parse(esriSuggestURL)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("f", "json")
	q.Set("text", query)
	q.Set("countryCode", "USA")
	q.Set("category", "Address")
	q.Set("searchExtent", washingtonExtent)
	q.Set("maxSuggestions", strconv.Itoa(limit))
	u.RawQuery = q.Encode()

	var body struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Suggestions []struct {
			Text     string `json:"text"`
			MagicKey string `json:"magicKey"`
		} `json:"suggestions"`
	}
	if err := fetchJSON(ctx, u.String(), &body); err != nil {
		return nil, fmt.Errorf("suggest address: %w", err)
	}
	if body.Error != nil {
		return nil, fmt.Errorf("suggest address: %s", body.Error.Message)
	}
	out := make([]addressSuggestion, 0, len(body.Suggestions))
	for _, s := range body.Suggestions {
		out = append(out, addressSuggestion{Text: s.Text, MagicKey: s.MagicKey})
	}
	return out, nil
}

func washingtonAddressQueryVariants(query string) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	variants := []string{query}
	lower := strings.ToLower(query)
	if !strings.Contains(lower, " wa") && !strings.Contains(lower, "washington") {
		variants = append(variants, query+" WA")
	}
	return variants
}

func geocodeAddress(ctx context.Context, address, magicKey string) (geocodedPoint, error) {
	if magicKey != "" {
		return geocodeAddressWithESRI(ctx, address, magicKey)
	}
	point, err := geocodeAddressWithCensus(ctx, address)
	if err == nil {
		return point, nil
	}
	if errors.Is(err, errAddressNotFound) {
		return geocodeAddressWithESRI(ctx, address, "")
	}
	return geocodedPoint{}, err
}

func geocodeAddressWithCensus(ctx context.Context, address string) (geocodedPoint, error) {
	u, err := url.Parse(censusGeocoderURL)
	if err != nil {
		return geocodedPoint{}, err
	}
	q := u.Query()
	q.Set("address", address)
	q.Set("benchmark", "Public_AR_Current")
	q.Set("format", "json")
	u.RawQuery = q.Encode()

	var body struct {
		Result struct {
			AddressMatches []struct {
				MatchedAddress string `json:"matchedAddress"`
				Coordinates    struct {
					X float64 `json:"x"`
					Y float64 `json:"y"`
				} `json:"coordinates"`
			} `json:"addressMatches"`
		} `json:"result"`
	}
	if err := fetchJSON(ctx, u.String(), &body); err != nil {
		return geocodedPoint{}, fmt.Errorf("geocode address: %w", err)
	}
	if len(body.Result.AddressMatches) == 0 {
		return geocodedPoint{}, errAddressNotFound
	}
	match := body.Result.AddressMatches[0]
	return geocodedPoint{
		lon:            match.Coordinates.X,
		lat:            match.Coordinates.Y,
		matchedAddress: match.MatchedAddress,
	}, nil
}

func geocodeAddressWithESRI(ctx context.Context, address, magicKey string) (geocodedPoint, error) {
	u, err := url.Parse(esriCandidateURL)
	if err != nil {
		return geocodedPoint{}, err
	}
	q := u.Query()
	q.Set("f", "json")
	q.Set("singleLine", address)
	q.Set("countryCode", "USA")
	q.Set("category", "Address")
	q.Set("searchExtent", washingtonExtent)
	q.Set("outFields", "Match_addr,Region")
	q.Set("maxLocations", "1")
	if magicKey != "" {
		q.Set("magicKey", magicKey)
	}
	u.RawQuery = q.Encode()

	var body struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Candidates []struct {
			Address  string  `json:"address"`
			Score    float64 `json:"score"`
			Location struct {
				X float64 `json:"x"`
				Y float64 `json:"y"`
			} `json:"location"`
			Attributes struct {
				Region string `json:"Region"`
			} `json:"attributes"`
		} `json:"candidates"`
	}
	if err := fetchJSON(ctx, u.String(), &body); err != nil {
		return geocodedPoint{}, fmt.Errorf("geocode address: %w", err)
	}
	if body.Error != nil {
		return geocodedPoint{}, fmt.Errorf("geocode address: %s", body.Error.Message)
	}
	if len(body.Candidates) == 0 {
		return geocodedPoint{}, errAddressNotFound
	}
	match := body.Candidates[0]
	if match.Score < 80 || !strings.EqualFold(match.Attributes.Region, "Washington") {
		return geocodedPoint{}, errAddressNotFound
	}
	return geocodedPoint{
		lon:            match.Location.X,
		lat:            match.Location.Y,
		matchedAddress: match.Address,
	}, nil
}

func legislativeDistrictForPoint(ctx context.Context, lon, lat float64) (string, error) {
	u, err := url.Parse(legDistrictURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("f", "json")
	q.Set("geometry", fmt.Sprintf("%.8f,%.8f", lon, lat))
	q.Set("geometryType", "esriGeometryPoint")
	q.Set("inSR", "4326")
	q.Set("spatialRel", "esriSpatialRelIntersects")
	q.Set("outFields", "DISTRICT,DISTRICTN")
	q.Set("returnGeometry", "false")
	u.RawQuery = q.Encode()

	var body struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Features []struct {
			Attributes struct {
				District  string  `json:"DISTRICT"`
				DistrictN float64 `json:"DISTRICTN"`
			} `json:"attributes"`
		} `json:"features"`
	}
	if err := fetchJSON(ctx, u.String(), &body); err != nil {
		return "", fmt.Errorf("district lookup: %w", err)
	}
	if body.Error != nil {
		return "", fmt.Errorf("district lookup: %s", body.Error.Message)
	}
	if len(body.Features) == 0 {
		return "", errDistrictNotFound
	}
	attrs := body.Features[0].Attributes
	district := normalizeDistrict(attrs.District)
	if district == "" && attrs.DistrictN > 0 {
		district = strconv.Itoa(int(attrs.DistrictN))
	}
	if district == "" {
		return "", errDistrictNotFound
	}
	return district, nil
}

func fetchJSON(ctx context.Context, endpoint string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "wa-digital-democracy/0.1 (+https://github.com/nolan-mccafferty/wa-digital-democracy)")
	res, err := districtHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return fmt.Errorf("status %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(target)
}

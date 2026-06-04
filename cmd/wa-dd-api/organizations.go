package main

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

func init() {
	registerRoute(route{Method: "GET", Path: "/organizations", Scope: scopeAPI, Store: listOrganizationsHandler})
	registerRoute(route{Method: "GET", Path: "/organizations/{slug}", Scope: scopeAPI, Store: getOrganizationHandler})
	registerRoute(route{Method: "GET", Path: "/organizations/{slug}/speech", Scope: scopeAPI, Store: getOrganizationSpeechHandler})
}

func listOrganizationsHandler(store *db.Store) http.HandlerFunc {
	type item struct {
		Slug            string         `json:"slug"`
		CanonicalName   string         `json:"canonical_name"`
		Aliases         []string       `json:"aliases"`
		MatchConfidence string         `json:"match_confidence"`
		MatchNotes      string         `json:"match_notes,omitempty"`
		TestifierCount  int            `json:"testifier_count"`
		Positions       map[string]int `json:"positions"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		topicKeywords := req.URL.Query()["topic_keyword"]
		orgs, err := store.ListOrganizations(req.Context(), topicKeywords)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		out := make([]item, 0, len(orgs))
		for _, o := range orgs {
			aliases := o.Aliases
			if aliases == nil {
				aliases = []string{}
			}
			out = append(out, item{
				Slug:            slugify(o.CanonicalName),
				CanonicalName:   o.CanonicalName,
				Aliases:         aliases,
				MatchConfidence: o.MatchConfidence,
				MatchNotes:      o.MatchNotes,
				TestifierCount:  o.TestifierCount,
				Positions: map[string]int{
					"Pro":     o.ProCount,
					"Con":     o.ConCount,
					"Other":   o.OtherCount,
					"Unknown": o.UnknownCount,
				},
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func getOrganizationHandler(store *db.Store) http.HandlerFunc {
	type appearance struct {
		Biennium        string    `json:"biennium"`
		BillID          string    `json:"bill_id"`
		BillPrefix      string    `json:"bill_prefix"`
		BillNumber      int       `json:"bill_number"`
		CSIAgendaItemID string    `json:"csi_agenda_item_id,omitempty"`
		HearingID       int64     `json:"hearing_id"`
		HearingTitle    string    `json:"hearing_title"`
		CommitteeName   string    `json:"committee_name"`
		Chamber         string    `json:"chamber,omitempty"`
		MeetingDateTime time.Time `json:"meeting_datetime"`
		Position        string    `json:"position,omitempty"`
		TestifierCount  int       `json:"testifier_count"`
	}
	type publicContext struct {
		ContextType     string   `json:"context_type"`
		SourceKind      string   `json:"source_kind"`
		SourceLabel     string   `json:"source_label"`
		SourceName      string   `json:"source_name"`
		Detail          string   `json:"detail,omitempty"`
		Amount          string   `json:"amount,omitempty"`
		RecordYear      int      `json:"record_year,omitempty"`
		RecordDate      string   `json:"record_date,omitempty"`
		URL             string   `json:"url,omitempty"`
		MatchConfidence string   `json:"match_confidence"`
		Evidence        []string `json:"evidence"`
	}
	type personAffiliation struct {
		PersonID            int64  `json:"person_id,omitempty"`
		PersonName          string `json:"person_name"`
		RelationshipType    string `json:"relationship_type"`
		RoleTitle           string `json:"role_title,omitempty"`
		SourceKind          string `json:"source_kind"`
		SourceLabel         string `json:"source_label"`
		RawOrganizationName string `json:"raw_organization_name,omitempty"`
		RecordYears         string `json:"record_years,omitempty"`
		SourceCount         int    `json:"source_count"`
		Confidence          string `json:"confidence"`
	}
	type body struct {
		Slug               string              `json:"slug"`
		CanonicalName      string              `json:"canonical_name"`
		Aliases            []string            `json:"aliases"`
		MatchConfidence    string              `json:"match_confidence"`
		MatchNotes         string              `json:"match_notes,omitempty"`
		TestifierCount     int                 `json:"testifier_count"`
		Positions          map[string]int      `json:"positions"`
		Appearances        []appearance        `json:"appearances"`
		Contexts           []publicContext     `json:"contexts"`
		PersonAffiliations []personAffiliation `json:"person_affiliations"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		slug := chi.URLParam(req, "slug")
		orgs, err := store.ListOrganizations(req.Context(), nil)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		var match *db.OrganizationAggregate
		for i := range orgs {
			if slugify(orgs[i].CanonicalName) == slug {
				match = &orgs[i]
				break
			}
		}
		if match == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "organization not found"})
			return
		}
		appsRaw, err := store.GetOrganizationAppearances(req.Context(), match.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		contextsRaw, err := store.GetOrganizationPublicContexts(req.Context(), match.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		affiliationsRaw, err := store.GetOrganizationPersonAffiliations(req.Context(), match.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		apps := make([]appearance, 0, len(appsRaw))
		for _, a := range appsRaw {
			apps = append(apps, appearance{
				Biennium: a.Biennium, BillID: a.BillID,
				BillPrefix: a.BillPrefix, BillNumber: a.BillNumber,
				CSIAgendaItemID: a.CSIAgendaItemID,
				HearingID:       a.HearingID,
				HearingTitle:    a.HearingTitle,
				CommitteeName:   a.CommitteeName,
				Chamber:         a.Chamber,
				MeetingDateTime: a.MeetingDateTime,
				Position:        a.Position,
				TestifierCount:  a.TestifierCount,
			})
		}
		contexts := make([]publicContext, 0, len(contextsRaw))
		for _, c := range contextsRaw {
			evidence := c.Evidence
			if evidence == nil {
				evidence = []string{}
			}
			contexts = append(contexts, publicContext{
				ContextType:     c.ContextType,
				SourceKind:      c.SourceKind,
				SourceLabel:     c.SourceLabel,
				SourceName:      c.SourceName,
				Detail:          c.Detail,
				Amount:          c.Amount,
				RecordYear:      c.RecordYear,
				RecordDate:      c.RecordDate,
				URL:             c.URL,
				MatchConfidence: c.MatchConfidence,
				Evidence:        evidence,
			})
		}
		affiliations := make([]personAffiliation, 0, len(affiliationsRaw))
		for _, a := range affiliationsRaw {
			affiliations = append(affiliations, personAffiliation{
				PersonID:            a.PersonID,
				PersonName:          a.PersonName,
				RelationshipType:    a.RelationshipType,
				RoleTitle:           a.RoleTitle,
				SourceKind:          a.SourceKind,
				SourceLabel:         a.SourceLabel,
				RawOrganizationName: a.RawOrganizationName,
				RecordYears:         a.RecordYears,
				SourceCount:         a.SourceCount,
				Confidence:          a.Confidence,
			})
		}
		aliases := match.Aliases
		if aliases == nil {
			aliases = []string{}
		}
		writeJSON(w, http.StatusOK, body{
			Slug:            slug,
			CanonicalName:   match.CanonicalName,
			Aliases:         aliases,
			MatchConfidence: match.MatchConfidence,
			MatchNotes:      match.MatchNotes,
			TestifierCount:  match.TestifierCount,
			Positions: map[string]int{
				"Pro": match.ProCount, "Con": match.ConCount,
				"Other": match.OtherCount, "Unknown": match.UnknownCount,
			},
			Appearances:        apps,
			Contexts:           contexts,
			PersonAffiliations: affiliations,
		})
	}
}

func getOrganizationSpeechHandler(store *db.Store) http.HandlerFunc {
	type speechSegment struct {
		SegmentID        int64      `json:"segment_id"`
		TVWEventID       string     `json:"tvw_event_id"`
		StartMS          int        `json:"start_ms"`
		EndMS            int        `json:"end_ms"`
		Text             string     `json:"text"`
		ClusterLabel     string     `json:"cluster_label"`
		SpeakerLabel     string     `json:"speaker_label"`
		SpeakerKind      string     `json:"speaker_kind"`
		TestifierID      int64      `json:"testifier_id"`
		TestifierName    string     `json:"testifier_name"`
		OrganizationName string     `json:"organization_name"`
		ReviewStatus     string     `json:"review_status"`
		ReviewedAt       *time.Time `json:"reviewed_at,omitempty"`
		Reviewer         string     `json:"reviewer,omitempty"`
	}
	type body struct {
		OrganizationSlug string          `json:"organization_slug"`
		OrganizationName string          `json:"organization_name"`
		Total            int             `json:"total"`
		Limit            int             `json:"limit"`
		Offset           int             `json:"offset"`
		Segments         []speechSegment `json:"segments"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		slug := chi.URLParam(req, "slug")

		// Find organization by slug
		orgs, err := store.ListOrganizations(req.Context(), nil)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		var orgID int64
		var orgName string
		for _, org := range orgs {
			if slugify(org.CanonicalName) == slug {
				orgID = org.ID
				orgName = org.CanonicalName
				break
			}
		}
		if orgID == 0 {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "organization not found"})
			return
		}

		// Parse pagination parameters
		limit := 50
		if v := req.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
				limit = n
			}
		}
		offset := 0
		if v := req.URL.Query().Get("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				offset = n
			}
		}

		// Get organization-attributed speech segments
		segs, err := store.ListOrganizationAttributedSegments(req.Context(), orgID, limit, offset)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		// Map to response format
		out := body{
			OrganizationSlug: slug,
			OrganizationName: orgName,
			Total:            len(segs), // TODO: Implement proper count query
			Limit:            limit,
			Offset:           offset,
			Segments:         make([]speechSegment, 0, len(segs)),
		}

		for _, seg := range segs {
			out.Segments = append(out.Segments, speechSegment{
				SegmentID:        seg.SegmentID,
				TVWEventID:       seg.TVWEventID,
				StartMS:          seg.StartMS,
				EndMS:            seg.EndMS,
				Text:             seg.Text,
				ClusterLabel:     seg.ClusterLabel,
				SpeakerLabel:     seg.SpeakerLabel,
				SpeakerKind:      seg.SpeakerKind,
				TestifierID:      seg.TestifierID,
				TestifierName:    seg.TestifierName,
				OrganizationName: seg.OrganizationName,
				ReviewStatus:     seg.ReviewStatus,
				ReviewedAt:       seg.ReviewedAt,
				Reviewer:         seg.Reviewer,
			})
		}

		writeJSON(w, http.StatusOK, out)
	}
}

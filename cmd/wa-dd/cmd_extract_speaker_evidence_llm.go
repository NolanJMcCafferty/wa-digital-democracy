package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/diarization"
	waddllm "github.com/nolan-mccafferty/wa-digital-democracy/internal/llm"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

func init() {
	register(Command{
		Name:     "extract-speaker-evidence-llm",
		Synopsis: "Generate reviewable LLM speaker identity tasks from diarization text",
		Run:      runExtractSpeakerEvidenceLLM,
	})
}

func runExtractSpeakerEvidenceLLM(args []string) int {
	fs := flag.NewFlagSet("extract-speaker-evidence-llm", flag.ContinueOnError)
	var (
		eventID  = fs.String("event-id", "", "TVW/Invintus event ID")
		jobID    = fs.Int64("job-id", 0, "diarization_job id")
		dsn      = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		limit    = fs.Int("limit", 0, "optional max diarized segments to scan")
		model    = fs.String("model", env("OPENROUTER_MODEL", ""), "OpenRouter model override (defaults to openai/gpt-5.5)")
		provider = fs.String("provider", env("LLM_PROVIDER", "anthropic"), "LLM provider: 'anthropic' or 'openrouter'")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *eventID == "" || *jobID == 0 {
		fmt.Fprintln(os.Stderr, "extract-speaker-evidence-llm: --event-id and --job-id are required")
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	store, err := db.Open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "extract-speaker-evidence-llm: db open: %v\n", err)
		return 1
	}
	defer store.Close()

	var client diarization.SpeakerEvidenceLLMClient

	switch strings.ToLower(*provider) {
	case "anthropic":
		var clientErr error
		client, clientErr = waddllm.NewAnthropicClient(waddllm.AnthropicConfig{
			APIKey: env("ANTHROPIC_API_KEY", ""),
			Model:  *model,
		})
		err = clientErr
	case "openrouter":
		var clientErr error
		client, clientErr = waddllm.NewOpenRouterClient(waddllm.OpenRouterConfig{
			APIKey: env("OPENROUTER_API_KEY", ""),
			Model:  *model,
		})
		err = clientErr
	default:
		fmt.Fprintf(os.Stderr, "extract-speaker-evidence-llm: unsupported provider '%s', must be 'anthropic' or 'openrouter'\n", *provider)
		return 2
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "extract-speaker-evidence-llm: %v\n", err)
		return 1
	}
	stats, err := extractSpeakerEvidenceLLMForJob(ctx, store, client, *jobID, *eventID, *limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "extract-speaker-evidence-llm: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "extract-speaker-evidence-llm: scanned %d segments, upserted %d evidence rows and %d review tasks (cache_create_tokens=%d cache_read_tokens=%d)\n",
		stats.SegmentsScanned, stats.EvidenceRows, stats.ReviewTasks, stats.Usage.CacheCreationInputTokens, stats.Usage.CacheReadInputTokens)
	return 0
}

type speakerEvidenceLLMRunStats struct {
	SegmentsScanned int
	EvidenceRows    int
	ReviewTasks     int
	Usage           diarization.SpeakerEvidenceLLMUsage
}

func extractSpeakerEvidenceLLMForJob(ctx context.Context, store *db.Store, client diarization.SpeakerEvidenceLLMClient, jobID int64, eventID string, limit int) (speakerEvidenceLLMRunStats, error) {
	segments, err := loadDiarizedSegmentsForEvidence(ctx, store, jobID, eventID, limit)
	if err != nil {
		return speakerEvidenceLLMRunStats{}, fmt.Errorf("load segments: %w", err)
	}
	agenda, err := loadSpeakerEvidenceAgenda(ctx, store, eventID)
	if err != nil {
		return speakerEvidenceLLMRunStats{}, fmt.Errorf("load agenda context: %w", err)
	}
	llmSegments := make([]diarization.SpeakerEvidenceLLMSegment, 0, len(segments))
	for _, seg := range segments {
		llmSegments = append(llmSegments, diarization.SpeakerEvidenceLLMSegment{
			ID:               seg.ID,
			SpeakerClusterID: seg.SpeakerClusterID,
			ClusterLabel:     seg.ClusterLabel,
			StartMS:          seg.StartMS,
			EndMS:            seg.EndMS,
			Text:             seg.Text,
		})
	}
	matches, usage, err := diarization.ExtractSpeakerEvidenceLLM(ctx, client, llmSegments, agenda)
	if err != nil {
		return speakerEvidenceLLMRunStats{SegmentsScanned: len(segments), Usage: usage}, err
	}
	stats := speakerEvidenceLLMRunStats{SegmentsScanned: len(segments), Usage: usage}
	for _, match := range matches {
		cand := match.Candidate
		evidenceKey := fmt.Sprintf("job:%d:seg:%d:%s:%s:%d:%s", jobID, match.Segment.ID, cand.EvidenceType, cand.Kind, cand.CandidateID, strings.ToLower(cand.Label))
		eid, err := store.UpsertSpeakerIdentityEvidence(ctx, db.SpeakerIdentityEvidenceParams{
			EvidenceKey:             evidenceKey,
			DiarizationJobID:        jobID,
			SpeakerClusterID:        match.Segment.SpeakerClusterID,
			DiarizedSpeechSegmentID: match.Segment.ID,
			EvidenceType:            cand.EvidenceType,
			EvidenceText:            cand.EvidenceText,
			CandidateKind:           cand.Kind,
			CandidateID:             cand.CandidateID,
			CandidateLabel:          cand.Label,
			Confidence:              cand.Confidence,
			StartMS:                 match.Segment.StartMS,
			EndMS:                   match.Segment.EndMS,
			Raw: map[string]any{
				"extractor":       "llm",
				"llm_result":      match.Raw,
				"reasoning":       cand.Reasoning,
				"cache_created":   usage.CacheCreationInputTokens,
				"cache_read":      usage.CacheReadInputTokens,
				"extracted_label": match.Raw.CandidateLabel,
			},
		})
		if err != nil {
			return stats, fmt.Errorf("store evidence: %w", err)
		}
		stats.EvidenceRows++
		priority := speakerEvidenceLLMPriority(cand)
		if _, err := store.UpsertSpeakerReviewTask(ctx, db.SpeakerReviewTaskParams{
			DiarizationJobID: jobID,
			SpeakerClusterID: match.Segment.SpeakerClusterID,
			Priority:         priority,
			CandidateKind:    cand.Kind,
			CandidateID:      cand.CandidateID,
			CandidateLabel:   cand.Label,
			Confidence:       cand.Confidence,
			EvidenceIDs:      []int64{eid},
		}); err != nil {
			return stats, fmt.Errorf("store review task: %w", err)
		}
		stats.ReviewTasks++
	}
	return stats, nil
}

func runSpeakerEvidenceLLMIfConfigured(ctx context.Context, store *db.Store, jobID int64, eventID string) {
	var client diarization.SpeakerEvidenceLLMClient
	var err error
	var providerUsed string

	// Try Anthropic first (preferred)
	if env("ANTHROPIC_API_KEY", "") != "" {
		client, err = waddllm.NewAnthropicClient(waddllm.AnthropicConfig{
			APIKey: env("ANTHROPIC_API_KEY", ""),
			Model:  env("ANTHROPIC_MODEL", ""),
		})
		providerUsed = "anthropic"
	} else if env("OPENROUTER_API_KEY", "") != "" {
		// Fall back to OpenRouter
		client, err = waddllm.NewOpenRouterClient(waddllm.OpenRouterConfig{
			APIKey: env("OPENROUTER_API_KEY", ""),
			Model:  env("OPENROUTER_MODEL", ""),
		})
		providerUsed = "openrouter"
	} else {
		fmt.Fprintln(os.Stderr, "speaker-evidence-llm: neither ANTHROPIC_API_KEY nor OPENROUTER_API_KEY set; skipping LLM evidence extraction")
		return
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "speaker-evidence-llm: %s client setup failed: %v\n", providerUsed, err)
		return
	}

	stats, err := extractSpeakerEvidenceLLMForJob(ctx, store, client, jobID, eventID, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "speaker-evidence-llm: %s extraction failed for job_id=%d: %v\n", providerUsed, jobID, err)
		return
	}
	fmt.Fprintf(os.Stderr, "speaker-evidence-llm (%s): scanned %d segments, upserted %d evidence rows and %d review tasks (cache_create_tokens=%d cache_read_tokens=%d)\n",
		providerUsed, stats.SegmentsScanned, stats.EvidenceRows, stats.ReviewTasks, stats.Usage.CacheCreationInputTokens, stats.Usage.CacheReadInputTokens)
}

func speakerEvidenceLLMPriority(cand diarization.SpeakerEvidenceCandidate) int {
	priority := 80 + int(cand.Confidence*20)
	if cand.Kind == "legislator" {
		priority += 40
	}
	if cand.CandidateID != 0 {
		priority += 20
	}
	return priority
}

func loadSpeakerEvidenceAgenda(ctx context.Context, store *db.Store, eventID string) (diarization.SpeakerEvidenceAgenda, error) {
	agenda := diarization.SpeakerEvidenceAgenda{TVWEventID: eventID}
	if err := loadSpeakerEvidenceHearings(ctx, store, eventID, &agenda); err != nil {
		return agenda, err
	}
	if err := loadSpeakerEvidenceLegislators(ctx, store, &agenda); err != nil {
		return agenda, err
	}
	if err := loadSpeakerEvidenceTestifiers(ctx, store, eventID, &agenda); err != nil {
		return agenda, err
	}
	if err := loadSpeakerEvidenceBillsAndSponsors(ctx, store, eventID, &agenda); err != nil {
		return agenda, err
	}
	return agenda, nil
}

func loadSpeakerEvidenceHearings(ctx context.Context, store *db.Store, eventID string, agenda *diarization.SpeakerEvidenceAgenda) error {
	const q = `
SELECT DISTINCT h.id, h.committee_name, h.chamber
  FROM hearing h
 WHERE h.tvw_event_id = $1
 ORDER BY h.id;`
	rows, err := store.Pool.Query(ctx, q, eventID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var h diarization.SpeakerEvidenceHearing
		if err := rows.Scan(&h.HearingID, &h.CommitteeName, &h.Chamber); err != nil {
			return err
		}
		agenda.Hearings = append(agenda.Hearings, h)
	}
	return rows.Err()
}

func loadSpeakerEvidenceLegislators(ctx context.Context, store *db.Store, agenda *diarization.SpeakerEvidenceAgenda) error {
	const q = `
WITH active_biennium AS (
  SELECT COALESCE(MAX(biennium), '2025-26') AS biennium FROM legislator_roster_membership
)
SELECT l.id,
       COALESCE(NULLIF(lrm.roster_name, ''), l.name),
       COALESCE(lrm.first_name, l.first_name, ''),
       COALESCE(lrm.last_name, l.last_name, ''),
       COALESCE(lrm.chamber, l.chamber, ''),
       COALESCE(lrm.district, l.district, ''),
       COALESCE(lrm.party, l.party, '')
  FROM legislator_roster_membership lrm
  JOIN active_biennium ab ON ab.biennium = lrm.biennium
  JOIN legislator l ON l.lws_sponsor_id = lrm.lws_sponsor_id
 ORDER BY lrm.chamber, lrm.district, lrm.last_name, lrm.first_name;`
	rows, err := store.Pool.Query(ctx, q)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var ref diarization.SpeakerEvidenceRef
		var first, last string
		if err := rows.Scan(&ref.ID, &ref.Label, &first, &last, &ref.Chamber, &ref.District, &ref.Party); err != nil {
			return err
		}
		ref.Role = legislatorRoleForPrompt(ref.Chamber)
		ref.Aliases = legislatorAliases(ref.Label, first, last, ref.Chamber)
		agenda.Legislators = append(agenda.Legislators, ref)
	}
	return rows.Err()
}

func loadSpeakerEvidenceTestifiers(ctx context.Context, store *db.Store, eventID string, agenda *diarization.SpeakerEvidenceAgenda) error {
	const q = `
SELECT t.id, t.raw_name, COALESCE(t.raw_organization, ''), t.position::text,
       t.testified, a.id, a.label
  FROM testifier t
  JOIN agenda_item a ON a.id = t.agenda_item_id
  JOIN hearing h ON h.id = a.hearing_id
 WHERE h.tvw_event_id = $1
 ORDER BY a.order_index NULLS LAST, t.time_signed_in NULLS LAST, t.id;`
	rows, err := store.Pool.Query(ctx, q, eventID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var ref diarization.SpeakerEvidenceRef
		if err := rows.Scan(&ref.ID, &ref.Label, &ref.Organization, &ref.Position, &ref.Testified, &ref.AgendaItemID, &ref.AgendaLabel); err != nil {
			return err
		}
		ref.Role = "CSI testifier"
		ref.Aliases = []string{ref.Label}
		agenda.Testifiers = append(agenda.Testifiers, ref)
	}
	return rows.Err()
}

func loadSpeakerEvidenceBillsAndSponsors(ctx context.Context, store *db.Store, eventID string, agenda *diarization.SpeakerEvidenceAgenda) error {
	const billsQ = `
SELECT DISTINCT b.id, b.bill_number, COALESCE(b.title, '')
  FROM hearing h
  JOIN agenda_item a ON a.hearing_id = h.id
  JOIN bill b ON b.id = a.bill_id
 WHERE h.tvw_event_id = $1
 ORDER BY b.bill_number;`
	billRows, err := store.Pool.Query(ctx, billsQ, eventID)
	if err != nil {
		return err
	}
	defer billRows.Close()
	for billRows.Next() {
		var bill diarization.SpeakerEvidenceBill
		if err := billRows.Scan(&bill.ID, &bill.Number, &bill.Title); err != nil {
			return err
		}
		agenda.Bills = append(agenda.Bills, bill)
	}
	if err := billRows.Err(); err != nil {
		return err
	}

	const sponsorsQ = `
SELECT DISTINCT b.id, b.bill_number,
       COALESCE(l_legacy.id, l_from_membership.id, 0) AS legislator_id,
       COALESCE(lrm.roster_name, l_legacy.name, l_from_membership.name, ''),
       bs.sponsor_type
  FROM hearing h
  JOIN agenda_item a ON a.hearing_id = h.id
  JOIN bill b ON b.id = a.bill_id
  JOIN bill_sponsor bs ON bs.bill_id = b.id
  LEFT JOIN legislator l_legacy ON l_legacy.id = bs.legislator_id
  LEFT JOIN legislator_roster_membership lrm ON lrm.id = bs.legislator_roster_membership_id
  LEFT JOIN legislator l_from_membership ON l_from_membership.lws_sponsor_id = lrm.lws_sponsor_id
 WHERE h.tvw_event_id = $1
 ORDER BY b.bill_number, bs.sponsor_type;`
	sponsorRows, err := store.Pool.Query(ctx, sponsorsQ, eventID)
	if err != nil {
		return err
	}
	defer sponsorRows.Close()
	for sponsorRows.Next() {
		var sponsor diarization.SpeakerEvidenceSponsor
		if err := sponsorRows.Scan(&sponsor.BillID, &sponsor.BillNumber, &sponsor.LegislatorID, &sponsor.Label, &sponsor.SponsorType); err != nil {
			return err
		}
		if sponsor.LegislatorID != 0 && strings.TrimSpace(sponsor.Label) != "" {
			agenda.BillSponsors = append(agenda.BillSponsors, sponsor)
		}
	}
	return sponsorRows.Err()
}

func legislatorRoleForPrompt(chamber string) string {
	switch chamber {
	case "Senate":
		return "State Senator"
	case "House":
		return "State Representative"
	default:
		return "Legislator"
	}
}

func legislatorAliases(label, first, last, chamber string) []string {
	seen := map[string]bool{}
	add := func(s string, out *[]string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[strings.ToLower(s)] {
			return
		}
		seen[strings.ToLower(s)] = true
		*out = append(*out, s)
	}
	out := []string{}
	full := strings.TrimSpace(first + " " + last)
	add(full, &out)
	if last != "" {
		switch chamber {
		case "Senate":
			add("Senator "+last, &out)
			add("Sen. "+last, &out)
		case "House":
			add("Representative "+last, &out)
			add("Rep. "+last, &out)
		}
	}
	add(label, &out)
	return out
}

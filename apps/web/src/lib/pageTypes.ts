// Shared API response/domain types returned by the Go API. Keep these in sync
// with cmd/wa-dd-api and internal/pageassembly response structs.

export type AgendaItemSection = {
  hearing: HearingSummary;
  testifiers: TestifierSummary[];
  transcript?: TranscriptSection;
  organizations: OrganizationSummary[];
};

export type BillSummary = {
  biennium: string;
  bill_id: string;
  title?: string;
  description?: string;
  chamber_origin?: string;
  official_url?: string;
  sponsors?: BillSponsor[];
};

export type BillSponsor = {
  name: string;
  chamber?: string;
  sponsor_type?: string;
  photo_url?: string;
  thumbnail_url?: string;
};

export type BillStatus = {
  current?: string;
  status_date?: string | null;
  timeline?: BillStatusEvent[];
};

export type BillStatusEvent = {
  action_date: string;
  history_line: string;
};

export type HearingSummary = {
  hearing_id?: number;
  committee_name: string;
  committee_acronym?: string;
  chamber: string;
  meeting_datetime: string;
  location?: string;
  official_agenda_url?: string;
  tvw_url?: string;
  tvw_event_id?: string;
  agenda_item_label?: string;
  csi_agenda_item_id?: string;
};

export type Position = "Pro" | "Con" | "Other" | "Unknown";

export type TestifierSummary = {
  raw_name: string;
  raw_organization?: string;
  position: Position;
  testified: boolean;
  time_signed_in?: string | null;
  organization_id?: number | null;
};

export type SpeakerConfidence =
  | "confirmed_legislator"
  | "likely_legislator"
  | "likely_testifier"
  | "unknown_speaker"
  | "ai_inferred_pending_review";

export type TranscriptSection = {
  caption_url?: string;
  bill_segment_start_ms?: number;
  bill_segment_end_ms?: number;
  windows?: TranscriptWindow[];
  segments?: TranscriptSegment[];
};

export type TranscriptWindow = {
  start_ms: number;
  end_ms: number;
};

export type TranscriptSegment = {
  start_ms: number;
  end_ms: number;
  text: string;
  speaker_label?: string;
  speaker_confidence: SpeakerConfidence;
};

export type OrganizationSummary = {
  canonical_name: string;
  aliases?: string[];
  match_confidence: "confirmed" | "probable" | "possible" | "unmatched";
  match_notes?: string;
  testifier_position?: string;
  testifier_count?: number;
  context_summary?: string[];
};

export type SourceRecordSummary = {
  system: string;
  endpoint: string;
  url: string;
  fetched_at: string;
};

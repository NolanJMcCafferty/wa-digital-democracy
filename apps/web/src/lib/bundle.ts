// Types mirror internal/render/firstpage/bundle.go's struct shape exactly.
// Keep in sync — the Go side is canonical.

export type Bundle = {
  generated_at: string;
  bill: Bill;
  status: Status;
  hearing: Hearing;
  testifiers: Testifier[];
  transcript: Transcript;
  organizations: Organization[];
  sources: Source[];
  known_limitations?: string[];
};

export type Bill = {
  biennium: string;
  bill_id: string;
  title?: string;
  description?: string;
  chamber_origin?: string;
  official_url?: string;
  sponsors?: Sponsor[];
};

export type Sponsor = {
  name: string;
  chamber?: string;
  sponsor_type?: string;
};

export type Status = {
  current?: string;
  status_date?: string | null;
  timeline?: StatusEntry[];
};

export type StatusEntry = {
  action_date: string;
  history_line: string;
};

export type Hearing = {
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

export type Testifier = {
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

export type Transcript = {
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

export type Organization = {
  canonical_name: string;
  aliases?: string[];
  match_confidence: "confirmed" | "probable" | "possible" | "unmatched";
  match_notes?: string;
  context?: OrgContext[];
  testifier_position?: string;
  testifier_count?: number;
};

export type OrgContext = {
  context_type: string;
  source_dataset_id: string;
  summary_fields: Record<string, unknown>;
  source_url?: string;
  match_confidence: string;
};

export type Source = {
  system: string;
  endpoint: string;
  url: string;
  fetched_at: string;
};

import "server-only";
import type { Bill, HearingSection, Organization, Position, Source, Sponsor, Status } from "./bundle";

// The Next.js bill-detail page is a Server Component, so its fetch runs
// in the Node runtime and does NOT pass through next.config.ts rewrites.
// We hit the Go API by absolute URL. The rewrite still proxies any
// future client-side fetches under /api/v1/* through the same origin.
const API_BASE = process.env.WADD_API_URL ?? "http://localhost:8080";

// Match daily-ingest cadence with margin. Override per-call by passing a
// different `next` option if a section ever needs sub-minute freshness.
const DEFAULT_REVALIDATE = 60;

export type BundleListEntry = {
  biennium: string;
  billPrefix: string;
  billNumber: number;
  billId: string;
  title: string;
  chamberOrigin?: string;
  currentStatus?: string;
  statusBucket?: "in_progress" | "passed" | "failed" | "";
  leadSponsor?: string;        // "Senator Reed" — back-compat label
  leadDisplay?: string;        // "Julia Reed" when first/last present
  leadParty?: string;          // "D" | "R"
  leadSlug?: string;
};

type listResponseItem = {
  biennium: string;
  bill_prefix: string;
  bill_number: number;
  bill_id: string;
  title?: string;
  chamber_origin?: string;
  current_status?: string;
  status_bucket?: "in_progress" | "passed" | "failed" | "";
  lead_sponsor?: string;
  lead_display?: string;
  lead_party?: string;
  lead_slug?: string;
};

type billsResponse = {
  bills: listResponseItem[];
  total: number;
  limit: number;
  offset: number;
  facets: {
    Prefixes?: string[];
    Chambers?: string[];
    Parties?: string[];
    Statuses?: string[];
  };
};

export type BillSearchFilters = {
  q?: string;
  prefix?: string;
  chamber?: string;
  party?: string;
  status?: string;
  sponsor?: string;
  leadSponsor?: string;
  billIds?: string[]; // restrict to this set of bill_ids (issue pages)
  page?: number; // 1-indexed; converted to offset when fetching
  limit?: number;
};

export type BillSearchResult = {
  bills: BundleListEntry[];
  total: number;
  limit: number;
  offset: number;
  facets: {
    prefixes: string[];
    chambers: string[];
    parties: string[];
    statuses: string[];
  };
};

function mapBillItem(b: listResponseItem): BundleListEntry {
  return {
    biennium: b.biennium,
    billPrefix: b.bill_prefix,
    billNumber: b.bill_number,
    billId: b.bill_id,
    title: b.title ?? "",
    chamberOrigin: b.chamber_origin,
    currentStatus: b.current_status,
    statusBucket: b.status_bucket,
    leadSponsor: b.lead_sponsor,
    leadDisplay: b.lead_display,
    leadParty: b.lead_party,
    leadSlug: b.lead_slug,
  };
}

export type HearingAgendaItemEntry = {
  csiAgendaItemId: string;
  agendaItemLabel: string;
  biennium: string;
  billId: string;
  billPrefix: string;
  billNumber: number;
  testifierCount: number;
  testifiedCount: number;
  section?: HearingSection;
};

export type HearingBundleEntry = {
  hearingId: number;
  title: string;
  committeeName: string;
  chamber: string;
  meetingDatetime: string;
  location?: string;
  tvwUrl?: string;
  tvwEventId?: string;
  agendaItems: HearingAgendaItemEntry[];
  diarizedTranscript?: DiarizedTranscript;
  // Back-compat convenience fields for search/list snippets. For true
  // hearing pages these refer to the first agenda item, when present.
  csiAgendaItemId?: string;
  billId?: string;
};

export type DiarizedTranscript = {
  segments: DiarizedSegment[];
};

export type DiarizedSegment = {
  start_ms: number;
  end_ms: number;
  text: string;
  cluster_label?: string;
};

export type LegislatorListEntry = {
  slug: string;
  name: string;
  displayName?: string;
  firstName?: string;
  lastName?: string;
  chamber?: string;
  district?: string;
  party?: string;
  email?: string;
  phone?: string;
  photoUrl?: string;
  thumbnailUrl?: string;
  billCount?: number;
};

export type LegislatorPage = LegislatorListEntry & {
  appearances: Array<{
    biennium: string;
    billId: string;
    billPrefix: string;
    billNumber: number;
    billTitle?: string;
    sponsorType?: string;
    chamberOrigin?: string;
    currentStatus?: string;
    statusBucket?: "in_progress" | "passed" | "failed" | "";
    leadSponsor?: string;
    leadDisplay?: string;
    leadParty?: string;
    leadSlug?: string;
    hearingTitle?: string;
    csiAgendaItemId?: string;
    meetingDatetime?: string;
  }>;
};

export type SourceSummary = {
  system: string;
  label: string;
  status: "active" | "planned" | "partial";
  usedFor: string;
  limitations?: string;
  officialUrl: string;
  calls: number;
  latestFetchedAt?: string;
  endpoints: string[];
};

export type OrganizationListEntry = {
  slug: string;
  canonicalName: string;
  aliases: string[];
  matchConfidence: Organization["match_confidence"];
  matchNotes?: string;
  testifierCount: number;
  positions: Record<Position, number>;
};

export type OrganizationPage = OrganizationListEntry & {
  appearances: Array<{
    biennium: string;
    billId: string;
    billPrefix: string;
    billNumber: number;
    csiAgendaItemId?: string;
    hearingId?: number;
    hearingTitle: string;
    committeeName: string;
    meetingDatetime: string;
    position?: string;
    testifierCount: number;
  }>;
};

// listLocalBundles returns the full set of bills (up to the API's
// hard cap of billsMaxLimit=100 per request, so we ask for the max).
// The home page uses this for an aggregate count; pages that need
// pagination + filters should use searchBills below.
export async function listLocalBundles(): Promise<BundleListEntry[]> {
  // Ask for a single page large enough to cover the count metric on
  // the home page; pages that actually render rows should call
  // searchBills with proper pagination.
  const res = await fetch(`${API_BASE}/api/v1/bills?limit=100`, {
    cache: "no-store",
  });
  if (!res.ok) {
    throw new Error(`listLocalBundles: ${API_BASE}/api/v1/bills returned ${res.status}`);
  }
  const body = (await res.json()) as billsResponse;
  return body.bills.map(mapBillItem);
}

// countBills hits the bills list endpoint with limit=1 to read the
// `total` field — the home page needs the true total, which the
// limit=100 listLocalBundles call silently truncates.
export async function countBills(): Promise<number> {
  const res = await fetch(`${API_BASE}/api/v1/bills?limit=1`, {
    cache: "no-store",
  });
  if (!res.ok) {
    throw new Error(`countBills: ${API_BASE}/api/v1/bills returned ${res.status}`);
  }
  const body = (await res.json()) as billsResponse;
  return body.total;
}

// searchBills is the paginated, filtered fetch that backs the /bills
// page. Returns the page of matching rows + total count + facet
// summary; the caller renders pagination from total/limit/offset.
export async function searchBills(filters: BillSearchFilters): Promise<BillSearchResult> {
  const params = new URLSearchParams();
  if (filters.q) params.set("q", filters.q);
  if (filters.prefix) params.set("prefix", filters.prefix);
  if (filters.chamber) params.set("chamber", filters.chamber);
  if (filters.party) params.set("party", filters.party);
  if (filters.status) params.set("status", filters.status);
  if (filters.sponsor) params.set("sponsor", filters.sponsor);
  if (filters.leadSponsor) params.set("lead_sponsor", filters.leadSponsor);
  if (filters.billIds && filters.billIds.length > 0) {
    params.set("bill_ids", filters.billIds.join(","));
  }
  const limit = filters.limit ?? 50;
  params.set("limit", String(limit));
  const page = Math.max(1, filters.page ?? 1);
  const offset = (page - 1) * limit;
  if (offset > 0) params.set("offset", String(offset));

  const url = `${API_BASE}/api/v1/bills?${params.toString()}`;
  const res = await fetch(url, { cache: "no-store" });
  if (!res.ok) {
    throw new Error(`searchBills: ${url} returned ${res.status}`);
  }
  const body = (await res.json()) as billsResponse;
  return {
    bills: body.bills.map(mapBillItem),
    total: body.total,
    limit: body.limit,
    offset: body.offset,
    facets: {
      prefixes: body.facets.Prefixes ?? [],
      chambers: body.facets.Chambers ?? [],
      parties: body.facets.Parties ?? [],
      statuses: body.facets.Statuses ?? [],
    },
  };
}

type hearingAgendaItemResponse = {
  csi_agenda_item_id: string;
  agenda_item_label: string;
  biennium: string;
  bill_id: string;
  bill_prefix: string;
  bill_number: number;
  testifier_count: number;
  testified_count: number;
  section?: HearingSection;
};

type hearingResponseItem = {
  hearing_id: number;
  committee_name: string;
  chamber: string;
  meeting_datetime: string;
  location?: string;
  tvw_url?: string;
  tvw_event_id?: string;
  agenda_items: hearingAgendaItemResponse[];
  diarized_transcript?: DiarizedTranscript;
};

type hearingsResponse = {
  hearings: hearingResponseItem[];
  total: number;
  limit: number;
  offset: number;
  facets: {
    Chambers?: string[];
    Committees?: string[];
    Biennia?: string[];
  };
};

export type HearingSearchFilters = {
  committee?: string;
  bill?: string;
  speaker?: string;
  chambers?: string[];
  topicKeywords?: string[];
  biennium?: string;
  page?: number; // 1-indexed; converted to offset when fetching
  limit?: number;
};

export type HearingSearchResult = {
  hearings: HearingBundleEntry[];
  total: number;
  limit: number;
  offset: number;
  facets: {
    chambers: string[];
    committees: string[];
    biennia: string[];
  };
};

function mapHearingItem(h: hearingResponseItem): HearingBundleEntry {
  const agendaItems = (h.agenda_items ?? []).map((a) => ({
    csiAgendaItemId: a.csi_agenda_item_id,
    agendaItemLabel: a.agenda_item_label,
    biennium: a.biennium,
    billId: a.bill_id,
    billPrefix: a.bill_prefix,
    billNumber: a.bill_number,
    testifierCount: a.testifier_count,
    testifiedCount: a.testified_count,
    section: a.section,
  }));
  const first = agendaItems[0];
  const title = `${h.committee_name} · ${new Date(h.meeting_datetime).toLocaleDateString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
  })}`;
  return {
    hearingId: h.hearing_id,
    title,
    committeeName: h.committee_name,
    chamber: h.chamber,
    meetingDatetime: h.meeting_datetime,
    location: h.location,
    tvwUrl: h.tvw_url,
    tvwEventId: h.tvw_event_id,
    agendaItems,
    diarizedTranscript: h.diarized_transcript,
    csiAgendaItemId: first?.csiAgendaItemId,
    billId: first?.billId,
  };
}

export async function listHearingBundles(): Promise<HearingBundleEntry[]> {
  // Ask for the API's hard cap so callers that need an overview (home
  // page, issue pages, generateStaticParams) get the full set in one
  // request. Pages that paginate should call searchHearings instead.
  const res = await fetch(`${API_BASE}/api/v1/hearings?limit=100`, {
    cache: "no-store",
  });
  if (!res.ok) {
    throw new Error(`listHearingBundles: ${API_BASE}/api/v1/hearings returned ${res.status}`);
  }
  const body = (await res.json()) as hearingsResponse;
  return (body.hearings ?? []).map(mapHearingItem);
}

export async function countHearings(): Promise<number> {
  const res = await fetch(`${API_BASE}/api/v1/hearings?limit=1`, {
    cache: "no-store",
  });
  if (!res.ok) {
    throw new Error(`countHearings: ${API_BASE}/api/v1/hearings returned ${res.status}`);
  }
  const body = (await res.json()) as hearingsResponse;
  return body.total;
}

export async function searchHearings(filters: HearingSearchFilters): Promise<HearingSearchResult> {
  const params = new URLSearchParams();
  if (filters.committee) params.set("committee", filters.committee);
  if (filters.bill) params.set("bill", filters.bill);
  if (filters.speaker) params.set("speaker", filters.speaker);
  for (const c of filters.chambers ?? []) {
    if (c) params.append("chamber", c);
  }
  for (const k of filters.topicKeywords ?? []) {
    if (k) params.append("topic_keyword", k);
  }
  if (filters.biennium) params.set("biennium", filters.biennium);
  const limit = filters.limit ?? 50;
  params.set("limit", String(limit));
  const page = Math.max(1, filters.page ?? 1);
  const offset = (page - 1) * limit;
  if (offset > 0) params.set("offset", String(offset));

  const url = `${API_BASE}/api/v1/hearings?${params.toString()}`;
  const res = await fetch(url, { cache: "no-store" });
  if (!res.ok) {
    throw new Error(`searchHearings: ${url} returned ${res.status}`);
  }
  const body = (await res.json()) as hearingsResponse;
  const hearings = (body.hearings ?? []).map(mapHearingItem);
  const facets = body.facets ?? {};
  return {
    hearings,
    total: body.total ?? hearings.length,
    limit: body.limit ?? limit,
    offset: body.offset ?? offset,
    facets: {
      chambers: facets.Chambers ?? [],
      committees: facets.Committees ?? [],
      biennia: facets.Biennia ?? [],
    },
  };
}

export async function loadHearingBundle(hearingId: string | number): Promise<HearingBundleEntry | null> {
  const url = `${API_BASE}/api/v1/hearings/${encodeURIComponent(String(hearingId))}`;
  const res = await fetch(url, { next: { revalidate: DEFAULT_REVALIDATE } });
  if (res.status === 404) return null;
  if (!res.ok) {
    throw new Error(`loadHearingBundle ${url} returned ${res.status}`);
  }
  return mapHearingItem((await res.json()) as hearingResponseItem);
}

const SOURCE_DEFINITIONS: Array<Omit<SourceSummary, "calls" | "latestFetchedAt" | "endpoints">> = [
  {
    system: "lws",
    label: "Washington Legislative Web Services",
    status: "active",
    usedFor: "Bill metadata, sponsors, status timeline, and official hearing references.",
    officialUrl: "https://wslwebservices.leg.wa.gov/",
  },
  {
    system: "csi",
    label: "Committee Sign In",
    status: "active",
    usedFor: "Public testimony sign-ins, positions, organizations, and testified/registered-only flags.",
    officialUrl: "https://app.leg.wa.gov/csi/",
  },
  {
    system: "tvw",
    label: "TVW",
    status: "active",
    usedFor: "Public legislative video metadata and watch links.",
    officialUrl: "https://tvw.org/",
  },
  {
    system: "invintus",
    label: "Invintus",
    status: "active",
    usedFor: "TVW event details and caption/VTT transcript files.",
    limitations: "Requires the local Invintus embedder key for event-detail calls.",
    officialUrl: "https://api.v3.invintus.com/",
  },
  {
    system: "pdc_socrata",
    label: "PDC / data.wa.gov",
    status: "active",
    usedFor: "PDC lobbyist-employer records used to verify organization matches.",
    limitations: "Socrata app token is optional; leave blank unless broader ingestion hits throttling.",
    officialUrl: "https://data.wa.gov/",
  },
  {
    system: "committee_schedules",
    label: "Committee Schedules",
    status: "partial",
    usedFor: "Supplemental legislative agenda/video lookup when Committee Schedules pages are needed directly.",
    limitations: "Date-filtered search requires CSRF/session capture; routine hearing discovery currently uses LWS, CSI, and TVW instead.",
    officialUrl: "https://app.leg.wa.gov/committeeschedules/",
  },
  {
    system: "datawa_socrata",
    label: "DataWA / DES contracts",
    status: "partial",
    usedFor: "State agency contracts, IT contracts, master-contract sales, and WEBS vendor/procurement rows.",
    limitations: "Operator-driven ingest commands exist; these rows are not part of the daily legislative pipeline yet.",
    officialUrl: "https://data.wa.gov/",
  },
  {
    system: "sao_reportsearch",
    label: "Washington State Auditor",
    status: "planned",
    usedFor: "Audit/report metadata, findings, and accountability context.",
    officialUrl: "https://sao.wa.gov/reports-data/audit-reports",
  },
  {
    system: "seattle_auditor",
    label: "Seattle City Auditor",
    status: "planned",
    usedFor: "Structured audit recommendations and follow-up status.",
    officialUrl: "https://www.seattle.gov/cityauditor/recommendations",
  },
  {
    system: "seattle_socrata",
    label: "Seattle Open Data",
    status: "partial",
    usedFor: "Seattle operating-budget rows, with room for permits, service requests, land use, and housing overlays.",
    limitations: "Only the operating-budget surface has a current ingest command.",
    officialUrl: "https://data.seattle.gov/",
  },
  {
    system: "kingcounty_socrata",
    label: "King County Open Data",
    status: "planned",
    usedFor: "Parcels, property, elections, health, transit, and public-safety overlays.",
    officialUrl: "https://data.kingcounty.gov/",
  },
  {
    system: "census",
    label: "Census ACS",
    status: "planned",
    usedFor: "Demographic and geography context for districts, issues, and neighborhoods.",
    officialUrl: "https://api.census.gov/data.html",
  },
  {
    system: "usaspending",
    label: "USAspending",
    status: "partial",
    usedFor: "Federal award-search rows for awards performed in Washington.",
    limitations: "Current ingest is a bounded award-search page; deeper pagination, subawards, and entity joins are follow-up work.",
    officialUrl: "https://api.usaspending.gov/",
  },
  {
    system: "fiscal_wa",
    label: "Fiscal.wa.gov",
    status: "partial",
    usedFor: "Open Checkbook vendor-payment rows for state fiscal spending context.",
    limitations: "Current ingest covers the vendor-payment workbook, not full operating/capital/transportation budget surfaces.",
    officialUrl: "https://fiscal.wa.gov/",
  },
  {
    system: "openfema",
    label: "OpenFEMA",
    status: "planned",
    usedFor: "Disaster declarations, assistance, mitigation, and hazard context.",
    officialUrl: "https://www.fema.gov/about/openfema/api",
  },
  {
    system: "bls",
    label: "BLS",
    status: "planned",
    usedFor: "Labor-market, wage, employment, unemployment, and economic context.",
    officialUrl: "https://www.bls.gov/developers/",
  },
  {
    system: "hud",
    label: "HUD",
    status: "planned",
    usedFor: "Housing affordability, subsidized housing, FMR, CHAS, and HUD overlays.",
    officialUrl: "https://data.hud.gov/",
  },
  {
    system: "epa",
    label: "EPA ECHO / EJScreen",
    status: "planned",
    usedFor: "Environmental compliance, enforcement, facilities, and environmental-justice context.",
    officialUrl: "https://echo.epa.gov/tools/web-services",
  },
];

type sourceResponseItem = {
  system: string;
  calls: number;
  latest_fetched_at: string;
  endpoints: string[];
};

export async function listSourceSummaries(): Promise<SourceSummary[]> {
  // Always refetch — this powers the /sources status dashboard which
  // should reflect the current source_record table rather than a
  // ~60s-old snapshot.
  const res = await fetch(`${API_BASE}/api/v1/sources`, {
    cache: "no-store",
  });
  if (!res.ok) {
    throw new Error(`listSourceSummaries: ${API_BASE}/api/v1/sources returned ${res.status}`);
  }
  const live = (await res.json()) as sourceResponseItem[];
  const bySystem = new Map<string, sourceResponseItem>();
  for (const s of live) bySystem.set(s.system, s);

  // Left-join with the local SOURCE_DEFINITIONS so "planned" rows still
  // show even before they're wired into the ingest pipeline.
  return SOURCE_DEFINITIONS.map((def) => {
    const seen = bySystem.get(def.system);
    return {
      ...def,
      calls: seen?.calls ?? 0,
      latestFetchedAt: seen?.latest_fetched_at,
      endpoints: (seen?.endpoints ?? []).slice().sort(),
    };
  });
}

type legislatorListItem = {
  slug: string;
  name: string;
  display_name?: string;
  first_name?: string;
  last_name?: string;
  chamber?: string;
  district?: string;
  party?: string;
  email?: string;
  phone?: string;
  photo_url?: string;
  thumbnail_url?: string;
  bill_count: number;
};

type legislatorDetailResponse = {
  slug: string;
  name: string;
  display_name?: string;
  first_name?: string;
  last_name?: string;
  chamber?: string;
  district?: string;
  party?: string;
  photo_url?: string;
  thumbnail_url?: string;
  appearances: Array<{
    biennium: string;
    bill_id: string;
    bill_prefix: string;
    bill_number: number;
    bill_title?: string;
    sponsor_type?: string;
    chamber_origin?: string;
    current_status?: string;
    status_bucket?: "in_progress" | "passed" | "failed" | "";
    lead_sponsor?: string;
    lead_display?: string;
    lead_party?: string;
    lead_slug?: string;
  }>;
};

function mapLegislatorListItem(l: legislatorListItem): LegislatorListEntry {
  return {
    slug: l.slug,
    name: l.name,
    displayName: l.display_name,
    firstName: l.first_name,
    lastName: l.last_name,
    chamber: l.chamber,
    district: l.district,
    party: l.party,
    email: l.email,
    phone: l.phone,
    photoUrl: l.photo_url,
    thumbnailUrl: l.thumbnail_url,
    billCount: l.bill_count,
  };
}

export async function listLegislators(): Promise<LegislatorListEntry[]> {
  const res = await fetch(`${API_BASE}/api/v1/legislators`, {
    cache: "no-store",
  });
  if (!res.ok) {
    throw new Error(`listLegislators: ${API_BASE}/api/v1/legislators returned ${res.status}`);
  }
  const items = (await res.json()) as legislatorListItem[];
  return items.map(mapLegislatorListItem);
}

export async function loadLegislatorPage(slug: string): Promise<LegislatorPage | null> {
  const url = `${API_BASE}/api/v1/legislators/${encodeURIComponent(slug)}`;
  const res = await fetch(url, { next: { revalidate: DEFAULT_REVALIDATE } });
  if (res.status === 404) return null;
  if (!res.ok) {
    throw new Error(`loadLegislatorPage ${url} returned ${res.status}`);
  }
  const detail = (await res.json()) as legislatorDetailResponse;
  return {
    slug: detail.slug,
    name: detail.name,
    displayName: detail.display_name,
    firstName: detail.first_name,
    lastName: detail.last_name,
    chamber: detail.chamber,
    district: detail.district,
    party: detail.party,
    photoUrl: detail.photo_url,
    thumbnailUrl: detail.thumbnail_url,
    appearances: detail.appearances.map((a) => ({
      biennium: a.biennium,
      billId: a.bill_id,
      billPrefix: a.bill_prefix,
      billNumber: a.bill_number,
      billTitle: a.bill_title,
      sponsorType: a.sponsor_type,
      chamberOrigin: a.chamber_origin,
      currentStatus: a.current_status,
      statusBucket: a.status_bucket,
      leadSponsor: a.lead_sponsor,
      leadDisplay: a.lead_display,
      leadParty: a.lead_party,
      leadSlug: a.lead_slug,
    })),
  };
}

export function legislatorSlug(s: Sponsor): string {
  return slugify(s.name);
}

type orgListItem = {
  slug: string;
  canonical_name: string;
  aliases: string[];
  match_confidence: Organization["match_confidence"];
  match_notes?: string;
  testifier_count: number;
  positions: Record<Position, number>;
};

type orgDetailResponse = orgListItem & {
  appearances: Array<{
    biennium: string;
    bill_id: string;
    bill_prefix: string;
    bill_number: number;
    csi_agenda_item_id?: string;
    hearing_id?: number;
    hearing_title: string;
    committee_name: string;
    meeting_datetime: string;
    position?: string;
    testifier_count: number;
  }>;
};

function mapOrganizationListItem(o: orgListItem): OrganizationListEntry {
  return {
    slug: o.slug,
    canonicalName: o.canonical_name,
    aliases: o.aliases ?? [],
    matchConfidence: o.match_confidence,
    matchNotes: o.match_notes,
    testifierCount: o.testifier_count,
    positions: o.positions,
  };
}

export async function listOrganizations(): Promise<OrganizationListEntry[]> {
  const res = await fetch(`${API_BASE}/api/v1/organizations`, {
    cache: "no-store",
  });
  if (!res.ok) {
    throw new Error(`listOrganizations: ${API_BASE}/api/v1/organizations returned ${res.status}`);
  }
  const items = (await res.json()) as orgListItem[];
  return items.map(mapOrganizationListItem);
}

export async function loadOrganizationPage(slug: string): Promise<OrganizationPage | null> {
  const url = `${API_BASE}/api/v1/organizations/${encodeURIComponent(slug)}`;
  const res = await fetch(url, { next: { revalidate: DEFAULT_REVALIDATE } });
  if (res.status === 404) return null;
  if (!res.ok) {
    throw new Error(`loadOrganizationPage ${url} returned ${res.status}`);
  }
  const detail = (await res.json()) as orgDetailResponse;
  return {
    ...mapOrganizationListItem(detail),
    appearances: detail.appearances.map((a) => ({
      biennium: a.biennium,
      billId: a.bill_id,
      billPrefix: a.bill_prefix,
      billNumber: a.bill_number,
      csiAgendaItemId: a.csi_agenda_item_id,
      hearingId: a.hearing_id,
      hearingTitle: a.hearing_title,
      committeeName: a.committee_name,
      meetingDatetime: a.meeting_datetime,
      position: a.position,
      testifierCount: a.testifier_count,
    })),
  };
}

export function slugify(s: string): string {
  return s
    .toLowerCase()
    .replace(/&/g, " and ")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

export type BillPage = {
  generated_at: string;
  bill: Bill;
  status: Status;
  hearings: HearingSection[];
  sources: Source[];
  known_limitations?: string[];
};

export async function loadBillPage(
  biennium: string,
  billPrefix: string,
  billNumber: number,
): Promise<BillPage | null> {
  const url = `${API_BASE}/api/v1/bills/${biennium}/${billPrefix}${billNumber}/page`;
  const res = await fetch(url, { next: { revalidate: DEFAULT_REVALIDATE } });
  if (res.status === 404) return null;
  if (!res.ok) {
    throw new Error(`loadBillPage ${url} returned ${res.status}`);
  }
  return (await res.json()) as BillPage;
}

export type SpeakerReviewSegment = {
  StartMS: number;
  EndMS: number;
  Text: string;
};

export type SpeakerReviewTask = {
  ID: number;
  DiarizationJobID: number;
  TVWEventID: string;
  ClusterID: number;
  ClusterLabel: string;
  TotalSpeechMS: number;
  TurnCount: number;
  Status: string;
  Priority: number;
  CandidateKind: string;
  CandidateID: number;
  CandidateLabel: string;
  CandidateConfidence: number;
  EvidenceIDs: number[];
  EvidenceText: string;
  EvidenceStartMS: number;
  EvidenceEndMS: number;
  SampleSegments?: SpeakerReviewSegment[];
};

export async function listSpeakerReviewTasks(status = "pending"): Promise<SpeakerReviewTask[]> {
  const res = await fetch(`${API_BASE}/api/v1/admin/review/speakers?status=${encodeURIComponent(status)}`, {
    cache: "no-store",
  });
  if (!res.ok) {
    throw new Error(`listSpeakerReviewTasks returned ${res.status}`);
  }
  const body = (await res.json()) as { tasks: SpeakerReviewTask[] };
  return body.tasks ?? [];
}

export async function loadSpeakerReviewTask(taskId: string): Promise<SpeakerReviewTask | null> {
  const res = await fetch(`${API_BASE}/api/v1/admin/review/speakers/${encodeURIComponent(taskId)}`, {
    cache: "no-store",
  });
  if (res.status === 404) return null;
  if (!res.ok) {
    throw new Error(`loadSpeakerReviewTask returned ${res.status}`);
  }
  return (await res.json()) as SpeakerReviewTask;
}

export async function decideSpeakerReviewTask(taskId: string, action: "accept" | "reject" | "needs-more-evidence", reviewer: string, notes: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/v1/admin/review/speakers/${encodeURIComponent(taskId)}/${action}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ reviewer, notes }),
    cache: "no-store",
  });
  if (!res.ok) {
    throw new Error(`decideSpeakerReviewTask ${action} returned ${res.status}: ${await res.text()}`);
  }
}

import "server-only";
import type { Bundle, OrgContext, Organization, Position, Sponsor } from "./bundle";

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
};

type listResponseItem = {
  biennium: string;
  bill_prefix: string;
  bill_number: number;
  bill_id: string;
  title?: string;
};

export type HearingBundleEntry = BundleListEntry & {
  csiAgendaItemId: string;
  title: string;
  committeeName: string;
  meetingDatetime: string;
  billId: string;
};

export type LegislatorBundleEntry = {
  slug: string;
  name: string;            // "Senator Alvarado" — kept for back-compat with /legislators/{slug} routes
  displayName?: string;    // "Emily Alvarado"
  firstName?: string;
  lastName?: string;
  chamber?: string;
  district?: string;       // "34"
  party?: string;          // "D" | "R"
  email?: string;
  phone?: string;
  billCount?: number;
  appearances: Array<{
    biennium: string;
    billId: string;
    billPrefix: string;
    billNumber: number;
    billTitle?: string;
    sponsorType?: string;
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

export type OrganizationBundleEntry = {
  slug: string;
  canonicalName: string;
  aliases: string[];
  matchConfidence: Organization["match_confidence"];
  matchNotes?: string;
  testifierCount: number;
  positions: Record<Position, number>;
  contextCount: number;
  contexts: OrgContext[];
  appearances: Array<{
    biennium: string;
    billId: string;
    billPrefix: string;
    billNumber: number;
    csiAgendaItemId?: string;
    hearingTitle: string;
    committeeName: string;
    meetingDatetime: string;
    position?: string;
    testifierCount: number;
  }>;
};

export async function listLocalBundles(): Promise<BundleListEntry[]> {
  const res = await fetch(`${API_BASE}/api/v1/bills`, {
    cache: "no-store",
  });
  if (!res.ok) {
    throw new Error(`listLocalBundles: ${API_BASE}/api/v1/bills returned ${res.status}`);
  }
  const items = (await res.json()) as listResponseItem[];
  return items.map((b) => ({
    biennium: b.biennium,
    billPrefix: b.bill_prefix,
    billNumber: b.bill_number,
    billId: b.bill_id,
    title: b.title ?? "",
  }));
}

type hearingResponseItem = {
  csi_agenda_item_id: string;
  agenda_item_label: string;
  committee_name: string;
  chamber: string;
  meeting_datetime: string;
  biennium: string;
  bill_id: string;
  bill_prefix: string;
  bill_number: number;
};

export async function listHearingBundles(): Promise<HearingBundleEntry[]> {
  const res = await fetch(`${API_BASE}/api/v1/hearings`, {
    cache: "no-store",
  });
  if (!res.ok) {
    throw new Error(`listHearingBundles: ${API_BASE}/api/v1/hearings returned ${res.status}`);
  }
  const items = (await res.json()) as hearingResponseItem[];
  return items.map((h) => ({
    biennium: h.biennium,
    billPrefix: h.bill_prefix,
    billNumber: h.bill_number,
    billId: h.bill_id,
    title: h.agenda_item_label || h.bill_id,
    csiAgendaItemId: h.csi_agenda_item_id,
    committeeName: h.committee_name,
    meetingDatetime: h.meeting_datetime,
  }));
}

export async function loadHearingBundle(csiAgendaItemId: string): Promise<Bundle | null> {
  const url = `${API_BASE}/api/v1/hearings/${encodeURIComponent(csiAgendaItemId)}`;
  const res = await fetch(url, { next: { revalidate: DEFAULT_REVALIDATE } });
  if (res.status === 404) return null;
  if (!res.ok) {
    throw new Error(`loadHearingBundle ${url} returned ${res.status}`);
  }
  return (await res.json()) as Bundle;
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
    usedFor: "Lobbying and campaign-finance context for reviewed organization matches.",
    limitations: "Socrata app token is optional; leave blank unless broader ingestion hits throttling.",
    officialUrl: "https://data.wa.gov/",
  },
  {
    system: "committee_schedules",
    label: "Committee Schedules",
    status: "partial",
    usedFor: "Agenda/video enrichment when a committee schedule mapping is needed.",
    limitations: "Date-filtered search requires CSRF/session capture; current demo uses configured TVW event ID.",
    officialUrl: "https://app.leg.wa.gov/committeeschedules/",
  },
  {
    system: "datawa_socrata",
    label: "DataWA / DES contracts",
    status: "planned",
    usedFor: "State contracts, procurement, master-contract sales, and statewide open-data overlays.",
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
    status: "planned",
    usedFor: "Permits, budget, service requests, land use, and housing outcome overlays.",
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
    status: "planned",
    usedFor: "Federal awards, grants, contracts, agencies, and recipients in Washington.",
    officialUrl: "https://api.usaspending.gov/",
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
  bill_count: number;
};

type legislatorDetailResponse = {
  slug: string;
  name: string;
  chamber?: string;
  appearances: Array<{
    biennium: string;
    bill_id: string;
    bill_prefix: string;
    bill_number: number;
    bill_title?: string;
    sponsor_type?: string;
  }>;
};

export async function listLegislatorBundles(): Promise<LegislatorBundleEntry[]> {
  const res = await fetch(`${API_BASE}/api/v1/legislators`, {
    cache: "no-store",
  });
  if (!res.ok) {
    throw new Error(`listLegislatorBundles: ${API_BASE}/api/v1/legislators returned ${res.status}`);
  }
  const items = (await res.json()) as legislatorListItem[];
  // The list endpoint returns aggregate counts, not appearances. We
  // synthesize empty appearances arrays here so the existing
  // LegislatorBundleEntry shape (which the legislators index page
  // consumes for slug + name) keeps working. Pages that need the full
  // appearance list call loadLegislatorBundle(slug) below.
  return items.map((l) => ({
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
    billCount: l.bill_count,
    appearances: [],
  }));
}

export async function loadLegislatorBundle(slug: string): Promise<LegislatorBundleEntry | null> {
  const url = `${API_BASE}/api/v1/legislators/${encodeURIComponent(slug)}`;
  const res = await fetch(url, { next: { revalidate: DEFAULT_REVALIDATE } });
  if (res.status === 404) return null;
  if (!res.ok) {
    throw new Error(`loadLegislatorBundle ${url} returned ${res.status}`);
  }
  const detail = (await res.json()) as legislatorDetailResponse;
  return {
    slug: detail.slug,
    name: detail.name,
    chamber: detail.chamber,
    appearances: detail.appearances.map((a) => ({
      biennium: a.biennium,
      billId: a.bill_id,
      billPrefix: a.bill_prefix,
      billNumber: a.bill_number,
      billTitle: a.bill_title,
      sponsorType: a.sponsor_type,
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
  context_count: number;
};

type orgDetailResponse = orgListItem & {
  appearances: Array<{
    biennium: string;
    bill_id: string;
    bill_prefix: string;
    bill_number: number;
    csi_agenda_item_id?: string;
    hearing_title: string;
    committee_name: string;
    meeting_datetime: string;
    position?: string;
    testifier_count: number;
  }>;
};

export async function listOrganizationBundles(): Promise<OrganizationBundleEntry[]> {
  const res = await fetch(`${API_BASE}/api/v1/organizations`, {
    cache: "no-store",
  });
  if (!res.ok) {
    throw new Error(`listOrganizationBundles: ${API_BASE}/api/v1/organizations returned ${res.status}`);
  }
  const items = (await res.json()) as orgListItem[];
  return items.map((o) => ({
    slug: o.slug,
    canonicalName: o.canonical_name,
    aliases: o.aliases ?? [],
    matchConfidence: o.match_confidence,
    matchNotes: o.match_notes,
    testifierCount: o.testifier_count,
    positions: o.positions,
    contextCount: o.context_count,
    // The aggregate list endpoint omits per-record contexts and
    // appearances — those need a detail fetch. Index pages only show
    // counts, so empty arrays here are fine.
    contexts: [],
    appearances: [],
  }));
}

export async function loadOrganizationBundle(slug: string): Promise<OrganizationBundleEntry | null> {
  const url = `${API_BASE}/api/v1/organizations/${encodeURIComponent(slug)}`;
  const res = await fetch(url, { next: { revalidate: DEFAULT_REVALIDATE } });
  if (res.status === 404) return null;
  if (!res.ok) {
    throw new Error(`loadOrganizationBundle ${url} returned ${res.status}`);
  }
  const detail = (await res.json()) as orgDetailResponse;
  return {
    slug: detail.slug,
    canonicalName: detail.canonical_name,
    aliases: detail.aliases ?? [],
    matchConfidence: detail.match_confidence,
    matchNotes: detail.match_notes,
    testifierCount: detail.testifier_count,
    positions: detail.positions,
    contextCount: detail.context_count,
    contexts: [], // detail endpoint doesn't return per-record contexts at this stage
    appearances: detail.appearances.map((a) => ({
      biennium: a.biennium,
      billId: a.bill_id,
      billPrefix: a.bill_prefix,
      billNumber: a.bill_number,
      csiAgendaItemId: a.csi_agenda_item_id,
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

export async function loadBundle(
  biennium: string,
  billPrefix: string,
  billNumber: number,
): Promise<Bundle | null> {
  const url = `${API_BASE}/api/v1/bills/${biennium}/${billPrefix}${billNumber}/first-page`;
  const res = await fetch(url, { next: { revalidate: DEFAULT_REVALIDATE } });
  if (res.status === 404) return null;
  if (!res.ok) {
    throw new Error(`loadBundle ${url} returned ${res.status}`);
  }
  return (await res.json()) as Bundle;
}

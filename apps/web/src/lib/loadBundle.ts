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
  name: string;
  chamber?: string;
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
    next: { revalidate: DEFAULT_REVALIDATE },
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

export async function listHearingBundles(): Promise<HearingBundleEntry[]> {
  const entries = await listLocalBundles();
  const out: HearingBundleEntry[] = [];
  for (const entry of entries) {
    const bundle = await loadBundle(entry.biennium, entry.billPrefix, entry.billNumber);
    const hearingId = bundle?.hearing.csi_agenda_item_id;
    if (!bundle || !hearingId) continue;
    out.push({
      ...entry,
      csiAgendaItemId: hearingId,
      title: bundle.hearing.agenda_item_label || bundle.bill.title || bundle.bill.bill_id,
      committeeName: bundle.hearing.committee_name,
      meetingDatetime: bundle.hearing.meeting_datetime,
      billId: bundle.bill.bill_id,
    });
  }
  out.sort((a, b) => b.meetingDatetime.localeCompare(a.meetingDatetime));
  return out;
}

export async function loadHearingBundle(csiAgendaItemId: string): Promise<Bundle | null> {
  const entries = await listLocalBundles();
  for (const entry of entries) {
    const bundle = await loadBundle(entry.biennium, entry.billPrefix, entry.billNumber);
    if (bundle?.hearing.csi_agenda_item_id === csiAgendaItemId) {
      return bundle;
    }
  }
  return null;
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

export async function listSourceSummaries(): Promise<SourceSummary[]> {
  const entries = await listLocalBundles();
  const bySystem = new Map<string, { calls: number; latestFetchedAt?: string; endpoints: Set<string> }>();
  for (const entry of entries) {
    const bundle = await loadBundle(entry.biennium, entry.billPrefix, entry.billNumber);
    for (const s of bundle?.sources ?? []) {
      const current = bySystem.get(s.system) ?? { calls: 0, endpoints: new Set<string>() };
      current.calls += 1;
      current.endpoints.add(s.endpoint);
      if (!current.latestFetchedAt || s.fetched_at > current.latestFetchedAt) {
        current.latestFetchedAt = s.fetched_at;
      }
      bySystem.set(s.system, current);
    }
  }
  return SOURCE_DEFINITIONS.map((def) => {
    const seen = bySystem.get(def.system);
    return {
      ...def,
      calls: seen?.calls ?? 0,
      latestFetchedAt: seen?.latestFetchedAt,
      endpoints: Array.from(seen?.endpoints ?? []).sort(),
    };
  });
}

export async function listLegislatorBundles(): Promise<LegislatorBundleEntry[]> {
  const entries = await listLocalBundles();
  const legislators = new Map<string, LegislatorBundleEntry>();
  for (const entry of entries) {
    const bundle = await loadBundle(entry.biennium, entry.billPrefix, entry.billNumber);
    if (!bundle) continue;
    for (const sponsor of bundle.bill.sponsors ?? []) {
      const slug = slugify(sponsor.name);
      const existing = legislators.get(slug) ?? {
        slug,
        name: sponsor.name,
        chamber: sponsor.chamber,
        appearances: [],
      };
      if (!existing.chamber && sponsor.chamber) existing.chamber = sponsor.chamber;
      existing.appearances.push({
        biennium: bundle.bill.biennium,
        billId: bundle.bill.bill_id,
        billPrefix: entry.billPrefix,
        billNumber: entry.billNumber,
        billTitle: bundle.bill.title,
        sponsorType: sponsor.sponsor_type,
        hearingTitle: bundle.hearing.agenda_item_label,
        csiAgendaItemId: bundle.hearing.csi_agenda_item_id,
        meetingDatetime: bundle.hearing.meeting_datetime,
      });
      legislators.set(slug, existing);
    }
  }
  return Array.from(legislators.values()).sort((a, b) => a.name.localeCompare(b.name));
}

export async function loadLegislatorBundle(slug: string): Promise<LegislatorBundleEntry | null> {
  const legislators = await listLegislatorBundles();
  return legislators.find((l) => l.slug === slug) ?? null;
}

export function legislatorSlug(s: Sponsor): string {
  return slugify(s.name);
}

export async function listOrganizationBundles(): Promise<OrganizationBundleEntry[]> {
  const entries = await listLocalBundles();
  const orgs = new Map<string, OrganizationBundleEntry>();
  for (const entry of entries) {
    const bundle = await loadBundle(entry.biennium, entry.billPrefix, entry.billNumber);
    if (!bundle) continue;
    for (const org of bundle.organizations ?? []) {
      const slug = slugify(org.canonical_name);
      const existing = orgs.get(slug) ?? {
        slug,
        canonicalName: org.canonical_name,
        aliases: org.aliases ?? [],
        matchConfidence: org.match_confidence,
        matchNotes: org.match_notes,
        testifierCount: 0,
        positions: { Pro: 0, Con: 0, Other: 0, Unknown: 0 },
        contextCount: 0,
        contexts: [],
        appearances: [],
      };
      existing.aliases = Array.from(new Set([...existing.aliases, ...(org.aliases ?? [])]));
      existing.testifierCount += org.testifier_count ?? 0;
      existing.contexts.push(...(org.context ?? []));
      existing.contextCount = existing.contexts.length;
      if (org.testifier_position && org.testifier_position in existing.positions) {
        existing.positions[org.testifier_position as Position] += org.testifier_count ?? 0;
      }
      existing.appearances.push({
        biennium: bundle.bill.biennium,
        billId: bundle.bill.bill_id,
        billPrefix: entry.billPrefix,
        billNumber: entry.billNumber,
        csiAgendaItemId: bundle.hearing.csi_agenda_item_id,
        hearingTitle: bundle.hearing.agenda_item_label || bundle.bill.title || bundle.bill.bill_id,
        committeeName: bundle.hearing.committee_name,
        meetingDatetime: bundle.hearing.meeting_datetime,
        position: org.testifier_position,
        testifierCount: org.testifier_count ?? 0,
      });
      orgs.set(slug, existing);
    }
  }
  return Array.from(orgs.values()).sort((a, b) => a.canonicalName.localeCompare(b.canonicalName));
}

export async function loadOrganizationBundle(slug: string): Promise<OrganizationBundleEntry | null> {
  const orgs = await listOrganizationBundles();
  return orgs.find((o) => o.slug === slug) ?? null;
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

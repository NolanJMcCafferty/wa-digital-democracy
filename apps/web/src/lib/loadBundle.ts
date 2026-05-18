import "server-only";
import { promises as fs } from "fs";
import path from "path";
import type { Bundle, OrgContext, Organization, Position, Sponsor } from "./bundle";

// Bundle filenames are produced by Go: data/processed/bundles/wa_<biennium>_<prefix><number>.json
// Example: data/processed/bundles/wa_2025-26_HB1501.json

const BUNDLE_DIR = path.resolve(process.cwd(), "..", "..", "data", "processed", "bundles");

export type BundleListEntry = {
  path: string;
  biennium: string;
  billPrefix: string;
  billNumber: number;
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
  let names: string[] = [];
  try {
    names = await fs.readdir(BUNDLE_DIR);
  } catch {
    return [];
  }
  const out: BundleListEntry[] = [];
  for (const n of names) {
    if (!n.startsWith("wa_") || !n.endsWith(".json")) continue;
    // wa_<biennium>_<prefix><number>.json
    const stem = n.slice(3, -5); // "2025-26_HB1501"
    const m = stem.match(/^([\d]{4}-[\d]{2})_([A-Z]+)([\d]+)$/);
    if (!m) continue;
    out.push({
      path: path.join("data/processed/bundles", n),
      biennium: m[1],
      billPrefix: m[2],
      billNumber: parseInt(m[3], 10),
    });
  }
  out.sort((a, b) => (a.biennium === b.biennium ? a.billNumber - b.billNumber : a.biennium.localeCompare(b.biennium)));
  return out;
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
  billNumber: number
): Promise<Bundle | null> {
  const filename = `wa_${biennium}_${billPrefix}${billNumber}.json`;
  const filepath = path.join(BUNDLE_DIR, filename);
  try {
    const raw = await fs.readFile(filepath, "utf-8");
    return JSON.parse(raw) as Bundle;
  } catch (err) {
    if ((err as NodeJS.ErrnoException).code === "ENOENT") return null;
    throw err;
  }
}

import "server-only";
import { promises as fs } from "fs";
import path from "path";
import type { Bundle } from "./bundle";

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

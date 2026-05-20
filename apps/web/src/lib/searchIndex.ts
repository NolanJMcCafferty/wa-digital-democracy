import "server-only";

import {
  listHearingBundles,
  listLegislatorBundles,
  listLocalBundles,
  listOrganizationBundles,
  loadBillPage,
  type BillPage,
} from "./loadBundle";

export type SearchResult = {
  // The "Transcript" variant is populated dynamically in SearchBox via
  // /api/v1/search/transcripts and is not part of the prebuilt index.
  type: "Bill" | "Hearing" | "Organization" | "Legislator" | "Transcript";
  title: string;
  subtitle: string;
  href: string;
  searchText: string;
};

export async function buildSearchIndex(): Promise<SearchResult[]> {
  const [bundles, hearings, organizations, legislators] = await Promise.all([
    listLocalBundles(),
    listHearingBundles(),
    listOrganizationBundles(),
    listLegislatorBundles(),
  ]);
  const loadedBundles = (
    await Promise.all(
      bundles.map((b) => loadBillPage(b.biennium, b.billPrefix, b.billNumber))
    )
  ).filter((b): b is BillPage => Boolean(b));

  return [
    ...loadedBundles.map((b) =>
      withSearchText({
        type: "Bill" as const,
        title: `${b.bill.bill_id} — ${b.bill.title ?? "Untitled bill"}`,
        subtitle: `${b.bill.biennium} · ${b.status.current ?? "Status unavailable"}`,
        href: `/bills/${b.bill.biennium}/${b.bill.bill_id.replace(/\s+/g, "")}`,
        keywords: `${b.bill.description ?? ""} ${(b.bill.sponsors ?? [])
          .map((s) => s.name)
          .join(" ")}`,
      })
    ),
    ...hearings.map((h) =>
      withSearchText({
        type: "Hearing" as const,
        title: h.title,
        subtitle: `${h.committeeName} · ${h.agendaItems.length} agenda item${h.agendaItems.length === 1 ? "" : "s"}`,
        href: `/hearings/${h.hearingId}`,
        keywords: `${h.meetingDatetime} ${h.agendaItems
          .map((a) => `${a.billId} ${a.agendaItemLabel} CSI ${a.csiAgendaItemId}`)
          .join(" ")}`,
      })
    ),
    ...organizations.map((o) =>
      withSearchText({
        type: "Organization" as const,
        title: o.canonicalName,
        subtitle: `${o.testifierCount.toLocaleString()} linked testifier${
          o.testifierCount === 1 ? "" : "s"
        }`,
        href: `/organizations/${o.slug}`,
        keywords: `${o.aliases.join(" ")} ${o.matchNotes ?? ""}`,
      })
    ),
    ...legislators.map((l) =>
      withSearchText({
        type: "Legislator" as const,
        title: l.name,
        subtitle: `${l.chamber ?? "Chamber unknown"} · ${l.appearances.length.toLocaleString()} sponsored bill${
          l.appearances.length === 1 ? "" : "s"
        }`,
        href: `/legislators/${l.slug}`,
        keywords: l.appearances
          .map((a) => `${a.billId} ${a.billTitle ?? ""} ${a.sponsorType ?? ""}`)
          .join(" "),
      })
    ),
  ];
}

function withSearchText(
  result: Omit<SearchResult, "searchText"> & { keywords: string }
): SearchResult {
  return {
    type: result.type,
    title: result.title,
    subtitle: result.subtitle,
    href: result.href,
    searchText: `${result.title} ${result.subtitle} ${result.keywords}`.toLowerCase(),
  };
}

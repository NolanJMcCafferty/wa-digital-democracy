import "server-only";

import {
  listHearings,
  listLegislators,
  listBills,
  listOrganizations,
} from "./api";

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
  const [bills, hearings, organizations, legislators] = await Promise.all([
    listBills(),
    listHearings(),
    listOrganizations(),
    listLegislators(),
  ]);

  return [
    ...bills.map((b) =>
      withSearchText({
        type: "Bill" as const,
        title: `${b.billId} — ${b.title || "Untitled bill"}`,
        subtitle: `${b.biennium} · ${b.currentStatus ?? "Status unavailable"}`,
        href: `/bills/${b.biennium}/${b.billId.replace(/\s+/g, "")}`,
        keywords: b.leadSponsor ?? "",
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
        subtitle: `${l.chamber ?? "Chamber unknown"} · ${(l.billCount ?? 0).toLocaleString()} sponsored bill${
          l.billCount === 1 ? "" : "s"
        }`,
        href: `/legislators/${l.slug}`,
        keywords: [l.displayName, l.firstName, l.lastName, l.district, l.party]
          .filter(Boolean)
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

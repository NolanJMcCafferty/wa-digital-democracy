import {
  listBills,
  loadBillPage,
  searchBills,
  searchHearings,
  slugify,
  type BillPage,
  type OrganizationListEntry,
} from "@/lib/api";
import type { Position } from "@/lib/pageTypes";
import {
  BillSearchResults,
  parseBillFilters,
  type RawBillSearchParams,
} from "../../bills/BillSearchResults";
import {
  HearingSearchResults,
  hearingFiltersToSearch,
  parseHearingFilters,
} from "../../hearings/HearingSearchResults";
import {
  OrganizationSearchResults,
  parseOrganizationFilters,
} from "../../organizations/OrganizationSearchResults";

const POSITION_ORDER: Position[] = ["Pro", "Con", "Other", "Unknown"];
const CONFIDENCE_RANK: Record<OrganizationListEntry["matchConfidence"], number> = {
  confirmed: 4,
  probable: 3,
  possible: 2,
  unmatched: 1,
};

export type IssuePageConfig = {
  slug: string;
  title: string;
  description: string;
  emptyLabel: string;
  keywords: string[];
};

export async function IssuePage({
  config,
  searchParams,
}: {
  config: IssuePageConfig;
  searchParams?: RawBillSearchParams;
}) {
  const filters = parseBillFilters(searchParams ?? {});
  const hearingFilters = parseHearingFilters(searchParams ?? {});
  const organizationFilters = parseOrganizationFilters(searchParams ?? {}, "org");
  const entries = await listBills();
  const allBillPages = (
    await Promise.all(
      entries.map((e) => loadBillPage(e.biennium, e.billPrefix, e.billNumber))
    )
  ).filter((b): b is BillPage => Boolean(b));
  const billPages = allBillPages.filter((b) => billPageMatchesIssue(b, config));
  const matchedBillIds = billPages.map((b) => b.bill.bill_id);

  const billResult = matchedBillIds.length === 0
    ? null
    : await searchBills({ ...filters, billIds: matchedBillIds });

  const hearingResult = await searchHearings(
    hearingFiltersToSearch(hearingFilters, config.keywords),
  );

  const totals = billPages.reduce(
    (acc, b) => {
      acc.bills += 1;
      for (const section of b.hearings) {
        acc.testifiers += section.testifiers.length;
        acc.testified += section.testifiers.filter((t) => t.testified).length;
        acc.transcriptSegments += section.transcript?.segments?.length ?? 0;
        acc.organizations += section.organizations.length;
        for (const t of section.testifiers) acc.positions[t.position] += 1;
      }
      acc.sources += b.sources.length;
      return acc;
    },
    {
      bills: 0,
      testifiers: 0,
      testified: 0,
      transcriptSegments: 0,
      organizations: 0,
      sources: 0,
      positions: { Pro: 0, Con: 0, Other: 0, Unknown: 0 } as Record<Position, number>,
    }
  );

  const organizations = issueOrganizations(billPages);

  return (
    <article className="space-y-10">
      <section className="space-y-3">
        <h1 className="text-3xl font-bold tracking-tight text-stone-900">
          {config.title}
        </h1>
        <p className="max-w-3xl text-stone-600">{config.description}</p>
      </section>

      {billPages.length === 0 ? (
        <p className="rounded border border-stone-300 bg-stone-50 p-4 text-sm text-stone-600">
          No {config.emptyLabel} legislation is available yet.
        </p>
      ) : (
        <>
          <section aria-labelledby={`${config.slug}-summary`} className="space-y-4">
            <h2 id={`${config.slug}-summary`} className="text-xl font-semibold text-stone-900">
              Current coverage
            </h2>
            <div className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-3 lg:grid-cols-6">
              <Metric label="Bills" value={totals.bills.toLocaleString()} />
              <Metric label="Hearings" value={hearingResult.total.toLocaleString()} />
              <Metric label="Signed in" value={totals.testifiers.toLocaleString()} />
              <Metric label="Testified" value={totals.testified.toLocaleString()} />
              <Metric label="Transcript" value={`${totals.transcriptSegments.toLocaleString()} excerpts`} />
              <Metric label="Sources" value={totals.sources.toLocaleString()} />
            </div>
          </section>

          <section aria-labelledby={`${config.slug}-positions`} className="space-y-4 rounded-lg border border-stone-300 bg-white p-6">
            <h2 id={`${config.slug}-positions`} className="text-xl font-semibold text-stone-900">
              Testimony positions
            </h2>
            <div className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
              {POSITION_ORDER.map((p) => (
                <Metric key={p} label={p} value={totals.positions[p].toLocaleString()} />
              ))}
            </div>
            <p className="text-xs text-stone-500">
              Counts are from Committee Sign In records, including people who registered a position but did not testify.
            </p>
          </section>

          {billResult ? (
            <section aria-labelledby={`${config.slug}-bills`} className="space-y-4">
              <h2 id={`${config.slug}-bills`} className="text-xl font-semibold text-stone-900">
                Bills
              </h2>
              <BillSearchResults
                basePath={`/issues/${config.slug}`}
                filters={filters}
                bills={billResult.bills}
                total={billResult.total}
                offset={billResult.offset}
                facets={billResult.facets}
              />
            </section>
          ) : null}

          <section aria-labelledby={`${config.slug}-hearings`} className="space-y-4">
            <h2 id={`${config.slug}-hearings`} className="text-xl font-semibold text-stone-900">
              Hearings
            </h2>
            <HearingSearchResults
              basePath={`/issues/${config.slug}`}
              filters={hearingFilters}
              hearings={hearingResult.hearings}
              total={hearingResult.total}
              offset={hearingResult.offset}
              facets={hearingResult.facets}
              hiddenFilters={["topic"]}
            />
          </section>

          <section aria-labelledby={`${config.slug}-orgs`} className="space-y-4">
            <h2 id={`${config.slug}-orgs`} className="text-xl font-semibold text-stone-900">
              Organizations
            </h2>
            <OrganizationSearchResults
              basePath={`/issues/${config.slug}`}
              filters={organizationFilters}
              organizations={organizations}
              paramPrefix="org"
            />
            <p className="text-xs text-stone-500">
              Organization matches come from testimony sign-ins and verified
              cross-source records where available.
            </p>
          </section>
        </>
      )}
    </article>
  );
}

function issueOrganizations(billPages: BillPage[]): OrganizationListEntry[] {
  const orgs = new Map<string, OrganizationListEntry>();
  for (const b of billPages) {
    for (const section of b.hearings) {
      for (const org of section.organizations) {
        const existing = orgs.get(org.canonical_name);
        const aliases = new Set([...(existing?.aliases ?? []), ...(org.aliases ?? [])]);
        const positions = {
          Pro: existing?.positions.Pro ?? 0,
          Con: existing?.positions.Con ?? 0,
          Other: existing?.positions.Other ?? 0,
          Unknown: existing?.positions.Unknown ?? 0,
        };
        const position = normalizePosition(org.testifier_position);
        positions[position] += org.testifier_count ?? 0;
        const matchConfidence =
          existing && CONFIDENCE_RANK[existing.matchConfidence] >= CONFIDENCE_RANK[org.match_confidence]
            ? existing.matchConfidence
            : org.match_confidence;

        orgs.set(org.canonical_name, {
          slug: existing?.slug ?? slugify(org.canonical_name),
          canonicalName: org.canonical_name,
          aliases: Array.from(aliases).filter((a) => a !== org.canonical_name).sort(),
          matchConfidence,
          matchNotes: existing?.matchNotes ?? org.match_notes,
          testifierCount: (existing?.testifierCount ?? 0) + (org.testifier_count ?? 0),
          positions,
        });
      }
    }
  }
  return Array.from(orgs.values()).sort((a, b) => {
    const byTestifiers = b.testifierCount - a.testifierCount;
    if (byTestifiers !== 0) return byTestifiers;
    return a.canonicalName.localeCompare(b.canonicalName);
  });
}

function normalizePosition(position?: string): Position {
  if (position === "Pro" || position === "Con" || position === "Other") return position;
  return "Unknown";
}

function billPageMatchesIssue(page: BillPage, config: IssuePageConfig): boolean {
  const haystack = [
    page.bill.bill_id,
    page.bill.title,
    page.bill.description,
    ...page.hearings.flatMap((section) => [
      section.hearing.agenda_item_label,
      section.hearing.committee_name,
      ...section.organizations.map((o) => o.canonical_name),
    ]),
  ]
    .filter(Boolean)
    .join(" ")
    .toLowerCase();

  return config.keywords.some((keyword) => haystack.includes(keyword.toLowerCase()));
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded border border-stone-300 bg-stone-50 p-3">
      <div className="text-xs uppercase tracking-wider text-stone-500">{label}</div>
      <div className="mt-1 font-semibold text-stone-900">{value}</div>
    </div>
  );
}

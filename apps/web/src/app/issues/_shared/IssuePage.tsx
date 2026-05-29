import {
  listOrganizations,
  searchBills,
  searchHearings,
} from "@/lib/api";
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
  const billSearchFilters = { ...filters, topicKeywords: config.keywords };

  const [billResult, hearingResult, organizations] = await Promise.all([
    searchBills(billSearchFilters),
    searchHearings(hearingFiltersToSearch(hearingFilters, config.keywords)),
    listOrganizations(config.keywords),
  ]);

  return (
    <article className="space-y-10">
      <section className="space-y-3">
        <h1 className="text-3xl font-bold tracking-tight text-stone-900">
          {config.title}
        </h1>
        <p className="max-w-6xl text-stone-600">{config.description}</p>
      </section>

      {billResult.total === 0 ? (
        <p className="rounded border border-stone-300 bg-stone-50 p-4 text-sm text-stone-600">
          No {config.emptyLabel} legislation is available yet.
        </p>
      ) : (
        <>
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


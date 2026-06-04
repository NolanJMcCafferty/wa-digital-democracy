import { notFound } from "next/navigation";
import {
  listOrganizations,
  loadOrganizationPage,
  type HearingPage,
  type OrganizationCompensation,
  type OrganizationPersonAffiliation,
} from "@/lib/api";
import {
  HearingSearchResults,
  parseHearingFilters,
  type RawHearingSearchParams,
} from "@/app/hearings/HearingSearchResults";
import { filterHearings } from "@/app/hearings/filterHearings";
import { CompensationSection } from "./CompensationSection";

export const dynamic = "force-dynamic";

export async function generateStaticParams() {
  if (process.env.SKIP_BUILD_STATIC_PARAMS === "1") return [];
  const orgs = await listOrganizations();
  return orgs.map((o) => ({ slug: o.slug }));
}

export default async function OrganizationPage({
  params,
  searchParams,
}: {
  params: Promise<{ slug: string }>;
  searchParams: Promise<RawHearingSearchParams>;
}) {
  const { slug } = await params;
  const org = await loadOrganizationPage(slug);
  if (!org) notFound();

  const allHearings = appearancesToHearings(org.appearances);
  const filters = parseHearingFilters(await searchParams);
  const { hearings, total, offset, facets } = filterHearings(allHearings, filters);

  return (
    <article className="space-y-10">
      <section className="space-y-4">
        <p className="text-sm uppercase tracking-wider text-stone-500">
          Organization
        </p>
        <div className="space-y-3">
          <h1 className="text-3xl font-bold tracking-tight text-stone-900">
            {org.canonicalName}
          </h1>
          {org.aliases.length > 0 ? (
            <p className="text-sm text-stone-500">
              Also known as: {org.aliases.join(", ")}
            </p>
          ) : null}
        </div>
      </section>

      <section className="grid grid-cols-2 gap-3 text-sm">
        <Metric label="Hearing appearances" value={org.appearances.length.toLocaleString()} />
        <Metric label="Linked testifiers" value={org.testifierCount.toLocaleString()} />
      </section>

      {org.personAffiliations.length > 0 ? (
        <section aria-labelledby="people-heading" className="space-y-4">
          <div className="space-y-1">
            <h2 id="people-heading" className="text-xl font-semibold text-stone-900">
              Affiliated people
            </h2>
            <p className="text-sm text-stone-600">
              Source-backed person-organization relationships from testimony
              sign-ins and public records.
            </p>
          </div>
          <AffiliatedPeopleTable
            people={org.personAffiliations}
            compensation={org.compensation}
          />
        </section>
      ) : null}

      {org.compensation.some((c) => c.role === "filer") ? (
        <section aria-labelledby="comp-filer-heading" className="space-y-4">
          <div className="space-y-1">
            <h2 id="comp-filer-heading" className="text-xl font-semibold text-stone-900">
              Lobbying clients (paid this firm)
            </h2>
            <p className="text-sm text-stone-600">
              Annual totals from PDC L-2 filings where
              this organization is the registered lobbying firm receiving
              payment from a client.
            </p>
          </div>
          <CompensationSection
            rows={org.compensation.filter((c) => c.role === "filer")}
            counterpartyHeader="Client"
            counterpartyLabel="Client"
          />
        </section>
      ) : null}

      {org.compensation.some((c) => c.role === "employer") ? (
        <section aria-labelledby="comp-employer-heading" className="space-y-4">
          <div className="space-y-1">
            <h2 id="comp-employer-heading" className="text-xl font-semibold text-stone-900">
              Lobbying firms paid by this organization
            </h2>
            <p className="text-sm text-stone-600">
              Annual totals from PDC L-2 filings where
              this organization is the client paying a registered lobbying
              firm.
            </p>
          </div>
          <CompensationSection
            rows={org.compensation.filter((c) => c.role === "employer")}
            counterpartyHeader="Lobbying firm"
            counterpartyLabel="Lobbying firm"
            showFilings={false}
          />
        </section>
      ) : null}

      {org.contexts.length > 0 ? (
        <section aria-labelledby="context-heading" className="space-y-4">
          <div className="space-y-1">
            <h2 id="context-heading" className="text-xl font-semibold text-stone-900">
              Related public-record context
            </h2>
            <p className="text-sm text-stone-600">
              These records are matched context for the organization, not proof
              that a record caused or influenced testimony on a bill.
            </p>
          </div>
          <ul className="divide-y divide-stone-300 rounded border border-stone-300 bg-white">
            {org.contexts.map((c, idx) => (
              <li key={`${c.sourceKind}-${idx}`} className="p-4">
                <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                  <div className="space-y-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="rounded border border-stone-300 bg-stone-50 px-2 py-0.5 text-xs font-medium text-stone-700">
                        {contextLabel(c.contextType)}
                      </span>
                      <span className="text-xs uppercase tracking-wider text-stone-500">
                        {c.sourceLabel}
                      </span>
                    </div>
                    <p className="font-medium text-stone-900">{c.sourceName}</p>
                    {c.detail ? (
                      <p className="text-sm text-stone-600">{c.detail}</p>
                    ) : null}
                    <p className="text-xs text-stone-500">
                      {[formatAmount(c.amount), c.recordYear ? String(c.recordYear) : c.recordDate]
                        .filter(Boolean)
                        .join(" · ")}
                    </p>
                  </div>
                  {c.url ? (
                    <a
                      href={c.url}
                      className="text-sm text-blue-700 underline hover:text-blue-900"
                      rel="noreferrer"
                      target="_blank"
                    >
                      Source record →
                    </a>
                  ) : null}
                </div>
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      <section aria-labelledby="appearances-heading" className="space-y-4">
        <h2 id="appearances-heading" className="text-xl font-semibold text-stone-900">
          Hearing appearances
        </h2>
        <HearingSearchResults
          basePath={`/organizations/${slug}`}
          filters={filters}
          hearings={hearings}
          total={total}
          offset={offset}
          facets={facets}
          hiddenFilters={["speaker"]}
        />
      </section>

    </article>
  );
}

function AffiliatedPeopleTable({
  people,
  compensation,
}: {
  people: OrganizationPersonAffiliation[];
  compensation: OrganizationCompensation[];
}) {
  // Index compensation by (counterparty name, year). Many "filers" in
  // 9nnw-c693 are individual lobbyists, so the counterparty matches a
  // person on the affiliations table; scoping by year lets us show
  // separate totals for separate registration spans of the same person.
  const compByCounterpartyYear = new Map<
    string,
    { compensation: number; expenses: number }
  >();
  for (const r of compensation) {
    const year = Number(r.filingPeriod.slice(0, 4));
    if (!Number.isFinite(year)) continue;
    const counterparty = r.role === "filer" ? r.employerName : r.filerName;
    const nameKey = normalizeName(counterparty);
    if (!nameKey) continue;
    const key = `${nameKey}|${year}`;
    const cur = compByCounterpartyYear.get(key) ?? { compensation: 0, expenses: 0 };
    cur.compensation += Number(r.compensation) || 0;
    cur.expenses += Number(r.totalExpenses) || 0;
    compByCounterpartyYear.set(key, cur);
  }

  // Merge rows that represent the same person across different registration
  // spans. Same person + same relationship => one row, with years unioned.
  type Merged = OrganizationPersonAffiliation & { mergedYears: number[] };
  const mergedPeople: Merged[] = [];
  const mergedIndex = new Map<string, Merged>();
  for (const p of people) {
    const idKey = p.personId ? `id:${p.personId}` : `name:${normalizeName(p.personName)}`;
    const key = `${idKey}|${p.relationshipType}|${p.sourceKind}`;
    const years = parseRecordYears(p.recordYears);
    const existing = mergedIndex.get(key);
    if (existing) {
      const seen = new Set(existing.mergedYears);
      for (const y of years) seen.add(y);
      existing.mergedYears = Array.from(seen).sort((a, b) => a - b);
      existing.sourceCount += p.sourceCount;
    } else {
      const merged: Merged = { ...p, mergedYears: years };
      mergedIndex.set(key, merged);
      mergedPeople.push(merged);
    }
  }

  return (
    <div className="overflow-x-auto rounded border border-stone-300 bg-white">
      <table className="min-w-full divide-y divide-stone-200 text-sm">
        <colgroup>
          <col className="w-72" />
          <col className="w-32" />
          <col className="w-40" />
          <col className="w-40" />
        </colgroup>
        <thead className="bg-stone-50 text-left text-xs uppercase tracking-wider text-stone-500">
          <tr>
            <th scope="col" className="px-4 py-3 font-medium">Person</th>
            <th scope="col" className="px-4 py-3 font-medium">Relationship</th>
            <th scope="col" className="px-4 py-3 font-medium">Years registered</th>
            <th scope="col" className="px-4 py-3 font-medium text-right">Total billings</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-stone-200">
          {mergedPeople.map((person, idx) => {
            const pdcLobbyistUrl = buildPDCLobbyistUrl(person);
            const isLobbyist = person.sourceKind === "pdc_lobbyist_employment";
            const years = isLobbyist ? person.mergedYears : [];
            const nameKey = normalizeName(person.personName);
            let compTotal = 0;
            let expTotal = 0;
            let anyYearMatched = false;
            for (const y of years) {
              const v = compByCounterpartyYear.get(`${nameKey}|${y}`);
              if (v) {
                compTotal += v.compensation;
                expTotal += v.expenses;
                anyYearMatched = true;
              }
            }
            const showTotals = isLobbyist && anyYearMatched;
            return (
              <tr key={`${person.personId ?? "raw"}-${person.relationshipType}-${person.sourceKind}-${idx}`}>
                <td className="px-4 py-3 align-top">
                  <div className="font-medium text-stone-900">
                    {pdcLobbyistUrl ? (
                      <a
                        href={pdcLobbyistUrl}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="text-blue-700 underline hover:text-blue-900"
                      >
                        {person.personName}
                      </a>
                    ) : (
                      person.personName
                    )}
                  </div>
                </td>
                <td className="px-4 py-3 align-top">
                  <div className="text-stone-800">{relationshipLabel(person.relationshipType)}</div>
                </td>
                <td className="px-4 py-3 align-top text-stone-700">
                  {years.length > 0 ? formatYearRange(years) : <span className="text-stone-400">—</span>}
                </td>
                <td className="px-4 py-3 align-top text-right tabular-nums text-stone-900">
                  {showTotals ? (
                    formatAmount(String(compTotal + expTotal))
                  ) : (
                    <span className="text-stone-400">—</span>
                  )}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function normalizeName(name: string): string {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, " ")
    .trim();
}

function parseRecordYears(recordYears?: string): number[] {
  if (!recordYears) return [];
  const seen = new Set<number>();
  for (const m of recordYears.matchAll(/\b(?:19|20)\d{2}\b/g)) {
    const n = Number(m[0]);
    if (Number.isInteger(n)) seen.add(n);
  }
  return Array.from(seen).sort((a, b) => a - b);
}

function formatYearRange(years: number[]): string {
  if (years.length === 0) return "";
  if (years.length === 1) return String(years[0]);
  const min = years[0];
  const max = years[years.length - 1];
  const isContiguous = max - min + 1 === years.length;
  if (isContiguous) return `${min}–${max}`;
  return years.join(", ");
}

function buildPDCLobbyistUrl(person: OrganizationPersonAffiliation): string | undefined {
  if (!person.pdcLobbyistId) return undefined;
  const latestYear = latestRecordYear(person.recordYears);
  if (!latestYear) return undefined;

  const pathID = encodeURIComponent(`${person.pdcLobbyistId}-${latestYear}`);
  return `https://www.pdc.wa.gov/political-disclosure-reporting-data/browse-search-data/lobbyists/${pathID}`;
}

function latestRecordYear(recordYears?: string): string | undefined {
  const years = recordYears
    ?.match(/\b(?:19|20)\d{2}\b/g)
    ?.map(Number)
    .filter((year) => Number.isInteger(year));
  if (!years || years.length === 0) return undefined;
  return String(Math.max(...years));
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded border border-stone-300 bg-stone-50 p-3">
      <div className="text-xs uppercase tracking-wider text-stone-500">{label}</div>
      <div className="mt-1 font-semibold text-stone-900">{value}</div>
    </div>
  );
}

function relationshipLabel(value: string): string {
  switch (value) {
    case "lobbyist_for":
      return "Lobbyist";
    case "testified_for":
      return "Testified";
    case "signed_in_for":
      return "Hearing sign in";
    case "lobbying_firm_for":
      return "Lobbying firm";
    case "paid_lobbying_for":
      return "Paid lobbying";
    case "paid_by":
      return "Paid by";
    case "employed_by":
      return "Employed by";
    case "vendor_contact_for":
      return "Vendor contact";
    case "campaign_contributor_affiliation":
      return "Contributor affiliation";
    case "spoke_for_org":
      return "Spoke for organization";
    case "reviewed_manual":
      return "Reviewed affiliation";
    default:
      return value.replaceAll("_", " ");
  }
}

function contextLabel(contextType: string): string {
  switch (contextType) {
    case "lobbying_registration":
      return "Lobbying";
    case "state_contract":
      return "Contract";
    case "state_vendor":
      return "Vendor";
    case "state_vendor_payment":
      return "Payment";
    case "federal_award":
      return "Federal award";
    default:
      return "Record";
  }
}

type OrgAppearance = NonNullable<Awaited<ReturnType<typeof loadOrganizationPage>>>["appearances"][number];

function appearancesToHearings(appearances: OrgAppearance[]): HearingPage[] {
  const byHearing = new Map<number, HearingPage>();
  for (const a of appearances) {
    if (!a.hearingId) continue;
    let h = byHearing.get(a.hearingId);
    if (!h) {
      const date = new Date(a.meetingDatetime);
      const title = `${a.committeeName} · ${date.toLocaleDateString("en-US", {
        month: "short",
        day: "numeric",
        year: "numeric",
      })}`;
      h = {
        hearingId: a.hearingId,
        title,
        committeeName: a.committeeName,
        chamber: a.chamber ?? "",
        meetingDatetime: a.meetingDatetime,
        agendaItems: [],
      };
      byHearing.set(a.hearingId, h);
    }
    h.agendaItems.push({
      csiAgendaItemId: a.csiAgendaItemId ?? "",
      agendaItemLabel: a.hearingTitle,
      biennium: a.biennium,
      billId: a.billId,
      billPrefix: a.billPrefix,
      billNumber: a.billNumber,
      testifierCount: a.testifierCount,
      testifiedCount: 0,
    });
  }
  return Array.from(byHearing.values()).sort((a, b) =>
    b.meetingDatetime.localeCompare(a.meetingDatetime),
  );
}

function formatAmount(amount?: string): string {
  if (!amount) return "";
  const n = Number(amount);
  if (!Number.isFinite(n)) return amount;
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    maximumFractionDigits: 0,
  }).format(n);
}

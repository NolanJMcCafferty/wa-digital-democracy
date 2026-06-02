import Link from "next/link";
import { classifyBillStage, type BillStage } from "@/lib/billStatus";
import type { BillSearchFilters, BillListEntry } from "@/lib/api";

export const STATUS_LABEL: Record<string, string> = {
  in_progress: "In progress",
  passed: "Passed",
  failed: "Failed / vetoed",
};

const STAGE_ORDER: string[] = [
  "Introduced",
  "In committee",
  "On floor calendar",
  "Passed chamber",
  "Passed Legislature",
  "On Governor's desk",
  "Signed by Governor",
  "Session law",
  "Vetoed",
  "Reintroduced",
  "Shelved",
  "Died",
  "In progress",
];

function orderStages(stages: string[]): string[] {
  const rank = (s: string) => {
    const i = STAGE_ORDER.indexOf(s);
    return i === -1 ? STAGE_ORDER.length : i;
  };
  return [...stages].sort((a, b) => rank(a) - rank(b));
}

export const PARTY_LABEL: Record<string, string> = {
  D: "Democrat",
  R: "Republican",
};

export const PREFIX_LABEL: Record<string, string> = {
  HB: "House Bill",
  SB: "Senate Bill",
  HJR: "House Joint Resolution",
  SJR: "Senate Joint Resolution",
  HCR: "House Concurrent Resolution",
  SCR: "Senate Concurrent Resolution",
  HJM: "House Joint Memorial",
  SJM: "Senate Joint Memorial",
  HR: "House Resolution",
  SR: "Senate Resolution",
  EHR: "Engrossed House Resolution",
  SGA: "Senate Gubernatorial Appointment",
};

export type RawBillSearchParams = Record<string, string | string[] | undefined>;

export type BillSearchFacets = {
  prefixes: string[];
  chambers: string[];
  parties: string[];
  statuses: string[];
};

export function pickString(v: string | string[] | undefined): string {
  if (Array.isArray(v)) return v[0] ?? "";
  return v ?? "";
}

export function parseBillFilters(raw: RawBillSearchParams): BillSearchFilters {
  const page = Math.max(1, parseInt(pickString(raw.page) || "1", 10) || 1);
  const limit = Math.min(100, Math.max(1, parseInt(pickString(raw.limit) || "50", 10) || 50));
  return {
    q: pickString(raw.q).trim() || undefined,
    prefix: pickString(raw.prefix) || undefined,
    chamber: pickString(raw.chamber) || undefined,
    party: pickString(raw.party) || undefined,
    status: pickString(raw.status) || undefined,
    leadSponsor: pickString(raw.leadSponsor) || undefined,
    page,
    limit,
  };
}

export function buildBillQueryString(
  filters: BillSearchFilters,
  overrides: Partial<BillSearchFilters> = {},
): string {
  const merged = { ...filters, ...overrides };
  const params = new URLSearchParams();
  if (merged.q) params.set("q", merged.q);
  if (merged.prefix) params.set("prefix", merged.prefix);
  if (merged.chamber) params.set("chamber", merged.chamber);
  if (merged.party) params.set("party", merged.party);
  if (merged.status) params.set("status", merged.status);
  if (merged.leadSponsor) params.set("leadSponsor", merged.leadSponsor);
  if (merged.page && merged.page > 1) params.set("page", String(merged.page));
  if (merged.limit && merged.limit !== 50) params.set("limit", String(merged.limit));
  const qs = params.toString();
  return qs ? `?${qs}` : "";
}

export function BillSearchResults({
  basePath,
  filters,
  bills,
  total,
  offset,
  facets,
  hiddenFilters = [],
  leadSponsorOptions = [],
  highlightedBillKeys = [],
}: {
  basePath: string;
  filters: BillSearchFilters;
  bills: BillListEntry[];
  total: number;
  offset: number;
  facets: BillSearchFacets;
  hiddenFilters?: Array<"chamber" | "party" | "status" | "prefix">;
  leadSponsorOptions?: Array<{ value: string; label: string }>;
  highlightedBillKeys?: string[];
}) {
  const hidden = new Set(hiddenFilters);
  const highlighted = new Set(highlightedBillKeys);
  const limit = filters.limit ?? 50;
  const page = filters.page ?? 1;
  const totalPages = Math.max(1, Math.ceil(total / limit));
  const startIndex = total === 0 ? 0 : offset + 1;
  const endIndex = Math.min(offset + bills.length, total);

  const activeFilters: Array<{ label: string; key: keyof BillSearchFilters }> = [];
  if (filters.q) activeFilters.push({ label: `“${filters.q}”`, key: "q" });
  if (filters.prefix) activeFilters.push({ label: filters.prefix, key: "prefix" });
  if (filters.chamber) activeFilters.push({ label: filters.chamber, key: "chamber" });
  if (filters.party) activeFilters.push({ label: PARTY_LABEL[filters.party] ?? filters.party, key: "party" });
  if (filters.status) activeFilters.push({ label: STATUS_LABEL[filters.status] ?? filters.status, key: "status" });
  if (filters.leadSponsor) {
    const label = leadSponsorOptions.find((o) => o.value === filters.leadSponsor)?.label ?? filters.leadSponsor;
    activeFilters.push({ label: `Lead sponsor: ${label}`, key: "leadSponsor" });
  }

  return (
    <div className="grid gap-6 lg:grid-cols-[18rem_1fr]">
      <aside className="lg:sticky lg:top-6 lg:mt-10 lg:self-start">
        <form
          method="GET"
          action={basePath}
          className="space-y-5 rounded-lg border border-stone-300 bg-white p-5"
        >
          <div className="flex items-baseline justify-between">
            <h2 className="text-sm font-semibold uppercase tracking-wider text-stone-700">
              Filters
            </h2>
            {activeFilters.length > 0 ? (
              <Link
                href={basePath}
                className="text-xs font-medium text-stone-500 hover:text-stone-800"
              >
                Clear all
              </Link>
            ) : null}
          </div>

          <div className="space-y-1.5">
            <label htmlFor="q" className="block text-xs font-semibold uppercase tracking-wider text-stone-500">
              Search
            </label>
            <input
              id="q"
              name="q"
              type="search"
              defaultValue={filters.q ?? ""}
              placeholder="Bill text, title, ID…"
              className="w-full rounded border border-stone-300 px-3 py-1.5 text-sm focus:border-stone-500 focus:outline-none focus:ring-1 focus:ring-stone-500"
            />
          </div>

          {leadSponsorOptions.length === 0 ? null : (
            <FilterGroup
              legend="Lead sponsor"
              name="leadSponsor"
              current={filters.leadSponsor}
              options={leadSponsorOptions}
            />
          )}

          {hidden.has("chamber") ? null : (
            <FilterGroup
              legend="Chamber"
              name="chamber"
              current={filters.chamber}
              options={facets.chambers.map((c) => ({ value: c, label: c }))}
            />
          )}

          {hidden.has("party") ? null : (
            <FilterGroup
              legend="Party"
              name="party"
              current={filters.party}
              options={facets.parties.map((p) => ({
                value: p,
                label: PARTY_LABEL[p] ?? p,
              }))}
            />
          )}

          {hidden.has("status") ? null : (
            <FilterGroup
              legend="Status"
              name="status"
              current={filters.status}
              options={orderStages(facets.statuses).map((s) => ({
                value: s,
                label: STATUS_LABEL[s] ?? s,
              }))}
            />
          )}

          {hidden.has("prefix") ? null : (
            <FilterGroup
              legend="Bill type"
              name="prefix"
              current={filters.prefix}
              options={facets.prefixes.map((p) => ({
                value: p,
                label: PREFIX_LABEL[p] ? `${p} — ${PREFIX_LABEL[p]}` : p,
              }))}
            />
          )}

          <div className="flex gap-2 pt-2">
            <button
              type="submit"
              className="flex-1 rounded bg-stone-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-stone-700"
            >
              Apply
            </button>
          </div>
        </form>
      </aside>

      <div className="space-y-4">
        <div className="flex flex-wrap items-baseline justify-between gap-3">
          <p className="text-sm text-stone-600">
            {total === 0 ? (
              <>No bills match.</>
            ) : (
              <>
                Showing{" "}
                <span className="font-medium text-stone-900">
                  {startIndex.toLocaleString()}–{endIndex.toLocaleString()}
                </span>{" "}
                of{" "}
                <span className="font-medium text-stone-900">
                  {total.toLocaleString()}
                </span>{" "}
                bills
              </>
            )}
          </p>
          {activeFilters.length > 0 ? (
            <div className="flex flex-wrap gap-1.5">
              {activeFilters.map((f) => (
                <Link
                  key={f.key}
                  href={`${basePath}${buildBillQueryString(filters, { [f.key]: undefined, page: 1 })}`}
                  className="inline-flex items-center gap-1 rounded-full border border-stone-300 bg-stone-100 px-2.5 py-0.5 text-xs text-stone-700 hover:border-stone-500 hover:bg-stone-200"
                  title="Remove filter"
                >
                  {f.label}
                  <span aria-hidden className="text-stone-500">×</span>
                </Link>
              ))}
            </div>
          ) : null}
        </div>

        {bills.length === 0 ? (
          <div className="rounded-lg border border-stone-300 bg-white p-10 text-center text-sm text-stone-600">
            No bills match the current filters.{" "}
            <Link href={basePath} className="font-medium text-stone-900 underline">
              Clear filters
            </Link>
          </div>
        ) : (
          <div className="overflow-x-auto rounded-lg border border-stone-300 bg-white">
            <table className="w-full text-sm">
              <thead className="border-b border-stone-300 bg-stone-50 text-left text-xs uppercase tracking-wider text-stone-600">
                <tr>
                  <th className="px-4 py-2.5 font-semibold">Bill</th>
                  <th className="px-4 py-2.5 font-semibold">Title</th>
                  <th className="px-4 py-2.5 font-semibold">Lead sponsor</th>
                  <th className="px-4 py-2.5 font-semibold">Chamber</th>
                  <th className="px-4 py-2.5 font-semibold">Status</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-stone-200">
                {bills.map((b) => {
                  const billHref = `/bills/${b.biennium}/${b.billPrefix}${b.billNumber}`;
                  const sponsorName = b.leadDisplay ?? b.leadSponsor ?? "";
                  const isHighlighted = highlighted.has(billRenderKey(b));
                  return (
                    <tr
                      key={billRenderKey(b)}
                      className={isHighlighted ? "bg-amber-50/70 hover:bg-amber-100/70" : "hover:bg-stone-50"}
                    >
                      <td className="px-4 py-2.5 align-top">
                        <Link
                          href={billHref}
                          className="font-mono font-medium text-stone-900 hover:underline"
                        >
                          {b.billId}
                        </Link>
                      </td>
                      <td className="px-4 py-2.5 align-top">
                        <Link href={billHref} className="text-stone-800 hover:underline">
                          {b.title || <span className="text-stone-400">—</span>}
                        </Link>
                      </td>
                      <td className="px-4 py-2.5 align-top">
                        {sponsorName ? (
                          b.leadSlug ? (
                            <Link
                              href={`/legislators/${b.leadSlug}`}
                              className="text-stone-800 hover:underline"
                            >
                              {sponsorName}
                            </Link>
                          ) : (
                            <span className="text-stone-800">{sponsorName}</span>
                          )
                        ) : (
                          <span className="text-stone-400">—</span>
                        )}
                        {b.leadParty ? (
                          <span className="ml-1 text-xs text-stone-500">({b.leadParty})</span>
                        ) : null}
                      </td>
                      <td className="px-4 py-2.5 align-top text-stone-700">
                        {b.chamberOrigin || <span className="text-stone-400">—</span>}
                      </td>
                      <td className="px-4 py-2.5 align-top">
                        <StatusPill bucket={b.statusBucket} label={b.currentStatus} />
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}

        {totalPages > 1 ? (
          <Pagination
            basePath={basePath}
            page={page}
            totalPages={totalPages}
            filters={filters}
          />
        ) : null}
      </div>
    </div>
  );
}

export function billRenderKey(b: Pick<BillListEntry, "biennium" | "billPrefix" | "billNumber">): string {
  return `${b.biennium}-${b.billPrefix}-${b.billNumber}`;
}

function FilterGroup({
  legend,
  name,
  current,
  options,
}: {
  legend: string;
  name: string;
  current?: string;
  options: Array<{ value: string; label: string }>;
}) {
  if (options.length === 0) return null;
  return (
    <fieldset className="space-y-1.5">
      <legend className="text-xs font-semibold uppercase tracking-wider text-stone-500">
        {legend}
      </legend>
      <div className="space-y-1">
        <label className="flex items-center gap-2 text-sm text-stone-700">
          <input
            type="radio"
            name={name}
            value=""
            defaultChecked={!current}
            className="text-stone-900 focus:ring-stone-500"
          />
          <span>Any</span>
        </label>
        {options.map((o) => (
          <label key={o.value} className="flex items-center gap-2 text-sm text-stone-700">
            <input
              type="radio"
              name={name}
              value={o.value}
              defaultChecked={current === o.value}
              className="text-stone-900 focus:ring-stone-500"
            />
            <span>{o.label}</span>
          </label>
        ))}
      </div>
    </fieldset>
  );
}

const STAGE_CLASSES: Record<BillStage, string> = {
  "Session law": "border-emerald-300 bg-emerald-50 text-emerald-800",
  "Signed by Governor": "border-emerald-300 bg-emerald-50 text-emerald-800",
  "On Governor's desk": "border-amber-300 bg-amber-50 text-amber-800",
  "Passed Legislature": "border-emerald-300 bg-emerald-50 text-emerald-800",
  "Passed chamber": "border-teal-300 bg-teal-50 text-teal-800",
  "On floor calendar": "border-sky-300 bg-sky-50 text-sky-800",
  "In committee": "border-purple-300 bg-purple-50 text-purple-800",
  "Introduced": "border-stone-300 bg-stone-50 text-stone-700",
  "In progress": "border-amber-300 bg-amber-50 text-amber-800",
  "Vetoed": "border-rose-300 bg-rose-50 text-rose-800",
  "Died": "border-rose-300 bg-rose-50 text-rose-800",
  "Shelved": "border-stone-600 bg-stone-200 text-stone-900",
  "Reintroduced": "border-stone-300 bg-stone-50 text-stone-600",
};

function StatusPill({ bucket, label }: { bucket?: string; label?: string }) {
  const stage = classifyBillStage(label, bucket);
  return (
    <span
      className={`inline-block max-w-[28ch] truncate rounded-full border px-2 py-0.5 text-xs ${STAGE_CLASSES[stage]}`}
      title={label || stage}
    >
      {stage}
    </span>
  );
}

function Pagination({
  basePath,
  page,
  totalPages,
  filters,
}: {
  basePath: string;
  page: number;
  totalPages: number;
  filters: BillSearchFilters;
}) {
  const pages = pageWindow(page, totalPages);
  const linkCls =
    "inline-flex h-8 min-w-[2rem] items-center justify-center rounded border border-stone-300 bg-white px-2 text-sm text-stone-700 hover:border-stone-500 hover:bg-stone-100";
  const currentCls =
    "inline-flex h-8 min-w-[2rem] items-center justify-center rounded border border-stone-900 bg-stone-900 px-2 text-sm font-medium text-white";
  const disabledCls =
    "inline-flex h-8 min-w-[2rem] items-center justify-center rounded border border-stone-200 bg-stone-50 px-2 text-sm text-stone-400";

  return (
    <nav className="flex flex-wrap items-center justify-center gap-1.5" aria-label="Pagination">
      {page > 1 ? (
        <Link
          href={`${basePath}${buildBillQueryString(filters, { page: page - 1 })}`}
          className={linkCls}
          rel="prev"
        >
          ← Prev
        </Link>
      ) : (
        <span className={disabledCls} aria-hidden>← Prev</span>
      )}
      {pages.map((p, i) =>
        p === "…" ? (
          <span key={`gap-${i}`} className="px-1 text-sm text-stone-500">
            …
          </span>
        ) : p === page ? (
          <span key={p} className={currentCls} aria-current="page">
            {p}
          </span>
        ) : (
          <Link
            key={p}
            href={`${basePath}${buildBillQueryString(filters, { page: p })}`}
            className={linkCls}
          >
            {p}
          </Link>
        ),
      )}
      {page < totalPages ? (
        <Link
          href={`${basePath}${buildBillQueryString(filters, { page: page + 1 })}`}
          className={linkCls}
          rel="next"
        >
          Next →
        </Link>
      ) : (
        <span className={disabledCls} aria-hidden>Next →</span>
      )}
    </nav>
  );
}

function pageWindow(page: number, total: number): Array<number | "…"> {
  const out: Array<number | "…"> = [];
  const window = 1;
  const add = (n: number) => {
    if (n >= 1 && n <= total && !out.includes(n)) out.push(n);
  };
  add(1);
  if (page - window > 2) out.push("…");
  for (let p = Math.max(2, page - window); p <= Math.min(total - 1, page + window); p++) {
    add(p);
  }
  if (page + window < total - 1) out.push("…");
  if (total > 1) add(total);
  return out;
}

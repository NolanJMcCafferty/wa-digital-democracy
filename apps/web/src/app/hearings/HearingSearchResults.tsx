import Link from "next/link";
import { formatDate } from "@/lib/format";
import type { HearingPage, HearingSearchFilters } from "@/lib/api";
import { ISSUE_PAGE_CONFIGS } from "../issues/_shared/config";

export type RawHearingSearchParams = Record<string, string | string[] | undefined>;

const TOPIC_OPTIONS = ISSUE_PAGE_CONFIGS.map((c) => ({
  slug: c.slug,
  label: c.title,
  keywords: c.keywords,
}));

export type HearingPageFilters = {
  committee?: string;
  bill?: string;
  speaker?: string;
  chambers: string[];
  topics: string[]; // issue slugs from ISSUE_PAGE_CONFIGS
  biennium?: string;
  page: number;
  limit: number;
};

export type HearingSearchFacets = {
  chambers: string[];
  committees: string[];
  biennia: string[];
};

function pickString(v: string | string[] | undefined): string {
  if (Array.isArray(v)) return v[0] ?? "";
  return v ?? "";
}

function pickStringArray(v: string | string[] | undefined): string[] {
  if (Array.isArray(v)) return v.filter(Boolean);
  if (typeof v === "string" && v) return [v];
  return [];
}

export function parseHearingFilters(raw: RawHearingSearchParams): HearingPageFilters {
  const page = Math.max(1, parseInt(pickString(raw.page) || "1", 10) || 1);
  const limit = Math.min(100, Math.max(1, parseInt(pickString(raw.limit) || "50", 10) || 50));
  return {
    committee: pickString(raw.committee).trim() || undefined,
    bill: pickString(raw.bill).trim() || undefined,
    speaker: pickString(raw.speaker).trim() || undefined,
    chambers: pickStringArray(raw.chamber),
    topics: pickStringArray(raw.topic).filter((slug) =>
      TOPIC_OPTIONS.some((t) => t.slug === slug),
    ),
    biennium: pickString(raw.biennium).trim() || undefined,
    page,
    limit,
  };
}

function topicKeywords(slugs: string[]): string[] {
  const set = new Set<string>();
  for (const slug of slugs) {
    const t = TOPIC_OPTIONS.find((o) => o.slug === slug);
    if (!t) continue;
    for (const kw of t.keywords) set.add(kw);
  }
  return Array.from(set);
}

// hearingFiltersToSearch converts the URL-shaped page filters to the
// HearingSearchFilters expected by searchHearings(). Pages call this
// before passing to searchHearings — extra fixed keywords (e.g. the
// issue page's config keywords) can be merged in at the call site.
export function hearingFiltersToSearch(
  f: HearingPageFilters,
  extraTopicKeywords: string[] = [],
): HearingSearchFilters {
  const merged = new Set<string>([...topicKeywords(f.topics), ...extraTopicKeywords]);
  return {
    committee: f.committee,
    bill: f.bill,
    speaker: f.speaker,
    chambers: f.chambers,
    topicKeywords: Array.from(merged),
    biennium: f.biennium,
    page: f.page,
    limit: f.limit,
  };
}

function buildQueryString(
  basePath: string,
  filters: HearingPageFilters,
  overrides: Partial<HearingPageFilters> = {},
): string {
  const merged: HearingPageFilters = { ...filters, ...overrides };
  const params = new URLSearchParams();
  if (merged.committee) params.set("committee", merged.committee);
  if (merged.bill) params.set("bill", merged.bill);
  if (merged.speaker) params.set("speaker", merged.speaker);
  for (const c of merged.chambers) params.append("chamber", c);
  for (const t of merged.topics) params.append("topic", t);
  if (merged.biennium) params.set("biennium", merged.biennium);
  if (merged.page > 1) params.set("page", String(merged.page));
  if (merged.limit !== 50) params.set("limit", String(merged.limit));
  const qs = params.toString();
  return qs ? `${basePath}?${qs}` : basePath;
}

export function HearingSearchResults({
  basePath,
  filters,
  hearings,
  total,
  offset,
  facets,
  hiddenFilters = [],
}: {
  basePath: string;
  filters: HearingPageFilters;
  hearings: HearingPage[];
  total: number;
  offset: number;
  facets: HearingSearchFacets;
  hiddenFilters?: Array<"committee" | "bill" | "speaker" | "chamber" | "topic" | "biennium">;
}) {
  const hidden = new Set(hiddenFilters);
  const totalPages = Math.max(1, Math.ceil(total / filters.limit));
  const startIndex = total === 0 ? 0 : offset + 1;
  const endIndex = Math.min(offset + hearings.length, total);

  type ActiveFilter =
    | { kind: "scalar"; key: "committee" | "bill" | "speaker" | "biennium"; label: string }
    | { kind: "chamber"; value: string; label: string }
    | { kind: "topic"; slug: string; label: string };

  const activeFilters: ActiveFilter[] = [];
  if (filters.committee) activeFilters.push({ kind: "scalar", key: "committee", label: `Committee: ${filters.committee}` });
  if (filters.bill) activeFilters.push({ kind: "scalar", key: "bill", label: `Bill: ${filters.bill}` });
  if (filters.speaker) activeFilters.push({ kind: "scalar", key: "speaker", label: `Speaker: ${filters.speaker}` });
  if (filters.biennium) activeFilters.push({ kind: "scalar", key: "biennium", label: filters.biennium });
  for (const c of filters.chambers) activeFilters.push({ kind: "chamber", value: c, label: c });
  for (const slug of filters.topics) {
    const t = TOPIC_OPTIONS.find((o) => o.slug === slug);
    if (t) activeFilters.push({ kind: "topic", slug, label: t.label });
  }

  function removeHref(f: ActiveFilter): string {
    if (f.kind === "scalar") {
      return buildQueryString(basePath, filters, { [f.key]: undefined, page: 1 });
    }
    if (f.kind === "chamber") {
      return buildQueryString(basePath, filters, {
        chambers: filters.chambers.filter((c) => c !== f.value),
        page: 1,
      });
    }
    return buildQueryString(basePath, filters, {
      topics: filters.topics.filter((s) => s !== f.slug),
      page: 1,
    });
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

          {hidden.has("committee") ? null : (
            <TextFilter
              id="committee"
              name="committee"
              label="Committee"
              defaultValue={filters.committee ?? ""}
              placeholder="Committee name…"
            />
          )}
          {hidden.has("bill") ? null : (
            <TextFilter
              id="bill"
              name="bill"
              label="Bill"
              defaultValue={filters.bill ?? ""}
              placeholder="Bill ID or title…"
            />
          )}
          {hidden.has("speaker") ? null : (
            <TextFilter
              id="speaker"
              name="speaker"
              label="Speaker"
              defaultValue={filters.speaker ?? ""}
              placeholder="Speaker name…"
            />
          )}

          {hidden.has("chamber") ? null : (
            <CheckboxGroup
              legend="Chamber"
              name="chamber"
              current={filters.chambers}
              options={facets.chambers.map((c) => ({ value: c, label: c }))}
            />
          )}

          {hidden.has("topic") ? null : (
            <CheckboxGroup
              legend="Topic"
              name="topic"
              current={filters.topics}
              options={TOPIC_OPTIONS.map((t) => ({ value: t.slug, label: t.label }))}
            />
          )}

          {hidden.has("biennium") ? null : facets.biennia.length > 0 ? (
            <RadioGroup
              legend="Session year"
              name="biennium"
              current={filters.biennium}
              options={facets.biennia.map((b) => ({ value: b, label: b }))}
            />
          ) : null}

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
              <>No hearings match.</>
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
                hearings
              </>
            )}
          </p>
          {activeFilters.length > 0 ? (
            <div className="flex flex-wrap gap-1.5">
              {activeFilters.map((f, i) => (
                <Link
                  key={`${f.kind}-${i}`}
                  href={removeHref(f)}
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

        {hearings.length === 0 ? (
          <div className="rounded-lg border border-stone-300 bg-white p-10 text-center text-sm text-stone-600">
            No hearings match the current filters.{" "}
            <Link href={basePath} className="font-medium text-stone-900 underline">
              Clear filters
            </Link>
          </div>
        ) : (
          <div className="overflow-x-auto rounded-lg border border-stone-300 bg-white">
            <table className="w-full text-sm">
              <thead className="border-b border-stone-300 bg-stone-50 text-left text-xs uppercase tracking-wider text-stone-600">
                <tr>
                  <th className="px-4 py-2.5 font-semibold">Name</th>
                  <th className="px-4 py-2.5 font-semibold">Committee</th>
                  <th className="px-4 py-2.5 font-semibold">Chamber</th>
                  <th className="w-40 px-4 py-2.5 font-semibold">Date</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-stone-200">
                {hearings.map((h) => {
                  const href = `/hearings/${h.hearingId}`;
                  const billSummary = h.agendaItems.map((a) => a.billId).filter(Boolean).join(", ");
                  return (
                    <tr key={h.hearingId} className="hover:bg-stone-50">
                      <td className="px-4 py-2.5 align-top">
                        <Link href={href} className="font-medium text-stone-900 hover:underline">
                          {h.title}
                        </Link>
                        <div className="mt-0.5 text-xs text-stone-500">
                          {h.agendaItems.length.toLocaleString()} agenda item{h.agendaItems.length === 1 ? "" : "s"}
                          {billSummary ? <> · <span className="font-mono">{billSummary}</span></> : null}
                        </div>
                      </td>
                      <td className="px-4 py-2.5 align-top text-stone-700">
                        {h.committeeName || <span className="text-stone-400">—</span>}
                      </td>
                      <td className="px-4 py-2.5 align-top text-stone-700">
                        {h.chamber || <span className="text-stone-400">—</span>}
                      </td>
                      <td className="w-40 whitespace-nowrap px-4 py-2.5 align-top text-stone-700 tabular-nums">
                        {formatDate(h.meetingDatetime)}
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
            page={filters.page}
            totalPages={totalPages}
            filters={filters}
          />
        ) : null}
      </div>
    </div>
  );
}

function TextFilter({
  id,
  name,
  label,
  defaultValue,
  placeholder,
}: {
  id: string;
  name: string;
  label: string;
  defaultValue: string;
  placeholder: string;
}) {
  return (
    <div className="space-y-1.5">
      <label htmlFor={id} className="block text-xs font-semibold uppercase tracking-wider text-stone-500">
        {label}
      </label>
      <input
        id={id}
        name={name}
        type="search"
        defaultValue={defaultValue}
        placeholder={placeholder}
        className="w-full rounded border border-stone-300 px-3 py-1.5 text-sm focus:border-stone-500 focus:outline-none focus:ring-1 focus:ring-stone-500"
      />
    </div>
  );
}

function CheckboxGroup({
  legend,
  name,
  current,
  options,
}: {
  legend: string;
  name: string;
  current: string[];
  options: Array<{ value: string; label: string }>;
}) {
  if (options.length === 0) return null;
  const selected = new Set(current);
  return (
    <fieldset className="space-y-1.5">
      <legend className="text-xs font-semibold uppercase tracking-wider text-stone-500">
        {legend}
      </legend>
      <div className="space-y-1">
        {options.map((o) => (
          <label key={o.value} className="flex items-center gap-2 text-sm text-stone-700">
            <input
              type="checkbox"
              name={name}
              value={o.value}
              defaultChecked={selected.has(o.value)}
              className="rounded border-stone-300 text-stone-900 focus:ring-stone-500"
            />
            <span>{o.label}</span>
          </label>
        ))}
      </div>
    </fieldset>
  );
}

function RadioGroup({
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

function Pagination({
  basePath,
  page,
  totalPages,
  filters,
}: {
  basePath: string;
  page: number;
  totalPages: number;
  filters: HearingPageFilters;
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
          href={buildQueryString(basePath, filters, { page: page - 1 })}
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
            href={buildQueryString(basePath, filters, { page: p })}
            className={linkCls}
          >
            {p}
          </Link>
        ),
      )}
      {page < totalPages ? (
        <Link
          href={buildQueryString(basePath, filters, { page: page + 1 })}
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

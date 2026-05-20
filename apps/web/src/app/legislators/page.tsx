import Link from "next/link";
import { listLegislators, type LegislatorListEntry } from "@/lib/api";

const PARTY_LABEL: Record<string, string> = {
  D: "Democrat",
  R: "Republican",
};

const CHAMBER_LABEL: Record<string, string> = {
  Senate: "Senate",
  House: "House",
};

type RawSearchParams = Record<string, string | string[] | undefined>;

type PageFilters = {
  q?: string;
  chambers: string[];
  parties: string[];
  page: number;
  limit: number;
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

function parseFilters(raw: RawSearchParams): PageFilters {
  const page = Math.max(1, parseInt(pickString(raw.page) || "1", 10) || 1);
  const limit = Math.min(200, Math.max(1, parseInt(pickString(raw.limit) || "50", 10) || 50));
  return {
    q: pickString(raw.q).trim() || undefined,
    chambers: pickStringArray(raw.chamber),
    parties: pickStringArray(raw.party),
    page,
    limit,
  };
}

function buildQueryString(
  filters: PageFilters,
  overrides: Partial<PageFilters> = {},
): string {
  const merged: PageFilters = { ...filters, ...overrides };
  const params = new URLSearchParams();
  if (merged.q) params.set("q", merged.q);
  for (const c of merged.chambers) params.append("chamber", c);
  for (const p of merged.parties) params.append("party", p);
  if (merged.page > 1) params.set("page", String(merged.page));
  if (merged.limit !== 50) params.set("limit", String(merged.limit));
  const qs = params.toString();
  return qs ? `?${qs}` : "";
}

function applyFilters(legislators: LegislatorListEntry[], f: PageFilters): LegislatorListEntry[] {
  const q = f.q?.toLowerCase() ?? "";
  const chamberSet = new Set(f.chambers);
  const partySet = new Set(f.parties);
  return legislators.filter((l) => {
    if (q) {
      const haystack = [
        l.name,
        l.displayName,
        l.firstName,
        l.lastName,
        l.district,
      ]
        .filter(Boolean)
        .join(" ")
        .toLowerCase();
      if (!haystack.includes(q)) return false;
    }
    if (chamberSet.size > 0 && !chamberSet.has(l.chamber ?? "")) return false;
    if (partySet.size > 0 && !partySet.has(l.party ?? "")) return false;
    return true;
  });
}

function distinctSorted(values: Array<string | undefined>): string[] {
  const set = new Set<string>();
  for (const v of values) {
    if (v && v.trim()) set.add(v);
  }
  return Array.from(set).sort();
}

function legislatorRenderKey(l: LegislatorListEntry): string {
  return [
    l.slug,
    l.displayName ?? l.name,
    l.chamber ?? "",
    l.district ?? "",
  ].join("|");
}

export default async function LegislatorsPage({
  searchParams,
}: {
  searchParams: Promise<RawSearchParams>;
}) {
  const raw = await searchParams;
  const filters = parseFilters(raw);
  const all = await listLegislators();
  const filtered = applyFilters(all, filters);

  // Stable display order: chamber, district numerically, then last name.
  filtered.sort((a, b) => {
    const ac = a.chamber ?? "";
    const bc = b.chamber ?? "";
    if (ac !== bc) return ac.localeCompare(bc);
    const ad = parseInt(a.district ?? "0", 10) || 0;
    const bd = parseInt(b.district ?? "0", 10) || 0;
    if (ad !== bd) return ad - bd;
    return (a.lastName ?? a.name).localeCompare(b.lastName ?? b.name);
  });

  const total = filtered.length;
  const totalPages = Math.max(1, Math.ceil(total / filters.limit));
  const offset = (filters.page - 1) * filters.limit;
  const pageItems = filtered.slice(offset, offset + filters.limit);
  const startIndex = total === 0 ? 0 : offset + 1;
  const endIndex = Math.min(offset + pageItems.length, total);

  const chamberOptions = distinctSorted(all.map((l) => l.chamber));
  const partyOptions = distinctSorted(all.map((l) => l.party));

  type ActiveFilter =
    | { kind: "scalar"; key: "q"; label: string }
    | { kind: "chamber"; value: string; label: string }
    | { kind: "party"; value: string; label: string };

  const activeFilters: ActiveFilter[] = [];
  if (filters.q) activeFilters.push({ kind: "scalar", key: "q", label: `“${filters.q}”` });
  for (const c of filters.chambers) {
    activeFilters.push({ kind: "chamber", value: c, label: CHAMBER_LABEL[c] ?? c });
  }
  for (const p of filters.parties) {
    activeFilters.push({ kind: "party", value: p, label: PARTY_LABEL[p] ?? p });
  }

  function removeHref(f: ActiveFilter): string {
    if (f.kind === "scalar") {
      return `/legislators${buildQueryString(filters, { [f.key]: undefined, page: 1 })}`;
    }
    if (f.kind === "chamber") {
      return `/legislators${buildQueryString(filters, {
        chambers: filters.chambers.filter((c) => c !== f.value),
        page: 1,
      })}`;
    }
    return `/legislators${buildQueryString(filters, {
      parties: filters.parties.filter((p) => p !== f.value),
      page: 1,
    })}`;
  }

  return (
    <div className="space-y-6">
      <div className="space-y-2">
        <h1 className="text-3xl font-bold tracking-tight text-stone-900">Legislators</h1>
        <p className="text-stone-600">
          Browse the {all.length.toLocaleString()} Washington legislators
          currently tracked. Each row links to their sponsored bills,
          district, and contact information.
        </p>
      </div>

      <div className="grid gap-6 lg:grid-cols-[18rem_1fr]">
        {/* Sidebar */}
        <aside className="lg:sticky lg:top-6 lg:mt-10 lg:self-start">
          <form
            method="GET"
            action="/legislators"
            className="space-y-5 rounded-lg border border-stone-300 bg-white p-5"
          >
            <div className="flex items-baseline justify-between">
              <h2 className="text-sm font-semibold uppercase tracking-wider text-stone-700">
                Filters
              </h2>
              {activeFilters.length > 0 ? (
                <Link
                  href="/legislators"
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
                placeholder="Name or district…"
                className="w-full rounded border border-stone-300 px-3 py-1.5 text-sm focus:border-stone-500 focus:outline-none focus:ring-1 focus:ring-stone-500"
              />
            </div>

            <CheckboxGroup
              legend="Chamber"
              name="chamber"
              current={filters.chambers}
              options={chamberOptions.map((c) => ({ value: c, label: CHAMBER_LABEL[c] ?? c }))}
            />

            <CheckboxGroup
              legend="Party"
              name="party"
              current={filters.parties}
              options={partyOptions.map((p) => ({ value: p, label: PARTY_LABEL[p] ?? p }))}
            />

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

        {/* Results */}
        <div className="space-y-4">
          <div className="flex flex-wrap items-baseline justify-between gap-3">
            <p className="text-sm text-stone-600">
              {total === 0 ? (
                <>No legislators match.</>
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
                  legislators
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

          {pageItems.length === 0 ? (
            <div className="rounded-lg border border-stone-300 bg-white p-10 text-center text-sm text-stone-600">
              No legislators match the current filters.{" "}
              <Link href="/legislators" className="font-medium text-stone-900 underline">
                Clear filters
              </Link>
            </div>
          ) : (
            <div className="overflow-x-auto rounded-lg border border-stone-300 bg-white">
              <table className="w-full text-sm">
                <thead className="border-b border-stone-300 bg-stone-50 text-left text-xs uppercase tracking-wider text-stone-600">
                  <tr>
                    <th className="px-4 py-2.5 font-semibold">Name</th>
                    <th className="px-4 py-2.5 font-semibold">Chamber</th>
                    <th className="px-4 py-2.5 font-semibold">District</th>
                    <th className="px-4 py-2.5 font-semibold">Party</th>
                    <th className="px-4 py-2.5 font-semibold text-right">Bills</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-stone-200">
                  {pageItems.map((l) => {
                    const display = l.displayName ?? l.name;
                    return (
                      <tr key={legislatorRenderKey(l)} className="hover:bg-stone-50">
                        <td className="px-4 py-2.5 align-top">
                          <Link
                            href={`/legislators/${l.slug}`}
                            className="font-medium text-stone-900 hover:underline"
                          >
                            {display}
                          </Link>
                        </td>
                        <td className="px-4 py-2.5 align-top text-stone-700">
                          {l.chamber || <span className="text-stone-400">—</span>}
                        </td>
                        <td className="px-4 py-2.5 align-top text-stone-700 tabular-nums">
                          {l.district || <span className="text-stone-400">—</span>}
                        </td>
                        <td className="px-4 py-2.5 align-top">
                          {l.party ? (
                            <PartyPill party={l.party} />
                          ) : (
                            <span className="text-stone-400">—</span>
                          )}
                        </td>
                        <td className="px-4 py-2.5 align-top text-right tabular-nums text-stone-700">
                          {(l.billCount ?? 0).toLocaleString()}
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
              page={filters.page}
              totalPages={totalPages}
              filters={filters}
            />
          ) : null}
        </div>
      </div>
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

function PartyPill({ party }: { party: string }) {
  const cls =
    party === "D"
      ? "border-blue-300 bg-blue-50 text-blue-800"
      : party === "R"
        ? "border-rose-300 bg-rose-50 text-rose-800"
        : "border-stone-300 bg-stone-50 text-stone-700";
  return (
    <span className={`inline-block rounded-full border px-2 py-0.5 text-xs ${cls}`}>
      {PARTY_LABEL[party] ?? party}
    </span>
  );
}

function Pagination({
  page,
  totalPages,
  filters,
}: {
  page: number;
  totalPages: number;
  filters: PageFilters;
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
          href={`/legislators${buildQueryString(filters, { page: page - 1 })}`}
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
            href={`/legislators${buildQueryString(filters, { page: p })}`}
            className={linkCls}
          >
            {p}
          </Link>
        ),
      )}
      {page < totalPages ? (
        <Link
          href={`/legislators${buildQueryString(filters, { page: page + 1 })}`}
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

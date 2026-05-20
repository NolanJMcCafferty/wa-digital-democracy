import Link from "next/link";
import type { OrganizationBundleEntry } from "@/lib/loadBundle";

type RawSearchParams = Record<string, string | string[] | undefined>;

export type OrganizationPageFilters = {
  q?: string;
  page: number;
  limit: number;
};

function pickString(v: string | string[] | undefined): string {
  if (Array.isArray(v)) return v[0] ?? "";
  return v ?? "";
}

function paramName(key: keyof OrganizationPageFilters, paramPrefix?: string): string {
  if (!paramPrefix) return key;
  return `${paramPrefix}${key[0].toUpperCase()}${key.slice(1)}`;
}

export function parseOrganizationFilters(
  raw: RawSearchParams,
  paramPrefix?: string,
): OrganizationPageFilters {
  const page = Math.max(1, parseInt(pickString(raw[paramName("page", paramPrefix)]) || "1", 10) || 1);
  const limit = Math.min(100, Math.max(1, parseInt(pickString(raw[paramName("limit", paramPrefix)]) || "50", 10) || 50));
  return {
    q: pickString(raw[paramName("q", paramPrefix)]).trim() || undefined,
    page,
    limit,
  };
}

function buildQueryString(
  filters: OrganizationPageFilters,
  overrides: Partial<OrganizationPageFilters> = {},
  paramPrefix?: string,
): string {
  const merged = { ...filters, ...overrides };
  const params = new URLSearchParams();
  if (merged.q) params.set(paramName("q", paramPrefix), merged.q);
  if (merged.page > 1) params.set(paramName("page", paramPrefix), String(merged.page));
  if (merged.limit !== 50) params.set(paramName("limit", paramPrefix), String(merged.limit));
  const qs = params.toString();
  return qs ? `?${qs}` : "";
}

function applyFilters(
  organizations: OrganizationBundleEntry[],
  filters: OrganizationPageFilters,
): OrganizationBundleEntry[] {
  const q = filters.q?.toLowerCase();
  return organizations.filter((o) => {
    if (o.matchConfidence !== "confirmed") {
      return false;
    }
    if (q) {
      const hay = [o.canonicalName, ...o.aliases].join(" ").toLowerCase();
      if (!hay.includes(q)) return false;
    }
    return true;
  });
}

export function OrganizationSearchResults({
  basePath,
  filters,
  organizations,
  paramPrefix,
}: {
  basePath: string;
  filters: OrganizationPageFilters;
  organizations: OrganizationBundleEntry[];
  paramPrefix?: string;
}) {
  const filtered = applyFilters(organizations, filters);
  const total = filtered.length;
  const totalPages = Math.max(1, Math.ceil(total / filters.limit));
  const offset = (filters.page - 1) * filters.limit;
  const visible = filtered.slice(offset, offset + filters.limit);
  const startIndex = total === 0 ? 0 : offset + 1;
  const endIndex = Math.min(offset + visible.length, total);

  type ActiveFilter = { label: string; key: "q" };
  const activeFilters: ActiveFilter[] = [];
  if (filters.q) activeFilters.push({ label: `“${filters.q}”`, key: "q" });

  const href = (overrides: Partial<OrganizationPageFilters> = {}) =>
    `${basePath}${buildQueryString(filters, overrides, paramPrefix)}`;

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
            <label htmlFor={paramName("q", paramPrefix)} className="block text-xs font-semibold uppercase tracking-wider text-stone-500">
              Search
            </label>
            <input
              id={paramName("q", paramPrefix)}
              name={paramName("q", paramPrefix)}
              type="search"
              defaultValue={filters.q ?? ""}
              placeholder="Name or alias..."
              className="w-full rounded border border-stone-300 px-3 py-1.5 text-sm focus:border-stone-500 focus:outline-none focus:ring-1 focus:ring-stone-500"
            />
          </div>

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
              <>No organizations match.</>
            ) : (
              <>
                Showing{" "}
                <span className="font-medium text-stone-900">
                  {startIndex.toLocaleString()}-{endIndex.toLocaleString()}
                </span>{" "}
                of{" "}
                <span className="font-medium text-stone-900">
                  {total.toLocaleString()}
                </span>{" "}
                organizations
              </>
            )}
          </p>
          {activeFilters.length > 0 ? (
            <div className="flex flex-wrap gap-1.5">
              {activeFilters.map((f) => (
                <Link
                  key={f.key}
                  href={href({ [f.key]: undefined, page: 1 })}
                  className="inline-flex items-center gap-1 rounded-full border border-stone-300 bg-stone-100 px-2.5 py-0.5 text-xs text-stone-700 hover:border-stone-500 hover:bg-stone-200"
                  title="Remove filter"
                >
                  {f.label}
                  <span aria-hidden className="text-stone-500">x</span>
                </Link>
              ))}
            </div>
          ) : null}
        </div>

        {visible.length === 0 ? (
          <div className="rounded-lg border border-stone-300 bg-white p-10 text-center text-sm text-stone-600">
            No organizations match the current filters.{" "}
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
                  <th className="w-28 px-4 py-2.5 text-right font-semibold">Testifiers</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-stone-200">
                {visible.map((o, i) => (
                  <tr key={`${o.slug}-${offset + i}`} className="hover:bg-stone-50">
                    <td className="px-4 py-2.5 align-top">
                      <Link
                        href={`/organizations/${o.slug}`}
                        className="font-medium text-stone-900 hover:underline"
                      >
                        {o.canonicalName}
                      </Link>
                      {o.aliases.length > 0 ? (
                        <div className="mt-0.5 line-clamp-1 text-xs text-stone-500">
                          aka {o.aliases.join(", ")}
                        </div>
                      ) : null}
                    </td>
                    <td className="w-28 px-4 py-2.5 text-right align-top tabular-nums text-stone-700">
                      {o.testifierCount.toLocaleString()}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {totalPages > 1 ? (
          <Pagination
            page={filters.page}
            totalPages={totalPages}
            filters={filters}
            basePath={basePath}
            paramPrefix={paramPrefix}
          />
        ) : null}
      </div>
    </div>
  );
}

function Pagination({
  page,
  totalPages,
  filters,
  basePath,
  paramPrefix,
}: {
  page: number;
  totalPages: number;
  filters: OrganizationPageFilters;
  basePath: string;
  paramPrefix?: string;
}) {
  const pages = pageWindow(page, totalPages);
  const linkCls =
    "inline-flex h-8 min-w-[2rem] items-center justify-center rounded border border-stone-300 bg-white px-2 text-sm text-stone-700 hover:border-stone-500 hover:bg-stone-100";
  const currentCls =
    "inline-flex h-8 min-w-[2rem] items-center justify-center rounded border border-stone-900 bg-stone-900 px-2 text-sm font-medium text-white";
  const disabledCls =
    "inline-flex h-8 min-w-[2rem] items-center justify-center rounded border border-stone-200 bg-stone-50 px-2 text-sm text-stone-400";
  const href = (overrides: Partial<OrganizationPageFilters>) =>
    `${basePath}${buildQueryString(filters, overrides, paramPrefix)}`;
  return (
    <nav className="flex flex-wrap items-center justify-center gap-1.5" aria-label="Pagination">
      {page > 1 ? (
        <Link
          href={href({ page: page - 1 })}
          className={linkCls}
          rel="prev"
        >
          Prev
        </Link>
      ) : (
        <span className={disabledCls} aria-hidden>Prev</span>
      )}
      {pages.map((p, i) =>
        p === "..." ? (
          <span key={`gap-${i}`} className="px-1 text-sm text-stone-500">
            ...
          </span>
        ) : p === page ? (
          <span key={p} className={currentCls} aria-current="page">
            {p}
          </span>
        ) : (
          <Link
            key={p}
            href={href({ page: p })}
            className={linkCls}
          >
            {p}
          </Link>
        ),
      )}
      {page < totalPages ? (
        <Link
          href={href({ page: page + 1 })}
          className={linkCls}
          rel="next"
        >
          Next
        </Link>
      ) : (
        <span className={disabledCls} aria-hidden>Next</span>
      )}
    </nav>
  );
}

function pageWindow(page: number, total: number): Array<number | "..."> {
  const out: Array<number | "..."> = [];
  const window = 1;
  const add = (n: number) => {
    if (n >= 1 && n <= total && !out.includes(n)) out.push(n);
  };
  add(1);
  if (page - window > 2) out.push("...");
  for (let p = Math.max(2, page - window); p <= Math.min(total - 1, page + window); p++) {
    add(p);
  }
  if (page + window < total - 1) out.push("...");
  if (total > 1) add(total);
  return out;
}

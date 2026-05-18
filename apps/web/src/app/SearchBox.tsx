"use client";

import Link from "next/link";
import { useDeferredValue, useMemo, useState } from "react";
import type { SearchResult } from "@/lib/searchIndex";

const TYPE_STYLE: Record<SearchResult["type"], string> = {
  Bill: "bg-blue-100 text-blue-800",
  Hearing: "bg-amber-100 text-amber-800",
  Organization: "bg-emerald-100 text-emerald-800",
  Legislator: "bg-purple-100 text-purple-800",
};

export function SearchBox({ results }: { results: SearchResult[] }) {
  const [query, setQuery] = useState("");
  const deferredQuery = useDeferredValue(query);
  const trimmed = deferredQuery.trim().toLowerCase();

  const matches = useMemo(() => {
    if (!trimmed) return results.slice(0, 8);
    const terms = trimmed.split(/\s+/).filter(Boolean);
    return results
      .map((r) => {
        const score = terms.reduce((n, term) => n + (r.searchText.includes(term) ? 1 : 0), 0);
        return { result: r, score };
      })
      .filter((r) => r.score > 0)
      .sort((a, b) => b.score - a.score || a.result.title.localeCompare(b.result.title))
      .slice(0, 12)
      .map((r) => r.result);
  }, [results, trimmed]);

  return (
    <section
      aria-labelledby="site-search-heading"
      className="space-y-4 rounded-lg border border-stone-300 bg-white p-5"
    >
      <div className="space-y-1">
        <h2 id="site-search-heading" className="text-xl font-semibold text-stone-900">
          Search the local graph
        </h2>
        <p className="text-sm text-stone-600">
          Search bills, hearings, organizations, and legislators generated from
          the local bundles.
        </p>
      </div>

      <label className="block">
        <span className="sr-only">Search bills, hearings, organizations, and legislators</span>
        <input
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Try “HB 1501”, “Housing”, “WSCAI”, or “Reed”"
          className="w-full rounded border border-stone-300 bg-stone-50 px-3 py-2 text-stone-900 placeholder:text-stone-400 focus:border-stone-500 focus:outline-none focus:ring-2 focus:ring-stone-300"
        />
      </label>

      <div className="text-xs text-stone-500">
        {trimmed
          ? `${matches.length.toLocaleString()} result${matches.length === 1 ? "" : "s"}`
          : `Showing ${matches.length.toLocaleString()} of ${results.length.toLocaleString()} indexed pages`}
      </div>

      {matches.length === 0 ? (
        <p className="rounded border border-stone-200 bg-stone-50 p-3 text-sm text-stone-600">
          No matches yet. The current prototype only searches generated local
          bundle pages.
        </p>
      ) : (
        <ul className="divide-y divide-stone-200 rounded border border-stone-200">
          {matches.map((r) => (
            <li key={`${r.type}-${r.href}`}>
              <Link href={r.href} className="block p-3 hover:bg-stone-50">
                <div className="flex flex-col gap-1 sm:flex-row sm:items-baseline sm:justify-between">
                  <div>
                    <span className="font-medium text-stone-900">{r.title}</span>
                    <p className="text-sm text-stone-600">{r.subtitle}</p>
                  </div>
                  <span
                    className={`${TYPE_STYLE[r.type]} w-fit rounded px-2 py-0.5 text-xs font-medium uppercase tracking-wider`}
                  >
                    {r.type}
                  </span>
                </div>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

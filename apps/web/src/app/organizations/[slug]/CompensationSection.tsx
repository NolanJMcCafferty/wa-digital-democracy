"use client";

import { useMemo, useState } from "react";
import type { OrganizationCompensation } from "@/lib/api";
import { aggregateCompensationByYear, formatAmount } from "./compensation";

export function CompensationSection({
  rows,
  counterpartyHeader,
  counterpartyLabel,
  showFilings = true,
}: {
  rows: OrganizationCompensation[];
  counterpartyHeader: string;
  counterpartyLabel: string;
  showFilings?: boolean;
}) {
  const aggregated = useMemo(() => aggregateCompensationByYear(rows), [rows]);
  const allYears = useMemo(
    () => Array.from(new Set(aggregated.map((a) => a.year))).sort((a, b) => b - a),
    [aggregated],
  );

  const [search, setSearch] = useState("");
  const [yearFilter, setYearFilter] = useState<Set<number>>(new Set());
  const [minComp, setMinComp] = useState("");

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    const min = Number(minComp) || 0;
    return aggregated.filter((r) => {
      if (q && !r.counterparty.toLowerCase().includes(q)) return false;
      if (yearFilter.size > 0 && !yearFilter.has(r.year)) return false;
      if (min > 0 && r.compensation < min) return false;
      return true;
    });
  }, [aggregated, search, yearFilter, minComp]);

  const totalComp = filtered.reduce((sum, r) => sum + r.compensation, 0);
  const totalExp = filtered.reduce((sum, r) => sum + r.expenses, 0);
  const distinctCounterparties = new Set(filtered.map((r) => r.counterparty)).size;

  const hasActiveFilters = search !== "" || yearFilter.size > 0 || minComp !== "";

  function toggleYear(year: number) {
    const next = new Set(yearFilter);
    if (next.has(year)) next.delete(year);
    else next.add(year);
    setYearFilter(next);
  }

  function clearAll() {
    setSearch("");
    setYearFilter(new Set());
    setMinComp("");
  }

  return (
    <div className="grid gap-6 lg:grid-cols-[18rem_1fr]">
      <aside className="lg:sticky lg:top-6 lg:self-start">
        <div className="space-y-5 rounded-lg border border-stone-300 bg-white p-5">
          <div className="flex items-baseline justify-between">
            <h3 className="text-sm font-semibold uppercase tracking-wider text-stone-700">
              Filters
            </h3>
            {hasActiveFilters ? (
              <button
                type="button"
                onClick={clearAll}
                className="text-xs font-medium text-stone-500 hover:text-stone-800"
              >
                Clear all
              </button>
            ) : null}
          </div>

          <div className="space-y-2">
            <label
              htmlFor="comp-search"
              className="block text-xs font-medium uppercase tracking-wider text-stone-600"
            >
              {counterpartyLabel}
            </label>
            <input
              id="comp-search"
              type="search"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder={`${counterpartyLabel} name…`}
              className="w-full rounded border border-stone-300 px-3 py-2 text-sm focus:border-stone-500 focus:outline-none"
            />
          </div>

          <div className="space-y-2">
            <label
              htmlFor="comp-min"
              className="block text-xs font-medium uppercase tracking-wider text-stone-600"
            >
              Min annual compensation
            </label>
            <input
              id="comp-min"
              type="number"
              min={0}
              step={1000}
              value={minComp}
              onChange={(e) => setMinComp(e.target.value)}
              placeholder="0"
              className="w-full rounded border border-stone-300 px-3 py-2 text-sm focus:border-stone-500 focus:outline-none"
            />
          </div>

          {allYears.length > 0 ? (
            <fieldset className="space-y-2">
              <legend className="text-xs font-medium uppercase tracking-wider text-stone-600">
                Year
              </legend>
              <div className="max-h-64 space-y-1 overflow-y-auto pr-1">
                {allYears.map((year) => (
                  <label
                    key={year}
                    className="flex items-center gap-2 text-sm text-stone-800"
                  >
                    <input
                      type="checkbox"
                      checked={yearFilter.has(year)}
                      onChange={() => toggleYear(year)}
                      className="h-4 w-4 rounded border-stone-400"
                    />
                    {year}
                  </label>
                ))}
              </div>
            </fieldset>
          ) : null}
        </div>
      </aside>

      <div className="space-y-3">
        <div className="flex flex-wrap items-baseline justify-between gap-3 text-sm text-stone-600">
          <span>
            Showing <strong className="text-stone-900">{filtered.length}</strong>{" "}
            of {aggregated.length} rows
            {distinctCounterparties > 0
              ? ` · ${distinctCounterparties} ${counterpartyLabel.toLowerCase()}${distinctCounterparties === 1 ? "" : "s"}`
              : null}
          </span>
          <span className="tabular-nums">
            Total billings:{" "}
            <strong className="text-stone-900">{formatAmount(totalComp + totalExp)}</strong>
          </span>
        </div>

        <div className="overflow-x-auto rounded border border-stone-300 bg-white">
          <table className="min-w-full divide-y divide-stone-200 text-sm">
            <thead className="bg-stone-50 text-left text-xs uppercase tracking-wider text-stone-500">
              <tr>
                <th scope="col" className="px-4 py-3 font-medium">Year</th>
                <th scope="col" className="px-4 py-3 font-medium">{counterpartyHeader}</th>
                <th scope="col" className="px-4 py-3 font-medium text-right">Compensation</th>
                <th scope="col" className="px-4 py-3 font-medium text-right">Expenses</th>
                {showFilings ? (
                  <th scope="col" className="px-4 py-3 font-medium text-right">Filings</th>
                ) : null}
                <th scope="col" className="px-4 py-3 font-medium">Latest filing</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-stone-200">
              {filtered.length === 0 ? (
                <tr>
                  <td colSpan={showFilings ? 6 : 5} className="px-4 py-8 text-center text-stone-500">
                    No rows match the current filters.
                  </td>
                </tr>
              ) : (
                filtered.map((r) => (
                  <tr key={r.key}>
                    <td className="px-4 py-3 align-top text-stone-800">{r.year}</td>
                    <td className="px-4 py-3 align-top text-stone-900">{r.counterparty}</td>
                    <td className="px-4 py-3 align-top text-right tabular-nums text-stone-900">
                      {formatAmount(r.compensation)}
                    </td>
                    <td className="px-4 py-3 align-top text-right tabular-nums text-stone-700">
                      {formatAmount(r.expenses)}
                    </td>
                    {showFilings ? (
                      <td className="px-4 py-3 align-top text-right tabular-nums text-stone-700">
                        {r.filings}
                      </td>
                    ) : null}
                    <td className="px-4 py-3 align-top">
                      {r.latestUrl ? (
                        <a
                          href={r.latestUrl}
                          className="text-sm text-blue-700 underline hover:text-blue-900"
                          rel="noreferrer"
                          target="_blank"
                        >
                          L-2 filing →
                        </a>
                      ) : (
                        <span className="text-stone-400">—</span>
                      )}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}

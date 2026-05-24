"use client";

import { useState } from "react";
import type { Position, TestifierSummary } from "@/lib/pageTypes";

const POSITION_ORDER: Position[] = ["Pro", "Con", "Other"];

const POSITION_STYLE: Record<Position, string> = {
  Pro: "text-emerald-800 bg-emerald-100",
  Con: "text-rose-800 bg-rose-100",
  Other: "text-stone-700 bg-stone-200",
  Unknown: "text-stone-500 bg-stone-100",
};

export function TestifierTable({ testifiers }: { testifiers: TestifierSummary[] }) {
  const [showRegisteredOnly, setShowRegisteredOnly] = useState(false);

  const groups: Record<Position, TestifierSummary[]> = {
    Pro: [],
    Con: [],
    Other: [],
    Unknown: [],
  };
  for (const t of testifiers) {
    groups[t.position]?.push(t);
  }

  const total = testifiers.length;
  const testified = testifiers.filter((t) => t.testified).length;
  const registeredOnly = total - testified;

  return (
    <section aria-labelledby="testifiers-heading" className="space-y-4">
      <div className="flex items-baseline justify-between">
        <h2
          id="testifiers-heading"
          className="text-xl font-semibold text-stone-900"
        >
          Who testified
        </h2>
        <span className="text-sm text-stone-500">
          {total.toLocaleString()} signed in · {testified.toLocaleString()}{" "}
          testified
        </span>
      </div>

      <label className="flex items-center gap-2 text-sm text-stone-600">
        <input
          type="checkbox"
          checked={showRegisteredOnly}
          onChange={(e) => setShowRegisteredOnly(e.target.checked)}
          className="h-4 w-4 rounded border-stone-300 text-stone-700 focus:ring-stone-500"
        />
        Include {registeredOnly.toLocaleString()} sign-ins who registered a
        position but did not testify
      </label>

      <div className="grid grid-cols-3 gap-4">
        {POSITION_ORDER.map((p) => {
          const all = groups[p];
          const rows = showRegisteredOnly ? all : all.filter((t) => t.testified);
          const countLabel = showRegisteredOnly
            ? rows.length.toLocaleString()
            : `${rows.length.toLocaleString()} of ${all.length.toLocaleString()}`;
          return (
            <div key={p} className="space-y-2">
              <h3 className="flex items-baseline gap-2 text-sm font-medium uppercase tracking-wider text-stone-600">
                {p}
                <span className="rounded bg-stone-100 px-1.5 py-0.5 text-xs tabular-nums">
                  {countLabel}
                </span>
              </h3>
              {rows.length === 0 ? (
                <p className="text-sm text-stone-400">
                  {all.length === 0
                    ? "—"
                    : "No one testified in person; toggle above to see registered positions."}
                </p>
              ) : (
                <ul className="divide-y divide-stone-200 rounded border border-stone-300 bg-white text-sm">
                  {rows.slice(0, 50).map((t, i) => (
                    <li
                      key={`${t.raw_name}-${i}`}
                      className="flex flex-col gap-0.5 px-3 py-2"
                    >
                      <span className="font-medium text-stone-900">
                        {t.raw_name}
                      </span>
                      {t.raw_organization ? (
                        <span className="text-stone-600">
                          {t.raw_organization}
                        </span>
                      ) : null}
                      {!t.testified ? (
                        <span className="text-xs text-stone-400">
                          registered position; did not testify
                        </span>
                      ) : null}
                    </li>
                  ))}
                  {rows.length > 50 ? (
                    <li className="px-3 py-2 text-xs text-stone-500">
                      … and {(rows.length - 50).toLocaleString()} more
                    </li>
                  ) : null}
                </ul>
              )}
            </div>
          );
        })}
      </div>

      {groups.Unknown.length > 0 ? (
        <p className="text-xs text-stone-500">
          {groups.Unknown.length.toLocaleString()} additional sign-ins with
          unrecorded position.
        </p>
      ) : null}
      <p className="text-xs text-stone-500">
        Source:{" "}
        <span className={POSITION_STYLE.Pro + " inline-block rounded px-1.5 py-0.5 mr-1"}>
          Pro
        </span>
        <span className={POSITION_STYLE.Con + " inline-block rounded px-1.5 py-0.5 mr-1"}>
          Con
        </span>
        <span className={POSITION_STYLE.Other + " inline-block rounded px-1.5 py-0.5 mr-1"}>
          Other
        </span>
        positions are taken from CSI sign-ins (public record per RCW 42.56).
      </p>
    </section>
  );
}

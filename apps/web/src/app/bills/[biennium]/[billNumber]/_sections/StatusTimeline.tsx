"use client";

import { useState } from "react";
import type { Status, StatusEntry } from "@/lib/pageTypes";
import { formatDate } from "@/lib/format";
import { isMilestoneStatus } from "@/lib/billStatus";

export function StatusTimeline({ status }: { status: Status }) {
  const timeline = status.timeline ?? [];
  const reversed: StatusEntry[] = [...timeline].reverse();
  const filtered = reversed.filter((e) => isMilestoneStatus(e.history_line));
  const milestones: StatusEntry[] =
    filtered.length > 0 ? filtered : reversed.slice(0, 3);
  const hasMore = reversed.length > milestones.length;

  const [showAll, setShowAll] = useState(false);
  const visible = showAll ? reversed : milestones;

  return (
    <section aria-labelledby="status-heading" className="space-y-4">
      <div className="flex items-baseline justify-between">
        <h2 id="status-heading" className="text-xl font-semibold text-stone-900">
          Status
        </h2>
        {timeline.length > 0 ? (
          <span className="text-sm text-stone-500 tabular-nums">
            {milestones.length.toLocaleString()} key event
            {milestones.length === 1 ? "" : "s"} ·{" "}
            {timeline.length.toLocaleString()} total
          </span>
        ) : null}
      </div>

      {status.current ? (
        <p className="text-stone-700">
          <span className="font-medium">Current status:</span>{" "}
          <span className="text-stone-900">{status.current}</span>
        </p>
      ) : null}

      {timeline.length === 0 ? (
        <p className="text-sm text-stone-500">No status changes recorded.</p>
      ) : (
        <>
          <ol
            id="status-timeline-list"
            className="border-l-2 border-stone-300 pl-6 space-y-3"
          >
            {visible.map((e, i) => (
              <li key={`${e.action_date}-${i}`} className="relative">
                <span
                  aria-hidden
                  className="absolute -left-[29px] top-1 h-3 w-3 rounded-full bg-stone-400 ring-2 ring-stone-50"
                />
                <div className="text-sm text-stone-500 tabular-nums">
                  {formatDate(e.action_date)}
                </div>
                <div className="text-stone-800">{e.history_line}</div>
              </li>
            ))}
          </ol>

          {hasMore ? (
            <button
              type="button"
              aria-expanded={showAll}
              aria-controls="status-timeline-list"
              onClick={() => setShowAll((v) => !v)}
              className="inline-flex items-center justify-center rounded border border-stone-400 bg-stone-50 px-4 py-2 text-sm font-medium text-stone-800 hover:bg-stone-100 focus:outline-none focus:ring-2 focus:ring-stone-500 focus:ring-offset-2"
            >
              {showAll
                ? "Show key events only"
                : `Show all ${reversed.length.toLocaleString()} events`}
            </button>
          ) : null}
        </>
      )}
    </section>
  );
}

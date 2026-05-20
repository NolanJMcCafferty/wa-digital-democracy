"use client";

import { useState } from "react";
import type { DiarizedTranscript } from "@/lib/loadBundle";
import { formatMS, tvwDeepLink } from "@/lib/format";

export function DiarizedTranscriptSection({
  transcript,
  tvwEventId,
}: {
  transcript: DiarizedTranscript;
  tvwEventId?: string;
}) {
  const segments = transcript.segments ?? [];
  const [show, setShow] = useState(false);

  if (segments.length === 0) return null;

  const last = segments[segments.length - 1];
  const totalMS = last ? last.end_ms : 0;

  return (
    <section aria-labelledby="full-transcript-heading" className="space-y-4">
      <div className="flex items-baseline justify-between">
        <h2
          id="full-transcript-heading"
          className="text-xl font-semibold text-stone-900"
        >
          Full hearing transcript
        </h2>
        <span className="text-sm text-stone-500 tabular-nums">
          {segments.length.toLocaleString()} segments · {formatMS(totalMS)}
        </span>
      </div>

      <div className="space-y-4 rounded-lg border border-stone-300 bg-white p-5">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="space-y-1">
            <p className="text-sm font-medium text-stone-900">
              Diarized transcript covering the entire hearing.
            </p>
            <p className="text-sm text-stone-600">
              Speakers are anonymous cluster labels until reviewed.
            </p>
          </div>
          <button
            type="button"
            aria-expanded={show}
            aria-controls="full-transcript-segments"
            onClick={() => setShow((v) => !v)}
            className="inline-flex items-center justify-center rounded border border-stone-400 bg-stone-50 px-4 py-2 text-sm font-medium text-stone-800 hover:bg-stone-100 focus:outline-none focus:ring-2 focus:ring-stone-500 focus:ring-offset-2"
          >
            {show
              ? "Hide full transcript"
              : `Show ${segments.length.toLocaleString()} segments`}
          </button>
        </div>

        {show ? (
          <ol id="full-transcript-segments" className="space-y-3">
            {segments.map((s, i) => {
              const link = tvwEventId
                ? tvwDeepLink(tvwEventId, s.start_ms)
                : "#";
              return (
                <li
                  key={`${s.start_ms}-${i}`}
                  className="rounded border border-stone-300 bg-stone-50 p-4"
                >
                  <div className="mb-2 flex items-baseline gap-3 text-xs">
                    <a
                      href={link}
                      target="_blank"
                      rel="noreferrer"
                      className="rounded bg-stone-100 px-2 py-0.5 font-mono tabular-nums text-stone-700 hover:bg-stone-200"
                    >
                      {formatMS(s.start_ms)}
                    </a>
                    {s.cluster_label ? (
                      <span className="font-mono text-stone-700">
                        {s.cluster_label}
                      </span>
                    ) : null}
                  </div>
                  <p className="text-stone-800 leading-relaxed">{s.text}</p>
                </li>
              );
            })}
          </ol>
        ) : null}
      </div>
    </section>
  );
}

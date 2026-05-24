"use client";

import { useState } from "react";
import type { TranscriptSection as TranscriptSectionData } from "@/lib/pageTypes";
import { confidenceLabel, formatMS, tvwDeepLink } from "@/lib/format";

export function TranscriptSection({
  transcript,
  tvwEventId,
}: {
  transcript: TranscriptSectionData;
  tvwEventId: string;
}) {
  const segments = transcript.segments ?? [];
  const windows = transcript.windows ?? [];
  const [showTranscript, setShowTranscript] = useState(false);

  return (
    <section aria-labelledby="transcript-heading" className="space-y-4">
      <div className="flex items-baseline justify-between">
        <h2
          id="transcript-heading"
          className="text-xl font-semibold text-stone-900"
        >
          What was said
        </h2>
        {segments.length > 0 ? (
          <span className="text-sm text-stone-500 tabular-nums">
            {segments.length.toLocaleString()} segments ·{" "}
            {windows.length > 1
              ? `${windows.length.toLocaleString()} discussion windows`
              : `${formatMS(transcript.bill_segment_start_ms ?? 0)} – ${formatMS(
                  transcript.bill_segment_end_ms ?? 0
                )}`}
          </span>
        ) : null}
      </div>

      {segments.length === 0 ? (
        <p className="rounded border border-stone-300 bg-stone-50 p-4 text-sm text-stone-600">
          No transcript segments matched this bill discussion.
          {transcript.caption_url ? (
            <>
              {" "}A caption file is available at{" "}
              <a
                href={transcript.caption_url}
                className="text-blue-700 underline hover:text-blue-900"
                target="_blank"
                rel="noreferrer"
              >
                Invintus
              </a>
              .
            </>
          ) : (
            " No captions are available for this hearing's TVW event."
          )}
        </p>
      ) : (
        <div className="space-y-4 rounded-lg border border-stone-300 bg-white p-5">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="space-y-1">
              <p className="text-sm font-medium text-stone-900">
                Transcript excerpts are available for this bill discussion.
              </p>
              <p className="text-sm text-stone-600">
                Hidden by default to keep the page focused. Open them when you
                want the timestamped evidence trail.
              </p>
            </div>
            <button
              type="button"
              aria-expanded={showTranscript}
              aria-controls="transcript-excerpts"
              onClick={() => setShowTranscript((v) => !v)}
              className="inline-flex items-center justify-center rounded border border-stone-400 bg-stone-50 px-4 py-2 text-sm font-medium text-stone-800 hover:bg-stone-100 focus:outline-none focus:ring-2 focus:ring-stone-500 focus:ring-offset-2"
            >
              {showTranscript
                ? "Hide transcript excerpts"
                : `Show ${segments.length.toLocaleString()} transcript excerpt${
                    segments.length === 1 ? "" : "s"
                  }`}
            </button>
          </div>

          {showTranscript ? (
            <ol id="transcript-excerpts" className="space-y-3">
              {segments.map((s, i) => {
                const link = tvwEventId
                  ? tvwDeepLink(tvwEventId, s.start_ms)
                  : transcript.caption_url ?? "#";
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
                      {s.speaker_label ? (
                        <span className="font-medium text-stone-800">
                          {s.speaker_label}
                        </span>
                      ) : null}
                      <span className="text-stone-500">
                        confidence: {confidenceLabel(s.speaker_confidence)}
                      </span>
                    </div>
                    <p className="text-stone-800 leading-relaxed">{s.text}</p>
                  </li>
                );
              })}
            </ol>
          ) : null}
        </div>
      )}
    </section>
  );
}

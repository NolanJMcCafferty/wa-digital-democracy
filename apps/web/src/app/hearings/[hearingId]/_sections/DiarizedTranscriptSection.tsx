import type { DiarizedTranscript } from "@/lib/api";
import { formatMS, tvwDeepLink } from "@/lib/format";

export function DiarizedTranscriptSection({
  transcript,
  tvwEventId,
}: {
  transcript: DiarizedTranscript;
  tvwEventId?: string;
}) {
  const segments = transcript.segments ?? [];

  if (segments.length === 0) return null;

  return (
    <section aria-labelledby="full-transcript-heading" className="space-y-4">
      <h2
        id="full-transcript-heading"
        className="text-xl font-semibold text-stone-900"
      >
        Full hearing transcript
      </h2>

      <div className="overflow-hidden rounded-lg border border-stone-300 bg-stone-50">
        <ol
          id="full-transcript-segments"
          className="max-h-[calc(100vh-25rem)] space-y-3 overflow-y-auto p-3 [scrollbar-gutter:stable]"
        >
          {segments.map((s, i) => {
            const link = tvwEventId
              ? tvwDeepLink(tvwEventId, s.start_ms)
              : "#";
            return (
              <li
                key={`${s.start_ms}-${i}`}
                className="rounded border border-stone-300 bg-white p-4"
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
      </div>
    </section>
  );
}

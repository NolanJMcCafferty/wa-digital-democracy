import type { SourceRecordSummary } from "@/lib/pageTypes";
import { formatDateTime } from "@/lib/format";

const SYSTEM_LABEL: Record<string, string> = {
  lws: "Washington Legislative Web Services",
  csi: "Committee Sign In",
  committee_schedules: "Committee Schedules",
  tvw: "TVW",
  invintus: "Invintus",
  pdc_socrata: "PDC (data.wa.gov)",
};

export function SourcePanel({
  sources,
  knownLimitations,
  generatedAt,
}: {
  sources: SourceRecordSummary[];
  knownLimitations?: string[];
  generatedAt: string;
}) {
  // Group by system so the panel reads as a list of feeds, not a flat dump.
  const bySystem = new Map<string, SourceRecordSummary[]>();
  for (const s of sources) {
    const arr = bySystem.get(s.system) ?? [];
    arr.push(s);
    bySystem.set(s.system, arr);
  }

  return (
    <section
      aria-labelledby="sources-heading"
      className="space-y-4 rounded-lg border-2 border-stone-300 bg-stone-50 p-6"
    >
      <div className="flex items-baseline justify-between">
        <h2
          id="sources-heading"
          className="text-xl font-semibold text-stone-900"
        >
          Sources & confidence
        </h2>
        <span className="text-xs text-stone-500">
          page assembled {formatDateTime(generatedAt)}
        </span>
      </div>

      <p className="text-sm text-stone-600">
        Every fact above traces back to one of the official feeds below. This
        panel is part of the product, not engineering metadata — if a source
        link breaks, the corresponding claim is no longer verifiable.
      </p>

      {sources.length === 0 ? (
        <p className="text-sm text-stone-500">No source records recorded.</p>
      ) : (
        <ul className="space-y-3">
          {Array.from(bySystem.entries()).map(([system, list]) => (
            <li key={system} className="space-y-1">
              <h3 className="text-sm font-medium text-stone-800">
                {SYSTEM_LABEL[system] ?? system}
              </h3>
              <ul className="space-y-1 text-xs">
                {list.map((s) => (
                  <li
                    key={`${s.endpoint}-${s.url}`}
                    className="flex flex-wrap items-baseline gap-x-3 gap-y-0.5"
                  >
                    <span className="font-mono text-stone-700">
                      {s.endpoint}
                    </span>
                    <a
                      href={s.url}
                      target="_blank"
                      rel="noreferrer"
                      className="break-all text-blue-700 underline hover:text-blue-900"
                    >
                      {s.url}
                    </a>
                    <span className="text-stone-500 tabular-nums">
                      fetched {formatDateTime(s.fetched_at)}
                    </span>
                  </li>
                ))}
              </ul>
            </li>
          ))}
        </ul>
      )}

      {knownLimitations && knownLimitations.length > 0 ? (
        <div className="rounded border border-amber-300 bg-amber-50 p-4">
          <h3 className="mb-2 text-sm font-semibold text-amber-900">
            Known limitations
          </h3>
          <ul className="list-disc space-y-1 pl-5 text-sm text-amber-900">
            {knownLimitations.map((l, i) => (
              <li key={i}>{l}</li>
            ))}
          </ul>
        </div>
      ) : null}
    </section>
  );
}

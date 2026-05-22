import { listSourceSummaries } from "@/lib/api";
import { formatDateTime } from "@/lib/format";

export const dynamic = "force-dynamic";

const STATUS_STYLE = {
  active: "bg-emerald-100 text-emerald-800",
  partial: "bg-amber-100 text-amber-800",
  planned: "bg-stone-200 text-stone-700",
};

// Live status dashboard — always reflect the current source_record table
// rather than a 60-second-stale snapshot. The cost is one extra API hit
// per page render; the data is small and the page is rarely loaded.
export const revalidate = 0;

export default async function SourcesPage() {
  const sources = await listSourceSummaries();
  const activeCalls = sources.reduce((n, s) => n + s.calls, 0);
  const liveSources = sources.filter((s) => s.calls > 0).length;

  return (
    <article className="space-y-10">
      <section className="space-y-4">
        <p className="text-sm uppercase tracking-wider text-stone-500">
          Provenance
        </p>
        <div className="space-y-3">
          <h1 className="text-3xl font-bold tracking-tight text-stone-900">
            Sources
          </h1>
          <p className="max-w-3xl text-stone-600">
            WA Digital Democracy is source-linked by design. Every public fact
            should trace back to an official feed, public record, document,
            video, transcript, or dataset. This page explains what powers the
            current public record and which source families are ready for future expansion.
          </p>
        </div>
      </section>

      <section className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
        <Metric label="Source families" value={sources.length.toLocaleString()} />
        <Metric label="In use" value={liveSources.toLocaleString()} />
        <Metric label="Recorded calls" value={activeCalls.toLocaleString()} />
        <Metric
          label="Expansion clients"
          value={sources.filter((s) => s.status === "planned").length.toLocaleString()}
        />
      </section>

      <section aria-labelledby="source-principles" className="space-y-4 rounded-lg border border-stone-300 bg-white p-6">
        <h2 id="source-principles" className="text-xl font-semibold text-stone-900">
          What “source-linked” means here
        </h2>
        <ul className="list-disc space-y-2 pl-5 text-sm text-stone-700">
          <li>Official/public sources are fetched through clients that preserve raw responses.</li>
          <li>Normalized facts point back to source records with URLs, timestamps, and content hashes.</li>
          <li>Human-reviewed matches and inferred joins are labeled with confidence rather than presented as certainty.</li>
          <li>Money, lobbying, and contract records are shown as context, not proof of causation.</li>
        </ul>
      </section>

      <section aria-labelledby="source-list" className="space-y-4">
        <h2 id="source-list" className="text-xl font-semibold text-stone-900">
          Source families
        </h2>
        <div className="space-y-4">
          {sources.map((s) => (
            <section
              key={s.system}
              className="rounded-lg border border-stone-300 bg-white p-5"
            >
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div className="space-y-2">
                  <div className="flex flex-wrap items-center gap-2">
                    <h3 className="text-lg font-semibold text-stone-900">
                      {s.label}
                    </h3>
                    <span
                      className={`${STATUS_STYLE[s.status]} rounded px-2 py-0.5 text-xs font-medium uppercase tracking-wider`}
                    >
                      {s.status}
                    </span>
                  </div>
                  <p className="max-w-3xl text-sm text-stone-700">{s.usedFor}</p>
                  {s.limitations ? (
                    <p className="max-w-3xl text-xs text-stone-500">
                      Limitation: {s.limitations}
                    </p>
                  ) : null}
                </div>
                <a
                  href={s.officialUrl}
                  target="_blank"
                  rel="noreferrer"
                  className="text-sm text-blue-700 underline hover:text-blue-900"
                >
                  Official source →
                </a>
              </div>

              <dl className="mt-4 grid grid-cols-1 gap-x-6 gap-y-2 text-sm sm:grid-cols-[max-content_1fr]">
                <dt className="text-stone-500">System key</dt>
                <dd className="font-mono text-stone-800">{s.system}</dd>
                <dt className="text-stone-500">Recorded calls</dt>
                <dd className="text-stone-800">{s.calls.toLocaleString()}</dd>
                {s.latestFetchedAt ? (
                  <>
                    <dt className="text-stone-500">Latest fetch</dt>
                    <dd className="text-stone-800">{formatDateTime(s.latestFetchedAt)}</dd>
                  </>
                ) : null}
                {s.endpoints.length > 0 ? (
                  <>
                    <dt className="text-stone-500">Endpoints used</dt>
                    <dd className="text-stone-800">
                      <ul className="flex flex-wrap gap-1">
                        {s.endpoints.map((e) => (
                          <li
                            key={e}
                            className="rounded bg-stone-100 px-1.5 py-0.5 font-mono text-xs"
                          >
                            {e}
                          </li>
                        ))}
                      </ul>
                    </dd>
                  </>
                ) : null}
              </dl>
            </section>
          ))}
        </div>
      </section>
    </article>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded border border-stone-300 bg-stone-50 p-3">
      <div className="text-xs uppercase tracking-wider text-stone-500">{label}</div>
      <div className="mt-1 font-semibold text-stone-900">{value}</div>
    </div>
  );
}

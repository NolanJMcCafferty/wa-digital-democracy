import Link from "next/link";
import { listSpeakerReviewEvents } from "@/lib/api";

const PAGE_SIZE = 50;

function fmtMinutes(ms: number): string {
  return `${(ms / 60000).toFixed(1)} min`;
}

export default async function SpeakerReviewIndex({
  searchParams,
}: {
  searchParams: Promise<{ page?: string; limit?: string }>;
}) {
  const { page: pageParam, limit: limitParam } = await searchParams;
  const page = Math.max(1, Number.parseInt(pageParam ?? "1", 10) || 1);
  const limit = Math.min(200, Math.max(1, Number.parseInt(limitParam ?? String(PAGE_SIZE), 10) || PAGE_SIZE));
  const { events, total, offset } = await listSpeakerReviewEvents({ page, limit });
  const totalPages = Math.max(1, Math.ceil(total / limit));
  const startIdx = total === 0 ? 0 : offset + 1;
  const endIdx = Math.min(offset + events.length, total);
  const buildHref = (overrides: { page?: number; limit?: number }) => {
    const params = new URLSearchParams();
    const nextPage = overrides.page ?? page;
    const nextLimit = overrides.limit ?? limit;
    if (nextPage > 1) params.set("page", String(nextPage));
    if (nextLimit !== PAGE_SIZE) params.set("limit", String(nextLimit));
    const qs = params.toString();
    return qs ? `/admin/review/speakers?${qs}` : "/admin/review/speakers";
  };

  return (
    <div className="space-y-6">
      <div>
        <p className="text-sm uppercase tracking-wider text-stone-500">Internal review</p>
        <h1 className="text-3xl font-bold text-stone-900">Speaker identity review</h1>
        <p className="mt-2 text-stone-600">
          Resolve speaker clusters hearing-by-hearing before publishing named speaker assignments.
        </p>
      </div>

      <div className="flex flex-wrap items-baseline justify-between gap-3 text-sm text-stone-600">
        <p>
          {total === 0 ? (
            <>No diarized speaker clusters found.</>
          ) : (
            <>
              Showing <span className="font-medium text-stone-900">{startIdx.toLocaleString()}-{endIdx.toLocaleString()}</span>{" "}
              of <span className="font-medium text-stone-900">{total.toLocaleString()}</span> event{total === 1 ? "" : "s"}.
            </>
          )}
        </p>
        {totalPages > 1 ? (
          <p className="text-xs text-stone-500">
            Page {page} of {totalPages}
          </p>
        ) : null}
      </div>

      <div className="overflow-hidden rounded-lg border border-stone-300 bg-white">
        <table className="w-full text-left text-sm">
          <thead className="bg-stone-100 text-xs uppercase tracking-wider text-stone-600">
            <tr>
              <th className="px-4 py-3">Hearing ID</th>
              <th className="px-4 py-3">TVW event</th>
              <th className="px-4 py-3 text-right">Clusters</th>
              <th className="px-4 py-3 text-right">Assigned</th>
              <th className="px-4 py-3 text-right">Pending tasks</th>
              <th className="px-4 py-3 text-right">Unresolved</th>
              <th className="px-4 py-3 text-right">Speech</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-stone-200">
            {events.map((e) => (
              <tr key={e.TVWEventID} className="hover:bg-stone-50">
                <td className="px-4 py-3">
                  {e.HearingID > 0 ? (
                    <Link className="font-mono font-medium underline" href={`/admin/review/speakers/events/${e.HearingID}`}>
                      {e.HearingID}
                    </Link>
                  ) : (
                    <span className="text-stone-400">-</span>
                  )}
                </td>
                <td className="px-4 py-3">
                  {e.HearingID > 0 ? (
                    <Link className="font-mono font-medium underline" href={`/admin/review/speakers/events/${e.HearingID}`}>
                      {e.TVWEventID}
                    </Link>
                  ) : (
                    <span className="font-mono text-stone-400">{e.TVWEventID}</span>
                  )}
                </td>
                <td className="px-4 py-3 text-right tabular-nums">{e.ClusterCount.toLocaleString()}</td>
                <td className="px-4 py-3 text-right tabular-nums">{e.AssignedCount.toLocaleString()}</td>
                <td className="px-4 py-3 text-right tabular-nums">{e.PendingTaskCount.toLocaleString()}</td>
                <td className="px-4 py-3 text-right tabular-nums">{e.UnresolvedClusterCount.toLocaleString()}</td>
                <td className="px-4 py-3 text-right tabular-nums">{fmtMinutes(e.TotalSpeechMS)}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {events.length === 0 ? <p className="p-6 text-stone-600">No diarized speaker clusters found.</p> : null}
      </div>

      {totalPages > 1 ? <Pagination page={page} totalPages={totalPages} buildHref={buildHref} /> : null}
    </div>
  );
}

function Pagination({
  page,
  totalPages,
  buildHref,
}: {
  page: number;
  totalPages: number;
  buildHref: (overrides: { page?: number }) => string;
}) {
  const linkCls =
    "inline-flex h-8 min-w-[2rem] items-center justify-center rounded border border-stone-300 bg-white px-2 text-sm text-stone-700 hover:border-stone-500 hover:bg-stone-100";
  const currentCls =
    "inline-flex h-8 min-w-[2rem] items-center justify-center rounded border border-stone-900 bg-stone-900 px-2 text-sm font-medium text-white";
  const disabledCls =
    "inline-flex h-8 min-w-[2rem] items-center justify-center rounded border border-stone-200 bg-stone-50 px-2 text-sm text-stone-400";
  const window = 1;
  const pages: Array<number | "..."> = [];
  const add = (n: number) => {
    if (n >= 1 && n <= totalPages && !pages.includes(n)) pages.push(n);
  };
  add(1);
  if (page - window > 2) pages.push("...");
  for (let p = Math.max(2, page - window); p <= Math.min(totalPages - 1, page + window); p++) {
    add(p);
  }
  if (page + window < totalPages - 1) pages.push("...");
  if (totalPages > 1) add(totalPages);
  return (
    <nav className="flex flex-wrap items-center justify-center gap-1.5" aria-label="Pagination">
      {page > 1 ? (
        <Link href={buildHref({ page: page - 1 })} className={linkCls} rel="prev">Prev</Link>
      ) : (
        <span className={disabledCls} aria-hidden>Prev</span>
      )}
      {pages.map((p, i) =>
        p === "..." ? (
          <span key={`gap-${i}`} className="px-1 text-sm text-stone-500">...</span>
        ) : p === page ? (
          <span key={p} className={currentCls} aria-current="page">{p}</span>
        ) : (
          <Link key={p} href={buildHref({ page: p })} className={linkCls}>{p}</Link>
        ),
      )}
      {page < totalPages ? (
        <Link href={buildHref({ page: page + 1 })} className={linkCls} rel="next">Next</Link>
      ) : (
        <span className={disabledCls} aria-hidden>Next</span>
      )}
    </nav>
  );
}

import Link from "next/link";
import { loadSpeakerReviewEvent, type SpeakerClusterReview } from "@/lib/api";

function fmtMinutes(ms: number): string {
  return `${(ms / 60000).toFixed(1)} min`;
}

function status(c: SpeakerClusterReview): string {
  if (c.CurrentSpeakerLabel) return "assigned";
  if ((c.Tasks ?? []).some((t) => t.Status === "pending")) return "pending";
  return "unresolved";
}

export default async function SpeakerReviewEventPage({ params }: { params: Promise<{ tvwEventId: string }> }) {
  const { tvwEventId } = await params;
  const clusters = await loadSpeakerReviewEvent(tvwEventId);
  return (
    <div className="space-y-6">
      <div>
        <Link href="/admin/review/speakers" className="text-sm text-stone-500 underline">← Back to events</Link>
        <p className="mt-4 text-sm uppercase tracking-wider text-stone-500">Speaker review event</p>
        <h1 className="text-3xl font-bold text-stone-900">TVW {tvwEventId}</h1>
      </div>

      <div className="overflow-hidden rounded-lg border border-stone-300 bg-white">
        <table className="w-full text-left text-sm">
          <thead className="bg-stone-100 text-xs uppercase tracking-wider text-stone-600">
            <tr>
              <th className="px-4 py-3">Cluster</th>
              <th className="px-4 py-3">Current assignment</th>
              <th className="px-4 py-3">Best candidate</th>
              <th className="px-4 py-3 text-right">Turns</th>
              <th className="px-4 py-3 text-right">Speech</th>
              <th className="px-4 py-3">Status</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-stone-200">
            {clusters.map((c) => {
              const pending = (c.Tasks ?? []).find((t) => t.Status === "pending") ?? c.Tasks?.[0];
              return (
                <tr key={c.ClusterID} className="hover:bg-stone-50 align-top">
                  <td className="px-4 py-3">
                    <Link className="font-mono font-medium underline" href={`/admin/review/speakers/clusters/${c.ClusterID}`}>
                      {c.ClusterLabel}
                    </Link>
                  </td>
                  <td className="px-4 py-3">{c.CurrentSpeakerLabel || <span className="text-stone-400">—</span>}</td>
                  <td className="px-4 py-3">
                    {pending ? (
                      <div>
                        <div className="font-medium">{pending.CandidateLabel}</div>
                        <div className="text-xs text-stone-500">{pending.CandidateKind} · {(pending.CandidateConfidence * 100).toFixed(0)}%</div>
                      </div>
                    ) : <span className="text-stone-400">No candidate</span>}
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums">{c.TurnCount.toLocaleString()}</td>
                  <td className="px-4 py-3 text-right tabular-nums">{fmtMinutes(c.TotalSpeechMS)}</td>
                  <td className="px-4 py-3">{status(c)}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
        {clusters.length === 0 ? <p className="p-6 text-stone-600">No clusters found for this event.</p> : null}
      </div>
    </div>
  );
}

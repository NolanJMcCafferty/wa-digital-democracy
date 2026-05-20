import Link from "next/link";
import { listSpeakerReviewEvents } from "@/lib/api";

function fmtMinutes(ms: number): string {
  return `${(ms / 60000).toFixed(1)} min`;
}

export default async function SpeakerReviewIndex() {
  const events = await listSpeakerReviewEvents();
  return (
    <div className="space-y-6">
      <div>
        <p className="text-sm uppercase tracking-wider text-stone-500">Internal review</p>
        <h1 className="text-3xl font-bold text-stone-900">Speaker identity review</h1>
        <p className="mt-2 text-stone-600">
          Resolve Deepgram speaker clusters hearing-by-hearing before publishing named speaker assignments.
        </p>
      </div>

      <div className="overflow-hidden rounded-lg border border-stone-300 bg-white">
        <table className="w-full text-left text-sm">
          <thead className="bg-stone-100 text-xs uppercase tracking-wider text-stone-600">
            <tr>
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
                  <Link className="font-mono font-medium underline" href={`/admin/review/speakers/events/${e.TVWEventID}`}>
                    {e.TVWEventID}
                  </Link>
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
    </div>
  );
}

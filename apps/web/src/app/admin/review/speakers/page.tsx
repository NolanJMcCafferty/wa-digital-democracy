import Link from "next/link";
import { listSpeakerReviewTasks, type SpeakerReviewTask } from "@/lib/loadBundle";

function fmtMS(ms: number): string {
  const total = Math.floor(ms / 1000);
  const m = Math.floor(total / 60);
  const s = total % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

export default async function SpeakerReviewIndex({ searchParams }: { searchParams?: Promise<{ status?: string }> }) {
  const sp = await searchParams;
  const status = sp?.status ?? "pending";
  const tasks = await listSpeakerReviewTasks(status);
  return (
    <div className="space-y-6">
      <div>
        <p className="text-sm uppercase tracking-wider text-stone-500">Internal review</p>
        <h1 className="text-3xl font-bold text-stone-900">Speaker identity tasks</h1>
        <p className="mt-2 text-stone-600">Review diarized speaker clusters before publishing named speaker assignments.</p>
      </div>
      <div className="flex gap-2 text-sm">
        {['pending', 'accepted', 'rejected', 'needs_more_evidence'].map((s) => (
          <Link key={s} href={`/admin/review/speakers?status=${s}`} className={`rounded border px-3 py-1 ${status === s ? 'border-stone-900 bg-stone-900 text-white' : 'border-stone-300 bg-white text-stone-700'}`}>{s}</Link>
        ))}
      </div>
      <div className="overflow-hidden rounded-lg border border-stone-300 bg-white">
        <table className="w-full text-left text-sm">
          <thead className="bg-stone-100 text-xs uppercase tracking-wider text-stone-600">
            <tr>
              <th className="px-4 py-3">Task</th>
              <th className="px-4 py-3">Cluster</th>
              <th className="px-4 py-3">Candidate</th>
              <th className="px-4 py-3">Evidence</th>
              <th className="px-4 py-3">Coverage</th>
              <th className="px-4 py-3">Status</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-stone-200">
            {tasks.map((t: SpeakerReviewTask) => (
              <tr key={t.ID} className="align-top hover:bg-stone-50">
                <td className="px-4 py-3"><Link className="font-medium underline" href={`/admin/review/speakers/${t.ID}`}>#{t.ID}</Link></td>
                <td className="px-4 py-3"><div className="font-mono">{t.ClusterLabel}</div><div className="text-xs text-stone-500">job {t.DiarizationJobID}</div></td>
                <td className="px-4 py-3"><div className="font-medium">{t.CandidateLabel}</div><div className="text-xs text-stone-500">{t.CandidateKind} · {(t.CandidateConfidence * 100).toFixed(0)}%</div></td>
                <td className="max-w-md px-4 py-3 text-stone-700"><div className="line-clamp-3">{t.EvidenceText}</div><div className="mt-1 text-xs text-stone-500">{fmtMS(t.EvidenceStartMS)}–{fmtMS(t.EvidenceEndMS)}</div></td>
                <td className="px-4 py-3 text-stone-600">{(t.TotalSpeechMS / 60000).toFixed(1)} min<br /><span className="text-xs">{t.TurnCount} segments</span></td>
                <td className="px-4 py-3">{t.Status}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {tasks.length === 0 ? <p className="p-6 text-stone-600">No tasks found.</p> : null}
      </div>
    </div>
  );
}

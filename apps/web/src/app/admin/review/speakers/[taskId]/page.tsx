import { redirect, notFound } from "next/navigation";
import Link from "next/link";
import { decideSpeakerReviewTask, loadSpeakerReviewTask, type SpeakerReviewSegment } from "@/lib/api";

function fmtMS(ms: number): string {
  const total = Math.floor(ms / 1000);
  const m = Math.floor(total / 60);
  const s = total % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

async function decide(formData: FormData) {
  "use server";
  const taskId = String(formData.get("taskId") ?? "");
  const action = String(formData.get("action") ?? "accept") as "accept" | "reject" | "needs-more-evidence";
  const reviewer = String(formData.get("reviewer") ?? "");
  const notes = String(formData.get("notes") ?? "");
  await decideSpeakerReviewTask(taskId, action, reviewer, notes);
  redirect("/admin/review/speakers");
}

export default async function SpeakerReviewDetail({ params }: { params: Promise<{ taskId: string }> }) {
  const { taskId } = await params;
  const task = await loadSpeakerReviewTask(taskId);
  if (!task) notFound();
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between gap-4">
        <div>
          <Link href="/admin/review/speakers" className="text-sm text-stone-500 underline">← Back to tasks</Link>
          <h1 className="mt-2 text-3xl font-bold text-stone-900">Review {task.ClusterLabel} → {task.CandidateLabel}</h1>
          <p className="text-stone-600">Event {task.TVWEventID} · job {task.DiarizationJobID} · {(task.TotalSpeechMS / 60000).toFixed(1)} spoken minutes</p>
          {task.CurrentSpeakerLabel ? (
            <p className="mt-2 rounded bg-emerald-50 px-3 py-2 text-sm text-emerald-900 ring-1 ring-emerald-200">
              Current accepted assignment: <strong>{task.CurrentSpeakerLabel}</strong>
            </p>
          ) : null}
        </div>
        <span className="rounded-full bg-amber-100 px-3 py-1 text-sm font-medium text-amber-800">{task.Status}</span>
      </div>

      <section className="rounded-lg border border-stone-300 bg-white p-5">
        <h2 className="text-lg font-semibold text-stone-900">Primary evidence</h2>
        <p className="mt-2 text-sm text-stone-500">{fmtMS(task.EvidenceStartMS)}–{fmtMS(task.EvidenceEndMS)} · confidence {(task.CandidateConfidence * 100).toFixed(0)}%</p>
        <blockquote className="mt-3 border-l-4 border-stone-300 pl-4 text-stone-800">{task.EvidenceText}</blockquote>
      </section>

      <section className="rounded-lg border border-stone-300 bg-white p-5">
        <h2 className="text-lg font-semibold text-stone-900">Cluster sample</h2>
        <div className="mt-3 space-y-3">
          {(task.SampleSegments ?? []).map((s: SpeakerReviewSegment, i: number) => (
            <div key={`${s.StartMS}-${i}`} className="rounded border border-stone-200 p-3">
              <div className="text-xs font-medium text-stone-500">{fmtMS(s.StartMS)}–{fmtMS(s.EndMS)}</div>
              <p className="mt-1 text-sm text-stone-800">{s.Text}</p>
            </div>
          ))}
        </div>
      </section>

      <form action={decide} className="space-y-4 rounded-lg border border-stone-300 bg-white p-5">
        <input type="hidden" name="taskId" value={task.ID} />
        <div className="grid gap-3 sm:grid-cols-2">
          <label className="text-sm font-medium text-stone-700">Reviewer
            <input name="reviewer" defaultValue="nolan" className="mt-1 w-full rounded border border-stone-300 px-3 py-2" />
          </label>
          <label className="text-sm font-medium text-stone-700">Notes
            <input name="notes" className="mt-1 w-full rounded border border-stone-300 px-3 py-2" />
          </label>
        </div>
        <div className="flex flex-wrap gap-2">
          <button name="action" value="accept" className="rounded bg-emerald-700 px-4 py-2 font-medium text-white">Accept assignment</button>
          <button name="action" value="reject" className="rounded bg-rose-700 px-4 py-2 font-medium text-white">Reject</button>
          <button name="action" value="needs-more-evidence" className="rounded bg-stone-700 px-4 py-2 font-medium text-white">Needs more evidence</button>
        </div>
      </form>
    </div>
  );
}

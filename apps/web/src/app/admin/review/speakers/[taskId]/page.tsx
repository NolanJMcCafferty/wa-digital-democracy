import { redirect, notFound } from "next/navigation";
import Link from "next/link";
import { decideSpeakerReviewTask, loadSpeakerReviewTask, type SpeakerReviewSegment } from "@/lib/api";
import { tvwDeepLink } from "@/lib/format";

function fmtMS(ms: number): string {
  const total = Math.floor(ms / 1000);
  const m = Math.floor(total / 60);
  const s = total % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

function SegmentVideoLink({
  endMS,
  startMS,
  tvwEventID,
}: {
  endMS: number;
  startMS: number;
  tvwEventID: string;
}) {
  const timestamp = `${fmtMS(startMS)}–${fmtMS(endMS)}`;
  if (!tvwEventID) {
    return <span>{timestamp}</span>;
  }
  return (
    <a
      href={tvwDeepLink(tvwEventID, startMS)}
      target="_blank"
      rel="noreferrer"
      className="inline-flex items-center gap-2 rounded bg-stone-100 px-2 py-0.5 font-mono tabular-nums text-stone-800 underline decoration-stone-400 underline-offset-2 hover:bg-stone-200 hover:text-stone-950"
    >
      <span>{timestamp}</span>
      <span className="font-sans text-[10px] font-semibold uppercase tracking-wider text-stone-600">
        TVW video
      </span>
    </a>
  );
}

async function decide(formData: FormData) {
  "use server";
  const taskId = String(formData.get("taskId") ?? "");
  const action = String(formData.get("action") ?? "accept") as "accept" | "reject" | "needs-more-evidence";
  const notes = String(formData.get("notes") ?? "");
  await decideSpeakerReviewTask(taskId, action, notes);
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
        <div className="mt-2 flex flex-wrap items-center gap-2 text-sm text-stone-500">
          <SegmentVideoLink
            endMS={task.EvidenceEndMS}
            startMS={task.EvidenceStartMS}
            tvwEventID={task.TVWEventID}
          />
          <span>confidence {(task.CandidateConfidence * 100).toFixed(0)}%</span>
        </div>
        <blockquote className="mt-3 border-l-4 border-stone-300 pl-4 text-stone-800">{task.EvidenceText}</blockquote>
      </section>

      <section className="rounded-lg border border-stone-300 bg-white p-5">
        <h2 className="text-lg font-semibold text-stone-900">Cluster sample</h2>
        <div className="mt-3 space-y-3">
          {(task.SampleSegments ?? []).map((s: SpeakerReviewSegment, i: number) => (
            <div key={`${s.StartMS}-${i}`} className="rounded border border-stone-200 p-3">
              <div className="text-xs font-medium text-stone-500">
                <SegmentVideoLink
                  endMS={s.EndMS}
                  startMS={s.StartMS}
                  tvwEventID={task.TVWEventID}
                />
              </div>
              <p className="mt-1 text-sm text-stone-800">{s.Text}</p>
            </div>
          ))}
        </div>
      </section>

      <form action={decide} className="space-y-4 rounded-lg border border-stone-300 bg-white p-5">
        <input type="hidden" name="taskId" value={task.ID} />
        <label className="block text-sm font-medium text-stone-700">Notes
          <input name="notes" className="mt-1 w-full rounded border border-stone-300 px-3 py-2" />
        </label>
        <div className="flex flex-wrap gap-2">
          <button name="action" value="accept" className="rounded bg-emerald-700 px-4 py-2 font-medium text-white">Accept assignment</button>
          <button name="action" value="reject" className="rounded bg-rose-700 px-4 py-2 font-medium text-white">Reject</button>
          <button name="action" value="needs-more-evidence" className="rounded bg-stone-700 px-4 py-2 font-medium text-white">Needs more evidence</button>
        </div>
      </form>
    </div>
  );
}

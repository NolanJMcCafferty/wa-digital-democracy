import { redirect, notFound } from "next/navigation";
import Link from "next/link";
import {
  decideSpeakerReviewTask,
  loadSpeakerClusterReview,
  manuallyAssignSpeakerCluster,
  type SpeakerIdentityEvidence,
  type SpeakerReviewSegment,
  type SpeakerReviewTask,
} from "@/lib/api";
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
  const clusterId = String(formData.get("clusterId") ?? "");
  const action = String(formData.get("action") ?? "accept") as "accept" | "reject" | "needs-more-evidence";
  const notes = String(formData.get("notes") ?? "");
  await decideSpeakerReviewTask(taskId, action, notes);
  redirect(`/admin/review/speakers/clusters/${clusterId}`);
}

async function assignManual(formData: FormData) {
  "use server";
  const clusterId = String(formData.get("clusterId") ?? "");
  const kind = String(formData.get("kind") ?? "person");
  const label = String(formData.get("label") ?? "");
  const notes = String(formData.get("notes") ?? "");
  await manuallyAssignSpeakerCluster(clusterId, kind, label, notes);
  redirect(`/admin/review/speakers/clusters/${clusterId}`);
}

export default async function SpeakerClusterReviewPage({ params }: { params: Promise<{ clusterId: string }> }) {
  const { clusterId } = await params;
  const cluster = await loadSpeakerClusterReview(clusterId);
  if (!cluster) notFound();
  const pending = (cluster.Tasks ?? []).filter((t) => t.Status === "pending");
  const backHref = cluster.HearingID > 0
    ? `/admin/review/speakers/events/${cluster.HearingID}`
    : "/admin/review/speakers";
  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <Link href={backHref} className="text-sm text-stone-500 underline">← Back to event</Link>
          <p className="mt-4 text-sm uppercase tracking-wider text-stone-500">Speaker cluster</p>
          <h1 className="text-3xl font-bold text-stone-900">{cluster.ClusterLabel}</h1>
          <p className="text-stone-600">Event {cluster.TVWEventID} · job {cluster.DiarizationJobID} · {(cluster.TotalSpeechMS / 60000).toFixed(1)} spoken minutes</p>
          {cluster.CurrentSpeakerLabel ? (
            <p className="mt-2 rounded bg-emerald-50 px-3 py-2 text-sm text-emerald-900 ring-1 ring-emerald-200">
              Current accepted assignment: <strong>{cluster.CurrentSpeakerLabel}</strong>
            </p>
          ) : null}
        </div>
        <span className="rounded-full bg-stone-100 px-3 py-1 text-sm font-medium text-stone-700">
          {cluster.TurnCount} turns
        </span>
      </div>

      <section className="space-y-3 rounded-lg border border-stone-300 bg-white p-5">
        <h2 className="text-lg font-semibold text-stone-900">Candidate tasks</h2>
        {cluster.Tasks?.length ? (
          <div className="space-y-3">
            {cluster.Tasks.map((task: SpeakerReviewTask) => (
              <form key={task.ID} action={decide} className="rounded border border-stone-200 p-3">
                <input type="hidden" name="taskId" value={task.ID} />
                <input type="hidden" name="clusterId" value={cluster.ClusterID} />
                <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                  <div>
                    <div className="font-medium">{task.CandidateLabel}</div>
                    <div className="text-xs text-stone-500">{task.CandidateKind} · {(task.CandidateConfidence * 100).toFixed(0)}% · {task.Status}</div>
                    {task.EvidenceText ? <p className="mt-2 text-sm text-stone-700">{task.EvidenceText}</p> : null}
                  </div>
                  {task.Status === "pending" ? (
                    <div className="flex flex-wrap gap-2">
                      <input name="notes" className="hidden" />
                      <button name="action" value="accept" className="rounded bg-emerald-700 px-3 py-1.5 text-sm font-medium text-white">Accept</button>
                      <button name="action" value="reject" className="rounded bg-rose-700 px-3 py-1.5 text-sm font-medium text-white">Reject</button>
                      <button name="action" value="needs-more-evidence" className="rounded bg-stone-700 px-3 py-1.5 text-sm font-medium text-white">More evidence</button>
                    </div>
                  ) : null}
                </div>
              </form>
            ))}
          </div>
        ) : <p className="text-sm text-stone-600">No candidate tasks for this cluster.</p>}
      </section>

      <form action={assignManual} className="space-y-4 rounded-lg border border-stone-300 bg-white p-5">
        <input type="hidden" name="clusterId" value={cluster.ClusterID} />
        <h2 className="text-lg font-semibold text-stone-900">Manual assignment</h2>
        <div className="grid gap-3 sm:grid-cols-4">
          <label className="text-sm font-medium text-stone-700">Kind
            <select name="kind" defaultValue="person" className="mt-1 w-full rounded border border-stone-300 px-3 py-2">
              <option value="person">person</option>
              <option value="legislator">legislator</option>
              <option value="testifier">testifier</option>
              <option value="unknown">unknown</option>
            </select>
          </label>
          <label className="text-sm font-medium text-stone-700 sm:col-span-3">Label
            <input name="label" placeholder="Jane Smith" className="mt-1 w-full rounded border border-stone-300 px-3 py-2" />
          </label>
        </div>
        <label className="block text-sm font-medium text-stone-700">Notes
          <input name="notes" className="mt-1 w-full rounded border border-stone-300 px-3 py-2" />
        </label>
        <button className="rounded bg-stone-900 px-4 py-2 font-medium text-white">Assign manually</button>
      </form>

      <section className="rounded-lg border border-stone-300 bg-white p-5">
        <h2 className="text-lg font-semibold text-stone-900">Evidence</h2>
        <div className="mt-3 space-y-3">
          {(cluster.Evidence ?? []).map((e: SpeakerIdentityEvidence) => (
            <div key={e.ID} className="rounded border border-stone-200 p-3">
              <div className="text-xs font-medium text-stone-500">{e.EvidenceType} · {e.CandidateLabel} · {(e.Confidence * 100).toFixed(0)}%</div>
              <p className="mt-1 text-sm text-stone-800">{e.EvidenceText}</p>
              <div className="mt-1 text-xs font-medium text-stone-500">
                <SegmentVideoLink
                  endMS={e.EndMS}
                  startMS={e.StartMS}
                  tvwEventID={cluster.TVWEventID}
                />
              </div>
            </div>
          ))}
          {cluster.Evidence?.length === 0 ? <p className="text-sm text-stone-600">No evidence rows for this cluster.</p> : null}
        </div>
      </section>

      <section className="rounded-lg border border-stone-300 bg-white p-5">
        <h2 className="text-lg font-semibold text-stone-900">Cluster sample</h2>
        <div className="mt-3 space-y-3">
          {(cluster.SampleSegments ?? []).map((s: SpeakerReviewSegment, i: number) => (
            <div key={`${s.StartMS}-${i}`} className="rounded border border-stone-200 p-3">
              <div className="text-xs font-medium text-stone-500">
                <SegmentVideoLink
                  endMS={s.EndMS}
                  startMS={s.StartMS}
                  tvwEventID={cluster.TVWEventID}
                />
              </div>
              <p className="mt-1 text-sm text-stone-800">{s.Text}</p>
            </div>
          ))}
        </div>
      </section>
    </div>
  );
}

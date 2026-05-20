import { revalidatePath } from "next/cache";
import Link from "next/link";
import {
  decideEntityMatchCandidate,
  listEntityMatchCandidates,
  slugify,
  type EntityMatchCandidate,
  type EntityMatchDecision,
  type EntityMatchTranscriptContext,
} from "@/lib/api";

const DECISION_FILTERS: Array<{ value: string; label: string }> = [
  { value: "needs_review", label: "Needs review" },
  { value: "confirmed", label: "Confirmed" },
  { value: "rejected", label: "Rejected" },
  { value: "", label: "All" },
];

const CONFIDENCE_OPTIONS = ["confirmed", "likely", "possible", "unlikely"];

async function decide(formData: FormData) {
  "use server";
  const candidateId = String(formData.get("candidateId") ?? "");
  const decision = String(formData.get("decision") ?? "") as EntityMatchDecision;
  const confidence = String(formData.get("confidence") ?? "possible");
  const reviewer = String(formData.get("reviewer") ?? "");
  const notes = String(formData.get("notes") ?? "");
  await decideEntityMatchCandidate(candidateId, decision, confidence, reviewer, notes);
  revalidatePath("/admin/review/entities");
}

function decisionBadge(decision: string): string {
  switch (decision) {
    case "confirmed":
      return "bg-emerald-100 text-emerald-800";
    case "rejected":
      return "bg-rose-100 text-rose-800";
    default:
      return "bg-amber-100 text-amber-800";
  }
}

export default async function EntityReviewIndex({
  searchParams,
}: {
  searchParams: Promise<{ decision?: string; source_kind?: string }>;
}) {
  const { decision = "needs_review", source_kind = "" } = await searchParams;
  const candidates = await listEntityMatchCandidates({
    decision: decision || undefined,
    sourceKind: source_kind || undefined,
    limit: 200,
  });
  const sourceKinds = Array.from(new Set(candidates.map((c) => c.source_kind))).sort();

  return (
    <div className="space-y-6">
      <div>
        <p className="text-sm uppercase tracking-wider text-stone-500">Internal review</p>
        <h1 className="text-3xl font-bold text-stone-900">Organization entity-match review</h1>
        <p className="mt-2 text-stone-600">
          Confirm or reject candidate links between vendor/PDC/contract rows and canonical organizations. Only confirmed
          decisions surface in <code>reviewed_vendor_entity_match</code>.
        </p>
      </div>

      <form className="flex flex-wrap items-end gap-3 rounded-lg border border-stone-300 bg-white p-4">
        <label className="flex flex-col text-xs font-medium uppercase tracking-wider text-stone-500">
          Decision
          <select name="decision" defaultValue={decision} className="mt-1 rounded border border-stone-300 px-3 py-2 text-sm text-stone-900">
            {DECISION_FILTERS.map((f) => (
              <option key={f.value} value={f.value}>
                {f.label}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col text-xs font-medium uppercase tracking-wider text-stone-500">
          Source kind
          <select name="source_kind" defaultValue={source_kind} className="mt-1 rounded border border-stone-300 px-3 py-2 text-sm text-stone-900">
            <option value="">All</option>
            {sourceKinds.map((s) => (
              <option key={s} value={s}>
                {s}
              </option>
            ))}
          </select>
        </label>
        <button className="rounded bg-stone-800 px-4 py-2 text-sm font-medium text-white">Filter</button>
      </form>

      <div className="text-sm text-stone-600">
        Showing {candidates.length} candidate{candidates.length === 1 ? "" : "s"}.
      </div>

      <div className="space-y-4">
        {candidates.map((c) => (
          <CandidateCard key={c.id} candidate={c} />
        ))}
        {candidates.length === 0 ? (
          <p className="rounded-lg border border-stone-300 bg-white p-6 text-stone-600">
            No candidates match this filter.
          </p>
        ) : null}
      </div>
    </div>
  );
}

function fmtMS(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000));
  const m = Math.floor(total / 60);
  const s = total % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

function highlight(text: string, needle: string) {
  if (!needle) return text;
  const idx = text.toLowerCase().indexOf(needle.toLowerCase());
  if (idx < 0) return text;
  return (
    <>
      {text.slice(0, idx)}
      <mark className="rounded bg-yellow-200 px-0.5">{text.slice(idx, idx + needle.length)}</mark>
      {text.slice(idx + needle.length)}
    </>
  );
}

function TranscriptContext({ ctx, mentionText }: { ctx: EntityMatchTranscriptContext; mentionText: string }) {
  return (
    <div className="mt-4 rounded border border-amber-200 bg-amber-50 p-4">
      <div className="flex flex-wrap items-baseline justify-between gap-2 text-xs text-amber-900">
        <div>
          <span className="font-semibold">Transcript mention</span> · {fmtMS(ctx.mention_start_ms)}–
          {fmtMS(ctx.mention_end_ms)} · TVW event{" "}
          <code className="font-mono">{ctx.tvw_event_id}</code>
          {ctx.mention_confidence > 0 ? (
            <> · deepgram conf {(ctx.mention_confidence * 100).toFixed(0)}%</>
          ) : null}
        </div>
        <span className="font-mono">job {ctx.diarization_job_id}</span>
      </div>
      <p className="mt-2 text-sm font-semibold text-stone-900">
        “{ctx.mention_text}”
      </p>
      {ctx.surrounding.length > 0 ? (
        <div className="mt-3 space-y-2">
          <p className="text-xs font-medium uppercase tracking-wider text-amber-900">Surrounding speech</p>
          <div className="space-y-2 rounded bg-white p-3 ring-1 ring-amber-200">
            {ctx.surrounding.map((s, i) => {
              const overlaps = s.end_ms >= ctx.mention_start_ms && s.start_ms <= ctx.mention_end_ms;
              return (
                <div
                  key={`${s.start_ms}-${i}`}
                  className={`text-sm ${overlaps ? "text-stone-900" : "text-stone-600"}`}
                >
                  <span className="mr-2 font-mono text-xs text-stone-500">
                    {fmtMS(s.start_ms)}
                    {s.cluster_label ? ` ${s.cluster_label}` : ""}
                  </span>
                  {overlaps ? highlight(s.text, mentionText) : s.text}
                </div>
              );
            })}
          </div>
        </div>
      ) : (
        <p className="mt-2 text-xs text-amber-900">No surrounding diarized segments found.</p>
      )}
    </div>
  );
}

function CandidateCard({ candidate: c }: { candidate: EntityMatchCandidate }) {
  return (
    <div className="rounded-lg border border-stone-300 bg-white p-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            <span className={`rounded-full px-2 py-0.5 text-xs font-medium ${decisionBadge(c.decision)}`}>
              {c.decision}
            </span>
            <span className="rounded-full bg-stone-100 px-2 py-0.5 text-xs font-mono text-stone-700">{c.source_kind}</span>
            <span className="text-xs text-stone-500">candidate confidence: {c.candidate_confidence}</span>
          </div>
          <h2 className="text-lg font-semibold text-stone-900">
            {c.source_name}
            <span className="text-stone-400"> → </span>
            <Link className="underline" href={`/organizations/${slugify(c.canonical_name)}`}>
              {c.canonical_name}
            </Link>
          </h2>
          <p className="text-xs text-stone-500">
            Normalized: <code>{c.normalized_name}</code>
            {c.source_table ? <> · table: <code>{c.source_table}</code></> : null}
            {c.source_pk ? <> · pk: <code>{c.source_pk}</code></> : null}
            {c.source_dataset_id ? <> · dataset: <code>{c.source_dataset_id}</code></> : null}
            {c.source_row_id ? <> · row: <code>{c.source_row_id}</code></> : null}
          </p>
        </div>
        <span className="font-mono text-xs text-stone-500">#{c.id}</span>
      </div>

      {c.evidence.length > 0 ? (
        <ul className="mt-3 list-disc space-y-1 pl-5 text-sm text-stone-700">
          {c.evidence.map((e, i) => (
            <li key={i}>{e}</li>
          ))}
        </ul>
      ) : null}

      {c.transcript ? <TranscriptContext ctx={c.transcript} mentionText={c.source_name} /> : null}

      <form action={decide} className="mt-4 grid gap-3 border-t border-stone-200 pt-4 sm:grid-cols-[1fr_1fr_auto]">
        <input type="hidden" name="candidateId" value={c.id} />
        <label className="text-xs font-medium uppercase tracking-wider text-stone-500">
          Reviewer
          <input
            name="reviewer"
            defaultValue="nolan"
            className="mt-1 w-full rounded border border-stone-300 px-3 py-2 text-sm text-stone-900"
          />
        </label>
        <label className="text-xs font-medium uppercase tracking-wider text-stone-500">
          Confidence
          <select
            name="confidence"
            defaultValue={c.reviewed_confidence || c.candidate_confidence || "possible"}
            className="mt-1 w-full rounded border border-stone-300 px-3 py-2 text-sm text-stone-900"
          >
            {CONFIDENCE_OPTIONS.map((opt) => (
              <option key={opt} value={opt}>
                {opt}
              </option>
            ))}
          </select>
        </label>
        <label className="text-xs font-medium uppercase tracking-wider text-stone-500 sm:col-span-3">
          Notes
          <input
            name="notes"
            className="mt-1 w-full rounded border border-stone-300 px-3 py-2 text-sm text-stone-900"
          />
        </label>
        <div className="flex flex-wrap gap-2 sm:col-span-3">
          <button name="decision" value="confirmed" className="rounded bg-emerald-700 px-4 py-2 text-sm font-medium text-white">
            Confirm
          </button>
          <button name="decision" value="rejected" className="rounded bg-rose-700 px-4 py-2 text-sm font-medium text-white">
            Reject
          </button>
          <button name="decision" value="needs_review" className="rounded bg-stone-700 px-4 py-2 text-sm font-medium text-white">
            Needs review
          </button>
        </div>
      </form>
    </div>
  );
}

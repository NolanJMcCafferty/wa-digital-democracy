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

const PAGE_SIZE = 50;

const SOURCE_KINDS = [
  "datawa_contract_contractor",
  "datawa_master_contract_vendor",
  "datawa_master_contract_customer",
  "datawa_it_contract_contractor",
  "datawa_it_contract_dba",
  "datawa_webs_vendor",
  "pdc_lobbying_organization",
  "organization_alias",
  "fiscalwa_vendor_payment",
  "federal_award_recipient",
  "deepgram_organization_mention",
];

async function decide(formData: FormData) {
  "use server";
  const candidateId = String(formData.get("candidateId") ?? "");
  const decision = String(formData.get("decision") ?? "") as EntityMatchDecision;
  const reviewer = String(formData.get("reviewer") ?? "");
  const notes = String(formData.get("notes") ?? "");
  await decideEntityMatchCandidate(candidateId, decision, reviewer, notes);
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
  searchParams: Promise<{ decision?: string; source_kind?: string; page?: string }>;
}) {
  const { decision = "needs_review", source_kind = "", page: pageParam } = await searchParams;
  const page = Math.max(1, Number.parseInt(pageParam ?? "1", 10) || 1);
  const offset = (page - 1) * PAGE_SIZE;
  const { candidates, total, limit } = await listEntityMatchCandidates({
    decision: decision || undefined,
    sourceKind: source_kind || undefined,
    limit: PAGE_SIZE,
    offset,
  });
  const totalPages = Math.max(1, Math.ceil(total / limit));
  const startIdx = total === 0 ? 0 : offset + 1;
  const endIdx = Math.min(offset + candidates.length, total);

  const buildHref = (overrides: { page?: number; decision?: string; source_kind?: string }) => {
    const params = new URLSearchParams();
    const d = overrides.decision ?? decision;
    const sk = overrides.source_kind ?? source_kind;
    if (d) params.set("decision", d);
    if (sk) params.set("source_kind", sk);
    const p = overrides.page ?? page;
    if (p > 1) params.set("page", String(p));
    const qs = params.toString();
    return qs ? `/admin/review/entities?${qs}` : "/admin/review/entities";
  };

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
            {SOURCE_KINDS.map((s) => (
              <option key={s} value={s}>
                {s}
              </option>
            ))}
          </select>
        </label>
        <button className="rounded bg-stone-800 px-4 py-2 text-sm font-medium text-white">Filter</button>
      </form>

      <div className="flex flex-wrap items-baseline justify-between gap-3 text-sm text-stone-600">
        <p>
          {total === 0 ? (
            <>No candidates match this filter.</>
          ) : (
            <>
              Showing <span className="font-medium text-stone-900">{startIdx.toLocaleString()}–{endIdx.toLocaleString()}</span>{" "}
              of <span className="font-medium text-stone-900">{total.toLocaleString()}</span> candidate{total === 1 ? "" : "s"}.
            </>
          )}
        </p>
        {totalPages > 1 ? (
          <p className="text-xs text-stone-500">
            Page {page} of {totalPages}
          </p>
        ) : null}
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

      <form action={decide} className="mt-4 grid gap-3 border-t border-stone-200 pt-4 sm:grid-cols-2">
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
          Notes
          <input
            name="notes"
            className="mt-1 w-full rounded border border-stone-300 px-3 py-2 text-sm text-stone-900"
          />
        </label>
        <div className="flex flex-wrap gap-2 sm:col-span-2">
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

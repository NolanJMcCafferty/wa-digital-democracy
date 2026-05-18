import Link from "next/link";
import { notFound } from "next/navigation";
import { listOrganizationBundles, loadOrganizationBundle } from "@/lib/loadBundle";
import { confidenceLabel, formatDateTime } from "@/lib/format";
import type { OrgContext, Position } from "@/lib/bundle";

const POSITION_ORDER: Position[] = ["Pro", "Con", "Other", "Unknown"];

export async function generateStaticParams() {
  const orgs = await listOrganizationBundles();
  return orgs.map((o) => ({ slug: o.slug }));
}

export default async function OrganizationPage({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  const org = await loadOrganizationBundle(slug);
  if (!org) notFound();

  const byContextType = new Map<string, OrgContext[]>();
  for (const c of org.contexts) {
    const list = byContextType.get(c.context_type) ?? [];
    list.push(c);
    byContextType.set(c.context_type, list);
  }

  return (
    <article className="space-y-10">
      <section className="space-y-4">
        <p className="text-sm uppercase tracking-wider text-stone-500">
          Organization
        </p>
        <div className="space-y-3">
          <h1 className="text-3xl font-bold tracking-tight text-stone-900">
            {org.canonicalName}
          </h1>
          {org.aliases.length > 0 ? (
            <p className="text-sm text-stone-500">
              Also known as: {org.aliases.join(", ")}
            </p>
          ) : null}
          {org.matchNotes ? (
            <p className="max-w-3xl text-stone-600">{org.matchNotes}</p>
          ) : null}
        </div>
      </section>

      <section className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
        <Metric label="Match" value={confidenceLabel(org.matchConfidence)} />
        <Metric label="Linked testifiers" value={org.testifierCount.toLocaleString()} />
        <Metric label="Appearances" value={org.appearances.length.toLocaleString()} />
        <Metric label="PDC records" value={org.contextCount.toLocaleString()} />
      </section>

      <section aria-labelledby="positions-heading" className="space-y-4 rounded-lg border border-stone-300 bg-white p-6">
        <h2 id="positions-heading" className="text-xl font-semibold text-stone-900">
          Positions in testimony sign-ins
        </h2>
        <div className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
          {POSITION_ORDER.map((p) => (
            <Metric key={p} label={p} value={org.positions[p].toLocaleString()} />
          ))}
        </div>
        <p className="text-xs text-stone-500">
          Counts come from CSI organization matches in generated hearing bundles.
        </p>
      </section>

      <section aria-labelledby="appearances-heading" className="space-y-4">
        <h2 id="appearances-heading" className="text-xl font-semibold text-stone-900">
          Hearing appearances
        </h2>
        <ul className="divide-y divide-stone-300 rounded border border-stone-300 bg-white">
          {org.appearances.map((a) => {
            const billSlug = `${a.billPrefix}${a.billNumber}`;
            return (
              <li key={`${a.billId}-${a.csiAgendaItemId}`} className="p-4">
                <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                  <div className="space-y-1">
                    <Link
                      href={`/bills/${a.biennium}/${billSlug}`}
                      className="font-medium text-blue-700 underline hover:text-blue-900"
                    >
                      {a.billId} — {a.hearingTitle}
                    </Link>
                    <p className="text-sm text-stone-600">
                      {a.committeeName} · {formatDateTime(a.meetingDatetime)}
                    </p>
                    <p className="text-xs text-stone-500">
                      {a.testifierCount.toLocaleString()} linked testifier
                      {a.testifierCount === 1 ? "" : "s"}
                      {a.position ? ` · ${a.position}` : ""}
                    </p>
                  </div>
                  {a.csiAgendaItemId ? (
                    <Link
                      href={`/hearings/${a.csiAgendaItemId}`}
                      className="text-sm text-blue-700 underline hover:text-blue-900"
                    >
                      Hearing page →
                    </Link>
                  ) : null}
                </div>
              </li>
            );
          })}
        </ul>
      </section>

      <section aria-labelledby="context-heading" className="space-y-4 rounded-lg border border-stone-300 bg-white p-6">
        <h2 id="context-heading" className="text-xl font-semibold text-stone-900">
          Lobbying and public-record context
        </h2>
        {org.contexts.length === 0 ? (
          <p className="text-sm text-stone-600">No PDC context records loaded.</p>
        ) : (
          <div className="space-y-3">
            {Array.from(byContextType.entries()).map(([type, rows]) => (
              <details key={type} open className="rounded border border-stone-200 bg-stone-50 p-3">
                <summary className="cursor-pointer text-sm font-medium text-stone-800">
                  {type.replaceAll("_", " ")} · {rows.length.toLocaleString()} record
                  {rows.length === 1 ? "" : "s"}
                </summary>
                <ul className="mt-3 space-y-2 text-sm">
                  {rows.map((r, i) => (
                    <li key={i} className="rounded bg-white p-3 ring-1 ring-stone-200">
                      <SummaryFields fields={r.summary_fields} />
                      <div className="mt-2 flex flex-wrap items-baseline gap-2 text-xs text-stone-500">
                        <span>Dataset: {r.source_dataset_id}</span>
                        <span>·</span>
                        <span>{confidenceLabel(r.match_confidence)}</span>
                        {r.source_url ? (
                          <>
                            <span>·</span>
                            <a
                              href={r.source_url}
                              target="_blank"
                              rel="noreferrer"
                              className="text-blue-700 underline hover:text-blue-900"
                            >
                              Source row →
                            </a>
                          </>
                        ) : null}
                      </div>
                    </li>
                  ))}
                </ul>
              </details>
            ))}
          </div>
        )}
        <p className="text-xs text-stone-500">
          This page shows context, not causation. Lobbying and campaign-finance
          records are public background and should not be read as proof that an
          organization caused a bill outcome.
        </p>
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

function SummaryFields({ fields }: { fields: Record<string, unknown> }) {
  const entries = Object.entries(fields).filter(
    ([, v]) => v !== null && v !== ""
  );
  if (entries.length === 0) return null;
  return (
    <dl className="grid grid-cols-[max-content_1fr] gap-x-3 gap-y-0.5 text-xs">
      {entries.map(([k, v]) => (
        <span key={k} className="contents">
          <dt className="text-stone-500">{k.replaceAll("_", " ")}</dt>
          <dd className="break-words text-stone-800">{String(v)}</dd>
        </span>
      ))}
    </dl>
  );
}

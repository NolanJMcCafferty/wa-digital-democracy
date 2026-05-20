import Link from "next/link";
import { notFound } from "next/navigation";
import { listOrganizations, loadOrganizationPage } from "@/lib/api";
import { formatDateTime } from "@/lib/format";
import type { Position } from "@/lib/pageTypes";

const POSITION_ORDER: Position[] = ["Pro", "Con", "Other", "Unknown"];

export async function generateStaticParams() {
  const orgs = await listOrganizations();
  return orgs.map((o) => ({ slug: o.slug }));
}

export default async function OrganizationPage({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  const org = await loadOrganizationPage(slug);
  if (!org) notFound();

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

      <section className="grid grid-cols-2 gap-3 text-sm">
        <Metric label="Linked testifiers" value={org.testifierCount.toLocaleString()} />
        <Metric label="Appearances" value={org.appearances.length.toLocaleString()} />
        <Metric label="Public records" value={org.contexts.length.toLocaleString()} />
      </section>

      {org.contexts.length > 0 ? (
        <section aria-labelledby="context-heading" className="space-y-4">
          <div className="space-y-1">
            <h2 id="context-heading" className="text-xl font-semibold text-stone-900">
              Related public-record context
            </h2>
            <p className="text-sm text-stone-600">
              These records are matched context for the organization, not proof
              that a record caused or influenced testimony on a bill.
            </p>
          </div>
          <ul className="divide-y divide-stone-300 rounded border border-stone-300 bg-white">
            {org.contexts.map((c, idx) => (
              <li key={`${c.sourceKind}-${c.sourceRecordId ?? idx}-${c.sourceName}`} className="p-4">
                <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                  <div className="space-y-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="rounded border border-stone-300 bg-stone-50 px-2 py-0.5 text-xs font-medium text-stone-700">
                        {contextLabel(c.contextType)}
                      </span>
                      <span className="text-xs uppercase tracking-wider text-stone-500">
                        {c.sourceLabel}
                      </span>
                    </div>
                    <p className="font-medium text-stone-900">{c.sourceName}</p>
                    {c.detail ? (
                      <p className="text-sm text-stone-600">{c.detail}</p>
                    ) : null}
                    <p className="text-xs text-stone-500">
                      {[formatAmount(c.amount), c.recordYear ? String(c.recordYear) : c.recordDate]
                        .filter(Boolean)
                        .join(" · ")}
                    </p>
                  </div>
                  {c.url ? (
                    <a
                      href={c.url}
                      className="text-sm text-blue-700 underline hover:text-blue-900"
                      rel="noreferrer"
                      target="_blank"
                    >
                      Source record →
                    </a>
                  ) : null}
                </div>
              </li>
            ))}
          </ul>
        </section>
      ) : null}

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
          Counts come from Committee Sign In organization matches.
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
                  {a.hearingId ? (
                    <Link
                      href={`/hearings/${a.hearingId}`}
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

function contextLabel(contextType: string): string {
  switch (contextType) {
    case "lobbying_registration":
      return "Lobbying";
    case "state_contract":
      return "Contract";
    case "state_vendor":
      return "Vendor";
    case "state_vendor_payment":
      return "Payment";
    case "federal_award":
      return "Federal award";
    default:
      return "Record";
  }
}

function formatAmount(amount?: string): string {
  if (!amount) return "";
  const n = Number(amount);
  if (!Number.isFinite(n)) return amount;
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    maximumFractionDigits: 0,
  }).format(n);
}

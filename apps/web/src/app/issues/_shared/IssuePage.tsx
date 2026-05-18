import Link from "next/link";
import {
  listHearingBundles,
  listLocalBundles,
  loadBundle,
  slugify,
} from "@/lib/loadBundle";
import { formatDateTime } from "@/lib/format";
import type { Bundle, Position } from "@/lib/bundle";

const POSITION_ORDER: Position[] = ["Pro", "Con", "Other", "Unknown"];

export type IssuePageConfig = {
  slug: string;
  title: string;
  description: string;
  emptyLabel: string;
  keywords: string[];
};

export async function IssuePage({ config }: { config: IssuePageConfig }) {
  const entries = await listLocalBundles();
  const allBundles = (
    await Promise.all(
      entries.map((e) => loadBundle(e.biennium, e.billPrefix, e.billNumber))
    )
  ).filter((b): b is Bundle => Boolean(b));
  const allHearings = await listHearingBundles();
  const bundles = allBundles.filter((b) => bundleMatchesIssue(b, config));
  const billIDs = new Set(bundles.map((b) => b.bill.bill_id));
  const hearings = allHearings.filter((h) => billIDs.has(h.billId));

  const totals = bundles.reduce(
    (acc, b) => {
      acc.bills += 1;
      acc.testifiers += b.testifiers.length;
      acc.testified += b.testifiers.filter((t) => t.testified).length;
      acc.transcriptSegments += b.transcript?.segments?.length ?? 0;
      acc.organizations += b.organizations.length;
      acc.sources += b.sources.length;
      for (const t of b.testifiers) acc.positions[t.position] += 1;
      return acc;
    },
    {
      bills: 0,
      testifiers: 0,
      testified: 0,
      transcriptSegments: 0,
      organizations: 0,
      sources: 0,
      positions: { Pro: 0, Con: 0, Other: 0, Unknown: 0 } as Record<Position, number>,
    }
  );

  const orgs = new Map<
    string,
    { count: number; confidence: string; contexts: number }
  >();
  for (const b of bundles) {
    for (const org of b.organizations) {
      const prev = orgs.get(org.canonical_name) ?? {
        count: 0,
        confidence: org.match_confidence,
        contexts: 0,
      };
      prev.count += org.testifier_count ?? 0;
      prev.contexts += org.context?.length ?? 0;
      orgs.set(org.canonical_name, prev);
    }
  }

  return (
    <article className="space-y-10">
      <section className="space-y-4">
        <p className="text-sm uppercase tracking-wider text-stone-500">
          Issue page
        </p>
        <div className="space-y-3">
          <h1 className="text-3xl font-bold tracking-tight text-stone-900">
            {config.title}
          </h1>
          <p className="max-w-3xl text-stone-600">{config.description}</p>
        </div>
      </section>

      {bundles.length === 0 ? (
        <p className="rounded border border-stone-300 bg-stone-50 p-4 text-sm text-stone-600">
          No {config.emptyLabel} legislation is available yet.
        </p>
      ) : (
        <>
          <section aria-labelledby={`${config.slug}-summary`} className="space-y-4">
            <h2 id={`${config.slug}-summary`} className="text-xl font-semibold text-stone-900">
              Current coverage
            </h2>
            <div className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-3 lg:grid-cols-6">
              <Metric label="Bills" value={totals.bills.toLocaleString()} />
              <Metric label="Hearings" value={hearings.length.toLocaleString()} />
              <Metric label="Signed in" value={totals.testifiers.toLocaleString()} />
              <Metric label="Testified" value={totals.testified.toLocaleString()} />
              <Metric label="Transcript" value={`${totals.transcriptSegments.toLocaleString()} excerpts`} />
              <Metric label="Sources" value={totals.sources.toLocaleString()} />
            </div>
          </section>

          <section aria-labelledby={`${config.slug}-positions`} className="space-y-4 rounded-lg border border-stone-300 bg-white p-6">
            <h2 id={`${config.slug}-positions`} className="text-xl font-semibold text-stone-900">
              Testimony positions
            </h2>
            <div className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
              {POSITION_ORDER.map((p) => (
                <Metric key={p} label={p} value={totals.positions[p].toLocaleString()} />
              ))}
            </div>
            <p className="text-xs text-stone-500">
              Counts are from Committee Sign In records, including people who registered a position but did not testify.
            </p>
          </section>

          <section aria-labelledby={`${config.slug}-bills`} className="space-y-4">
            <h2 id={`${config.slug}-bills`} className="text-xl font-semibold text-stone-900">
              Bills in this issue view
            </h2>
            <ul className="divide-y divide-stone-300 rounded border border-stone-300 bg-white">
              {bundles.map((b) => {
                const billSlug = b.bill.bill_id.replace(/\s+/g, "");
                return (
                  <li key={`${b.bill.biennium}-${b.bill.bill_id}`} className="p-4">
                    <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                      <div className="space-y-1">
                        <Link
                          href={`/bills/${b.bill.biennium}/${billSlug}`}
                          className="font-medium text-blue-700 underline hover:text-blue-900"
                        >
                          {b.bill.bill_id} — {b.bill.title}
                        </Link>
                        <p className="text-sm text-stone-600">
                          {b.status.current ?? "Status unavailable"}
                        </p>
                        <p className="text-xs text-stone-500">
                          {b.testifiers.length.toLocaleString()} sign-ins · {b.transcript?.segments?.length ?? 0} transcript excerpts · {b.sources.length} sources
                        </p>
                      </div>
                      {b.hearing?.csi_agenda_item_id ? (
                        <Link
                          href={`/hearings/${b.hearing.csi_agenda_item_id}`}
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

          <section aria-labelledby={`${config.slug}-hearings`} className="space-y-4">
            <h2 id={`${config.slug}-hearings`} className="text-xl font-semibold text-stone-900">
              Hearings
            </h2>
            <ul className="divide-y divide-stone-300 rounded border border-stone-300 bg-white">
              {hearings.map((h) => (
                <li key={h.csiAgendaItemId} className="p-4">
                  <Link
                    href={`/hearings/${h.csiAgendaItemId}`}
                    className="font-medium text-blue-700 underline hover:text-blue-900"
                  >
                    {h.title}
                  </Link>
                  <p className="mt-1 text-sm text-stone-600">
                    {h.committeeName} · {formatDateTime(h.meetingDatetime)}
                  </p>
                  <p className="mt-1 text-xs text-stone-500">
                    {h.billId} · CSI agenda item {h.csiAgendaItemId}
                  </p>
                </li>
              ))}
            </ul>
          </section>

          <section aria-labelledby={`${config.slug}-orgs`} className="space-y-4 rounded-lg border border-stone-300 bg-white p-6">
            <h2 id={`${config.slug}-orgs`} className="text-xl font-semibold text-stone-900">
              Organizations with reviewed context
            </h2>
            {orgs.size === 0 ? (
              <p className="text-sm text-stone-600">
                No organizations have reviewed matches yet.
              </p>
            ) : (
              <ul className="space-y-3 text-sm">
                {Array.from(orgs.entries()).map(([name, o]) => (
                  <li key={name} className="rounded border border-stone-200 bg-stone-50 p-3">
                    <Link
                      href={`/organizations/${slugify(name)}`}
                      className="font-medium text-blue-700 underline hover:text-blue-900"
                    >
                      {name}
                    </Link>
                    <div className="mt-1 text-stone-600">
                      {o.count.toLocaleString()} linked testifier{o.count === 1 ? "" : "s"} · {o.contexts.toLocaleString()} PDC context record{o.contexts === 1 ? "" : "s"} · {o.confidence}
                    </div>
                  </li>
                ))}
              </ul>
            )}
            <p className="text-xs text-stone-500">
              Showing context, not causation. Organization matches come from
              reviewed aliases and official PDC/data.wa.gov records.
            </p>
          </section>
        </>
      )}
    </article>
  );
}

function bundleMatchesIssue(bundle: Bundle, config: IssuePageConfig): boolean {
  const haystack = [
    bundle.bill.bill_id,
    bundle.bill.title,
    bundle.bill.description,
    bundle.hearing?.agenda_item_label,
    bundle.hearing?.committee_name,
    ...bundle.organizations.map((o) => o.canonical_name),
  ]
    .filter(Boolean)
    .join(" ")
    .toLowerCase();

  return config.keywords.some((keyword) => haystack.includes(keyword.toLowerCase()));
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded border border-stone-300 bg-stone-50 p-3">
      <div className="text-xs uppercase tracking-wider text-stone-500">{label}</div>
      <div className="mt-1 font-semibold text-stone-900">{value}</div>
    </div>
  );
}

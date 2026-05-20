import Link from "next/link";
import { notFound } from "next/navigation";
import { loadHearingBundle, listHearingBundles, type HearingAgendaItemEntry } from "@/lib/loadBundle";
import { formatDateTime } from "@/lib/format";

type Params = { hearingId: string };

export async function generateStaticParams() {
  const hearings = await listHearingBundles();
  return hearings.map((h) => ({ hearingId: String(h.hearingId) }));
}

export default async function HearingPage({
  params,
}: {
  params: Promise<Params>;
}) {
  const { hearingId } = await params;
  const hearing = await loadHearingBundle(hearingId);
  if (!hearing) notFound();

  const totalSignIns = hearing.agendaItems.reduce((n, a) => n + a.testifierCount, 0);
  const totalTestified = hearing.agendaItems.reduce((n, a) => n + a.testifiedCount, 0);

  return (
    <article className="space-y-12">
      <section className="space-y-5">
        <div className="space-y-2">
          <p className="text-sm uppercase tracking-wider text-stone-500">
            Hearing
          </p>
          <h1 className="text-3xl font-bold tracking-tight text-stone-900">
            {hearing.committeeName}
          </h1>
          <p className="text-stone-600">
            {hearing.chamber} · {formatDateTime(hearing.meetingDatetime)}
            {hearing.location ? ` · ${hearing.location}` : ""}
          </p>
        </div>

        <div className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
          <Metric label="Agenda items" value={hearing.agendaItems.length.toLocaleString()} />
          <Metric label="Signed in" value={totalSignIns.toLocaleString()} />
          <Metric label="Testified" value={totalTestified.toLocaleString()} />
          <Metric label="TVW event" value={hearing.tvwEventId || "—"} />
        </div>

        <div className="flex flex-wrap gap-3 text-sm">
          {hearing.tvwUrl ? (
            <a
              href={hearing.tvwUrl}
              target="_blank"
              rel="noreferrer"
              className="rounded border border-stone-300 bg-white px-3 py-1.5 text-stone-800 hover:bg-stone-50"
            >
              Watch hearing on TVW →
            </a>
          ) : null}
        </div>
      </section>

      <section aria-labelledby="agenda-items-heading" className="space-y-4 rounded-lg border border-stone-300 bg-white p-6">
        <div className="space-y-1">
          <h2 id="agenda-items-heading" className="text-xl font-semibold text-stone-900">
            Agenda items
          </h2>
          <p className="text-sm text-stone-600">
            Bills and testimony sign-in records associated with this committee hearing.
          </p>
        </div>

        {hearing.agendaItems.length === 0 ? (
          <p className="text-sm text-stone-600">No agenda items have been linked to this hearing yet.</p>
        ) : (
          <ul className="divide-y divide-stone-200 rounded border border-stone-200">
            {hearing.agendaItems.map((item) => (
              <AgendaItemRow key={item.csiAgendaItemId || `${item.billId}-${item.agendaItemLabel}`} item={item} />
            ))}
          </ul>
        )}
      </section>
    </article>
  );
}

function AgendaItemRow({ item }: { item: HearingAgendaItemEntry }) {
  const billSlug = `${item.billPrefix}${item.billNumber}`;
  return (
    <li className="p-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="space-y-1">
          {item.billId ? (
            <Link
              href={`/bills/${item.biennium}/${billSlug}`}
              className="font-medium text-blue-700 underline hover:text-blue-900"
            >
              {item.billId} — {item.agendaItemLabel || "Agenda item"}
            </Link>
          ) : (
            <h3 className="font-medium text-stone-900">{item.agendaItemLabel || "Agenda item"}</h3>
          )}
          <p className="text-sm text-stone-600">
            {item.testifierCount.toLocaleString()} signed in; {item.testifiedCount.toLocaleString()} testified
          </p>
          {item.csiAgendaItemId ? (
            <p className="text-xs text-stone-500">
              CSI agenda item <span className="font-mono">{item.csiAgendaItemId}</span>
            </p>
          ) : null}
        </div>
        {item.billId ? (
          <Link
            href={`/bills/${item.biennium}/${billSlug}`}
            className="text-sm text-blue-700 underline hover:text-blue-900"
          >
            Bill detail →
          </Link>
        ) : null}
      </div>
    </li>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded border border-stone-300 bg-white p-3">
      <div className="text-xs uppercase tracking-wider text-stone-500">{label}</div>
      <div className="mt-1 font-semibold text-stone-900">{value}</div>
    </div>
  );
}

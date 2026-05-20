import Link from "next/link";
import { notFound } from "next/navigation";
import { loadHearingPage, listHearings, type HearingAgendaItemEntry } from "@/lib/loadBundle";
import type { Position, Testifier } from "@/lib/pageTypes";
import { formatDateTime } from "@/lib/format";
import { DiarizedTranscriptSection } from "./_sections/DiarizedTranscriptSection";

const POSITION_ORDER: Position[] = ["Pro", "Con", "Other"];

const POSITION_STYLE: Record<Position, string> = {
  Pro: "text-emerald-800 bg-emerald-100",
  Con: "text-rose-800 bg-rose-100",
  Other: "text-stone-700 bg-stone-200",
  Unknown: "text-stone-500 bg-stone-100",
};

type Params = { hearingId: string };

export async function generateStaticParams() {
  const hearings = await listHearings();
  return hearings.map((h) => ({ hearingId: String(h.hearingId) }));
}

export default async function HearingPage({
  params,
}: {
  params: Promise<Params>;
}) {
  const { hearingId } = await params;
  const hearing = await loadHearingPage(hearingId);
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

      <div className="grid grid-cols-1 gap-8 lg:grid-cols-[1fr_320px]">
        <div className="min-w-0 space-y-12">
          {hearing.diarizedTranscript ? (
            <DiarizedTranscriptSection
              transcript={hearing.diarizedTranscript}
              tvwEventId={hearing.tvwEventId}
            />
          ) : (
            <p className="rounded border border-stone-300 bg-stone-50 p-4 text-sm text-stone-600">
              No diarized transcript is available for this hearing yet.
            </p>
          )}
        </div>

        <aside className="lg:sticky lg:top-6 lg:self-start">
          <section
            aria-labelledby="agenda-items-heading"
            className="space-y-3 rounded-lg border border-stone-300 bg-white p-4"
          >
            <h2
              id="agenda-items-heading"
              className="text-sm font-semibold uppercase tracking-wider text-stone-600"
            >
              Agenda
            </h2>
            {hearing.agendaItems.length === 0 ? (
              <p className="text-sm text-stone-600">
                No agenda items have been linked to this hearing yet.
              </p>
            ) : (
              <ul className="space-y-2">
                {hearing.agendaItems.map((item) => (
                  <AgendaItemCard
                    key={item.csiAgendaItemId || `${item.billId}-${item.agendaItemLabel}`}
                    item={item}
                  />
                ))}
              </ul>
            )}
          </section>
        </aside>
      </div>
    </article>
  );
}

function AgendaItemCard({ item }: { item: HearingAgendaItemEntry }) {
  const billSlug = `${item.billPrefix}${item.billNumber}`;
  const href = item.billId ? `/bills/${item.biennium}/${billSlug}` : null;
  const testifiers = item.section?.testifiers ?? [];
  return (
    <li className="space-y-2 rounded border border-stone-200 bg-stone-50 p-3">
      <div className="space-y-1">
        {item.billId ? (
          href ? (
            <Link
              href={href}
              className="text-sm font-semibold text-blue-700 hover:text-blue-900"
            >
              {item.billId}
            </Link>
          ) : (
            <div className="text-sm font-semibold text-blue-700">{item.billId}</div>
          )
        ) : null}
        <div className="text-sm text-stone-800">
          {item.agendaItemLabel || "Agenda item"}
        </div>
        <div className="text-xs text-stone-500 tabular-nums">
          {item.testifierCount.toLocaleString()} signed in ·{" "}
          {item.testifiedCount.toLocaleString()} testified
        </div>
      </div>
      {testifiers.length > 0 ? (
        <PositionBreakdown testifiers={testifiers} />
      ) : null}
    </li>
  );
}

function PositionBreakdown({ testifiers }: { testifiers: Testifier[] }) {
  const groups: Record<Position, Testifier[]> = {
    Pro: [],
    Con: [],
    Other: [],
    Unknown: [],
  };
  for (const t of testifiers) groups[t.position]?.push(t);
  return (
    <div className="space-y-2 border-t border-stone-200 pt-2">
      {POSITION_ORDER.map((p) => {
        const testified = groups[p].filter((t) => t.testified);
        if (groups[p].length === 0) return null;
        return (
          <details key={p} className="text-xs">
            <summary className="flex cursor-pointer items-center justify-between gap-2 text-stone-700">
              <span
                className={`rounded px-1.5 py-0.5 font-medium uppercase tracking-wider ${POSITION_STYLE[p]}`}
              >
                {p}
              </span>
              <span className="tabular-nums text-stone-500">
                {testified.length.toLocaleString()} of{" "}
                {groups[p].length.toLocaleString()}
              </span>
            </summary>
            <ul className="mt-1 space-y-0.5 pl-1 text-stone-700">
              {groups[p].map((t, i) => (
                <li
                  key={`${t.raw_name}-${i}`}
                  className={t.testified ? "" : "text-stone-400"}
                >
                  {t.raw_name}
                  {t.raw_organization ? (
                    <span className="text-stone-500">
                      {" "}
                      · {t.raw_organization}
                    </span>
                  ) : null}
                </li>
              ))}
            </ul>
          </details>
        );
      })}
    </div>
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

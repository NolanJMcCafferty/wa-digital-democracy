import Link from "next/link";
import { notFound } from "next/navigation";
import { loadHearingPage, listHearings, type HearingAgendaItemEntry } from "@/lib/api";
import type { Position, Testifier } from "@/lib/pageTypes";
import { formatDateTime } from "@/lib/format";
import { DiarizedTranscriptSection } from "./_sections/DiarizedTranscriptSection";

export const dynamic = "force-dynamic";

const POSITION_ORDER: Position[] = ["Pro", "Con", "Other"];

const POSITION_STYLE: Record<Position, string> = {
  Pro: "border-emerald-400",
  Con: "border-rose-400",
  Other: "border-stone-400",
  Unknown: "border-stone-300",
};

const POSITION_BADGE_STYLE: Record<Position, string> = {
  Pro: "bg-emerald-100 text-emerald-900",
  Con: "bg-rose-100 text-rose-900",
  Other: "bg-stone-200 text-stone-800",
  Unknown: "bg-stone-100 text-stone-500",
};

type Params = { hearingId: string };

export async function generateStaticParams() {
  if (process.env.SKIP_BUILD_STATIC_PARAMS === "1") return [];
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
          <TVWMetric href={hearing.tvwUrl} />
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

        <aside className={`lg:sticky lg:top-6 lg:self-start ${hearing.diarizedTranscript ? "lg:pt-11" : ""}`}>
          <section
            aria-labelledby="agenda-items-heading"
            className="space-y-3 rounded-lg border border-stone-300 bg-white p-4"
          >
            <h2
              id="agenda-items-heading"
              className="text-sm font-semibold uppercase tracking-wider text-stone-600"
            >
              Agenda bills
            </h2>
            {hearing.agendaItems.length === 0 ? (
              <p className="text-sm text-stone-600">
                No agenda items have been linked to this hearing yet.
              </p>
            ) : (
              <ul className="divide-y divide-stone-200">
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
  const testifiedSpeakers = (item.section?.testifiers ?? []).filter((t) => t.testified);
  return (
    <li className="space-y-3 py-3 first:pt-0 last:pb-0">
      <div className="space-y-2">
        <div className="flex items-start justify-between gap-3">
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
        </div>
        <div className="text-sm leading-snug text-stone-800">
          {item.agendaItemLabel || "Agenda item"}
        </div>
        <div className="text-xs text-stone-500">
          {item.testifierCount.toLocaleString()} signed in ·{" "}
          {item.testifiedCount.toLocaleString()} testified
        </div>
      </div>
      {testifiedSpeakers.length > 0 ? (
        <PositionBreakdown testifiers={testifiedSpeakers} />
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
    <div className="space-y-3 border-t border-stone-200 pt-3">
      {POSITION_ORDER.map((p) => {
        const rows = groups[p];
        return (
          <section key={p} className={`space-y-2 border-l-2 pl-3 ${POSITION_STYLE[p]}`}>
            <h3 className="flex items-center justify-between gap-2">
              <span
                className={`rounded-full px-2 py-0.5 text-[11px] font-semibold uppercase tracking-wider ${POSITION_BADGE_STYLE[p]}`}
              >
                {p}
              </span>
              <span className="text-xs tabular-nums text-stone-500">
                {rows.length.toLocaleString()} speaker{rows.length === 1 ? "" : "s"}
              </span>
            </h3>
            {rows.length > 0 ? (
              <ul className="space-y-1.5">
                {rows.map((t, i) => (
                  <li
                    key={`${t.raw_name}-${i}`}
                    className="grid grid-cols-[1.75rem_1fr] gap-2 rounded-md px-2 py-1.5 hover:bg-stone-50"
                  >
                    <span
                      aria-hidden="true"
                      className={`flex h-7 w-7 items-center justify-center rounded-full text-[11px] font-semibold ring-1 ring-white ${POSITION_BADGE_STYLE[p]}`}
                    >
                      {initials(t.raw_name)}
                    </span>
                    <span className="min-w-0">
                      <span className="block break-words text-sm font-medium leading-snug text-stone-900">
                        {t.raw_name}
                      </span>
                    </span>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="text-xs text-stone-400">No testified speakers</p>
            )}
          </section>
        );
      })}
    </div>
  );
}

function initials(name: string): string {
  const parts = name
    .trim()
    .split(/\s+/)
    .filter(Boolean);
  if (parts.length === 0) return "?";
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return `${parts[0][0]}${parts[parts.length - 1][0]}`.toUpperCase();
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded border border-stone-300 bg-white p-3">
      <div className="text-xs uppercase tracking-wider text-stone-500">{label}</div>
      <div className="mt-1 font-semibold text-stone-900">{value}</div>
    </div>
  );
}

function TVWMetric({ href }: { href?: string }) {
  if (!href) {
    return (
      <div className="rounded border border-stone-300 bg-white p-3">
        <div className="text-xs uppercase tracking-wider text-stone-500">TVW</div>
        <div className="mt-1 font-semibold text-stone-400">Unavailable</div>
      </div>
    );
  }

  return (
    <a
      href={href}
      target="_blank"
      rel="noreferrer"
      className="rounded border border-stone-300 bg-white p-3 text-stone-900 hover:border-blue-300 hover:bg-blue-50"
    >
      <div className="text-xs uppercase tracking-wider text-stone-500">TVW</div>
      <div className="mt-1 font-semibold text-blue-700">Watch hearing →</div>
    </a>
  );
}

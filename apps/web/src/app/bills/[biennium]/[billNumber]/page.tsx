import Link from "next/link";
import { notFound } from "next/navigation";
import { loadBundle } from "@/lib/loadBundle";
import type { HearingSection } from "@/lib/bundle";
import { BillSnapshot } from "./_sections/BillSnapshot";
import { StatusTimeline } from "./_sections/StatusTimeline";
import { formatDateTime } from "@/lib/format";

type Params = { biennium: string; billNumber: string };

// Parses route's [billNumber] like "HB1501" into ("HB", 1501).
function parseBillSlug(slug: string): { prefix: string; number: number } | null {
  const m = slug.match(/^([A-Z]+)([0-9]+)$/i);
  if (!m) return null;
  return { prefix: m[1].toUpperCase(), number: parseInt(m[2], 10) };
}

export default async function BillHearingPage({
  params,
}: {
  params: Promise<Params>;
}) {
  const { biennium, billNumber: slug } = await params;
  const parsed = parseBillSlug(slug);
  if (!parsed) notFound();

  const bundle = await loadBundle(biennium, parsed.prefix, parsed.number);
  if (!bundle) notFound();

  const sections: HearingSection[] =
    bundle.hearings && bundle.hearings.length > 0
      ? bundle.hearings
      : bundle.hearing
        ? [
            {
              hearing: bundle.hearing,
              testifiers: bundle.testifiers,
              transcript: bundle.transcript,
              organizations: bundle.organizations,
            },
          ]
        : [];

  return (
    <article className="space-y-12">
      <BillSnapshot bill={bundle.bill} />

      <StatusTimeline status={bundle.status} />

      {sections.length > 0 ? (
        <section aria-labelledby="hearings-heading" className="space-y-4">
          <h2
            id="hearings-heading"
            className="text-xl font-semibold text-stone-900"
          >
            Hearings
          </h2>
          <ul className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            {sections.map((s) => (
              <HearingSummaryCard
                key={s.hearing.csi_agenda_item_id ?? s.hearing.meeting_datetime}
                section={s}
              />
            ))}
          </ul>
        </section>
      ) : null}
    </article>
  );
}

function HearingSummaryCard({ section }: { section: HearingSection }) {
  const { hearing, testifiers } = section;
  const testifiedCount = testifiers.filter((t) => t.testified).length;
  const href = hearing.hearing_id ? `/hearings/${hearing.hearing_id}` : null;
  const card = (
    <div className="flex h-full flex-col gap-2 rounded-lg border border-stone-300 bg-white p-4 transition hover:border-stone-500 hover:shadow-sm">
      <div className="flex items-start justify-between gap-3">
        <span className="font-semibold text-stone-900">
          {hearing.committee_name}
        </span>
        {hearing.committee_acronym ? (
          <span className="rounded bg-stone-100 px-1.5 py-0.5 text-xs uppercase tracking-wider text-stone-600">
            {hearing.committee_acronym}
          </span>
        ) : null}
      </div>
      <div className="text-sm text-stone-600 tabular-nums">
        {formatDateTime(hearing.meeting_datetime)}
      </div>
      {hearing.agenda_item_label ? (
        <div className="text-sm text-stone-700">{hearing.agenda_item_label}</div>
      ) : null}
      <div className="mt-auto flex flex-wrap gap-x-4 gap-y-1 text-xs text-stone-500">
        <span>{testifiers.length.toLocaleString()} signed in</span>
        <span>{testifiedCount.toLocaleString()} testified</span>
      </div>
    </div>
  );

  return (
    <li>
      {href ? (
        <Link href={href} className="block h-full text-stone-900">
          {card}
        </Link>
      ) : (
        card
      )}
    </li>
  );
}

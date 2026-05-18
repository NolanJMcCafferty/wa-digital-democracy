import Link from "next/link";
import { notFound } from "next/navigation";
import { loadHearingBundle, listHearingBundles } from "@/lib/loadBundle";
import { formatDateTime, formatMS } from "@/lib/format";
import { HearingCard } from "../../bills/[biennium]/[billNumber]/_sections/HearingCard";
import { TestifierTable } from "../../bills/[biennium]/[billNumber]/_sections/TestifierTable";
import { TranscriptSection } from "../../bills/[biennium]/[billNumber]/_sections/TranscriptSection";
import { OrganizationsSection } from "../../bills/[biennium]/[billNumber]/_sections/OrganizationsSection";
import { SourcePanel } from "../../bills/[biennium]/[billNumber]/_sections/SourcePanel";

type Params = { csiAgendaItemId: string };

export async function generateStaticParams() {
  const hearings = await listHearingBundles();
  return hearings.map((h) => ({ csiAgendaItemId: h.csiAgendaItemId }));
}

export default async function HearingPage({
  params,
}: {
  params: Promise<Params>;
}) {
  const { csiAgendaItemId } = await params;
  const bundle = await loadHearingBundle(csiAgendaItemId);
  if (!bundle) notFound();

  const testified = bundle.testifiers.filter((t) => t.testified).length;
  const registeredOnly = bundle.testifiers.length - testified;
  const segments = bundle.transcript.segments ?? [];
  const billSlug = bundle.bill.bill_id.replace(/\s+/g, "");

  return (
    <article className="space-y-12">
      <section className="space-y-5">
        <div className="space-y-2">
          <p className="text-sm uppercase tracking-wider text-stone-500">
            Hearing brief
          </p>
          <h1 className="text-3xl font-bold tracking-tight text-stone-900">
            {bundle.hearing.agenda_item_label || bundle.bill.title || bundle.bill.bill_id}
          </h1>
          <p className="text-stone-600">
            {bundle.hearing.committee_name} · {formatDateTime(bundle.hearing.meeting_datetime)}
          </p>
        </div>

        <div className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
          <Metric label="Bill" value={bundle.bill.bill_id} />
          <Metric label="Signed in" value={bundle.testifiers.length.toLocaleString()} />
          <Metric label="Testified" value={testified.toLocaleString()} />
          <Metric label="Transcript" value={`${segments.length.toLocaleString()} excerpts`} />
        </div>

        <div className="flex flex-wrap gap-3 text-sm">
          <Link
            href={`/bills/${bundle.bill.biennium}/${billSlug}`}
            className="rounded border border-stone-300 bg-white px-3 py-1.5 text-stone-800 hover:bg-stone-50"
          >
            View bill page →
          </Link>
          {bundle.hearing.tvw_url ? (
            <a
              href={bundle.hearing.tvw_url}
              target="_blank"
              rel="noreferrer"
              className="rounded border border-stone-300 bg-white px-3 py-1.5 text-stone-800 hover:bg-stone-50"
            >
              Watch on TVW →
            </a>
          ) : null}
        </div>
      </section>

      <HearingCard hearing={bundle.hearing} />

      <section aria-labelledby="agenda-heading" className="space-y-4 rounded-lg border border-stone-300 bg-white p-6">
        <h2 id="agenda-heading" className="text-xl font-semibold text-stone-900">
          Agenda item at a glance
        </h2>
        <dl className="grid grid-cols-1 gap-x-6 gap-y-2 text-sm sm:grid-cols-[max-content_1fr]">
          <dt className="text-stone-500">Bill</dt>
          <dd>
            <Link
              href={`/bills/${bundle.bill.biennium}/${billSlug}`}
              className="text-blue-700 underline hover:text-blue-900"
            >
              {bundle.bill.bill_id} — {bundle.bill.title}
            </Link>
          </dd>
          <dt className="text-stone-500">CSI agenda item</dt>
          <dd className="font-mono text-stone-800">{bundle.hearing.csi_agenda_item_id}</dd>
          <dt className="text-stone-500">Position records</dt>
          <dd className="text-stone-800">
            {bundle.testifiers.length.toLocaleString()} signed in; {testified.toLocaleString()} testified; {registeredOnly.toLocaleString()} registered a position without testifying.
          </dd>
          {segments.length > 0 ? (
            <>
              <dt className="text-stone-500">Matched transcript window</dt>
              <dd className="text-stone-800 tabular-nums">
                {formatMS(bundle.transcript.bill_segment_start_ms ?? 0)} – {formatMS(bundle.transcript.bill_segment_end_ms ?? 0)}
              </dd>
            </>
          ) : null}
        </dl>
      </section>

      <TestifierTable testifiers={bundle.testifiers} />

      <TranscriptSection
        transcript={bundle.transcript}
        tvwEventId={bundle.hearing.tvw_event_id ?? ""}
      />

      <OrganizationsSection organizations={bundle.organizations} />

      <SourcePanel
        sources={bundle.sources}
        knownLimitations={bundle.known_limitations}
        generatedAt={bundle.generated_at}
      />
    </article>
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

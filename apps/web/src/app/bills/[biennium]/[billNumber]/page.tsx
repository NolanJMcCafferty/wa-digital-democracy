import { notFound } from "next/navigation";
import { loadBundle } from "@/lib/loadBundle";
import { BillSnapshot } from "./_sections/BillSnapshot";
import { StatusTimeline } from "./_sections/StatusTimeline";
import { HearingCard } from "./_sections/HearingCard";
import { TestifierTable } from "./_sections/TestifierTable";
import { TranscriptSection } from "./_sections/TranscriptSection";
import { OrganizationsSection } from "./_sections/OrganizationsSection";

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

  // Prefer the per-hearing list returned by the API; fall back to the
  // legacy single-hearing fields for older bundles.
  const sections =
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

      {sections.map((s) => (
        <section
          key={s.hearing.csi_agenda_item_id ?? s.hearing.meeting_datetime}
          className="space-y-12"
        >
          <HearingCard hearing={s.hearing} />
          <TestifierTable testifiers={s.testifiers} />
          <TranscriptSection
            transcript={s.transcript ?? {}}
            tvwEventId={s.hearing.tvw_event_id ?? ""}
          />
          <OrganizationsSection organizations={s.organizations} />
        </section>
      ))}
    </article>
  );
}

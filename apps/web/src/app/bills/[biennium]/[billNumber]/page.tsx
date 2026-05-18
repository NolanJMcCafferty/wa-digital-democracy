import { notFound } from "next/navigation";
import { loadBundle } from "@/lib/loadBundle";
import { BillSnapshot } from "./_sections/BillSnapshot";
import { StatusTimeline } from "./_sections/StatusTimeline";
import { HearingCard } from "./_sections/HearingCard";
import { TestifierTable } from "./_sections/TestifierTable";
import { TranscriptSection } from "./_sections/TranscriptSection";
import { OrganizationsSection } from "./_sections/OrganizationsSection";
import { SourcePanel } from "./_sections/SourcePanel";

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

  const hearing = bundle.hearing;
  return (
    <article className="space-y-12">
      <BillSnapshot bill={bundle.bill} />

      <StatusTimeline status={bundle.status} />

      {hearing ? (
        <>
          <HearingCard hearing={hearing} />
          <TestifierTable testifiers={bundle.testifiers} />
          <TranscriptSection
            transcript={bundle.transcript ?? {}}
            tvwEventId={hearing.tvw_event_id ?? ""}
          />
          <OrganizationsSection organizations={bundle.organizations} />
        </>
      ) : null}

      <SourcePanel
        sources={bundle.sources}
        knownLimitations={bundle.known_limitations}
        generatedAt={bundle.generated_at}
      />
    </article>
  );
}

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

  return (
    <article className="space-y-12">
      {/* Section 1 + 2: bill snapshot + plain-English summary share a card. */}
      <BillSnapshot bill={bundle.bill} />

      {/* Section 3: status timeline. */}
      <StatusTimeline status={bundle.status} />

      {/* Section 4: hearing card. */}
      <HearingCard hearing={bundle.hearing} />

      {/* Section 5: "What happened?" — for v1 we lean on the transcript section. */}

      {/* Section 6: testifier list grouped Pro/Con/Other. */}
      <TestifierTable testifiers={bundle.testifiers} />

      {/* Section 7: transcript excerpts with TVW deep links. */}
      <TranscriptSection
        transcript={bundle.transcript}
        tvwEventId={bundle.hearing.tvw_event_id ?? ""}
      />

      {/* Section 8 + 9: organizations + money/lobbying context. */}
      <OrganizationsSection organizations={bundle.organizations} />

      {/* Section 10: source + confidence panel. Always last. */}
      <SourcePanel
        sources={bundle.sources}
        knownLimitations={bundle.known_limitations}
        generatedAt={bundle.generated_at}
      />
    </article>
  );
}

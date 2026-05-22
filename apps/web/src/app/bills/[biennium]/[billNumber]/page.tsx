import { notFound } from "next/navigation";
import { loadBillPage, type HearingPage } from "@/lib/api";
import type { HearingSection } from "@/lib/pageTypes";
import { BillSnapshot } from "./_sections/BillSnapshot";
import { StatusTimeline } from "./_sections/StatusTimeline";
import {
  HearingSearchResults,
  parseHearingFilters,
  type RawHearingSearchParams,
} from "@/app/hearings/HearingSearchResults";
import { filterHearings } from "@/app/hearings/filterHearings";

export const dynamic = "force-dynamic";

type Params = { biennium: string; billNumber: string };

// Parses route's [billNumber] like "HB1501" into ("HB", 1501).
function parseBillSlug(slug: string): { prefix: string; number: number } | null {
  const m = slug.match(/^([A-Z]+)([0-9]+)$/i);
  if (!m) return null;
  return { prefix: m[1].toUpperCase(), number: parseInt(m[2], 10) };
}

export default async function BillHearingPage({
  params,
  searchParams,
}: {
  params: Promise<Params>;
  searchParams: Promise<RawHearingSearchParams>;
}) {
  const { biennium, billNumber: slug } = await params;
  const parsed = parseBillSlug(slug);
  if (!parsed) notFound();

  const page = await loadBillPage(biennium, parsed.prefix, parsed.number);
  if (!page) notFound();

  const allHearings = billHearingsToHearings(page.bill, page.hearings);
  const filters = parseHearingFilters(await searchParams);
  const { hearings, total, offset, facets } = filterHearings(allHearings, filters);
  const basePath = `/bills/${biennium}/${slug}`;

  return (
    <article className="space-y-12">
      <BillSnapshot bill={page.bill} />

      <StatusTimeline status={page.status} />

      {allHearings.length > 0 ? (
        <section aria-labelledby="hearings-heading" className="space-y-4">
          <h2
            id="hearings-heading"
            className="text-xl font-semibold text-stone-900"
          >
            Hearings
          </h2>
          <HearingSearchResults
            basePath={basePath}
            filters={filters}
            hearings={hearings}
            total={total}
            offset={offset}
            facets={facets}
            hiddenFilters={["bill", "speaker", "biennium", "topic"]}
          />
        </section>
      ) : null}
    </article>
  );
}

function billHearingsToHearings(
  bill: { biennium: string; bill_id: string },
  sections: HearingSection[],
): HearingPage[] {
  const billPrefixMatch = bill.bill_id.match(/^([A-Z]+)([0-9]+)$/i);
  const billPrefix = billPrefixMatch ? billPrefixMatch[1].toUpperCase() : "";
  const billNumber = billPrefixMatch ? parseInt(billPrefixMatch[2], 10) : 0;
  return sections.map((s) => {
    const { hearing, testifiers } = s;
    const date = new Date(hearing.meeting_datetime);
    const title = `${hearing.committee_name} · ${date.toLocaleDateString("en-US", {
      month: "short",
      day: "numeric",
      year: "numeric",
    })}`;
    return {
      hearingId: hearing.hearing_id ?? 0,
      title,
      committeeName: hearing.committee_name,
      chamber: hearing.chamber,
      meetingDatetime: hearing.meeting_datetime,
      agendaItems: [
        {
          csiAgendaItemId: hearing.csi_agenda_item_id ?? "",
          agendaItemLabel: hearing.agenda_item_label ?? "",
          biennium: bill.biennium,
          billId: bill.bill_id,
          billPrefix,
          billNumber,
          testifierCount: testifiers.length,
          testifiedCount: testifiers.filter((t) => t.testified).length,
        },
      ],
    };
  });
}

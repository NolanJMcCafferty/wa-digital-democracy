import { searchHearings, type HearingSearchResult } from "@/lib/api";
import {
  HearingSearchResults,
  hearingFiltersToSearch,
  parseHearingFilters,
  type RawHearingSearchParams,
} from "./HearingSearchResults";

export default async function HearingsPage({
  searchParams,
}: {
  searchParams: Promise<RawHearingSearchParams>;
}) {
  const raw = await searchParams;
  const filters = parseHearingFilters(raw);
  const result: HearingSearchResult = await searchHearings(hearingFiltersToSearch(filters));

  return (
    <div className="space-y-6">
      <div className="space-y-2">
        <h1 className="text-3xl font-bold tracking-tight text-stone-900">Hearings</h1>
        <p className="text-stone-600">
          Search the {result.total.toLocaleString()} committee hearings
          currently tracked. Each row links to a detail page with
          testimony, transcript excerpts, video, and source records.
        </p>
      </div>

      <HearingSearchResults
        basePath="/hearings"
        filters={filters}
        hearings={result.hearings}
        total={result.total}
        offset={result.offset}
        facets={result.facets}
      />
    </div>
  );
}

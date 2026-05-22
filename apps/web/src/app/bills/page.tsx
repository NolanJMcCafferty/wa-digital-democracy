import { searchBills, type BillSearchResult } from "@/lib/api";
import {
  BillSearchResults,
  parseBillFilters,
  type RawBillSearchParams,
} from "./BillSearchResults";

export const dynamic = "force-dynamic";

export default async function BillsPage({
  searchParams,
}: {
  searchParams: Promise<RawBillSearchParams>;
}) {
  const raw = await searchParams;
  const filters = parseBillFilters(raw);
  const result: BillSearchResult = await searchBills(filters);

  return (
    <div className="space-y-6">
      <div className="space-y-2">
        <h1 className="text-3xl font-bold tracking-tight text-stone-900">Bills</h1>
        <p className="text-stone-600">
          Search the {(result.total).toLocaleString()} bills currently tracked
          for the active Washington biennium. Each row links to a
          detail page with sponsors, status timeline, and any
          available hearings.
        </p>
      </div>

      <BillSearchResults
        basePath="/bills"
        filters={filters}
        bills={result.bills}
        total={result.total}
        offset={result.offset}
        facets={result.facets}
      />
    </div>
  );
}

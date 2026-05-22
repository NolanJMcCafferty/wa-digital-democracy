import { notFound } from "next/navigation";
import {
  listLegislators,
  loadLegislatorPage,
  searchBills,
} from "@/lib/api";
import {
  BillSearchResults,
  parseBillFilters,
  type RawBillSearchParams,
} from "../../bills/BillSearchResults";

export const dynamic = "force-dynamic";

export async function generateStaticParams() {
  if (process.env.SKIP_BUILD_STATIC_PARAMS === "1") return [];
  const legislators = await listLegislators();
  return legislators.map((l) => ({ slug: l.slug }));
}

export default async function LegislatorPage({
  params,
  searchParams,
}: {
  params: Promise<{ slug: string }>;
  searchParams: Promise<RawBillSearchParams>;
}) {
  const { slug } = await params;
  const raw = await searchParams;
  const filters = parseBillFilters(raw);
  const legislator = await loadLegislatorPage(slug);
  if (!legislator) notFound();
  const sponsoredBillResult = await searchBills({ ...filters, sponsor: slug });

  const primary = legislator.appearances.filter(
    (a) => a.sponsorType === "Primary"
  ).length;
  const secondary = legislator.appearances.length - primary;
  const displayName = legislator.displayName ?? legislator.name;
  const subtitle = [
    legislator.chamber ?? "Chamber unknown",
    legislator.district ? `District ${legislator.district}` : "",
    legislator.party ?? "",
  ].filter(Boolean).join(" · ");

  return (
    <article className="space-y-10">
      <section className="flex flex-col gap-5 sm:flex-row sm:items-start">
        <div className="flex h-44 w-36 shrink-0 items-center justify-center overflow-hidden rounded border border-stone-300 bg-stone-100">
          {legislator.photoUrl || legislator.thumbnailUrl ? (
            <img
              src={legislator.photoUrl ?? legislator.thumbnailUrl}
              alt=""
              className="h-full w-full object-contain object-top"
            />
          ) : (
            <span className="text-3xl font-semibold text-stone-500">
              {legislatorInitials(displayName)}
            </span>
          )}
        </div>
        <div className="flex flex-1 flex-col gap-4">
          <div className="space-y-3">
            <p className="text-sm uppercase tracking-wider text-stone-500">
              Legislator
            </p>
            <h1 className="text-3xl font-bold tracking-tight text-stone-900">
              {displayName}
            </h1>
            {subtitle ? <p className="text-stone-600">{subtitle}</p> : null}
          </div>
          <div className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-3">
            <Metric label="Sponsored bills" value={legislator.appearances.length.toLocaleString()} />
            <Metric label="Primary" value={primary.toLocaleString()} />
            <Metric label="Secondary" value={secondary.toLocaleString()} />
          </div>
        </div>
      </section>

      <section aria-labelledby="sponsored-bills" className="space-y-4">
        <h2 id="sponsored-bills" className="text-xl font-semibold text-stone-900">
          Sponsored bills
        </h2>
        <BillSearchResults
          basePath={`/legislators/${slug}`}
          filters={filters}
          bills={sponsoredBillResult.bills}
          total={sponsoredBillResult.total}
          offset={sponsoredBillResult.offset}
          facets={sponsoredBillResult.facets}
          hiddenFilters={["chamber", "party"]}
          leadSponsorOptions={[{ value: slug, label: displayName }]}
        />
      </section>
    </article>
  );
}

function legislatorInitials(displayName: string): string {
  return displayName
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase() ?? "")
    .join("");
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded border border-stone-300 bg-stone-50 p-3">
      <div className="text-xs uppercase tracking-wider text-stone-500">{label}</div>
      <div className="mt-1 font-semibold text-stone-900">{value}</div>
    </div>
  );
}

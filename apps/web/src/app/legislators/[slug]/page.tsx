import Link from "next/link";
import { notFound } from "next/navigation";
import { listLegislatorBundles, loadLegislatorBundle } from "@/lib/loadBundle";
import { formatDateTime } from "@/lib/format";

export async function generateStaticParams() {
  const legislators = await listLegislatorBundles();
  return legislators.map((l) => ({ slug: l.slug }));
}

export default async function LegislatorPage({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  const legislator = await loadLegislatorBundle(slug);
  if (!legislator) notFound();

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
        <div className="space-y-4">
          <p className="text-sm uppercase tracking-wider text-stone-500">
            Legislator
          </p>
          <div className="space-y-3">
            <h1 className="text-3xl font-bold tracking-tight text-stone-900">
              {displayName}
            </h1>
            {subtitle ? <p className="text-stone-600">{subtitle}</p> : null}
          </div>
        </div>
      </section>

      <section className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
        <Metric label="Sponsored bills" value={legislator.appearances.length.toLocaleString()} />
        <Metric label="Primary" value={primary.toLocaleString()} />
        <Metric label="Secondary" value={secondary.toLocaleString()} />
        <Metric label="Chamber" value={legislator.chamber ?? "Unknown"} />
      </section>

      <section aria-labelledby="sponsored-bills" className="space-y-4">
        <h2 id="sponsored-bills" className="text-xl font-semibold text-stone-900">
          Sponsored bills
        </h2>
        <ul className="divide-y divide-stone-300 rounded border border-stone-300 bg-white">
          {legislator.appearances.map((a) => {
            const billSlug = `${a.billPrefix}${a.billNumber}`;
            return (
              <li key={`${a.biennium}-${a.billId}`} className="p-4">
                <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                  <div className="space-y-1">
                    <Link
                      href={`/bills/${a.biennium}/${billSlug}`}
                      className="font-medium text-blue-700 underline hover:text-blue-900"
                    >
                      {a.billId} — {a.billTitle}
                    </Link>
                    <p className="text-sm text-stone-600">
                      {a.sponsorType ?? "Sponsor"}
                      {a.meetingDatetime ? ` · hearing ${formatDateTime(a.meetingDatetime)}` : ""}
                    </p>
                    {a.hearingTitle ? (
                      <p className="text-xs text-stone-500">{a.hearingTitle}</p>
                    ) : null}
                  </div>
                  {a.csiAgendaItemId ? (
                    <Link
                      href={`/hearings/${a.csiAgendaItemId}`}
                      className="text-sm text-blue-700 underline hover:text-blue-900"
                    >
                      Hearing page →
                    </Link>
                  ) : null}
                </div>
              </li>
            );
          })}
        </ul>
      </section>

      <section className="rounded-lg border border-stone-300 bg-stone-50 p-5 text-sm text-stone-600">
        <h2 className="mb-2 font-semibold text-stone-900">Data caveat</h2>
        <p>
          This page is intentionally narrow for now: it starts with official sponsorship records. District, party, committee assignments, vote history, campaign-finance profiles, and full legislator identity matching are not populated yet.
        </p>
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

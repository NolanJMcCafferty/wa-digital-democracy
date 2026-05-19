import Link from "next/link";
import type { Bill, Sponsor } from "@/lib/bundle";
import { legislatorSlug } from "@/lib/loadBundle";

export function BillSnapshot({ bill }: { bill: Bill }) {
  const sponsors = bill.sponsors ?? [];
  const primarySponsors = sponsors.filter((s) => s.sponsor_type === "Primary");
  const otherSponsors = sponsors.filter((s) => s.sponsor_type !== "Primary");

  return (
    <section aria-labelledby="bill-heading" className="space-y-4">
      <div className="flex flex-wrap items-baseline gap-x-4 gap-y-2">
        <h1
          id="bill-heading"
          className="text-3xl font-bold tracking-tight text-stone-900"
        >
          {bill.bill_id}
        </h1>
        <span className="text-sm text-stone-500">
          Biennium {bill.biennium}
          {bill.chamber_origin ? ` · ${bill.chamber_origin} of origin` : ""}
        </span>
      </div>

      {bill.title ? (
        <h2 className="text-xl font-semibold text-stone-800">{bill.title}</h2>
      ) : null}

      {bill.description ? (
        <p className="text-stone-700 leading-relaxed">{bill.description}</p>
      ) : null}

      {sponsors.length > 0 ? (
        <div className="space-y-3">
          <span className="block text-sm font-medium text-stone-700">Sponsors</span>
          {primarySponsors.length > 0 ? (
            <div className="flex flex-wrap gap-3">
              {primarySponsors.map((s) => (
                <SponsorCard
                  key={`${s.name}-${s.sponsor_type}-${s.chamber}`}
                  sponsor={s}
                  variant="primary"
                />
              ))}
            </div>
          ) : null}
          {otherSponsors.length > 0 ? (
            <div className="flex flex-wrap gap-2">
              {otherSponsors.map((s) => (
                <SponsorCard
                  key={`${s.name}-${s.sponsor_type}-${s.chamber}`}
                  sponsor={s}
                  variant="secondary"
                />
              ))}
            </div>
          ) : null}
        </div>
      ) : null}

      {bill.official_url ? (
        <p className="text-sm">
          <a
            className="text-blue-700 underline hover:text-blue-900"
            href={bill.official_url}
            target="_blank"
            rel="noreferrer"
          >
            Official bill summary on app.leg.wa.gov →
          </a>
        </p>
      ) : null}
    </section>
  );
}

function SponsorCard({
  sponsor,
  variant,
}: {
  sponsor: Sponsor;
  variant: "primary" | "secondary";
}) {
  const imageUrl =
    variant === "primary"
      ? sponsor.photo_url ?? sponsor.thumbnail_url
      : sponsor.thumbnail_url ?? sponsor.photo_url;
  const isPrimary = variant === "primary";

  return (
    <Link
      href={`/legislators/${legislatorSlug(sponsor)}`}
      className={
        isPrimary
          ? "flex min-w-0 items-center gap-3 rounded border border-stone-300 bg-white p-3 text-stone-900 shadow-sm transition hover:border-stone-500 hover:shadow"
          : "flex min-w-0 items-center gap-2 rounded border border-stone-300 bg-white px-2.5 py-2 text-stone-800 transition hover:border-stone-500 hover:shadow-sm"
      }
    >
      <span
        className={
          isPrimary
            ? "flex h-20 w-16 shrink-0 items-center justify-center overflow-hidden rounded-sm bg-stone-100"
            : "flex h-12 w-10 shrink-0 items-center justify-center overflow-hidden rounded-sm bg-stone-100"
        }
      >
        {imageUrl ? (
          <img
            src={imageUrl}
            alt=""
            loading="lazy"
            className="h-full w-full object-contain object-top"
          />
        ) : (
          <span
            className={
              isPrimary
                ? "text-lg font-semibold text-stone-500"
                : "text-xs font-semibold text-stone-500"
            }
          >
            {sponsorInitials(sponsor.name)}
          </span>
        )}
      </span>
      <span className="min-w-0">
        <span
          className={
            isPrimary
              ? "block font-semibold leading-tight"
              : "block text-sm font-medium leading-tight"
          }
        >
          {sponsor.name}
        </span>
        {isPrimary ? (
          <span className="mt-1 inline-block rounded bg-stone-200 px-1.5 py-0.5 text-xs font-medium uppercase tracking-wider text-stone-700">
            Primary
          </span>
        ) : sponsor.sponsor_type ? (
          <span className="mt-0.5 block text-xs text-stone-500">
            {sponsor.sponsor_type}
          </span>
        ) : null}
      </span>
    </Link>
  );
}

function sponsorInitials(name: string): string {
  return name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase() ?? "")
    .join("");
}

import Link from "next/link";
import { FindLegislators } from "./FindLegislators";
import { SearchBox } from "./SearchBox";
import { buildSearchIndex } from "@/lib/searchIndex";
import {
  countBills,
  countHearings,
  listLegislators,
  listOrganizations,
} from "@/lib/api";
// Issue tiles. Live issues link to /issues/{slug}; "coming soon"
// placeholders read as a roadmap. Pattern mirrors CalMatters' "California
// Agenda" issue row. To promote a placeholder, update its href + status
// once the corresponding /issues/<slug>/page.tsx exists.
const ISSUE_TILES: Array<{
  slug: string;
  title: string;
  emoji: string;
  href: string;
  status: "live" | "planned";
}> = [
  { slug: "health", title: "Health", emoji: "🏥", href: "/issues/health", status: "live" },
  { slug: "housing", title: "Housing", emoji: "🏠", href: "/issues/housing", status: "live" },
  { slug: "transportation", title: "Transportation", emoji: "🚆", href: "/issues/transportation", status: "live" },
  { slug: "education", title: "Education", emoji: "🎓", href: "/issues/education", status: "live" },
  { slug: "climate", title: "Climate", emoji: "🌲", href: "/issues/climate", status: "live" },
  { slug: "public-safety", title: "Public Safety", emoji: "⚖️", href: "/issues/public-safety", status: "live" },
];

export default async function HomePage() {
  const [billCount, hearingCount, organizations, legislators, searchResults] = await Promise.all([
    countBills(),
    countHearings(),
    listOrganizations(),
    listLegislators(),
    buildSearchIndex(),
  ]);

  return (
    <div className="space-y-12">
      {/* Hero */}
      <section className="space-y-6 rounded-lg border border-stone-300 bg-white p-8 sm:p-10">
        <div className="space-y-3">
          <h1 className="text-4xl font-bold tracking-tight text-stone-900 sm:text-5xl">
            WA Digital Democracy
          </h1>
          <p className="max-w-3xl text-lg text-stone-700">
            Technology that reveals how decisions are made in Washington.
          </p>
        </div>
        <SearchBox results={searchResults} />
      </section>

      {/* Issue tiles */}
      <section aria-labelledby="issues-heading" className="space-y-4">
        <div className="flex items-baseline justify-between gap-4">
          <h2 id="issues-heading" className="text-2xl font-bold text-stone-900">
            Washington Agenda
          </h2>
          <span className="text-xs uppercase tracking-wider text-stone-500">
            {ISSUE_TILES.filter((i) => i.status === "live").length} of{" "}
            {ISSUE_TILES.length} live
          </span>
        </div>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
          {ISSUE_TILES.map((issue) => {
            const isLive = issue.status === "live";
            const Wrapper = ({ children }: { children: React.ReactNode }) =>
              isLive ? (
                <Link href={issue.href} className="block h-full">
                  {children}
                </Link>
              ) : (
                <div className="block h-full opacity-60">{children}</div>
              );
            return (
              <Wrapper key={issue.slug}>
                <div
                  className={`flex h-full flex-col items-center gap-2 rounded-lg border bg-white p-4 text-center transition ${
                    isLive
                      ? "border-stone-300 hover:border-stone-500 hover:shadow-sm"
                      : "border-dashed border-stone-300"
                  }`}
                >
                  <span className="text-3xl" aria-hidden>
                    {issue.emoji}
                  </span>
                  <span className="font-semibold text-stone-900">{issue.title}</span>
                  {!isLive ? (
                    <span className="text-xs uppercase tracking-wider text-stone-500">
                      Coming soon
                    </span>
                  ) : null}
                </div>
              </Wrapper>
            );
          })}
        </div>
      </section>

      <FindLegislators legislatorCount={legislators.length} />

      {/* Legislator mosaic */}
      <LegislatorMosaic legislators={legislators} />

      {/* Section cards */}
      <section aria-labelledby="explore-heading" className="space-y-4">
        <h2 id="explore-heading" className="text-2xl font-bold text-stone-900">
          Explore the public record
        </h2>
        <div className="grid gap-4 sm:grid-cols-2">
          <SectionCard
            title="Bills"
            count={billCount}
            href="/bills"
            description="Every bill in the active biennium with sponsors, status timeline, and — where data exists — hearing video, testimony, and transcripts."
          />
          <SectionCard
            title="Hearings"
            count={hearingCount}
            href="/hearings"
            description="Committee hearings with testifier sign-ins, video, and transcript excerpts mapped to the bill on the agenda."
          />
          <SectionCard
            title="Organizations"
            count={organizations.length}
            href="/organizations"
            description="Reviewed organization matches with their public-record context — lobbying registrations, campaign contributions, testimony positions."
          />
          <SectionCard
            title="Legislators"
            count={legislators.length}
            href="/legislators"
            description="Every state senator and representative with their bills, sponsorships, committee assignments, and votes."
          />
        </div>
      </section>
    </div>
  );
}

function SectionCard({
  title,
  count,
  href,
  description,
}: {
  title: string;
  count: number | null;
  href: string;
  description: string;
}) {
  return (
    <Link
      href={href}
      className="group flex flex-col gap-2 rounded-lg border border-stone-300 bg-white p-5 transition hover:border-stone-500 hover:shadow-sm"
    >
      <div className="flex items-baseline justify-between gap-3">
        <h3 className="text-lg font-semibold text-stone-900 group-hover:underline">
          {title}
        </h3>
        {count !== null ? (
          <span className="text-sm font-medium text-stone-500">
            {count.toLocaleString()}
          </span>
        ) : null}
      </div>
      <p className="text-sm text-stone-600">{description}</p>
    </Link>
  );
}

function LegislatorMosaic({
  legislators,
}: {
  legislators: Awaited<ReturnType<typeof listLegislators>>;
}) {
  if (legislators.length === 0) return null;
  const senate = legislators.filter((l) => l.chamber === "Senate").length;
  const house = legislators.filter((l) => l.chamber === "House").length;
  return (
    <section aria-labelledby="mosaic-heading" className="space-y-3">
      <div className="flex items-baseline justify-between gap-4">
        <h2 id="mosaic-heading" className="text-2xl font-bold text-stone-900">
          Legislature at a glance
        </h2>
        <span className="text-xs uppercase tracking-wider text-stone-500">
          {senate.toLocaleString()} Senate · {house.toLocaleString()} House
        </span>
      </div>
      <p className="text-sm text-stone-600">
        Every member of the Washington House and Senate for the active
        biennium, ingested directly from LWS. Click a tile to see their
        sponsored bills.
      </p>
      <div className="grid grid-cols-3 gap-2 sm:grid-cols-4 md:grid-cols-6 lg:grid-cols-8">
        {legislators.map((l) => {
          const fallbackColor =
            l.party === "D"
              ? "bg-blue-100"
              : l.party === "R"
                ? "bg-rose-100"
                : "bg-stone-200";
          const borderColor =
            l.party === "D"
              ? "border-blue-300"
              : l.party === "R"
                ? "border-rose-300"
                : "border-stone-300";
          const display = l.displayName ?? l.name;
          const tag = [
            l.party ?? "",
            l.district ? `D${l.district}` : "",
          ].filter(Boolean).join(" ");
          return (
            <Link
              key={legislatorRenderKey(l)}
              href={`/legislators/${l.slug}`}
              className={`group relative flex aspect-square overflow-hidden rounded border ${borderColor} ${fallbackColor} text-center text-xs leading-tight text-stone-800 transition hover:scale-105 hover:shadow-sm`}
              title={`${display}${tag ? " (" + tag + ")" : ""}`}
            >
              {l.thumbnailUrl ? (
                <img
                  src={l.thumbnailUrl}
                  alt=""
                  loading="lazy"
                  className="absolute inset-1.5 h-[calc(100%-0.75rem)] w-[calc(100%-0.75rem)] rounded-sm object-contain object-top transition duration-200 group-hover:scale-105"
                />
              ) : (
                <span className="m-auto text-lg font-semibold text-stone-700">
                  {legislatorInitials(display)}
                </span>
              )}
              <span className="absolute inset-x-0 bottom-0 bg-white px-1.5 py-1.5 shadow-sm">
                <span className="line-clamp-2 font-medium text-stone-900">{display}</span>
              </span>
            </Link>
          );
        })}
      </div>
    </section>
  );
}

function legislatorRenderKey(l: Awaited<ReturnType<typeof listLegislators>>[number]): string {
  return [
    l.slug,
    l.displayName ?? l.name,
    l.chamber ?? "",
    l.district ?? "",
  ].join("|");
}

function legislatorInitials(displayName: string | undefined): string {
  if (!displayName) return "";
  return displayName
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase() ?? "")
    .join("");
}

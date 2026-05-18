import Link from "next/link";
import { FindLegislators } from "./FindLegislators";
import { SearchBox } from "./SearchBox";
import { buildSearchIndex } from "@/lib/searchIndex";
import {
  listHearingBundles,
  listLegislatorBundles,
  listLocalBundles,
  listOrganizationBundles,
} from "@/lib/loadBundle";
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
  { slug: "housing", title: "Housing", emoji: "🏠", href: "/issues/housing", status: "live" },
  { slug: "transportation", title: "Transportation", emoji: "🚆", href: "/issues/transportation", status: "live" },
  { slug: "education", title: "Education", emoji: "🎓", href: "#", status: "planned" },
  { slug: "climate", title: "Climate", emoji: "🌲", href: "#", status: "planned" },
  { slug: "public-safety", title: "Public Safety", emoji: "⚖️", href: "#", status: "planned" },
];

export default async function HomePage() {
  const [bundles, hearings, organizations, legislators, searchResults] = await Promise.all([
    listLocalBundles(),
    listHearingBundles(),
    listOrganizationBundles(),
    listLegislatorBundles(),
    buildSearchIndex(),
  ]);

  return (
    <div className="space-y-12">
      {/* Hero */}
      <section className="space-y-6 rounded-lg border border-stone-300 bg-white p-8 sm:p-10">
        <div className="space-y-3">
          <p className="text-sm font-medium uppercase tracking-wider text-stone-500">
            A source-linked public graph of Washington State government
          </p>
          <h1 className="text-4xl font-bold tracking-tight text-stone-900 sm:text-5xl">
            WA Digital Democracy
          </h1>
          <p className="max-w-3xl text-lg text-stone-700">
            Technology that reveals how decisions are made in Washington.
            Search bills, hearings, testimony, transcripts, organizations,
            and legislators — every fact links back to its official source.
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
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
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

      {/* Leaderboard */}
      <Leaderboard legislators={legislators} />

      {/* Section cards */}
      <section aria-labelledby="explore-heading" className="space-y-4">
        <h2 id="explore-heading" className="text-2xl font-bold text-stone-900">
          Explore the public record
        </h2>
        <div className="grid gap-4 sm:grid-cols-2">
          <SectionCard
            title="Bills"
            count={bundles.length}
            href="/bills"
            description="Every bill in the active biennium with sponsors, status timeline, and — where data exists — hearing video, testimony, and transcripts."
          />
          <SectionCard
            title="Hearings"
            count={hearings.length}
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
            title="Sources"
            count={null}
            href="/sources"
            description="Every official feed and public dataset behind the public record. Active sources, expansion clients, and what powers each."
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
  legislators: Awaited<ReturnType<typeof listLegislatorBundles>>;
}) {
  if (legislators.length === 0) return null;
  // Stable color from the slug so the mosaic doesn't reshuffle each render.
  const palette = ["bg-stone-200", "bg-blue-100", "bg-emerald-100", "bg-rose-100", "bg-amber-100", "bg-purple-100"];
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
        {legislators.map((l, i) => {
          const color = palette[i % palette.length];
          const display = l.displayName ?? l.name;
          const tag = [
            l.party ?? "",
            l.district ? `D${l.district}` : "",
          ].filter(Boolean).join(" ");
          return (
            <Link
              key={l.slug}
              href={`/legislators/${l.slug}`}
              className={`${color} flex aspect-square flex-col items-center justify-center rounded p-2 text-center text-xs leading-tight text-stone-800 transition hover:scale-105 hover:shadow-sm`}
              title={`${display}${tag ? " (" + tag + ")" : ""}`}
            >
              <span className="line-clamp-3 font-medium">{display}</span>
              {tag ? (
                <span className="mt-1 text-[10px] uppercase tracking-wider text-stone-600">
                  {tag}
                </span>
              ) : l.chamber ? (
                <span className="mt-1 text-[10px] uppercase tracking-wider text-stone-600">
                  {l.chamber === "House" ? "Rep" : "Sen"}
                </span>
              ) : null}
            </Link>
          );
        })}
      </div>
    </section>
  );
}

function Leaderboard({
  legislators,
}: {
  legislators: Awaited<ReturnType<typeof listLegislatorBundles>>;
}) {
  // listLegislatorBundles returns LegislatorBundleEntry which doesn't carry
  // bill_count today (that field lives on the API list payload but isn't
  // surfaced to the frontend type). Re-fetch briefly via the API so we can
  // render the leaderboard. This keeps the home-page render server-side.
  return <LeaderboardServer legislators={legislators} />;
}

async function LeaderboardServer({
  legislators,
}: {
  legislators: Awaited<ReturnType<typeof listLegislatorBundles>>;
}) {
  const apiBase = process.env.WADD_API_URL ?? "http://localhost:8080";
  type ApiItem = {
    slug: string;
    name: string;
    display_name?: string;
    chamber?: string;
    district?: string;
    party?: string;
    bill_count: number;
  };
  let ranked: ApiItem[] = [];
  try {
    const res = await fetch(`${apiBase}/api/v1/legislators`, { cache: "no-store" });
    if (res.ok) {
      const all = (await res.json()) as ApiItem[];
      ranked = [...all].sort((a, b) => b.bill_count - a.bill_count).slice(0, 10);
    }
  } catch {
    // Fall back silently to the prebuilt list (no counts).
  }
  if (ranked.length === 0) {
    if (legislators.length === 0) return null;
    return null;
  }
  return (
    <section aria-labelledby="leaderboard-heading" className="space-y-3">
      <div className="flex items-baseline justify-between gap-4">
        <h2 id="leaderboard-heading" className="text-2xl font-bold text-stone-900">
          Top 10 sponsors
        </h2>
        <span className="text-xs uppercase tracking-wider text-stone-500">
          By sponsored-bill count
        </span>
      </div>
      <p className="text-sm text-stone-600">
        Legislators ranked by the number of bills they appear on as
        primary or secondary sponsor in the active biennium.
      </p>
      <ol className="divide-y divide-stone-200 rounded-lg border border-stone-300 bg-white">
        {ranked.map((l, i) => (
          <li key={l.slug}>
            <Link
              href={`/legislators/${l.slug}`}
              className="flex items-center gap-4 px-4 py-3 hover:bg-stone-50"
            >
              <span className="w-6 text-right font-mono text-sm text-stone-500">
                {i + 1}
              </span>
              <div className="min-w-0 flex-1">
                <div className="font-medium text-stone-900">
                  {l.display_name ?? l.name}
                </div>
                <div className="text-xs text-stone-500">
                  {[
                    l.party && l.district ? `${l.party}-${l.district}` : l.party,
                    l.chamber,
                  ]
                    .filter(Boolean)
                    .join(" · ") || "Chamber unknown"}
                </div>
              </div>
              <span className="font-mono text-sm font-medium text-stone-700">
                {l.bill_count.toLocaleString()}{" "}
                <span className="text-xs font-normal text-stone-500">bills</span>
              </span>
            </Link>
          </li>
        ))}
      </ol>
    </section>
  );
}

import Link from "next/link";
import {
  listHearingBundles,
  listLegislatorBundles,
  listLocalBundles,
  listOrganizationBundles,
} from "@/lib/loadBundle";

const ISSUE_PAGES = [
  {
    slug: "housing",
    title: "Housing",
    description:
      "Bills, hearings, testimony positions, organizations, and source coverage for the housing MVP slice.",
  },
];

export default async function HomePage() {
  const [bundles, hearings, organizations, legislators] = await Promise.all([
    listLocalBundles(),
    listHearingBundles(),
    listOrganizationBundles(),
    listLegislatorBundles(),
  ]);
  return (
    <div className="space-y-8">
      <div className="space-y-3">
        <h1 className="text-3xl font-bold tracking-tight">First-page demos</h1>
        <p className="text-stone-600">
          The MVP renders one source-linked bill-hearing brief at a time. Each
          bundle below was assembled by{" "}
          <code className="rounded bg-stone-200 px-1.5 py-0.5 text-sm">
            wa-dd build-bundle
          </code>{" "}
          from live LWS, CSI, TVW/Invintus, and PDC data.
        </p>
      </div>

      <section aria-labelledby="issues-heading" className="space-y-3">
        <div className="flex items-baseline justify-between gap-4">
          <h2 id="issues-heading" className="text-xl font-semibold text-stone-900">
            Issues
          </h2>
          <span className="text-xs uppercase tracking-wider text-stone-500">
            {ISSUE_PAGES.length} page{ISSUE_PAGES.length === 1 ? "" : "s"}
          </span>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          {ISSUE_PAGES.map((issue) => (
            <div
              key={issue.slug}
              className="rounded border border-stone-300 bg-white p-4"
            >
              <div className="flex items-baseline justify-between gap-4">
                <div>
                  <h3 className="font-semibold text-stone-900">{issue.title}</h3>
                  <p className="text-sm text-stone-600">{issue.description}</p>
                </div>
                <Link
                  href={`/issues/${issue.slug}`}
                  className="text-sm text-blue-700 underline hover:text-blue-900"
                >
                  View →
                </Link>
              </div>
            </div>
          ))}
        </div>
      </section>

      {organizations.length > 0 ? (
        <div className="rounded border border-stone-300 bg-white p-4">
          <div className="flex items-baseline justify-between gap-4">
            <div>
              <h2 className="font-semibold text-stone-900">Organizations</h2>
              <p className="text-sm text-stone-600">
                Browse reviewed organization matches and their source-linked
                public-record context.
              </p>
            </div>
            <Link
              href="/organizations"
              className="text-sm text-blue-700 underline hover:text-blue-900"
            >
              View organizations →
            </Link>
          </div>
        </div>
      ) : null}

      <div className="rounded border border-stone-300 bg-white p-4">
        <div className="flex items-baseline justify-between gap-4">
          <div>
            <h2 className="font-semibold text-stone-900">Sources</h2>
            <p className="text-sm text-stone-600">
              See the official feeds, public records, and planned expansion
              sources behind the prototype.
            </p>
          </div>
          <Link
            href="/sources"
            className="text-sm text-blue-700 underline hover:text-blue-900"
          >
            View sources →
          </Link>
        </div>
      </div>

      {legislators.length > 0 ? (
        <div className="rounded border border-stone-300 bg-white p-4">
          <div className="flex items-baseline justify-between gap-4">
            <div>
              <h2 className="font-semibold text-stone-900">Legislators</h2>
              <p className="text-sm text-stone-600">
                Browse sponsor pages generated from LWS sponsor records in the
                local bill bundles.
              </p>
            </div>
            <Link
              href="/legislators"
              className="text-sm text-blue-700 underline hover:text-blue-900"
            >
              View legislators →
            </Link>
          </div>
        </div>
      ) : null}

      {hearings.length > 0 ? (
        <div className="rounded border border-stone-300 bg-white p-4">
          <div className="flex items-baseline justify-between gap-4">
            <div>
              <h2 className="font-semibold text-stone-900">Hearings</h2>
              <p className="text-sm text-stone-600">
                Browse hearing-centered pages for committee testimony, video,
                transcript excerpts, and source records.
              </p>
            </div>
            <Link
              href="/hearings"
              className="text-sm text-blue-700 underline hover:text-blue-900"
            >
              View hearings →
            </Link>
          </div>
        </div>
      ) : null}

      {bundles.length === 0 ? (
        <p className="rounded border border-stone-300 bg-stone-50 p-4 text-sm text-stone-600">
          No bundles found. Run{" "}
          <code className="rounded bg-stone-200 px-1.5 py-0.5">
            wa-dd build-bundle
          </code>{" "}
          to produce one.
        </p>
      ) : (
        <ul className="divide-y divide-stone-300 rounded border border-stone-300 bg-white">
          {bundles.map((b) => (
            <li key={b.path} className="px-4 py-3">
              <Link
                href={`/bills/${b.biennium}/${b.billPrefix}${b.billNumber}`}
                className="flex flex-col gap-1 hover:bg-stone-50"
              >
                <span className="font-medium text-stone-900">
                  {b.billPrefix} {b.billNumber}
                </span>
                <span className="text-sm text-stone-500">
                  Biennium {b.biennium} · {b.path}
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

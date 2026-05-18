import Link from "next/link";
import { listHearingBundles, listLocalBundles } from "@/lib/loadBundle";

export default async function HomePage() {
  const [bundles, hearings] = await Promise.all([
    listLocalBundles(),
    listHearingBundles(),
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

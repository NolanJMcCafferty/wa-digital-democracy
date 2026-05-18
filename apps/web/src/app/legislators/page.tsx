import Link from "next/link";
import { listLegislatorBundles } from "@/lib/loadBundle";

export default async function LegislatorsPage() {
  const legislators = await listLegislatorBundles();

  return (
    <div className="space-y-8">
      <div className="space-y-3">
        <h1 className="text-3xl font-bold tracking-tight">Legislators</h1>
        <p className="max-w-3xl text-stone-600">
          Legislator pages are currently built from LWS sponsor records in the
          generated bill bundles. They show sponsorship appearances first; full
          district, committee, voting, and PDC profile data can be added as the
          identity layer expands.
        </p>
      </div>

      {legislators.length === 0 ? (
        <p className="rounded border border-stone-300 bg-stone-50 p-4 text-sm text-stone-600">
          No legislator sponsor records found. Generate a bill bundle first.
        </p>
      ) : (
        <ul className="divide-y divide-stone-300 rounded border border-stone-300 bg-white">
          {legislators.map((l) => (
            <li key={l.slug} className="p-4">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div className="space-y-1">
                  <Link
                    href={`/legislators/${l.slug}`}
                    className="font-medium text-blue-700 underline hover:text-blue-900"
                  >
                    {l.name}
                  </Link>
                  <p className="text-sm text-stone-600">
                    {l.chamber ?? "Chamber unknown"} ·{" "}
                    {l.appearances.length.toLocaleString()} sponsored bill
                    {l.appearances.length === 1 ? "" : "s"} in local bundles
                  </p>
                </div>
                <Link
                  href={`/legislators/${l.slug}`}
                  className="text-sm text-blue-700 underline hover:text-blue-900"
                >
                  View legislator →
                </Link>
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

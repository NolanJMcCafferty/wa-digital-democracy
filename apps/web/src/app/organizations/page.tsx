import Link from "next/link";
import { listOrganizationBundles } from "@/lib/loadBundle";

export default async function OrganizationsPage() {
  const organizations = await listOrganizationBundles();

  return (
    <div className="space-y-8">
      <div className="space-y-3">
        <h1 className="text-3xl font-bold tracking-tight">Organizations</h1>
        <p className="max-w-3xl text-stone-600">
          Reviewed organization/entity matches from public testimony sign-ins,
          connected to official lobbying and campaign-finance context where we
          have confident source-linked matches.
        </p>
      </div>

      {organizations.length === 0 ? (
        <p className="rounded border border-stone-300 bg-stone-50 p-4 text-sm text-stone-600">
          No reviewed organization matches found yet. Add reviewed matches and
          rebuild the local demo bundle.
        </p>
      ) : (
        <ul className="divide-y divide-stone-300 rounded border border-stone-300 bg-white">
          {organizations.map((o) => (
            <li key={o.slug} className="p-4">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div className="space-y-1">
                  <Link
                    href={`/organizations/${o.slug}`}
                    className="font-medium text-blue-700 underline hover:text-blue-900"
                  >
                    {o.canonicalName}
                  </Link>
                  {o.aliases.length > 0 ? (
                    <p className="text-xs text-stone-500">
                      Also known as: {o.aliases.join(", ")}
                    </p>
                  ) : null}
                  <p className="text-sm text-stone-600">
                    {o.testifierCount.toLocaleString()} linked testifier
                    {o.testifierCount === 1 ? "" : "s"} ·{" "}
                    {o.contextCount.toLocaleString()} PDC context record
                    {o.contextCount === 1 ? "" : "s"} · {o.matchConfidence}
                  </p>
                </div>
                <Link
                  href={`/organizations/${o.slug}`}
                  className="text-sm text-blue-700 underline hover:text-blue-900"
                >
                  View organization →
                </Link>
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

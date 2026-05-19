import { listOrganizationBundles } from "@/lib/loadBundle";
import {
  OrganizationSearchResults,
  parseOrganizationFilters,
} from "./OrganizationSearchResults";

type RawSearchParams = Record<string, string | string[] | undefined>;

export default async function OrganizationsPage({
  searchParams,
}: {
  searchParams: Promise<RawSearchParams>;
}) {
  const raw = await searchParams;
  const filters = parseOrganizationFilters(raw);
  const organizations = await listOrganizationBundles();

  return (
    <div className="space-y-6">
      <div className="space-y-2">
        <h1 className="text-3xl font-bold tracking-tight text-stone-900">Organizations</h1>
        <p className="max-w-3xl text-stone-600">
          Reviewed organization/entity matches from public testimony sign-ins,
          connected to official lobbying and campaign-finance context where we
          have confident source-linked matches.
        </p>
      </div>

      <OrganizationSearchResults
        basePath="/organizations"
        filters={filters}
        organizations={organizations}
      />
    </div>
  );
}

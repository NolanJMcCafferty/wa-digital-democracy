import Link from "next/link";
import type { Organization } from "@/lib/pageTypes";
import { slugify } from "@/lib/api";

export function OrganizationsSection({
  organizations,
}: {
  organizations: Organization[];
}) {
  return (
    <section aria-labelledby="orgs-heading" className="space-y-4">
      <h2 id="orgs-heading" className="text-xl font-semibold text-stone-900">
        Organizations
      </h2>

      {organizations.length === 0 ? (
        <p className="rounded border border-stone-300 bg-stone-50 p-4 text-sm text-stone-600">
          No organizations have been confidently linked to this hearing yet.
          No reviewed organization links are available for this hearing yet.
        </p>
      ) : (
        <ul className="space-y-4">
          {organizations.map((o) => (
            <li
              key={o.canonical_name}
              className="rounded-lg border border-stone-300 bg-white p-5"
            >
              <h3 className="mb-2 text-lg font-semibold text-stone-900">
                <Link
                  href={`/organizations/${slugify(o.canonical_name)}`}
                  className="text-blue-700 underline hover:text-blue-900"
                >
                  {o.canonical_name}
                </Link>
              </h3>

              {o.aliases && o.aliases.length > 0 ? (
                <p className="mb-2 text-xs text-stone-500">
                  Also known as: {o.aliases.join(", ")}
                </p>
              ) : null}

              {o.match_notes ? (
                <p className="mb-2 text-sm text-stone-600">{o.match_notes}</p>
              ) : null}

              {o.context_summary && o.context_summary.length > 0 ? (
                <div className="mb-3 flex flex-wrap gap-2">
                  {o.context_summary.map((label) => (
                    <span
                      key={label}
                      className="rounded border border-stone-300 bg-stone-50 px-2 py-0.5 text-xs font-medium text-stone-700"
                    >
                      {label}
                    </span>
                  ))}
                </div>
              ) : null}

              {typeof o.testifier_count === "number" &&
              o.testifier_count > 0 ? (
                <p className="mb-3 text-sm text-stone-700">
                  Linked to {o.testifier_count.toLocaleString()} testifier
                  {o.testifier_count === 1 ? "" : "s"} on this agenda item.
                </p>
              ) : null}

            </li>
          ))}
        </ul>
      )}

      <p className="text-xs text-stone-500">
        Organization matches come from testimony sign-ins and verified
        cross-source records where available.
      </p>
    </section>
  );
}

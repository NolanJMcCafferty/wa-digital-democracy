import type { Organization, OrgContext } from "@/lib/bundle";
import { confidenceLabel } from "@/lib/format";

export function OrganizationsSection({
  organizations,
}: {
  organizations: Organization[];
}) {
  return (
    <section aria-labelledby="orgs-heading" className="space-y-4">
      <h2 id="orgs-heading" className="text-xl font-semibold text-stone-900">
        Organizations & lobbying context
      </h2>

      {organizations.length === 0 ? (
        <p className="rounded border border-stone-300 bg-stone-50 p-4 text-sm text-stone-600">
          No organizations have been confidently linked to this hearing yet.
          The MVP uses operator-curated matches in{" "}
          <code className="rounded bg-stone-200 px-1 py-0.5">
            config/reviewed_matches.yml
          </code>
          .
        </p>
      ) : (
        <ul className="space-y-4">
          {organizations.map((o) => (
            <li
              key={o.canonical_name}
              className="rounded-lg border border-stone-300 bg-white p-5"
            >
              <div className="mb-2 flex items-baseline justify-between gap-3">
                <h3 className="text-lg font-semibold text-stone-900">
                  {o.canonical_name}
                </h3>
                <span className="text-xs uppercase tracking-wider text-stone-500">
                  {confidenceLabel(o.match_confidence)}
                </span>
              </div>

              {o.aliases && o.aliases.length > 0 ? (
                <p className="mb-2 text-xs text-stone-500">
                  Also known as: {o.aliases.join(", ")}
                </p>
              ) : null}

              {o.match_notes ? (
                <p className="mb-2 text-sm text-stone-600">{o.match_notes}</p>
              ) : null}

              {typeof o.testifier_count === "number" &&
              o.testifier_count > 0 ? (
                <p className="mb-3 text-sm text-stone-700">
                  Linked to {o.testifier_count.toLocaleString()} testifier
                  {o.testifier_count === 1 ? "" : "s"} on this agenda item.
                </p>
              ) : null}

              <ContextList rows={o.context ?? []} />
            </li>
          ))}
        </ul>
      )}

      <p className="text-xs text-stone-500">
        Lobbying registrations and compensation are pulled from the Washington
        Public Disclosure Commission via data.wa.gov. Showing the records is
        not a claim that lobbying caused testimony — it is context.
      </p>
    </section>
  );
}

function ContextList({ rows }: { rows: OrgContext[] }) {
  if (rows.length === 0) {
    return (
      <p className="text-sm text-stone-500">No PDC records loaded.</p>
    );
  }

  // Group by context_type so the UI surfaces the diversity of context
  // signals (registrations vs compensation vs contributions).
  const byType = new Map<string, OrgContext[]>();
  for (const r of rows) {
    const arr = byType.get(r.context_type) ?? [];
    arr.push(r);
    byType.set(r.context_type, arr);
  }

  return (
    <div className="space-y-3">
      {Array.from(byType.entries()).map(([type, list]) => (
        <details key={type} className="rounded border border-stone-200 bg-stone-50 p-3">
          <summary className="cursor-pointer text-sm font-medium text-stone-800">
            {type.replaceAll("_", " ")} ·{" "}
            <span className="font-normal text-stone-500">
              {list.length} record{list.length === 1 ? "" : "s"}
            </span>
          </summary>
          <ul className="mt-3 space-y-2 text-sm">
            {list.slice(0, 10).map((r, i) => (
              <li
                key={i}
                className="flex flex-col gap-1 rounded bg-white p-2 ring-1 ring-stone-200"
              >
                <SummaryFields fields={r.summary_fields} />
                {r.source_url ? (
                  <a
                    href={r.source_url}
                    target="_blank"
                    rel="noreferrer"
                    className="text-xs text-blue-700 underline hover:text-blue-900"
                  >
                    Source: {r.source_dataset_id} →
                  </a>
                ) : (
                  <span className="text-xs text-stone-500">
                    Source dataset: {r.source_dataset_id}
                  </span>
                )}
              </li>
            ))}
            {list.length > 10 ? (
              <li className="text-xs text-stone-500">
                … and {list.length - 10} more.
              </li>
            ) : null}
          </ul>
        </details>
      ))}
    </div>
  );
}

function SummaryFields({ fields }: { fields: Record<string, unknown> }) {
  const entries = Object.entries(fields).filter(
    ([, v]) => v !== null && v !== ""
  );
  if (entries.length === 0) return null;
  return (
    <dl className="grid grid-cols-[max-content_1fr] gap-x-3 gap-y-0.5 text-xs">
      {entries.map(([k, v]) => (
        <span key={k} className="contents">
          <dt className="text-stone-500">{k.replaceAll("_", " ")}</dt>
          <dd className="text-stone-800">{String(v)}</dd>
        </span>
      ))}
    </dl>
  );
}

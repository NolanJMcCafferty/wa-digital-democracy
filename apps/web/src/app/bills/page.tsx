import Link from "next/link";
import { listLocalBundles, type BundleListEntry } from "@/lib/loadBundle";

// Group rows by bill prefix so the page reads as "all HBs together,
// all SBs together" rather than mixing chambers. Order: HB, SB,
// HJR, SJR, HCR, SCR, HJM, SJM, then any other prefixes alphabetically.
const PREFIX_ORDER = ["HB", "SB", "HJR", "SJR", "HCR", "SCR", "HJM", "SJM"];

function groupByPrefix(bundles: BundleListEntry[]): Map<string, BundleListEntry[]> {
  const groups = new Map<string, BundleListEntry[]>();
  for (const b of bundles) {
    const arr = groups.get(b.billPrefix) ?? [];
    arr.push(b);
    groups.set(b.billPrefix, arr);
  }
  for (const [, arr] of groups) {
    arr.sort((a, b) => a.billNumber - b.billNumber);
  }
  return groups;
}

function sortPrefixes(prefixes: string[]): string[] {
  return prefixes.sort((a, b) => {
    const ai = PREFIX_ORDER.indexOf(a);
    const bi = PREFIX_ORDER.indexOf(b);
    if (ai !== -1 && bi !== -1) return ai - bi;
    if (ai !== -1) return -1;
    if (bi !== -1) return 1;
    return a.localeCompare(b);
  });
}

export default async function BillsPage() {
  const bundles = await listLocalBundles();
  const groups = groupByPrefix(bundles);
  const prefixes = sortPrefixes(Array.from(groups.keys()));

  return (
    <div className="space-y-8">
      <div className="space-y-3">
        <h1 className="text-3xl font-bold tracking-tight">Bills</h1>
        <p className="text-stone-600">
          {bundles.length.toLocaleString()} bills from the active Washington biennium. Click a bill to open its source-linked public-record page.
        </p>
      </div>

      {bundles.length === 0 ? (
        <p className="rounded border border-stone-300 bg-stone-50 p-4 text-sm text-stone-600">
          No bills are available yet.
        </p>
      ) : (
        prefixes.map((prefix) => {
          const rows = groups.get(prefix) ?? [];
          return (
            <section key={prefix} className="space-y-3">
              <h2 className="text-xl font-semibold text-stone-900">
                {prefix}{" "}
                <span className="text-sm font-normal text-stone-500">
                  ({rows.length.toLocaleString()})
                </span>
              </h2>
              <ul className="divide-y divide-stone-200 rounded border border-stone-300 bg-white">
                {rows.map((b) => (
                  <li
                    key={`${b.biennium}-${b.billPrefix}-${b.billNumber}`}
                    className="px-4 py-2"
                  >
                    <Link
                      href={`/bills/${b.biennium}/${b.billPrefix}${b.billNumber}`}
                      className="flex flex-col gap-0.5 hover:bg-stone-50"
                    >
                      <span className="font-medium text-stone-900">
                        {b.billId}
                        {b.title ? (
                          <span className="ml-2 font-normal text-stone-600">
                            — {b.title}
                          </span>
                        ) : null}
                      </span>
                      <span className="text-xs text-stone-500">
                        Biennium {b.biennium}
                      </span>
                    </Link>
                  </li>
                ))}
              </ul>
            </section>
          );
        })
      )}
    </div>
  );
}

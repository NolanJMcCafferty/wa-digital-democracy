import type { Bill } from "@/lib/bundle";

export function BillSnapshot({ bill }: { bill: Bill }) {
  return (
    <section aria-labelledby="bill-heading" className="space-y-4">
      <div className="flex flex-wrap items-baseline gap-x-4 gap-y-2">
        <h1
          id="bill-heading"
          className="text-3xl font-bold tracking-tight text-stone-900"
        >
          {bill.bill_id}
        </h1>
        <span className="text-sm text-stone-500">
          Biennium {bill.biennium}
          {bill.chamber_origin ? ` · ${bill.chamber_origin} of origin` : ""}
        </span>
      </div>

      {bill.title ? (
        <h2 className="text-xl font-semibold text-stone-800">{bill.title}</h2>
      ) : null}

      {bill.description ? (
        <p className="text-stone-700 leading-relaxed">{bill.description}</p>
      ) : null}

      {bill.sponsors && bill.sponsors.length > 0 ? (
        <div className="flex flex-wrap gap-x-4 gap-y-1 text-sm text-stone-600">
          <span className="font-medium text-stone-700">Sponsors:</span>
          {bill.sponsors.map((s) => (
            <span key={`${s.name}-${s.sponsor_type}`}>
              {s.name}
              {s.sponsor_type === "Primary" ? (
                <span className="ml-1 inline-block rounded bg-stone-200 px-1.5 py-0.5 text-xs font-medium uppercase tracking-wider text-stone-700">
                  Primary
                </span>
              ) : null}
            </span>
          ))}
        </div>
      ) : null}

      {bill.official_url ? (
        <p className="text-sm">
          <a
            className="text-blue-700 underline hover:text-blue-900"
            href={bill.official_url}
            target="_blank"
            rel="noreferrer"
          >
            Official bill summary on app.leg.wa.gov →
          </a>
        </p>
      ) : null}
    </section>
  );
}

import type { Hearing } from "@/lib/bundle";
import { formatDateTime } from "@/lib/format";

export function HearingCard({ hearing }: { hearing: Hearing }) {
  return (
    <section
      aria-labelledby="hearing-heading"
      className="space-y-4 rounded-lg border border-stone-300 bg-white p-6"
    >
      <div className="flex items-baseline justify-between gap-4">
        <h2
          id="hearing-heading"
          className="text-xl font-semibold text-stone-900"
        >
          Hearing
        </h2>
        {hearing.tvw_url ? (
          <a
            className="text-sm text-blue-700 underline hover:text-blue-900"
            href={hearing.tvw_url}
            target="_blank"
            rel="noreferrer"
          >
            Watch on TVW →
          </a>
        ) : null}
      </div>

      <dl className="grid grid-cols-1 gap-x-6 gap-y-2 text-sm sm:grid-cols-[max-content_1fr]">
        <dt className="text-stone-500">Committee</dt>
        <dd className="text-stone-900">
          {hearing.committee_name}
          {hearing.committee_acronym ? (
            <span className="ml-2 rounded bg-stone-100 px-1.5 py-0.5 text-xs uppercase tracking-wider text-stone-600">
              {hearing.committee_acronym}
            </span>
          ) : null}
        </dd>

        <dt className="text-stone-500">When</dt>
        <dd className="text-stone-900 tabular-nums">
          {formatDateTime(hearing.meeting_datetime)}
        </dd>

        {hearing.location ? (
          <>
            <dt className="text-stone-500">Where</dt>
            <dd className="text-stone-900">{hearing.location}</dd>
          </>
        ) : null}

        {hearing.agenda_item_label ? (
          <>
            <dt className="text-stone-500">Agenda item</dt>
            <dd className="text-stone-900">{hearing.agenda_item_label}</dd>
          </>
        ) : null}
      </dl>
    </section>
  );
}

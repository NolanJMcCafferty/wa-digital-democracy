import type { Status } from "@/lib/bundle";
import { formatDate } from "@/lib/format";

export function StatusTimeline({ status }: { status: Status }) {
  const timeline = status.timeline ?? [];
  return (
    <section aria-labelledby="status-heading" className="space-y-4">
      <h2 id="status-heading" className="text-xl font-semibold text-stone-900">
        Status
      </h2>
      {status.current ? (
        <p className="text-stone-700">
          <span className="font-medium">Current status:</span>{" "}
          <span className="text-stone-900">{status.current}</span>
        </p>
      ) : null}

      {timeline.length === 0 ? (
        <p className="text-sm text-stone-500">No status changes recorded.</p>
      ) : (
        <ol className="border-l-2 border-stone-300 pl-6 space-y-3">
          {timeline.map((e, i) => (
            <li key={`${e.action_date}-${i}`} className="relative">
              <span
                aria-hidden
                className="absolute -left-[29px] top-1 h-3 w-3 rounded-full bg-stone-400 ring-2 ring-stone-50"
              />
              <div className="text-sm text-stone-500 tabular-nums">
                {formatDate(e.action_date)}
              </div>
              <div className="text-stone-800">{e.history_line}</div>
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}

import Link from "next/link";
import { listHearingBundles } from "@/lib/loadBundle";
import { formatDateTime } from "@/lib/format";

export default async function HearingsPage() {
  const hearings = await listHearingBundles();

  return (
    <div className="space-y-8">
      <div className="space-y-3">
        <h1 className="text-3xl font-bold tracking-tight">Hearings</h1>
        <p className="text-stone-600">
          Committee hearing pages center the public record: committee, agenda item, testimony, transcript excerpts, video, and source records.
        </p>
      </div>

      {hearings.length === 0 ? (
        <p className="rounded border border-stone-300 bg-stone-50 p-4 text-sm text-stone-600">
          No hearings are available yet.
        </p>
      ) : (
        <ul className="divide-y divide-stone-300 rounded border border-stone-300 bg-white">
          {hearings.map((h) => (
            <li key={h.csiAgendaItemId}>
              <Link
                href={`/hearings/${h.csiAgendaItemId}`}
                className="block px-4 py-4 hover:bg-stone-50"
              >
                <div className="flex flex-col gap-1">
                  <span className="font-medium text-stone-900">{h.title}</span>
                  <span className="text-sm text-stone-600">
                    {h.committeeName} · {formatDateTime(h.meetingDatetime)}
                  </span>
                  <span className="text-xs text-stone-500">
                    {h.billId} · CSI agenda item {h.csiAgendaItemId}
                  </span>
                </div>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

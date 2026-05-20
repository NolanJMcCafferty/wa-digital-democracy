import Link from "next/link";
import { formatDate } from "@/lib/format";
import type { HearingPage } from "@/lib/api";

export function HearingsTable({ hearings }: { hearings: HearingPage[] }) {
  return (
    <div className="overflow-x-auto rounded-lg border border-stone-300 bg-white">
      <table className="w-full text-sm">
        <thead className="border-b border-stone-300 bg-stone-50 text-left text-xs uppercase tracking-wider text-stone-600">
          <tr>
            <th className="px-4 py-2.5 font-semibold">Name</th>
            <th className="px-4 py-2.5 font-semibold">Committee</th>
            <th className="px-4 py-2.5 font-semibold">Chamber</th>
            <th className="w-40 px-4 py-2.5 font-semibold">Date</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-stone-200">
          {hearings.map((h) => {
            const href = `/hearings/${h.hearingId}`;
            const billSummary = h.agendaItems.map((a) => a.billId).filter(Boolean).join(", ");
            return (
              <tr key={h.hearingId} className="hover:bg-stone-50">
                <td className="px-4 py-2.5 align-top">
                  <Link href={href} className="font-medium text-stone-900 hover:underline">
                    {h.title}
                  </Link>
                  <div className="mt-0.5 text-xs text-stone-500">
                    {h.agendaItems.length.toLocaleString()} agenda item{h.agendaItems.length === 1 ? "" : "s"}
                    {billSummary ? <> · <span className="font-mono">{billSummary}</span></> : null}
                  </div>
                </td>
                <td className="px-4 py-2.5 align-top text-stone-700">
                  {h.committeeName || <span className="text-stone-400">—</span>}
                </td>
                <td className="px-4 py-2.5 align-top text-stone-700">
                  {h.chamber || <span className="text-stone-400">—</span>}
                </td>
                <td className="w-40 whitespace-nowrap px-4 py-2.5 align-top text-stone-700 tabular-nums">
                  {formatDate(h.meetingDatetime)}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

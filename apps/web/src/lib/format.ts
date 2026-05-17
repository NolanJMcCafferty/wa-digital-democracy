// Helpers shared by the bill-hearing page sections.

export function formatMS(ms: number): string {
  if (!Number.isFinite(ms) || ms < 0) return "0:00";
  const total = Math.floor(ms / 1000);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  if (h > 0) return `${h}:${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
  return `${m}:${String(s).padStart(2, "0")}`;
}

export function formatDate(iso?: string | null): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleDateString("en-US", { year: "numeric", month: "short", day: "numeric" });
}

export function formatDateTime(iso?: string | null, opts?: { tz?: string }): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
    timeZone: opts?.tz ?? "America/Los_Angeles",
    timeZoneName: "short",
  });
}

// TVW deep-link to a specific timestamp in seconds.
export function tvwDeepLink(eventId: string, startMs: number): string {
  const seconds = Math.max(0, Math.floor(startMs / 1000));
  return `https://tvw.org/watch?eventID=${encodeURIComponent(eventId)}&startStreamAt=${seconds}`;
}

export function confidenceLabel(c: string): string {
  switch (c) {
    case "confirmed_legislator":
      return "confirmed";
    case "likely_legislator":
      return "likely legislator";
    case "likely_testifier":
      return "likely testifier";
    case "ai_inferred_pending_review":
      return "AI-inferred (pending review)";
    case "confirmed":
      return "confirmed match";
    case "probable":
      return "probable match";
    case "possible":
      return "possible match";
    case "unmatched":
      return "unmatched";
    case "unknown_speaker":
    default:
      return "unknown";
  }
}

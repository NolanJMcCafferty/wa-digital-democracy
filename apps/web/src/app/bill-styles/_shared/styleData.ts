import type { Bundle, Position } from "@/lib/bundle";

export type StyleKey =
  | "decision-brief"
  | "testimony-theatre"
  | "power-map"
  | "civic-docket"
  | "signal-desk";

export type StyleConfig = {
  key: StyleKey;
  demoNumber: number;
  name: string;
  routePrefix: string;
  oneLine: string;
  tone: string;
  palette: string[];
  bg: string;
  surface: string;
  border: string;
  text: string;
  muted: string;
  accent: string;
  secondary: string;
  fontHint: string;
};

export const STYLE_CONFIGS: StyleConfig[] = [
  {
    key: "decision-brief",
    demoNumber: 1,
    name: "Decision Brief",
    routePrefix: "bill1",
    oneLine: "An executive-grade civic memo: what changed, who weighed in, what to inspect next.",
    tone: "crisp · editorial · high-signal · boardroom civic",
    palette: ["#F4F1EA", "#FFFFFF", "#17120D", "#B45309", "#0F766E"],
    bg: "#F4F1EA",
    surface: "#FFFFFF",
    border: "#D8CCB8",
    text: "#17120D",
    muted: "#6E6257",
    accent: "#B45309",
    secondary: "#EFE6D8",
    fontHint: "serif headlines / precise sans / restrained amber",
  },
  {
    key: "testimony-theatre",
    demoNumber: 2,
    name: "Testimony Theatre",
    routePrefix: "bill2",
    oneLine: "A cinematic hearing page built around voices, timestamps, and the public room.",
    tone: "human · cinematic · transcript-first · immersive",
    palette: ["#100B1F", "#23143D", "#F8EEDF", "#FFB86B", "#A78BFA"],
    bg: "#100B1F",
    surface: "#1B1230",
    border: "#3D2A63",
    text: "#F8EEDF",
    muted: "#C7B8D8",
    accent: "#FFB86B",
    secondary: "#2A1A49",
    fontHint: "dramatic display / readable captions / warm spotlight",
  },
  {
    key: "power-map",
    demoNumber: 3,
    name: "Power Map",
    routePrefix: "bill3",
    oneLine: "A stakeholder map showing pressure, alignment, organizations, and provenance at a glance.",
    tone: "strategic · relational · analytical · influence-aware",
    palette: ["#F6F7FB", "#FFFFFF", "#101828", "#7C3AED", "#12B76A"],
    bg: "#F6F7FB",
    surface: "#FFFFFF",
    border: "#D9D6FE",
    text: "#101828",
    muted: "#667085",
    accent: "#7C3AED",
    secondary: "#F4F3FF",
    fontHint: "modern product / graph cards / relationship color",
  },
  {
    key: "civic-docket",
    demoNumber: 4,
    name: "Civic Docket",
    routePrefix: "bill4",
    oneLine: "A public-record case file: durable, chronological, citeable, and calm.",
    tone: "legal · orderly · archival · citeable",
    palette: ["#F7F7F2", "#FFFFFF", "#202124", "#1D4ED8", "#475569"],
    bg: "#F7F7F2",
    surface: "#FFFFFF",
    border: "#C9C9BF",
    text: "#202124",
    muted: "#5F6368",
    accent: "#1D4ED8",
    secondary: "#EEF2F7",
    fontHint: "court docket / legal typography / dense but humane",
  },
  {
    key: "signal-desk",
    demoNumber: 5,
    name: "Signal Desk",
    routePrefix: "bill5",
    oneLine: "A live civic intelligence desk for reporters and policy operators tracking signal in the noise.",
    tone: "fast · newsroom ops · dashboard · alerts-ready",
    palette: ["#06111F", "#0B1B2E", "#E6F6FF", "#22D3EE", "#F43F5E"],
    bg: "#06111F",
    surface: "#0B1B2E",
    border: "#1E3A5F",
    text: "#E6F6FF",
    muted: "#8FB3CC",
    accent: "#22D3EE",
    secondary: "#10233A",
    fontHint: "terminal precision / newsroom dashboard / electric cyan",
  },
];

export function styleByPrefix(prefix: string): StyleConfig | undefined {
  return STYLE_CONFIGS.find((s) => s.routePrefix === prefix);
}

export function parseBillSlug(slug: string): { prefix: string; number: number } | null {
  const m = slug.match(/^([A-Z]+)([0-9]+)$/i);
  if (!m) return null;
  return { prefix: m[1].toUpperCase(), number: parseInt(m[2], 10) };
}

export function billPathForStyle(style: StyleConfig, bundle: Bundle): string {
  const number = bundle.bill.bill_id.replace(/\s+/g, "");
  return `/${style.routePrefix}/${number}`;
}

export function testimonyCounts(bundle: Bundle): Record<Position, number> & { total: number; testified: number } {
  const counts: Record<Position, number> = { Pro: 0, Con: 0, Other: 0, Unknown: 0 };
  let testified = 0;
  for (const t of bundle.testifiers ?? []) {
    counts[t.position] = (counts[t.position] ?? 0) + 1;
    if (t.testified) testified += 1;
  }
  return { ...counts, total: bundle.testifiers?.length ?? 0, testified };
}

export function topOrganizations(bundle: Bundle, limit = 5): string[] {
  return [...(bundle.organizations ?? [])]
    .sort((a, b) => (b.testifier_count ?? 0) - (a.testifier_count ?? 0))
    .slice(0, limit)
    .map((o) => o.canonical_name);
}

export function sourceSystems(bundle: Bundle): string[] {
  return Array.from(new Set((bundle.sources ?? []).map((s) => s.system))).sort();
}

export function formatDateShort(iso?: string | null): string {
  if (!iso) return "Unknown";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return new Intl.DateTimeFormat("en-US", { month: "short", day: "numeric", year: "numeric" }).format(d);
}

import type { Bundle, Position } from "@/lib/bundle";

export type StyleKey = "public-record" | "civic-newsroom" | "source-graph" | "civic-atlas" | "ai-assistant";

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
    key: "public-record",
    demoNumber: 1,
    name: "Public Record Modern",
    routePrefix: "bill1",
    oneLine: "Official, restrained, high-trust public-record interface.",
    tone: "Calm · institutional · source-forward · accessible",
    palette: ["#F8F9FA", "#FFFFFF", "#111827", "#1A5A96", "#F2C94C"],
    bg: "#F8F9FA",
    surface: "#FFFFFF",
    border: "#D9DEE7",
    text: "#111827",
    muted: "#5B6472",
    accent: "#1A5A96",
    secondary: "#EEF4FA",
    fontHint: "Libre Franklin / Source Sans 3 / IBM Plex Mono",
  },
  {
    key: "civic-newsroom",
    demoNumber: 2,
    name: "Civic Newsroom",
    routePrefix: "bill2",
    oneLine: "Public-interest journalism brief with receipts underneath.",
    tone: "Editorial · clear · human · story plus receipts",
    palette: ["#FAF7F0", "#FFFFFF", "#1C1917", "#2457A6", "#C2410C"],
    bg: "#FAF7F0",
    surface: "#FFFFFF",
    border: "#E6DED2",
    text: "#1C1917",
    muted: "#6B625A",
    accent: "#2457A6",
    secondary: "#EAF1F8",
    fontHint: "Source Serif 4 / Inter / IBM Plex Mono",
  },
  {
    key: "source-graph",
    demoNumber: 3,
    name: "Source Graph Intelligence",
    routePrefix: "bill3",
    oneLine: "Investigative workspace for bills, hearings, people, orgs, money, and source records.",
    tone: "Analytical · dense · fast · evidence graph",
    palette: ["#0B1020", "#111827", "#E5E7EB", "#38BDF8", "#34D399"],
    bg: "#0B1020",
    surface: "#111827",
    border: "#2A3448",
    text: "#E5E7EB",
    muted: "#9CA3AF",
    accent: "#38BDF8",
    secondary: "#172033",
    fontHint: "Geist Sans / Inter / JetBrains Mono",
  },
  {
    key: "civic-atlas",
    demoNumber: 4,
    name: "Washington Civic Atlas",
    routePrefix: "bill4",
    oneLine: "Place-based civic interface grounded in districts, communities, issues, and source records.",
    tone: "Place-based · regional · calm · map-forward",
    palette: ["#FAFAF7", "#E8F0EF", "#12372A", "#216869", "#E76F51"],
    bg: "#FAFAF7",
    surface: "#FFFFFF",
    border: "#D4E0DE",
    text: "#2F3437",
    muted: "#7A8589",
    accent: "#1F5C45",
    secondary: "#E8F0EF",
    fontHint: "Source Serif 4 or Fraunces / Public Sans / IBM Plex Mono",
  },
  {
    key: "ai-assistant",
    demoNumber: 5,
    name: "Transparent AI Civic Assistant",
    routePrefix: "bill5",
    oneLine: "Natural-language civic search with cited, confidence-labeled answers.",
    tone: "Smart · minimal · helpful · auditable AI",
    palette: ["#FBFBFD", "#FFFFFF", "#111827", "#4F46E5", "#059669"],
    bg: "#FBFBFD",
    surface: "#FFFFFF",
    border: "#E5E7EB",
    text: "#111827",
    muted: "#6B7280",
    accent: "#4F46E5",
    secondary: "#EEF2FF",
    fontHint: "Geist Sans / IBM Plex Mono",
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

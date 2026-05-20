import type { HearingPage } from "@/lib/api";
import { ISSUE_PAGE_CONFIGS } from "@/app/issues/_shared/config";
import type { parseHearingFilters } from "./HearingSearchResults";

export function filterHearings(
  all: HearingPage[],
  filters: ReturnType<typeof parseHearingFilters>,
): {
  hearings: HearingPage[];
  total: number;
  offset: number;
  facets: { chambers: string[]; committees: string[]; biennia: string[] };
} {
  const facets = {
    chambers: Array.from(new Set(all.map((h) => h.chamber).filter(Boolean))).sort(),
    committees: Array.from(new Set(all.map((h) => h.committeeName).filter(Boolean))).sort(),
    biennia: Array.from(
      new Set(all.flatMap((h) => h.agendaItems.map((a) => a.biennium).filter(Boolean))),
    ).sort(),
  };

  const topicKeywords = new Set<string>();
  for (const slug of filters.topics) {
    const cfg = ISSUE_PAGE_CONFIGS.find((c) => c.slug === slug);
    if (!cfg) continue;
    for (const kw of cfg.keywords) topicKeywords.add(kw.toLowerCase());
  }

  const matchesTopic = (h: HearingPage): boolean => {
    if (topicKeywords.size === 0) return true;
    const haystack = (h.title + " " + h.agendaItems.map((a) => a.billId).join(" ")).toLowerCase();
    for (const kw of topicKeywords) {
      if (haystack.includes(kw)) return true;
    }
    return false;
  };

  const filtered = all.filter((h) => {
    if (filters.committee && !h.committeeName.toLowerCase().includes(filters.committee.toLowerCase())) return false;
    if (filters.bill) {
      const needle = filters.bill.toLowerCase();
      if (!h.agendaItems.some((a) => a.billId.toLowerCase().includes(needle))) return false;
    }
    if (filters.chambers.length > 0 && !filters.chambers.includes(h.chamber)) return false;
    if (filters.biennium && !h.agendaItems.some((a) => a.biennium === filters.biennium)) return false;
    if (!matchesTopic(h)) return false;
    return true;
  });

  const total = filtered.length;
  const offset = (filters.page - 1) * filters.limit;
  const hearings = filtered.slice(offset, offset + filters.limit);
  return { hearings, total, offset, facets };
}

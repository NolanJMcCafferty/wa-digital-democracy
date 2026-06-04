import type { OrganizationCompensation } from "@/lib/api";

export type CompensationYear = {
  key: string;
  year: number;
  counterparty: string;
  compensation: number;
  expenses: number;
  filings: number;
  latestUrl?: string;
};

export function aggregateCompensationByYear(
  rows: OrganizationCompensation[],
): CompensationYear[] {
  const buckets = new Map<string, CompensationYear & { latestPeriod: string }>();
  for (const r of rows) {
    const yearStr = r.filingPeriod.slice(0, 4);
    if (!/^\d{4}$/.test(yearStr)) continue;
    const year = Number(yearStr);
    const counterparty = r.role === "filer" ? r.employerName : r.filerName;
    const counterpartyId = r.role === "filer" ? r.employerId : r.filerId;
    const key = `${counterpartyId}|${year}`;
    const comp = Number(r.compensation) || 0;
    const exp = Number(r.totalExpenses) || 0;
    const existing = buckets.get(key);
    if (existing) {
      existing.compensation += comp;
      existing.expenses += exp;
      existing.filings += 1;
      if (r.filingPeriod > existing.latestPeriod) {
        existing.latestPeriod = r.filingPeriod;
        existing.latestUrl = r.url;
      }
    } else {
      buckets.set(key, {
        key,
        year,
        counterparty,
        compensation: comp,
        expenses: exp,
        filings: 1,
        latestUrl: r.url,
        latestPeriod: r.filingPeriod,
      });
    }
  }
  return Array.from(buckets.values()).sort((a, b) => {
    if (b.year !== a.year) return b.year - a.year;
    return b.compensation - a.compensation;
  });
}

export function formatAmount(amount: number): string {
  if (!Number.isFinite(amount)) return "";
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    maximumFractionDigits: 0,
  }).format(amount);
}

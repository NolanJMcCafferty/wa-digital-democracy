import { describe, expect, it } from "vitest";
import { aggregateCompensationByYear, formatAmount } from "./compensation";
import type { OrganizationCompensation } from "@/lib/api";

function row(over: Partial<OrganizationCompensation>): OrganizationCompensation {
  return {
    role: "employer",
    filerId: "F1",
    filerName: "Some Lobbyist",
    employerId: "E1",
    employerName: "Some Client",
    filingPeriod: "2024-01-01T00:00:00.000",
    compensation: "0",
    totalExpenses: "0",
    netTotal: "0",
    url: undefined,
    ...over,
  };
}

describe("aggregateCompensationByYear", () => {
  it("collapses monthly filings into one row per (counterparty, year)", () => {
    const rows = [
      row({ filingPeriod: "2024-01-01T00:00:00.000", compensation: "1000", totalExpenses: "5" }),
      row({ filingPeriod: "2024-02-01T00:00:00.000", compensation: "1000", totalExpenses: "10" }),
      row({ filingPeriod: "2024-03-01T00:00:00.000", compensation: "1000", totalExpenses: "0" }),
    ];
    const out = aggregateCompensationByYear(rows);
    expect(out).toHaveLength(1);
    expect(out[0]).toMatchObject({
      year: 2024,
      counterparty: "Some Lobbyist",
      compensation: 3000,
      expenses: 15,
      filings: 3,
    });
  });

  it("keeps separate rows per year and sorts year desc", () => {
    const rows = [
      row({ filingPeriod: "2022-06-01T00:00:00.000", compensation: "500" }),
      row({ filingPeriod: "2024-06-01T00:00:00.000", compensation: "1000" }),
      row({ filingPeriod: "2023-06-01T00:00:00.000", compensation: "750" }),
    ];
    const out = aggregateCompensationByYear(rows);
    expect(out.map((r) => r.year)).toEqual([2024, 2023, 2022]);
  });

  it("uses the latest filing's url within a year", () => {
    const rows = [
      row({ filingPeriod: "2024-01-01T00:00:00.000", url: "https://example.com/jan" }),
      row({ filingPeriod: "2024-06-01T00:00:00.000", url: "https://example.com/jun" }),
      row({ filingPeriod: "2024-04-01T00:00:00.000", url: "https://example.com/apr" }),
    ];
    const out = aggregateCompensationByYear(rows);
    expect(out[0].latestUrl).toBe("https://example.com/jun");
  });

  it("uses employer name as counterparty when role=filer (org is the firm)", () => {
    const rows = [
      row({
        role: "filer",
        filerName: "This Firm",
        employerId: "C42",
        employerName: "Client Co",
        compensation: "100",
      }),
    ];
    const out = aggregateCompensationByYear(rows);
    expect(out[0].counterparty).toBe("Client Co");
  });

  it("uses filer name as counterparty when role=employer (org is the client)", () => {
    const rows = [
      row({
        role: "employer",
        filerId: "F42",
        filerName: "Hired Firm",
        employerName: "This Client",
        compensation: "100",
      }),
    ];
    const out = aggregateCompensationByYear(rows);
    expect(out[0].counterparty).toBe("Hired Firm");
  });

  it("groups by counterparty id, not name (handles renames)", () => {
    const rows = [
      row({ filerId: "F1", filerName: "ACME LLC", compensation: "100" }),
      row({ filerId: "F1", filerName: "Acme LLC*", compensation: "200" }),
    ];
    const out = aggregateCompensationByYear(rows);
    expect(out).toHaveLength(1);
    expect(out[0].compensation).toBe(300);
  });

  it("breaks ties within a year by compensation desc", () => {
    const rows = [
      row({ filerId: "F1", filerName: "Small", compensation: "100" }),
      row({ filerId: "F2", filerName: "Big", compensation: "9000" }),
      row({ filerId: "F3", filerName: "Mid", compensation: "1000" }),
    ];
    const out = aggregateCompensationByYear(rows);
    expect(out.map((r) => r.counterparty)).toEqual(["Big", "Mid", "Small"]);
  });

  it("skips rows with unparseable filing periods", () => {
    const rows = [
      row({ filingPeriod: "" }),
      row({ filingPeriod: "garbage" }),
      row({ filingPeriod: "2024-01-01T00:00:00.000", compensation: "100" }),
    ];
    const out = aggregateCompensationByYear(rows);
    expect(out).toHaveLength(1);
    expect(out[0].year).toBe(2024);
  });

  it("treats non-numeric compensation as zero without throwing", () => {
    const rows = [
      row({ compensation: "not-a-number", totalExpenses: "abc" }),
      row({ compensation: "500", totalExpenses: "50" }),
    ];
    const out = aggregateCompensationByYear(rows);
    expect(out[0].compensation).toBe(500);
    expect(out[0].expenses).toBe(50);
  });

  it("returns empty array for empty input", () => {
    expect(aggregateCompensationByYear([])).toEqual([]);
  });
});

describe("formatAmount", () => {
  it("formats whole-dollar USD with no fractional digits", () => {
    expect(formatAmount(1234)).toBe("$1,234");
  });

  it("rounds fractional values to whole dollars", () => {
    expect(formatAmount(1234.56)).toBe("$1,235");
  });

  it("formats zero", () => {
    expect(formatAmount(0)).toBe("$0");
  });

  it("returns empty string for non-finite values", () => {
    expect(formatAmount(NaN)).toBe("");
    expect(formatAmount(Infinity)).toBe("");
  });
});

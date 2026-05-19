// Patterns identifying the small set of "milestone" history lines that
// app.leg.wa.gov surfaces in its at-a-glance view. Used by the bill
// detail page's status timeline to filter procedural noise.
const MILESTONE_PATTERNS: RegExp[] = [
  /^First reading/i,
  /Third reading, passed/i,
  /Speaker signed/i,
  /President signed/i,
  /Delivered to Governor/i,
  /Governor signed/i,
  /Governor partially vetoed/i,
  /Governor vetoed/i,
  /Effective date/i,
  /Chapter \d+,/i,
  /Filed with Secretary of State/i,
];

export function isMilestoneStatus(historyLine: string): boolean {
  return MILESTONE_PATTERNS.some((re) => re.test(historyLine));
}

// Stage labels mirror the "Where is it in the process?" diagram on
// app.leg.wa.gov/billsummary — each bill should map to exactly one.
export type BillStage =
  | "Introduced"
  | "In committee"
  | "On floor calendar"
  | "Passed chamber"
  | "Passed Legislature"
  | "On Governor's desk"
  | "Signed by Governor"
  | "Session law"
  | "Vetoed"
  | "Reintroduced"
  | "Shelved"
  | "Died"
  | "In progress";

// Order matters: most-advanced stage wins. We test from "end of life"
// backwards so a bill that was eventually vetoed/signed reads as such
// even if its raw current_status mentions earlier procedural events.
const STAGE_RULES: Array<{ stage: BillStage; pattern: RegExp }> = [
  { stage: "Session law", pattern: /effective date|chapter \d+,|filed with secretary of state/i },
  { stage: "Vetoed", pattern: /governor (partially )?vetoed/i },
  { stage: "Signed by Governor", pattern: /governor signed/i },
  { stage: "On Governor's desk", pattern: /delivered to governor/i },
  { stage: "Passed Legislature", pattern: /speaker signed|president signed/i },
  { stage: "Shelved", pattern: /"x" file/i },
  { stage: "Reintroduced", pattern: /by resolution, reintroduced/i },
  { stage: "Died", pattern: /\bdied\b/i },
  { stage: "Passed chamber", pattern: /third reading, passed/i },
  { stage: "On floor calendar", pattern: /placed on (second|third) reading|rules committee.*(second|third) reading|second reading/i },
  { stage: "In committee", pattern: /referred to|public hearing|executive (action|session)|majority report|minority report|passed to rules/i },
  { stage: "Introduced", pattern: /first reading|prefiled|introduced/i },
];

// classifyBillStage maps a raw current_status string + bucket into one
// of the official WA legislature lifecycle stages. Falls back to the
// bucket's friendly label when no rule matches.
export function classifyBillStage(
  currentStatus: string | undefined,
  bucket: string | undefined,
): BillStage {
  const s = (currentStatus ?? "").trim();
  if (s) {
    for (const r of STAGE_RULES) {
      if (r.pattern.test(s)) return r.stage;
    }
  }
  if (bucket === "passed") return "Session law";
  if (bucket === "failed") return "Died";
  return "In progress";
}

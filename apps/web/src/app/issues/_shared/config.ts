import type { IssuePageConfig } from "./IssuePage";

export const ISSUE_PAGE_CONFIGS = [
  {
    slug: "housing",
    title: "Housing",
    description:
      "A first issue-level view over Washington housing legislation, testimony, transcript excerpts, organizations, and source records. As coverage expands, this page becomes the entry point for housing bills, hearings, permitting, affordability, and accountability data.",
    emptyLabel: "housing",
    keywords: [
      "housing",
      "zoning",
      "rent",
      "tenant",
      "homelessness",
      "affordable housing",
      "transit-oriented development",
      "permitting",
      "land use",
      "common interest communities",
      "unit owner",
    ],
  },
  {
    slug: "transportation",
    title: "Transportation",
    description:
      "A first issue-level view over Washington transportation legislation, hearings, testimony, organizations, and source records. Over time this becomes the entry point for transit, roads, ferries, safety, infrastructure projects, emissions, and transportation accountability data.",
    emptyLabel: "transportation",
    keywords: [
      "transportation",
      "transit",
      "rail",
      "bus",
      "ferry",
      "ferries",
      "highway",
      "road",
      "roads",
      "bridge",
      "bridges",
      "traffic safety",
      "wsdot",
      "sound transit",
      "public transportation",
      "vehicle miles traveled",
      "pedestrian",
      "bicycle safety",
    ],
  },
] satisfies IssuePageConfig[];

export function issuePageConfig(slug: string): IssuePageConfig | undefined {
  return ISSUE_PAGE_CONFIGS.find((issue) => issue.slug === slug);
}

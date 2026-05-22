import { IssuePage } from "../_shared/IssuePage";
import { issuePageConfig } from "../_shared/config";
import type { RawBillSearchParams } from "../../bills/BillSearchResults";

export const dynamic = "force-dynamic";

const config = issuePageConfig("education");

export default async function EducationIssuePage({
  searchParams,
}: {
  searchParams: Promise<RawBillSearchParams>;
}) {
  if (!config) throw new Error("Missing education issue page config");
  const raw = await searchParams;
  return <IssuePage config={config} searchParams={raw} />;
}

import { IssuePage } from "../_shared/IssuePage";
import { issuePageConfig } from "../_shared/config";
import type { RawBillSearchParams } from "../../bills/BillSearchResults";

const config = issuePageConfig("climate");

export default async function ClimateIssuePage({
  searchParams,
}: {
  searchParams: Promise<RawBillSearchParams>;
}) {
  if (!config) throw new Error("Missing climate issue page config");
  const raw = await searchParams;
  return <IssuePage config={config} searchParams={raw} />;
}

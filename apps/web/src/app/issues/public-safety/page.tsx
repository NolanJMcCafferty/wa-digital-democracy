import { IssuePage } from "../_shared/IssuePage";
import { issuePageConfig } from "../_shared/config";
import type { RawBillSearchParams } from "../../bills/BillSearchResults";

export const dynamic = "force-dynamic";

const config = issuePageConfig("public-safety");

export default async function PublicSafetyIssuePage({
  searchParams,
}: {
  searchParams: Promise<RawBillSearchParams>;
}) {
  if (!config) throw new Error("Missing public-safety issue page config");
  const raw = await searchParams;
  return <IssuePage config={config} searchParams={raw} />;
}

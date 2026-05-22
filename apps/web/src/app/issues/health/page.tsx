import { IssuePage } from "../_shared/IssuePage";
import { issuePageConfig } from "../_shared/config";
import type { RawBillSearchParams } from "../../bills/BillSearchResults";

export const dynamic = "force-dynamic";

const config = issuePageConfig("health");

export default async function HealthIssuePage({
  searchParams,
}: {
  searchParams: Promise<RawBillSearchParams>;
}) {
  if (!config) throw new Error("Missing health issue page config");
  const raw = await searchParams;
  return <IssuePage config={config} searchParams={raw} />;
}

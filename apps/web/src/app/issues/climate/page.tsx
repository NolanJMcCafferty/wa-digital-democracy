import { IssuePage } from "../_shared/IssuePage";
import { issuePageConfig } from "../_shared/config";

const config = issuePageConfig("climate");

export default function ClimateIssuePage() {
  if (!config) throw new Error("Missing climate issue page config");
  return <IssuePage config={config} />;
}

import { IssuePage } from "../_shared/IssuePage";
import { issuePageConfig } from "../_shared/config";

const config = issuePageConfig("housing");

export default function HousingIssuePage() {
  if (!config) throw new Error("Missing housing issue page config");
  return <IssuePage config={config} />;
}

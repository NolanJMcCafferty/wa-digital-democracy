import { IssuePage } from "../_shared/IssuePage";
import { issuePageConfig } from "../_shared/config";

const config = issuePageConfig("transportation");

export default function TransportationIssuePage() {
  if (!config) throw new Error("Missing transportation issue page config");
  return <IssuePage config={config} />;
}

import { IssuePage } from "../_shared/IssuePage";
import { issuePageConfig } from "../_shared/config";

const config = issuePageConfig("health");

export default function HealthIssuePage() {
  if (!config) throw new Error("Missing health issue page config");
  return <IssuePage config={config} />;
}

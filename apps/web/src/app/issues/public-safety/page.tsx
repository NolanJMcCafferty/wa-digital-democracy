import { IssuePage } from "../_shared/IssuePage";
import { issuePageConfig } from "../_shared/config";

const config = issuePageConfig("public-safety");

export default function PublicSafetyIssuePage() {
  if (!config) throw new Error("Missing public-safety issue page config");
  return <IssuePage config={config} />;
}

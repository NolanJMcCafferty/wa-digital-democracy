import { IssuePage } from "../_shared/IssuePage";
import { issuePageConfig } from "../_shared/config";

const config = issuePageConfig("education");

export default function EducationIssuePage() {
  if (!config) throw new Error("Missing education issue page config");
  return <IssuePage config={config} />;
}

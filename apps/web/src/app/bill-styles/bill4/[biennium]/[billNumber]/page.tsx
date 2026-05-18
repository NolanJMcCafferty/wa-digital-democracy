import { notFound } from "next/navigation";
import { loadBundle } from "@/lib/loadBundle";
import { parseBillSlug, STYLE_CONFIGS } from "../../../_shared/styleData";
import { StyledBillPage } from "../../../_shared/StyledBillPage";

type Params = { biennium: string; billNumber: string };

export default async function BillStyle4Page({ params }: { params: Promise<Params> }) {
  const { biennium, billNumber } = await params;
  const parsed = parseBillSlug(billNumber);
  if (!parsed) notFound();
  const bundle = await loadBundle(biennium, parsed.prefix, parsed.number);
  if (!bundle) notFound();
  return <StyledBillPage style={STYLE_CONFIGS[3]} bundle={bundle} />;
}

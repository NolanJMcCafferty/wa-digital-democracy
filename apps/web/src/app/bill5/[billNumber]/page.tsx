import { notFound } from "next/navigation";
import { loadBundle } from "@/lib/loadBundle";
import { parseBillSlug, STYLE_CONFIGS } from "../../bill-styles/_shared/styleData";
import { StyledBillPage } from "../../bill-styles/_shared/StyledBillPage";

type Params = { billNumber: string };

const DEFAULT_BIENNIUM = "2025-26";

export default async function BillStyle5ShortcutPage({ params }: { params: Promise<Params> }) {
  const { billNumber } = await params;
  const parsed = parseBillSlug(billNumber);
  if (!parsed) notFound();
  const bundle = await loadBundle(DEFAULT_BIENNIUM, parsed.prefix, parsed.number);
  if (!bundle) notFound();
  return <StyledBillPage style={STYLE_CONFIGS[4]} bundle={bundle} />;
}

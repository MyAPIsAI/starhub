import { Copy } from "lucide-react";
import { useTranslation } from "react-i18next";
import { copyText } from "../features/code-example";
import { message as translatedMessage } from "../i18n/messages";
import { usePreferences } from "../lib/preferences";
import type { PlazaPricing } from "../lib/api/plaza";

export function CatalogPriceCopy({
  model,
  group,
  pricing,
}: {
  model: string;
  group: string;
  pricing: PlazaPricing;
}) {
  const { t } = useTranslation();
  const lang = usePreferences((s) => s.language);
  const labels = translatedMessage("catalog22_copyPrices");
  const label = labels[lang === "zh" ? 0 : lang === "ar" ? 2 : 1];
  if (pricing.mode !== "per_token") return null;
  const amount = (value: number | null | undefined) =>
    value == null ? "—" : `$${value}`;
  const value = `${model} · ${group}\nUSD / 1M tokens\nInput: ${amount(pricing.input_per_m)}\nOutput: ${amount(pricing.output_per_m)}`;
  return (
    <button
      type="button"
      className="catalog-copy-prices"
      aria-label={label}
      onClick={() => void copyText(value, t("copied"), t("copyFailed"))}
    >
      <Copy size={13} />
      {label}
    </button>
  );
}

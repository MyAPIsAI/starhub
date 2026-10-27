import {
  placeholders,
  type Language,
  type TierId,
} from "../config/placeholders";
import type { PricingCatalogConfig } from "../config/models";
import { writtenLanguage } from "./localize";

export function serviceGroupLabel(
  id: TierId,
  language: Language,
  config: PricingCatalogConfig,
) {
  const tier = placeholders.tiers.find((item) => item.id === id)!;
  const name = config.groups[id]?.name;
  return !name || name === tier?.name.zh
    ? (tier?.name[writtenLanguage(language)] ?? id)
    : name;
}

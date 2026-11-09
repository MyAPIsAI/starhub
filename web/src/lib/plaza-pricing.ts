import { message as translatedMessage } from "../i18n/messages";
import type { Language } from "../config/placeholders";
import type { ModelPricing } from "../config/models";
import type { PlazaDirectoryModel, PlazaPricing } from "./api/plaza";
import type { PublicCatalogPrice } from "./catalog-public-pricing";

export type PlazaQuote = { model: PlazaDirectoryModel; group: string };

export function plazaAmount(value: number | null | undefined) {
  return typeof value === "number" && Number.isFinite(value) && value >= 0
    ? String(value)
    : undefined;
}

export function plazaAmounts(pricing: PlazaPricing) {
  return {
    input: plazaAmount(pricing.input_per_m),
    output: plazaAmount(pricing.output_per_m),
    cacheRead: plazaAmount(pricing.cache_read_per_m),
    cacheWrite: plazaAmount(pricing.cache_write_per_m),
    perRequest: plazaAmount(pricing.per_request),
  };
}

export function plazaOfficialReference(pricing: PlazaPricing) {
  const tokens: Partial<ModelPricing> = {
    input_per_million_usd: plazaAmount(pricing.official_input_per_m),
    output_per_million_usd: plazaAmount(pricing.official_output_per_m),
    cache_read_per_million_usd: plazaAmount(pricing.official_cache_read_per_m),
    cache_write_per_million_usd: plazaAmount(
      pricing.official_cache_write_per_m,
    ),
  };
  const perRequest = plazaAmount(pricing.official_per_request);
  if (pricing.mode === "per_request" && perRequest !== undefined)
    return {
      status: "current" as const,
      rate: {
        mode: "unit" as const,
        unit:
          pricing.unit === "image" || pricing.unit === "second"
            ? pricing.unit
            : ("request" as const),
        priceUsd: perRequest,
      },
    };
  if (Object.values(tokens).some((value) => value !== undefined))
    return {
      status: "current" as const,
      rate: { mode: "tokens" as const, pricing: tokens },
    };
  return null;
}

export function describePlazaPrice(pricing: PlazaPricing, lang: Language) {
  const text = (zh: string, en: string, ar: string) =>
    lang === "zh" ? zh : lang === "ar" ? ar : en;
  const amounts = plazaAmounts(pricing);
  const token = pricing.mode === "per_token";
  const request = pricing.mode === "per_request";
  const hasAmount = token
    ? amounts.input !== undefined || amounts.output !== undefined
    : request && amounts.perRequest !== undefined;
  return {
    mode: token
      ? text(...translatedMessage("token_pricing_cc103abe12"))
      : request
        ? text(...translatedMessage("per_request_f6e4e2b1f7"))
        : text(...translatedMessage("conditional_pricing_8a72d20e3e")),
    value: token
      ? `${amounts.input === undefined ? "—" : `$${amounts.input}`} / ${amounts.output === undefined ? "—" : `$${amounts.output}`}`
      : request
        ? amounts.perRequest === undefined
          ? "—"
          : `$${amounts.perRequest}`
        : text(...translatedMessage("usage_based_pricing_efcd71458e")),
    unit: token
      ? text(...translatedMessage("input_output_usd_million_tokens_f00f399943"))
      : request
        ? pricing.unit === "second"
          ? text(...translatedMessage("usd_second_b4496726da"))
          : pricing.unit === "image"
            ? text(...translatedMessage("usd_image_efd0385640"))
            : text(...translatedMessage("usd_request_5265075e68"))
        : "",
    hasAmount,
    details:
      !token && !request
        ? text(
            ...translatedMessage(
              "calculated_from_request_conditions_the_base_r_2febbde24f",
            ),
          )
        : pricing.explicit
          ? text(
              ...translatedMessage(
                "backend_quote_including_the_group_multiplier_77ee33c8d6",
              ),
            )
          : text(
              ...translatedMessage(
                "no_explicit_rate_using_the_backend_default_in_caea4079e1",
              ),
            ),
  };
}

export function plazaCatalogOffer({
  model,
  group,
}: PlazaQuote): PublicCatalogPrice {
  const amounts = plazaAmounts(model.pricing);
  const ready =
    model.pricing.mode === "tiered" ||
    (model.pricing.mode === "per_token"
      ? amounts.input !== undefined || amounts.output !== undefined
      : model.pricing.mode === "per_request" &&
        amounts.perRequest !== undefined);
  return {
    status: ready ? "ready" : "quote-error",
    mode:
      model.pricing.mode === "per_token"
        ? "tokens"
        : model.pricing.mode === "per_request"
          ? "unit"
          : "expression",
    groupId: null,
    groupName: group,
    price: null,
    quotedAt: "",
    publicGroupCount: 0,
    quotedGroupCount: 0,
    comparison: "example-total",
    errors: [],
  };
}

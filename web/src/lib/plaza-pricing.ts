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
        unit: "request" as const,
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
      ? text("按词元计价", "Token pricing", "تسعير الرموز")
      : request
        ? text("按次计价", "Per request", "لكل طلب")
        : text("条件计价", "Conditional pricing", "تسعير مشروط"),
    value: token
      ? `${amounts.input === undefined ? "—" : `$${amounts.input}`} / ${amounts.output === undefined ? "—" : `$${amounts.output}`}`
      : request
        ? amounts.perRequest === undefined
          ? "—"
          : `$${amounts.perRequest}`
        : text("按用量计价", "Usage-based pricing", "حسب الاستخدام"),
    unit: token
      ? text(
          "输入 / 输出 · 美元 / 百万词元",
          "Input / output · USD / million tokens",
          "إدخال / إخراج · دولار / مليون رمز",
        )
      : request
        ? text("美元 / 次", "USD / request", "دولار / طلب")
        : "",
    hasAmount,
    details:
      !token && !request
        ? text(
            "按实际条件计算，基础倍率不是固定报价。",
            "Calculated from request conditions; the base ratio is not a fixed quote.",
            "تحسب وفق شروط الطلب؛ المعامل الأساسي ليس سعرًا ثابتًا.",
          )
        : pricing.explicit
          ? text(
              "后端报价，已包含分组倍率。",
              "Backend quote, including the group multiplier.",
              "سعر الخادم شامل معامل المجموعة.",
            )
          : text(
              "尚未单独配置，使用后端默认报价；已包含分组倍率。",
              "No explicit rate; using the backend default, including the group multiplier.",
              "لا يوجد سعر مستقل؛ يستخدم السعر الافتراضي شامل معامل المجموعة.",
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

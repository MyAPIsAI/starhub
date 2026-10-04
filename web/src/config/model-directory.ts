import { examplePricingAttributes } from "../lib/pricing-rules";
import {
  modelCatalog,
  groupResources,
  accountCanUseGroup,
} from "../lib/platform-catalog";
import {
  catalogModels,
  type ModelItem,
  type PricingCatalogConfig,
} from "./models";
import type { TierId } from "./placeholders";
import {
  getPricingSnapshot,
  quoteModelCharge,
  type MeteredUsage,
} from "../lib/model-pricing";

export type DirectoryModel = Omit<ModelItem, "vendor" | "pricing"> & {
  vendor: ModelItem["vendor"] | "Veyra";
  kind: "specified" | "smart";
  candidateIds?: string[];
};

export const smartDirectoryModel: DirectoryModel = {
  id: "veyra-auto",
  name: "Veyra Auto",
  vendor: "Veyra",
  kind: "smart",
  capabilities: ["text", "code", "vision", "reasoning"],
  contextLength: 0,
  contextDisplay: "64K–1M",
  protocols: ["OpenAI", "Anthropic", "Gemini"],
  tierAvailability: { ultra: true, pro: true, standard: true, lite: true },
  online: true,
  candidateIds: catalogModels
    .filter(
      (model) =>
        model.capabilities.includes("text") && model.protocols.length > 0,
    )
    .map((model) => model.id),
  description: {
    zh: "智能路由入口：在当前密钥授权、服务分组与请求能力范围内，按估算费用选择候选模型。",
    en: "Smart routing entry: choose an eligible model by estimated cost within key authorization, service group, and request capabilities.",
    ar: "مدخل توجيه ذكي يختار نموذجًا مؤهلاً وفق التكلفة المقدرة ضمن صلاحيات المفتاح والمجموعة وقدرات الطلب.",
  },
};
export const directoryModels: DirectoryModel[] = [
  smartDirectoryModel,
  ...catalogModels.map((model) => ({ ...model, kind: "specified" as const })),
];
export function getDirectoryModels(
  config = getPricingSnapshot(),
): DirectoryModel[] {
  return [
    smartDirectoryModel,
    ...modelCatalog(config).map((model) => ({
      ...model,
      kind: "specified" as const,
    })),
  ];
}
export function getDirectoryModel(id?: string, config = getPricingSnapshot()) {
  return getDirectoryModels(config).find((model) => model.id === id);
}

export function directoryCandidates(
  model: DirectoryModel,
  tier: TierId,
  pricing: PricingCatalogConfig,
  accountId = "account-demo",
) {
  return modelCatalog(pricing).filter(
    (candidate) =>
      model.online &&
      (model.kind === "smart"
        ? candidate.capabilities.includes("text") &&
          candidate.protocols.length > 0
        : candidate.id === model.id) &&
      accountCanUseGroup(pricing, tier, accountId) &&
      candidate.online &&
      pricing.models[candidate.id]?.tierAvailability[tier] &&
      (!pricing.resources ||
        groupResources(pricing, tier, candidate.id).length > 0),
  );
}
/** Display assumptions are explicit and shared by cards and detail tables. */
export function directoryExampleUsage(
  model: DirectoryModel | ModelItem,
  pricing: PricingCatalogConfig,
): MeteredUsage {
  const rule = pricing.models[model.id].rule;
  const usage: MeteredUsage = {
    input_tokens: 0,
    output_tokens: 0,
    cache_read_tokens: 0,
    cache_write_tokens: 0,
    images: 0,
    seconds: 0,
    requests: 1,
  };
  if (rule.mode === "unit") {
    if (rule.unit === "image") usage.images = 1;
    if (rule.unit === "second") usage.seconds = 1;
  } else if (model.capabilities.includes("image")) usage.images = 1;
  else if (model.capabilities.includes("video")) usage.seconds = 1;
  else {
    usage.input_tokens = 1000;
    usage.output_tokens = 300;
  }
  return usage;
}
export function directoryPrices(
  model: DirectoryModel,
  tier: TierId,
  pricing: PricingCatalogConfig,
  at = new Date().toISOString(),
  accountId = "account-demo",
) {
  const candidates = directoryCandidates(model, tier, pricing, accountId);
  const rates = candidates.map((candidate) => {
    const usage = directoryExampleUsage(candidate, pricing);
    const rule = pricing.models[candidate.id].rule;
    const attributes = examplePricingAttributes(rule);
    try {
      const quote = quoteModelCharge(
        { modelId: candidate.id, tierId: tier, usage, attributes, at },
        pricing,
      );
      return {
        model: candidate,
        rule,
        usage,
        attributes,
        quote,
        error: undefined,
        ...quote.rate,
      };
    } catch (error) {
      return {
        model: candidate,
        rule,
        usage,
        attributes,
        quote: undefined,
        error: error instanceof Error ? error.message : String(error),
        input: undefined,
        output: undefined,
        cacheRead: undefined,
        cacheWrite: undefined,
      };
    }
  });
  const amounts = rates
    .filter((rate) => rate.quote)
    .map((rate) => BigInt(rate.quote!.amountNano))
    .sort((a, b) => (a < b ? -1 : a > b ? 1 : 0));
  return {
    rates,
    available: candidates.length > 0,
    canEstimate: amounts.length > 0,
    hasEstimateErrors: rates.some((rate) => rate.error),
    minAmountNano: amounts[0]?.toString(),
    maxAmountNano: amounts.at(-1)?.toString(),
    minEstimate: amounts.length ? Number(amounts[0]) / 1e9 : Number.NaN,
    maxEstimate: amounts.length ? Number(amounts.at(-1)) / 1e9 : Number.NaN,
    quotedAt: at,
  };
}
export type DirectoryRate = ReturnType<typeof directoryPrices>["rates"][number];

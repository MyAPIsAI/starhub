import { create } from "zustand";
import { isAdmin, type SystemRole } from "./system-access";
import {
  catalogModels,
  getModelTierPrices,
  multiplyDecimalPrice,
  type ModelPricing,
  type PricingAttributes,
  type ModelPricingEntry,
  type PricingRule,
  type PricingCatalogConfig,
} from "../config/models";
import { placeholders, type TierId } from "../config/placeholders";

import { BILLING_STORAGE_KEY } from "./billing";
import { validateOfficialReference } from "./official-reference";
import {
  quotePricingRule,
  validatePricingRule,
  type MeteredUsage,
} from "./pricing-rules";
import {
  buildModelRatioJson,
  buildCompletionRatioJson,
} from "./api/option";
export { pricingRuleError, validatePricingRule } from "./pricing-rules";
export type { MeteredUsage } from "./pricing-rules";

import {
  modelCatalog,
  upgradePlatform,
  materializeAvailability,
  groupResources,
  resourceSnapshotError,
} from "./platform-catalog";

export const PRICING_STORAGE_KEY = "veyra-platform-pricing-v1";
const fields: (keyof ModelPricing)[] = [
  "input_per_million_usd",
  "output_per_million_usd",
  "cache_read_per_million_usd",
  "cache_write_per_million_usd",
];
export function createDefaultPricing(): PricingCatalogConfig {
  return upgradePlatform({
    revision: "sample-1",
    updatedAt: null,
    models: Object.fromEntries(
      catalogModels.map((model) => [
        model.id,
        {
          pricing: { ...model.pricing },
          rule:
            model.id === "veyra-demo-image"
              ? {
                  mode: "unit",
                  unit: "image",
                  priceUsd: "0.04",
                  minimum: "0",
                  ceilTo: "1",
                }
              : model.id === "veyra-demo-video"
                ? {
                    mode: "unit",
                    unit: "second",
                    priceUsd: "0.08",
                    minimum: "1",
                    ceilTo: "1",
                  }
                : { mode: "tokens" },
          source: "sample",
          sourceUrl: "",
          verifiedAt: null,
          effectiveAt: null,
          version: "sample-1",
          tierAvailability: { ...model.tierAvailability },
        },
      ]),
    ),
    multipliers: Object.fromEntries(
      placeholders.tiers.map((tier) => [tier.id, tier.multiplier]),
    ) as Record<TierId, string>,
    groups: Object.fromEntries(
      placeholders.tiers.map((tier) => [
        tier.id,
        {
          name: tier.name.zh,
          description: "实例投放决定可用模型；本分组倍率用于对客计费。",
          enabled: true,
        },
      ]),
    ) as PricingCatalogConfig["groups"],
  });
}

export function isValidPrice(value: unknown): value is string {
  return (
    typeof value === "string" &&
    /^(0|[1-9]\d*)(\.\d{1,6})?$/.test(value) &&
    Number(value) <= 1_000_000
  );
}
export function isValidMultiplier(value: unknown): value is string {
  return (
    typeof value === "string" &&
    /^(0|[1-9]\d*)(\.\d{1,4})?$/.test(value) &&
    Number(value) > 0 &&
    Number(value) <= 100
  );
}
/** Reject incomplete or corrupt snapshots instead of silently publishing partial rates. */
export function validatePricing(
  config: unknown,
): config is PricingCatalogConfig {
  if (!config || typeof config !== "object") return false;
  const candidate = config as PricingCatalogConfig;
  if (
    typeof candidate.revision !== "string" ||
    !candidate.revision ||
    !(
      candidate.updatedAt === null ||
      (typeof candidate.updatedAt === "string" &&
        Number.isFinite(Date.parse(candidate.updatedAt)))
    ) ||
    !candidate.models ||
    !candidate.multipliers ||
    !candidate.groups
  )
    return false;
  if (
    !Object.keys(candidate.groups).every((id) =>
      isValidMultiplier(candidate.multipliers[id]),
    )
  )
    return false;
  if (
    !Object.keys(candidate.groups).every((id) => {
      if (!/^[a-z0-9][a-z0-9_-]{1,59}$/.test(id)) return false;
      const group = candidate.groups[id];
      return (
        group &&
        typeof group.name === "string" &&
        group.name.trim().length > 0 &&
        group.name.length <= 60 &&
        typeof group.description === "string" &&
        group.description.length <= 500 &&
        typeof group.enabled === "boolean" &&
        (!group.visibility ||
          ["common", "custom"].includes(group.visibility)) &&
        (!group.accountIds ||
          (Array.isArray(group.accountIds) &&
            group.accountIds.every((id) => typeof id === "string"))) &&
        (group.visibility !== "custom" || Boolean(group.accountIds?.length))
      );
    })
  )
    return false;
  if (candidate.resources && resourceSnapshotError(candidate.resources))
    return false;
  if (
    candidate.catalog &&
    Object.entries(candidate.catalog).some(
      ([id, model]) =>
        id !== model.id ||
        !/^[a-z0-9][a-z0-9._:/-]{1,99}$/.test(id) ||
        !model.name?.trim() ||
        !model.vendor?.trim() ||
        !Array.isArray(model.protocols) ||
        model.protocols.some(
          (p) => !["OpenAI", "Anthropic", "Gemini"].includes(p),
        ) ||
        !Array.isArray(model.capabilities) ||
        model.capabilities.some(
          (p) =>
            !["text", "code", "vision", "reasoning", "image", "video"].includes(
              p,
            ),
        ) ||
        typeof model.online !== "boolean" ||
        !model.description ||
        typeof model.description.zh !== "string" ||
        !Number.isFinite(model.contextLength) ||
        model.contextLength < 0,
    )
  )
    return false;
  if (
    candidate.placements &&
    Object.values(candidate.placements).some(
      (binding) =>
        (binding.modelId !== null && !candidate.models[binding.modelId]) ||
        !Array.isArray(binding.groupIds) ||
        binding.groupIds.some((id) => !candidate.groups[id]) ||
        typeof binding.enabled !== "boolean",
    )
  )
    return false;
  return modelCatalog(candidate).every((model) => {
    const entry = candidate.models[model.id];
    return Boolean(
      entry?.pricing &&
      (entry.officialReference === undefined ||
        entry.officialReference === null ||
        validateOfficialReference(entry.officialReference)) &&
      entry.tierAvailability &&
      validatePricingRule(entry.rule) &&
      ["sample", "manual-official"].includes(entry.source) &&
      typeof entry.sourceUrl === "string" &&
      (!entry.sourceUrl || /^https?:\/\/[^\s]+$/.test(entry.sourceUrl)) &&
      (entry.verifiedAt === null ||
        (typeof entry.verifiedAt === "string" &&
          Number.isFinite(Date.parse(entry.verifiedAt)))) &&
      (entry.effectiveAt === null ||
        (typeof entry.effectiveAt === "string" &&
          Number.isFinite(Date.parse(entry.effectiveAt)))) &&
      typeof entry.version === "string" &&
      entry.version.trim().length > 0 &&
      entry.version.length <= 100 &&
      fields.every(
        (field, index) =>
          (index >= 2 && entry.pricing[field] === undefined) ||
          isValidPrice(entry.pricing[field]),
      ) &&
      Object.keys(candidate.groups).every(
        (id) => typeof entry.tierAvailability[id] === "boolean",
      ),
    );
  });
}
/** Upgrade only the known v1 shape. New sample media stay closed in old groups until explicitly enabled. */
export function migratePricing(data: unknown): PricingCatalogConfig | null {
  if (validatePricing(data)) return upgradePlatform(data);
  if (!data || typeof data !== "object") return null;
  const old = data as Partial<PricingCatalogConfig>;
  if (old.groups !== undefined || !old.models || !old.multipliers) return null;
  const defaults = createDefaultPricing();
  const models = { ...defaults.models };
  for (const model of catalogModels) {
    const existing = old.models[model.id];
    if (existing)
      models[model.id] = { ...defaults.models[model.id], ...existing };
    else if (model.id.startsWith("veyra-demo-"))
      models[model.id] = {
        ...models[model.id],
        tierAvailability: {
          ultra: false,
          pro: false,
          standard: false,
          lite: false,
        },
      };
    else return null;
  }
  const migrated = upgradePlatform({
    ...defaults,
    ...old,
    models,
    resources: undefined,
    placements: undefined,
  });
  return validatePricing(migrated) ? migrated : null;
}
export function readPricing(): PricingCatalogConfig {
  try {
    const raw = localStorage.getItem(PRICING_STORAGE_KEY);
    if (raw) {
      const data: unknown = JSON.parse(raw);
      const migrated = migratePricing(data);
      if (migrated) return migrated;
    } else {
      const defaults = createDefaultPricing();
      // Previously issued all-model keys must not gain new demo media rights on upgrade.
      if (localStorage.getItem(BILLING_STORAGE_KEY)) {
        for (const model of catalogModels.filter((item) =>
          item.id.startsWith("veyra-demo-"),
        ))
          defaults.models[model.id].tierAvailability = {
            ultra: false,
            pro: false,
            standard: false,
            lite: false,
          };
      }
      for (const resource of defaults.resources!.instances)
        defaults.placements![resource.id].groupIds = Object.keys(
          defaults.groups,
        ).filter(
          (id) => defaults.models[resource.canonicalModel].tierAvailability[id],
        );
      try {
        localStorage.setItem(PRICING_STORAGE_KEY, JSON.stringify(defaults));
      } catch {
        /* Read-only storage still has a safe in-memory default. */
      }
      return defaults;
    }
  } catch {
    /* Storage may be unavailable in preview/private browsing. */
  }
  const defaults = createDefaultPricing();
  try {
    if (localStorage.getItem(BILLING_STORAGE_KEY))
      for (const model of catalogModels.filter((item) =>
        item.id.startsWith("veyra-demo-"),
      ))
        defaults.models[model.id].tierAvailability = {
          ultra: false,
          pro: false,
          standard: false,
          lite: false,
        };
  } catch {
    /* Storage-unavailable previews contain no migrated account. */
  }
  for (const resource of defaults.resources!.instances)
    defaults.placements![resource.id].groupIds = Object.keys(
      defaults.groups,
    ).filter(
      (id) => defaults.models[resource.canonicalModel].tierAvailability[id],
    );
  return defaults;
}
export type PricingSaveResult =
  | { ok: true; config: PricingCatalogConfig }
  | { ok: false; reason: "invalid" | "forbidden" | "conflict" | "storage" };
export const useModelPricing = create<{
  config: PricingCatalogConfig;
  save: (
    draft: PricingCatalogConfig,
    role: SystemRole,
    expectedRevision: string,
  ) => PricingSaveResult;
}>((set, get) => ({
  config: readPricing(),
  save: (draft, role, expectedRevision) => {
    // Demo role guard only. A production service must authorize and validate server-side.
    if (!isAdmin(role)) return { ok: false, reason: "forbidden" };
    if (
      !validatePricing(draft) ||
      Object.values(draft.models).some(
        (entry) =>
          (entry.effectiveAt && Date.parse(entry.effectiveAt) > Date.now()) ||
          (entry.verifiedAt && Date.parse(entry.verifiedAt) > Date.now()) ||
          (entry.source === "manual-official" &&
            (!entry.sourceUrl || !entry.verifiedAt)),
      )
    )
      return { ok: false, reason: "invalid" };
    draft = materializeAvailability(draft);
    const stored = readPricing();
    if (
      expectedRevision !== get().config.revision ||
      expectedRevision !== stored.revision
    ) {
      set({ config: stored });
      return { ok: false, reason: "conflict" };
    }
    const config: PricingCatalogConfig = {
      ...structuredClone(draft),
      revision: `price-${crypto.randomUUID()}`,
      updatedAt: new Date().toISOString(),
    };
    for (const [id, entry] of Object.entries(config.models)) {
      if (JSON.stringify(entry) !== JSON.stringify(stored.models[id]))
        entry.version = config.revision;
    }
    try {
      localStorage.setItem(PRICING_STORAGE_KEY, JSON.stringify(config));
    } catch {
      return { ok: false, reason: "storage" };
    }
    set({ config });
    // Best-effort sync of model pricing ratios to the backend so they
    // survive across sessions and instances. A failure is logged but
    // does not fail the local save (localStorage is the primary store
    // for the full config; the backend only gets the ratios).
    syncRatiosToBackend(config.models);
    return { ok: true, config };
  },
}));
if (typeof window !== "undefined") {
  window.addEventListener("storage", (event) => {
    if (event.key === PRICING_STORAGE_KEY || event.key === null)
      useModelPricing.setState({ config: readPricing() });
  });
}

// syncRatiosToBackend pushes the model pricing ratios to the backend's
// option store (PUT /api/option) so relay billing and other instances
// see the same prices. Best-effort: failures are logged and never
// affect the local save flow.
function syncRatiosToBackend(models: Record<string, ModelPricingEntry>) {
  try {
    const modelRatio = buildModelRatioJson(models);
    const completionRatio = buildCompletionRatioJson(models);
    const headers = {
      "Content-Type": "application/json",
      Accept: "application/json",
    };
    // Fire-and-forget with basic error logging.
    fetch("/api/option", {
      method: "PUT",
      credentials: "same-origin",
      headers,
      body: JSON.stringify({ key: "model_ratio", value: modelRatio }),
    }).catch((err) =>
      console.warn("model_ratio sync to backend failed:", err),
    );
    fetch("/api/option", {
      method: "PUT",
      credentials: "same-origin",
      headers,
      body: JSON.stringify({ key: "completion_ratio", value: completionRatio }),
    }).catch((err) =>
      console.warn("completion_ratio sync to backend failed:", err),
    );
  } catch (err) {
    console.warn("pricing ratio sync failed:", err);
  }
}

export function getPricingSnapshot() {
  return useModelPricing.getState().config;
}
export function getModelsForTier(
  tierId: TierId,
  config = getPricingSnapshot(),
) {
  return modelCatalog(config).filter(
    (model) =>
      model.online &&
      config.groups[tierId]?.enabled &&
      config.models[model.id]?.tierAvailability[tierId] &&
      (!config.resources ||
        groupResources(config, tierId, model.id).length > 0),
  );
}
export function getModelRate(
  modelId: string,
  tierId: TierId,
  config = getPricingSnapshot(),
) {
  const model = modelCatalog(config).find((item) => item.id === modelId);
  if (!model || !config.groups[tierId]) throw new Error("模型或服务分组不存在");
  const rate = getModelTierPrices(model, config).find(
    (item) => item.tier.id === tierId,
  )!;
  return {
    available: Boolean(rate?.available),
    input: rate.input,
    output: rate.output,
    cacheRead: rate.cacheRead,
    cacheWrite: rate.cacheWrite,
    multiplier: rate.tier.multiplier,
    revision: config.revision,
  };
}
export interface PricingQuoteSnapshot {
  modelId: string;
  tierId: TierId;
  rule: PricingRule;
  pricing: ModelPricing;
  usage: MeteredUsage;
  groupMultiplier: string;
  timeMultiplier: string;
  matchedWindow: ReturnType<typeof quotePricingRule>["matchedWindow"];
  billedQuantity?: string;
  attributes?: PricingAttributes;
  matchedCondition?: ReturnType<typeof quotePricingRule>["matchedCondition"];
  revision: string;
  quotedAt: string;
  source: ModelPricingEntry["source"];
  sourceUrl: string;
  verifiedAt: string | null;
  effectiveAt: string | null;
  version: string;
}
export function quoteModelCharge(
  input: {
    modelId: string;
    tierId: TierId;
    usage: MeteredUsage;
    attributes?: PricingAttributes;
    at?: string;
  },
  config = getPricingSnapshot(),
) {
  const rate = getModelRate(input.modelId, input.tierId, config);
  if (!rate.available)
    throw new Error("Model unavailable in this group（此分组未开放该模型）");
  const entry = config.models[input.modelId];
  const quotedAt = input.at ?? new Date().toISOString();
  const result = quotePricingRule(
    entry.rule,
    entry.pricing,
    input.usage,
    rate.multiplier,
    quotedAt,
    input.attributes,
  );
  const snapshot: PricingQuoteSnapshot = structuredClone({
    modelId: input.modelId,
    tierId: input.tierId,
    rule: entry.rule,
    pricing: result.effectivePricing,
    usage: input.usage,
    groupMultiplier: rate.multiplier,
    timeMultiplier: result.timeMultiplier,
    matchedWindow: result.matchedWindow,
    billedQuantity: result.billedQuantity,
    attributes: input.attributes,
    matchedCondition: result.matchedCondition,
    revision: config.revision,
    quotedAt,
    source: entry.source,
    sourceUrl: entry.sourceUrl,
    verifiedAt: entry.verifiedAt,
    effectiveAt: entry.effectiveAt,
    version: entry.version,
  });
  const effectivePrice = (value: string | undefined) =>
    value === undefined
      ? undefined
      : multiplyDecimalPrice(
          multiplyDecimalPrice(value, rate.multiplier),
          result.timeMultiplier,
        );
  const effectiveRate = {
    ...rate,
    input: effectivePrice(result.effectivePricing.input_per_million_usd)!,
    output: effectivePrice(result.effectivePricing.output_per_million_usd)!,
    cacheRead: effectivePrice(
      result.effectivePricing.cache_read_per_million_usd,
    ),
    cacheWrite: effectivePrice(
      result.effectivePricing.cache_write_per_million_usd,
    ),
  };
  return { amountNano: result.amountNano, rate: effectiveRate, snapshot };
}
/** Compatibility wrapper; all callers settle through the generic quotation engine. */
export function estimateModelCharge(
  input: {
    modelId: string;
    tierId: TierId;
    inputTokens: number;
    outputTokens: number;
    cacheReadTokens?: number;
    cacheWriteTokens?: number;
    at?: string;
  },
  config = getPricingSnapshot(),
) {
  for (const tokens of [
    input.inputTokens,
    input.outputTokens,
    input.cacheReadTokens ?? 0,
    input.cacheWriteTokens ?? 0,
  ])
    if (!Number.isSafeInteger(tokens) || tokens < 0)
      throw new Error("Invalid token count（无效词元用量）");
  return quoteModelCharge(
    {
      modelId: input.modelId,
      tierId: input.tierId,
      at: input.at,
      usage: {
        input_tokens: input.inputTokens,
        output_tokens: input.outputTokens,
        cache_read_tokens: input.cacheReadTokens ?? 0,
        cache_write_tokens: input.cacheWriteTokens ?? 0,
        requests: 1,
      },
    },
    config,
  );
}

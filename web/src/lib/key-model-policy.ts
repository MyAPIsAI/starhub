import { type ModelItem, type ResourceInstance } from "../config/models";
import type { TierId } from "../config/placeholders";
import {
  getModelsForTier,
  quoteModelCharge,
  getPricingSnapshot,
} from "./model-pricing";
import type { DemoCapability, DemoProtocol } from "./request-contract";

import {
  accountCanUseGroup,
  modelCatalog,
  groupResources,
} from "./platform-catalog";

export interface KeyModelPolicy {
  accountId?: string;
  tier: TierId;
  model: string;
  type: "specified" | "client" | "smart";
  modelAccess?: "all" | "custom";
  allowedModels?: string[];
  smartAccess?: boolean;
  smartStrategy?: "cost" | "latency" | "stability";
}

import { SMART_MODEL_ID, isSmartModelId } from "./smart-model";
export { SMART_MODEL_ID, isSmartModelId } from "./smart-model";
export const allowsSmartModel = (key: KeyModelPolicy) =>
  key.smartAccess ?? key.type === "smart";
export const allowsSpecifiedModels = (key: KeyModelPolicy) =>
  key.smartAccess !== undefined || key.type !== "smart";

/** Group availability and the saved allowlist bound both direct calls and routing. */
export function getKeyModels(
  key: KeyModelPolicy,
  config = getPricingSnapshot(),
) {
  if (!accountCanUseGroup(config, key.tier, key.accountId)) return [];
  const models = getModelsForTier(key.tier, config);
  if (key.modelAccess === "all") return models;
  const allowlist = key.allowedModels ?? [key.model];
  return models.filter((model) => allowlist.includes(model.id));
}

export function getKeyModelOptions(
  key: KeyModelPolicy,
  protocol?: DemoProtocol,
  config = getPricingSnapshot(),
) {
  const candidates = getKeyModels(key, config).filter(
    (model) =>
      model.protocols.length > 0 &&
      (!protocol ||
        (model.protocols.includes(protocol) &&
          (!config.resources ||
            groupResources(config, key.tier, model.id).some((resource) =>
              resource.protocols.includes(protocol),
            )))),
  );
  const options: { id: string; name: string; kind: "specified" | "smart" }[] =
    allowsSpecifiedModels(key)
      ? candidates.map((model) => ({
          id: model.id,
          name: model.name,
          kind: "specified",
        }))
      : [];
  if (allowsSmartModel(key) && candidates.length)
    options.push({
      id: SMART_MODEL_ID,
      name: "MyAPIs Auto（智能路由）",
      kind: "smart",
    });
  return options;
}
export type ModelResolutionError =
  | "group_unauthorized"
  | "resource_unavailable"
  | "unknown_model"
  | "model_unavailable"
  | "model_unauthorized"
  | "protocol_incompatible"
  | "capability_incompatible"
  | "tool_chain_incompatible"
  | "pricing_invalid";
export interface ModelRequirements {
  protocol?: DemoProtocol;
  capability?: DemoCapability;
  chainModel?: string;
  inputTokens?: number;
  maxTokens?: number;
  at?: string;
}
export function resolveKeyModelResult(
  key: KeyModelPolicy,
  requestedModel?: string,
  config = getPricingSnapshot(),
  requirements: ModelRequirements = {},
):
  | { model: ModelItem; resource?: ResourceInstance; error?: never }
  | { model?: never; resource?: never; error: ModelResolutionError } {
  if (!accountCanUseGroup(config, key.tier, key.accountId))
    return { error: "group_unauthorized" };
  const requested =
    requestedModel ??
    (key.type === "smart" && key.smartAccess === undefined
      ? SMART_MODEL_ID
      : key.model);
  const smart = isSmartModelId(requested);
  const existing = modelCatalog(config).find((model) => model.id === requested);
  if (!smart && !existing) return { error: "unknown_model" };
  if (!smart && !existing!.online) return { error: "model_unavailable" };
  if (
    (smart && !allowsSmartModel(key)) ||
    (!smart && !allowsSpecifiedModels(key))
  )
    return { error: "model_unauthorized" };
  let candidates = modelCatalog(config)
    .filter(
      (model) =>
        model.online &&
        config.models[model.id]?.tierAvailability[key.tier] &&
        (key.modelAccess === "all" ||
          (key.allowedModels ?? [key.model]).includes(model.id)),
    )
    .filter((model) =>
      smart
        ? model.protocols.length > 0 && model.capabilities.includes("text")
        : model.id === requested,
    );
  if (!candidates.length) return { error: "model_unauthorized" };
  if (requirements.protocol)
    candidates = candidates.filter((model) =>
      model.protocols.includes(requirements.protocol!),
    );
  if (!candidates.length) return { error: "protocol_incompatible" };
  if (requirements.capability)
    candidates = candidates.filter((model) =>
      model.capabilities.includes(requirements.capability!),
    );
  candidates = candidates.filter(
    (model) =>
      (requirements.inputTokens ?? 0) + (requirements.maxTokens ?? 0) <=
      model.contextLength,
  );
  if (!candidates.length) return { error: "capability_incompatible" };
  if (requirements.chainModel) {
    candidates = candidates.filter(
      (model) => model.id === requirements.chainModel,
    );
    if (!candidates.length) return { error: "tool_chain_incompatible" };
  }
  const resources = new Map<string, ResourceInstance>();
  if (config.resources) {
    candidates = candidates.filter((model) => {
      const selected = groupResources(config, key.tier, model.id)
        .filter(
          (resource) =>
            (!requirements.protocol ||
              resource.protocols.includes(requirements.protocol)) &&
            (!requirements.capability ||
              resource.capabilities.includes(requirements.capability)) &&
            (requirements.inputTokens ?? 0) + (requirements.maxTokens ?? 0) <=
              resource.contextLength,
        )
        .sort((a, b) => a.latencyMs - b.latencyMs)[0];
      if (selected) resources.set(model.id, selected);
      return Boolean(selected);
    });
    if (!candidates.length) return { error: "resource_unavailable" };
  }
  const estimatedCosts = new Map<string, bigint>();
  if (smart) {
    candidates = candidates.filter((model) => {
      try {
        const quote = quoteModelCharge(
          {
            modelId: model.id,
            tierId: key.tier,
            at: requirements.at,
            usage: {
              input_tokens: requirements.inputTokens ?? 620,
              output_tokens: requirements.maxTokens ?? 148,
              cache_read_tokens: 0,
              cache_write_tokens: 0,
              requests: 1,
            },
          },
          config,
        );
        estimatedCosts.set(model.id, BigInt(quote.amountNano));
        return true;
      } catch {
        return false;
      }
    });
    if (!candidates.length) return { error: "pricing_invalid" };
  }
  if (smart && (!key.smartStrategy || key.smartStrategy === "cost")) {
    candidates.sort((a, b) => {
      const left = estimatedCosts.get(a.id)!,
        right = estimatedCosts.get(b.id)!;
      return left < right ? -1 : left > right ? 1 : 0;
    });
  } else if (smart) {
    // These are deterministic demonstration policies, never measured upstream routing.
    candidates.sort(
      (a, b) => Number(b.id === key.model) - Number(a.id === key.model),
    );
  }
  return { model: candidates[0], resource: resources.get(candidates[0].id) };
}
/** Compatibility helper for existing views. Explicit model IDs never fall back. */
export function resolveKeyModel(
  key: KeyModelPolicy,
  requestedModel?: string,
  config = getPricingSnapshot(),
  requirements: ModelRequirements = {},
) {
  return (
    resolveKeyModelResult(key, requestedModel, config, requirements).model ??
    null
  );
}

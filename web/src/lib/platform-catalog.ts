import {
  catalogModels,
  type ModelItem,
  type PricingCatalogConfig,
  type ResourceSnapshot,
  type ResourceInstance,
} from "../config/models";
import { placeholders } from "../config/placeholders";
import {
  SMART_MODEL_ID,
  LEGACY_SMART_MODEL_ID,
  isSmartModelId,
} from "./smart-model";
export { isSmartModelId } from "./smart-model";

export const demoAccounts = [
  { id: "account-demo", name: "当前演示账号" },
  { id: "account-partner", name: "定制客户 · 星河科技" },
];
/** Routing entry IDs are reserved; neither is a physical upstream model. */

export function modelCatalog(config?: PricingCatalogConfig): ModelItem[] {
  const models = config?.catalog
    ? Object.values(config.catalog)
    : catalogModels;
  return models.filter((model) => !isSmartModelId(model.id));
}
export function serviceGroups(config: PricingCatalogConfig) {
  return Object.entries(config.groups).map(([id, group]) => {
    const original = placeholders.tiers.find((tier) => tier.id === id);
    return {
      id,
      ...group,
      multiplier: config.multipliers[id],
      mark: original?.mark ?? 1,
      name: {
        zh: group.name,
        en: group.name === original?.name.zh ? original.name.en : group.name,
        ar: group.name === original?.name.zh ? original.name.ar : group.name,
      },
    };
  });
}
export function accountCanUseGroup(
  config: PricingCatalogConfig,
  id: string,
  accountId = "account-demo",
) {
  const group = config.groups[id];
  return Boolean(
    group?.enabled &&
    (group.visibility !== "custom" || group.accountIds?.includes(accountId)),
  );
}
export const visibleGroups = (
  config: PricingCatalogConfig,
  accountId = "account-demo",
) =>
  serviceGroups(config).filter((group) =>
    accountCanUseGroup(config, group.id, accountId),
  );

export function initialResources(): ResourceSnapshot {
  return {
    id: "resource-sample-1",
    sequence: 1,
    publishedAt: "2026-09-26T00:00:00Z",
    receivedAt: new Date().toISOString(),
    source: "资源管理平台 · 本地模拟",
    channels: [
      {
        id: "channel-1",
        name: "渠道供应商 1",
        provider: "多模型供应商（演示）",
        enabled: true,
      },
      {
        id: "channel-2",
        name: "渠道供应商 2",
        provider: "备用资源供应商（演示）",
        enabled: true,
      },
    ],
    instances: catalogModels.flatMap((model, index) => {
      if (isSmartModelId(model.id)) return [];
      const create = (suffix: string, channelId: string): ResourceInstance => ({
        id: `resource-${index + 1}-${suffix}`,
        channelId,
        upstreamModel: model.id,
        canonicalModel: model.id,
        protocols: [...model.protocols],
        capabilities: [...model.capabilities],
        contextLength: model.contextLength,
        region: "演示区域",
        status: "available",
        latencyMs: 300 + index * 40,
      });
      return index < 2
        ? [create("a", "channel-1"), create("b", "channel-2")]
        : [create("a", "channel-1")];
    }),
  };
}
/** Only initial migration derives placements from the old model/group permission matrix. */
export function upgradePlatform(
  config: PricingCatalogConfig,
): PricingCatalogConfig {
  const next = structuredClone(config);
  next.catalog ??= Object.fromEntries(
    catalogModels
      .filter((model) => next.models[model.id])
      .map((model) => [model.id, structuredClone(model)]),
  );
  const legacy = next.catalog[LEGACY_SMART_MODEL_ID];
  if (legacy) {
    next.catalog[SMART_MODEL_ID] ??= { ...legacy, id: SMART_MODEL_ID };
    delete next.catalog[LEGACY_SMART_MODEL_ID];
  }
  if (next.models[LEGACY_SMART_MODEL_ID]) {
    next.models[SMART_MODEL_ID] ??= next.models[LEGACY_SMART_MODEL_ID];
    delete next.models[LEGACY_SMART_MODEL_ID];
  }
  // Virtual entries never represent upstream resources or placements.
  if (next.resources) {
    const virtualIds = new Set(
      next.resources.instances
        .filter(
          (resource) =>
            isSmartModelId(resource.canonicalModel) ||
            isSmartModelId(resource.upstreamModel),
        )
        .map((resource) => resource.id),
    );
    next.resources.instances = next.resources.instances.filter(
      (resource) => !virtualIds.has(resource.id),
    );
    if (next.placements)
      for (const id of virtualIds) delete next.placements[id];
  }
  if (next.placements)
    for (const placement of Object.values(next.placements)) {
      if (placement.modelId && isSmartModelId(placement.modelId)) {
        placement.modelId = null;
        placement.enabled = false;
      }
    }
  for (const group of Object.values(next.groups)) {
    group.visibility ??= "common";
    group.accountIds ??= [];
  }
  if (!next.resources) {
    next.resources = initialResources();
    next.placements = Object.fromEntries(
      next.resources.instances.map((resource) => [
        resource.id,
        {
          modelId: resource.canonicalModel,
          groupIds: Object.keys(next.groups).filter(
            (group) =>
              next.models[resource.canonicalModel]?.tierAvailability[group],
          ),
          enabled: true,
        },
      ]),
    );
  }
  next.placements ??= {};
  return next;
}
export function groupResources(
  config: PricingCatalogConfig,
  groupId: string,
  modelId?: string,
  availableOnly = true,
) {
  return (config.resources?.instances ?? []).filter((resource) => {
    const placement = config.placements?.[resource.id];
    const channel = config.resources?.channels.find(
      (item) => item.id === resource.channelId,
    );
    return (
      placement?.enabled &&
      placement.groupIds.includes(groupId) &&
      (!modelId || placement.modelId === modelId) &&
      (!availableOnly || (resource.status === "available" && channel?.enabled))
    );
  });
}
export function materializeAvailability(config: PricingCatalogConfig) {
  if (!config.resources) return config;
  const next = structuredClone(config);
  for (const [modelId, entry] of Object.entries(next.models)) {
    entry.tierAvailability = Object.fromEntries(
      Object.keys(next.groups).map((id) => [
        id,
        groupResources(next, id, modelId, false).length > 0,
      ]),
    );
  }
  return next;
}
export function resourceSnapshotError(data: unknown): string | null {
  if (!data || typeof data !== "object") return "快照不是有效对象";
  const value = data as ResourceSnapshot;
  if (
    !value.id ||
    !Number.isSafeInteger(value.sequence) ||
    value.sequence < 1 ||
    !Number.isFinite(Date.parse(value.publishedAt)) ||
    !Array.isArray(value.channels) ||
    !Array.isArray(value.instances)
  )
    return "快照版本、时间或列表无效";
  if (
    new Set(value.channels.map((c) => c?.id)).size !== value.channels.length ||
    new Set(value.instances.map((r) => r?.id)).size !== value.instances.length
  )
    return "外部标识重复";
  if (
    value.channels.some(
      (c) => !c || !c.id || !c.name || typeof c.enabled !== "boolean",
    )
  )
    return "渠道字段无效";
  if (
    value.instances.some(
      (r) =>
        !r ||
        !r.id ||
        !r.upstreamModel ||
        !r.canonicalModel ||
        !value.channels.some((c) => c.id === r.channelId) ||
        !["available", "unavailable", "retired"].includes(r.status) ||
        !Array.isArray(r.protocols) ||
        r.protocols.some(
          (p) => !["OpenAI", "Anthropic", "Gemini"].includes(p),
        ) ||
        !Array.isArray(r.capabilities) ||
        r.capabilities.some(
          (p) =>
            !["text", "code", "vision", "reasoning", "image", "video"].includes(
              p,
            ),
        ) ||
        !Number.isFinite(r.latencyMs) ||
        r.latencyMs < 0 ||
        !Number.isFinite(r.contextLength) ||
        r.contextLength < 0,
    )
  )
    return "资源实例引用、协议或能力字段无效";
  return null;
}
export function applyResourceSnapshot(
  config: PricingCatalogConfig,
  snapshot: ResourceSnapshot,
) {
  const error = resourceSnapshotError(snapshot);
  if (error) throw new Error(error);
  if (snapshot.sequence <= (config.resources?.sequence ?? 0))
    throw new Error("快照版本过旧或重复；现有快照未改变");
  const next = structuredClone(config);
  next.resources = {
    ...structuredClone(snapshot),
    receivedAt: new Date().toISOString(),
  };
  // New IDs have no local model mapping or group placement; source sync never expands access.
  for (const resource of snapshot.instances)
    next.placements![resource.id] ??= {
      modelId: null,
      groupIds: [],
      enabled: false,
    };
  for (const resource of snapshot.instances) {
    const old = config.resources?.instances.find(
      (item) => item.id === resource.id,
    );
    if (
      old &&
      (old.canonicalModel !== resource.canonicalModel ||
        old.upstreamModel !== resource.upstreamModel)
    )
      next.placements![resource.id].enabled = false;
  }
  return materializeAvailability(next);
}
export function nextDemoSnapshot(
  config: PricingCatalogConfig,
  mode: "add" | "outage" | "restore",
): ResourceSnapshot {
  const snapshot = structuredClone(config.resources!);
  snapshot.sequence++;
  snapshot.id = `resource-sample-${snapshot.sequence}`;
  snapshot.publishedAt = new Date().toISOString();
  if (mode === "add")
    snapshot.instances.push({
      id: `resource-import-${snapshot.sequence}`,
      channelId: "channel-1",
      upstreamModel: "supplier-text-next",
      canonicalModel: "demo-text-next",
      protocols: ["OpenAI"],
      capabilities: ["text", "code"],
      contextLength: 128000,
      region: "演示区域",
      status: "available",
      latencyMs: 280,
    });
  else
    for (const resource of snapshot.instances)
      resource.status = mode === "outage" ? "unavailable" : "available";
  return snapshot;
}

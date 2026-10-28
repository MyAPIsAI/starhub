import type { ModelItem, PricingCatalogConfig } from "../config/models";
import type { PreviewKey, PreviewRequest } from "./product-preview";
import { isSmartModelId, materializeAvailability } from "./platform-catalog";

export function addStandardModel(
  config: PricingCatalogConfig,
  model: ModelItem,
  price: string,
): PricingCatalogConfig {
  if (
    !/^[a-z0-9][a-z0-9._:/-]{1,99}$/.test(model.id) ||
    isSmartModelId(model.id)
  )
    throw new Error(
      "调用标识须为 2–100 位小写英文、数字或 . _ : / -，且不能占用智能入口",
    );
  if (config.models[model.id])
    throw new Error("调用标识已存在，请编辑已有标准模型");
  if (
    !model.name.trim() ||
    !model.vendor.trim() ||
    !/^\d+(\.\d{1,6})?$/.test(price)
  )
    throw new Error("请填写模型名称、厂商和有效价格");
  const next = structuredClone(config);
  next.catalog![model.id] = structuredClone(model);
  next.models[model.id] = {
    pricing: { ...model.pricing },
    rule: model.capabilities.includes("image")
      ? {
          mode: "unit",
          unit: "image",
          priceUsd: price,
          minimum: "0",
          ceilTo: "1",
        }
      : model.capabilities.includes("video")
        ? {
            mode: "unit",
            unit: "second",
            priceUsd: price,
            minimum: "0",
            ceilTo: "0",
          }
        : { mode: "tokens" },
    tierAvailability: Object.fromEntries(
      Object.keys(config.groups).map((id) => [id, false]),
    ),
    source: "sample",
    sourceUrl: "",
    verifiedAt: null,
    effectiveAt: null,
    version: "draft",
  };
  return next;
}
export function removeStandardModel(
  config: PricingCatalogConfig,
  id: string,
  keys: PreviewKey[],
  requests: PreviewRequest[],
) {
  if (
    Object.values(config.placements ?? {}).some((p) => p.modelId === id) ||
    keys.some((k) => k.model === id || k.allowedModels?.includes(id)) ||
    requests.some(
      (r) =>
        r.model === id || r.requested_model === id || r.actual_model === id,
    )
  )
    throw new Error(
      "此模型存在资源映射、密钥或历史调用引用。请先解除引用，或停用模型；历史账单不会删除。",
    );
  const next = structuredClone(config);
  delete next.catalog![id];
  delete next.models[id];
  return next;
}
export function removeServiceGroup(
  config: PricingCatalogConfig,
  id: string,
  keys: PreviewKey[],
  requests: PreviewRequest[],
) {
  if (
    keys.some((k) => k.tier === id) ||
    requests.some((r) => r.tier === id) ||
    Object.values(config.placements ?? {}).some((p) => p.groupIds.includes(id))
  )
    throw new Error(
      "此分组存在密钥、调用或实例投放引用。请解除引用后删除；也可先停用，历史账单保持不变。",
    );
  const next = structuredClone(config);
  delete next.groups[id];
  delete next.multipliers[id];
  return materializeAvailability(next);
}

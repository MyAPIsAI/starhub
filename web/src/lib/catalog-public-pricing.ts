import {
  directoryPrices,
  getDirectoryModel,
  type DirectoryModel,
} from "../config/model-directory";
import type { PricingCatalogConfig } from "../config/models";
import type { TierId } from "../config/placeholders";

export type PublicCatalogPriceStatus =
  | "ready"
  | "unknown"
  | "offline"
  | "no-public-group"
  | "no-resource"
  | "quote-error";

export interface PublicCatalogPrice {
  status: PublicCatalogPriceStatus;
  /** Smart ranges describe eligible candidates in the selected public group only. */
  mode: "tokens" | "unit" | "expression" | "smart-group-estimate" | null;
  groupId: TierId | null;
  groupName: string | null;
  price: ReturnType<typeof directoryPrices> | null;
  quotedAt: string;
  publicGroupCount: number;
  quotedGroupCount: number;
  /** Comparison uses the complete example charge, never independently picked rate fields. */
  comparison: "example-total";
  errors: { groupId: string | null; modelId: string | null; message: string }[];
}

function compareMultipliers(left: string, right: string) {
  const [leftWhole, leftFraction = ""] = left.split(".");
  const [rightWhole, rightFraction = ""] = right.split(".");
  const scale = Math.max(leftFraction.length, rightFraction.length);
  const a = BigInt(leftWhole + leftFraction.padEnd(scale, "0"));
  const b = BigInt(rightWhole + rightFraction.padEnd(scale, "0"));
  return a < b ? -1 : a > b ? 1 : 0;
}

/**
 * Public advertising must not inherit a signed-in account's custom prices.
 * This read-only comparison shares the directory/settlement quotation engine,
 * uses one instant and keeps the selected group's whole rate/assumption set.
 * It is an example-based floor, not a promise for every usage or time window.
 */
export function getPublicCatalogPrice(
  requestedModel: DirectoryModel | undefined,
  config: PricingCatalogConfig,
  at = new Date().toISOString(),
): PublicCatalogPrice {
  const result: PublicCatalogPrice = {
    status: "unknown",
    mode: null,
    groupId: null,
    groupName: null,
    price: null,
    quotedAt: at,
    publicGroupCount: 0,
    quotedGroupCount: 0,
    comparison: "example-total",
    errors: [],
  };
  // Resolve current catalog state rather than trusting a stale card object.
  const model = getDirectoryModel(requestedModel?.id, config);
  if (!model) return result;
  if (!model.online) return { ...result, status: "offline" };

  const publicGroups = Object.entries(config.groups)
    .filter(([, group]) => group.enabled && group.visibility !== "custom")
    .sort(([left], [right]) => left.localeCompare(right));
  result.publicGroupCount = publicGroups.length;
  if (!publicGroups.length) return { ...result, status: "no-public-group" };
  // Legacy quotation permits missing snapshots. A public offer requires actual
  // mapped, available resources; do not publish the legacy permissive fallback.
  if (!config.resources) return { ...result, status: "no-resource" };
  if (!Number.isFinite(Date.parse(at)))
    return {
      ...result,
      status: "quote-error",
      errors: [{ groupId: null, modelId: model.id, message: "报价时间无效" }],
    };

  const quotes: {
    groupId: string;
    groupName: string;
    price: ReturnType<typeof directoryPrices>;
  }[] = [];
  for (const [groupId, group] of publicGroups) {
    try {
      // Empty identity is deliberate: no historical demo-account privileges.
      const price = directoryPrices(model, groupId, config, at, "");
      if (!price.available) continue;
      for (const rate of price.rates)
        if (rate.error)
          result.errors.push({
            groupId,
            modelId: rate.model.id,
            message: rate.error,
          });
      if (price.canEstimate)
        quotes.push({ groupId, groupName: group.name, price });
    } catch (error) {
      result.errors.push({
        groupId,
        modelId: model.id,
        message: error instanceof Error ? error.message : String(error),
      });
    }
  }
  result.quotedGroupCount = quotes.length;
  // An unquoted eligible option could be cheaper. Do not call a partial result
  // the public minimum, or hide an expression failure behind a free-price fallback.
  if (result.errors.length) return { ...result, status: "quote-error" };
  if (!quotes.length) return { ...result, status: "no-resource" };
  quotes.sort((left, right) => {
    const a = BigInt(left.price.minAmountNano!);
    const b = BigInt(right.price.minAmountNano!);
    if (a !== b) return a < b ? -1 : 1;
    // Settlement rounds to nano-dollars. A tie must not choose a higher token
    // rate merely because its group ID sorts first.
    return (
      compareMultipliers(
        config.multipliers[left.groupId],
        config.multipliers[right.groupId],
      ) || left.groupId.localeCompare(right.groupId)
    );
  });
  const selected = quotes[0];
  return {
    ...result,
    ...selected,
    status: "ready",
    mode:
      model.kind === "smart"
        ? "smart-group-estimate"
        : selected.price.rates[0].rule.mode,
  };
}

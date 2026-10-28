import type { DirectoryModel } from "../config/model-directory";
import type { PricingCatalogConfig } from "../config/models";
import type { TierId } from "../config/placeholders";
import { accountCanUseGroup, visibleGroups } from "./platform-catalog";
import type { PreviewKey } from "./product-preview";

/** Routing belongs to the platform, regardless of the candidate vendors. */
export function catalogVendor(model: DirectoryModel): string {
  return model.kind === "smart" ? "MyAPIs" : model.vendor.trim();
}

/** Counts come from the whole live directory, independently of search results. */
export function getCatalogVendors(models: readonly DirectoryModel[]) {
  const counts = new Map<string, number>();
  for (const model of models) {
    const vendor = catalogVendor(model);
    counts.set(vendor, (counts.get(vendor) ?? 0) + 1);
  }
  return Array.from(counts, ([id, count]) => ({ id, count }));
}

export function filterCatalogModels(
  models: readonly DirectoryModel[],
  { vendor, query = "" }: { vendor?: string; query?: string },
) {
  const search = query.trim().toLocaleLowerCase();
  return models.filter(
    (model) =>
      (vendor === undefined || catalogVendor(model) === vendor) &&
      `${model.name} ${model.id} ${model.vendor} ${catalogVendor(model)}`
        .toLocaleLowerCase()
        .includes(search),
  );
}

export type CatalogPricingKey = Pick<
  PreviewKey,
  "id" | "name" | "tier" | "active" | "accountId" | "validUntil"
>;

export interface CatalogPricingContext {
  tier: TierId | null;
  key: CatalogPricingKey | null;
  source: "key" | "default" | "unavailable";
}

/**
 * Browsing uses a current usable key's rate context, without selecting a model
 * or changing its authorization. Ownerless historical keys belong only to the
 * original demo account, never to whichever account happens to sign in later.
 */
export function resolveCatalogPricingContext(
  config: PricingCatalogConfig,
  accountId: string | undefined,
  keys: readonly CatalogPricingKey[],
  selectedKey: string,
  now = Date.now(),
): CatalogPricingContext {
  // Pass an explicit empty identity for signed-out/unknown accounts. The group
  // helpers' legacy account-demo default must not grant a custom group here.
  const identity = accountId ?? "";
  const key = keys.find((item) => {
    const expiry = item.validUntil ? Date.parse(item.validUntil) : null;
    return (
      item.id === selectedKey &&
      item.active &&
      (item.accountId ?? "account-demo") === identity &&
      (expiry === null || (Number.isFinite(expiry) && expiry > now)) &&
      accountCanUseGroup(config, item.tier, identity)
    );
  });
  if (key) return { tier: key.tier, key, source: "key" };

  const groups = visibleGroups(config, identity);
  const group = groups.find((item) => item.id === "standard") ?? groups[0];
  return group
    ? { tier: group.id, key: null, source: "default" }
    : { tier: null, key: null, source: "unavailable" };
}

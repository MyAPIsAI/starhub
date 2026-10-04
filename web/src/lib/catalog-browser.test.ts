import { describe, expect, it } from "vitest";
import {
  getDirectoryModels,
  smartDirectoryModel,
  type DirectoryModel,
} from "../config/model-directory";
import {
  catalogVendor,
  filterCatalogModels,
  getCatalogVendors,
  resolveCatalogPricingContext,
  type CatalogPricingKey,
} from "./catalog-browser";
import { createDefaultPricing } from "./model-pricing";

const now = Date.parse("2026-09-27T12:00:00Z");
const selectedKey: CatalogPricingKey = {
  id: "key-current",
  name: "当前用途",
  tier: "pro",
  active: true,
  accountId: "account-current",
};

describe("vendor directory navigation", () => {
  it("uses an omitted vendor for all results without reserving a real vendor named all", () => {
    const models = getDirectoryModels(createDefaultPricing());
    const vendorNamedAll = {
      ...models.find((model) => model.id === "gpt-4.1")!,
      id: "vendor-all-model",
      vendor: "all",
    };
    const directory = [...models, vendorNamedAll];
    expect(filterCatalogModels(directory, {})).toEqual(directory);
    expect(filterCatalogModels(directory, { vendor: undefined })).toEqual(
      directory,
    );
    expect(filterCatalogModels(directory, { vendor: "all" })).toEqual([
      vendorNamedAll,
    ]);
    expect(filterCatalogModels(directory, { vendor: "" })).toEqual([]);
  });
  it("derives vendors and counts from the current catalog, including new providers", () => {
    const config = createDefaultPricing();
    const models = getDirectoryModels(config);
    const added = {
      ...models.find((model) => model.id === "gpt-4.1")!,
      id: "new-provider-model",
      vendor: "New Provider",
    };
    const vendors = getCatalogVendors([...models, added]);
    expect(vendors.find((vendor) => vendor.id === "Veyra")).toEqual({
      id: "Veyra",
      count: 3,
    });
    expect(vendors.find((vendor) => vendor.id === "New Provider")).toEqual({
      id: "New Provider",
      count: 1,
    });
    expect(vendors.reduce((count, vendor) => count + vendor.count, 0)).toBe(
      models.length + 1,
    );
    expect(
      getCatalogVendors(models).some((vendor) => vendor.id === added.vendor),
    ).toBe(false);
  });

  it("places routing and Veyra sample models together without a model-kind filter", () => {
    const models = getDirectoryModels(createDefaultPricing());
    const matches = filterCatalogModels(models, { vendor: "Veyra" });
    expect(matches.map((model) => model.id)).toEqual([
      "veyra-auto",
      "veyra-demo-image",
      "veyra-demo-video",
    ]);
    expect(new Set(matches.map((model) => model.kind))).toEqual(
      new Set(["smart", "specified"]),
    );
    expect(filterCatalogModels(models, {})).toEqual(models);
  });

  it("searches names, IDs, raw provider and platform category with trimmed case-insensitive input", () => {
    const routing: DirectoryModel = {
      ...smartDirectoryModel,
      id: "route-next",
      name: "Adaptive Route",
      vendor: "Candidate Provider",
    };
    const models = [routing];
    expect(catalogVendor(routing)).toBe("Veyra");
    for (const query of [
      " ADAPTIVE ",
      " ROUTE-NEXT ",
      " candidate PROVIDER ",
      " VEYRA ",
    ])
      expect(filterCatalogModels(models, { vendor: "Veyra", query })).toEqual(
        models,
      );
    expect(getCatalogVendors(models)).toEqual([{ id: "Veyra", count: 1 }]);
  });

  it("keeps unknown vendor and empty search results empty instead of falling back", () => {
    const models = getDirectoryModels(createDefaultPricing());
    expect(filterCatalogModels(models, { vendor: "removed-provider" })).toEqual(
      [],
    );
    expect(
      filterCatalogModels(models, { vendor: "OpenAI", query: "Claude" }),
    ).toEqual([]);
    expect(getCatalogVendors([])).toEqual([]);
  });
});

describe("catalog pricing account context", () => {
  it("uses the selected current-account key and authorized custom group before the standard group", () => {
    const config = createDefaultPricing();
    config.groups.pro.visibility = "custom";
    config.groups.pro.accountIds = ["account-current"];
    expect(
      resolveCatalogPricingContext(
        config,
        "account-current",
        [selectedKey],
        selectedKey.id,
        now,
      ),
    ).toEqual({
      tier: "pro",
      key: selectedKey,
      source: "key",
    });
  });

  it.each([
    { label: "another account", change: { accountId: "account-other" } },
    { label: "inactive", change: { active: false } },
    { label: "expired", change: { validUntil: "2026-09-27T11:59:59Z" } },
    {
      label: "expires at the evaluation instant",
      change: { validUntil: "2026-09-27T12:00:00Z" },
    },
    { label: "invalid expiry", change: { validUntil: "invalid-date" } },
  ])("does not use a selected key that is $label", ({ change }) => {
    const config = createDefaultPricing();
    expect(
      resolveCatalogPricingContext(
        config,
        "account-current",
        [{ ...selectedKey, ...change }],
        selectedKey.id,
        now,
      ),
    ).toEqual({
      tier: "standard",
      key: null,
      source: "default",
    });
  });

  it("rechecks group access and does not borrow an unselected key's rate", () => {
    const config = createDefaultPricing();
    config.groups.pro.visibility = "custom";
    config.groups.pro.accountIds = ["account-other"];
    const otherKey = { ...selectedKey, id: "unselected", tier: "ultra" };
    expect(
      resolveCatalogPricingContext(
        config,
        "account-current",
        [selectedKey, otherKey],
        selectedKey.id,
        now,
      ),
    ).toMatchObject({
      tier: "standard",
      key: null,
      source: "default",
    });
    config.groups.pro.visibility = "common";
    config.groups.pro.enabled = false;
    expect(
      resolveCatalogPricingContext(
        config,
        "account-current",
        [selectedKey],
        selectedKey.id,
        now,
      ).tier,
    ).toBe("standard");
  });

  it("allows ownerless legacy keys only for their original demo identity", () => {
    const config = createDefaultPricing();
    const legacyKey = { ...selectedKey, accountId: undefined };
    expect(
      resolveCatalogPricingContext(
        config,
        "account-demo",
        [legacyKey],
        selectedKey.id,
        now,
      ).source,
    ).toBe("key");
    expect(
      resolveCatalogPricingContext(
        config,
        "account-current",
        [legacyKey],
        selectedKey.id,
        now,
      ),
    ).toMatchObject({ tier: "standard", source: "default" });
  });

  it("prefers visible standard, then a visible group, without using the helpers' demo-account default", () => {
    const config = createDefaultPricing();
    config.groups.standard.visibility = "custom";
    config.groups.standard.accountIds = ["account-demo"];
    config.groups.ultra.visibility = "custom";
    config.groups.ultra.accountIds = ["account-demo"];
    expect(
      resolveCatalogPricingContext(config, "account-current", [], "", now).tier,
    ).toBe("pro");
    expect(
      resolveCatalogPricingContext(config, undefined, [], "", now).tier,
    ).toBe("pro");
    expect(
      resolveCatalogPricingContext(config, "account-demo", [], "", now).tier,
    ).toBe("standard");
  });

  it("returns no pricing context when all groups are inaccessible without mutating data", () => {
    const config = createDefaultPricing();
    for (const group of Object.values(config.groups)) {
      group.visibility = "custom";
      group.accountIds = ["account-demo"];
    }
    const before = structuredClone(config);
    const keys = [structuredClone(selectedKey)];
    expect(
      resolveCatalogPricingContext(
        config,
        "account-current",
        keys,
        selectedKey.id,
        now,
      ),
    ).toEqual({ tier: null, key: null, source: "unavailable" });
    expect(
      resolveCatalogPricingContext(config, undefined, [], "", now).tier,
    ).toBeNull();
    expect(config).toEqual(before);
    expect(keys).toEqual([selectedKey]);
  });
});

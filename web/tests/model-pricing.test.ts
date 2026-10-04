import { beforeEach, describe, expect, it, vi } from "vitest";
import { catalogModels, getModelLowestPrice, getModelTierPrices } from "../src/config/models";
import { createDefaultPricing, estimateModelCharge, getModelRate, getModelsForTier, isValidMultiplier, isValidPrice, PRICING_STORAGE_KEY, useModelPricing, validatePricing } from "../src/lib/model-pricing";
import { ROLE_ADMIN } from "../src/lib/system-access";

let storage: Map<string, string>;
beforeEach(() => {
  storage = new Map();
  vi.stubGlobal("localStorage", {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => storage.set(key, value),
  });
  useModelPricing.setState({ config: createDefaultPricing() });
});

describe("published platform pricing", () => {
  it("validates exact decimal limits and keeps an omitted cache rate distinct from free", () => {
    for (const invalid of ["-1", "1e2", "NaN", "Infinity", " 1", ".5", "1.", "1.0000001", "1000000.000001", "", 1]) expect(isValidPrice(invalid)).toBe(false);
    expect(isValidPrice("0")).toBe(true);
    expect(isValidPrice("1000000.000000")).toBe(true);
    for (const invalid of ["0", "0.0000", "-1", "100.0001", "0.00001", "1e1"]) expect(isValidMultiplier(invalid)).toBe(false);
    expect(isValidMultiplier("0.0001")).toBe(true);
    const config = createDefaultPricing();
    delete config.models["gpt-4.1"].pricing.cache_read_per_million_usd;
    config.models["gpt-4.1"].pricing.cache_write_per_million_usd = "0";
    expect(validatePricing(config)).toBe(true);
    expect(getModelRate("gpt-4.1", "pro", config)).toMatchObject({ cacheRead: undefined, cacheWrite: "0" });
  });

  it("uses one configuration for catalog minima, tier prices, key availability and charge estimates", () => {
    const config = createDefaultPricing();
    config.models["gpt-4.1"].pricing.input_per_million_usd = "2.123456";
    config.models["gpt-4.1"].tierAvailability.lite = true;
    config.placements!["resource-1-a"].groupIds.push("lite");
    config.multipliers.lite = "0.1234";
    const rate = getModelRate("gpt-4.1", "lite", config);
    expect(rate.input).toBe("0.2620344704");
    expect(getModelLowestPrice(catalogModels[0], config)).toMatchObject({ asLowAsUsd: rate.input, tierId: "lite", available: true });
    expect(getModelTierPrices(catalogModels[0], config).find((item) => item.tier.id === "lite")?.input).toBe(rate.input);
    expect(getModelsForTier("lite", config).some((item) => item.id === "gpt-4.1")).toBe(true);
    expect(estimateModelCharge({ modelId: "gpt-4.1", tierId: "lite", inputTokens: 1_000_000, outputTokens: 0 }, config).amountNano).toBe("262034470");
  });

  it("does not offer disabled models or display a fictitious lowest price", () => {
    const config = createDefaultPricing();
    config.models["gpt-4.1"].tierAvailability = { ultra: false, pro: false, standard: false, lite: false };
    expect(getModelsForTier("pro", config).some((item) => item.id === "gpt-4.1")).toBe(false);
    expect(getModelLowestPrice(catalogModels[0], config)).toMatchObject({ available: false, asLowAsUsd: "—" });
    expect(() => estimateModelCharge({ modelId: "gpt-4.1", tierId: "pro", inputTokens: 1, outputTokens: 0 }, config)).toThrow("unavailable");
  });

  it("sums token buckets exactly and rounds once to a nanodollar", () => {
    const config = createDefaultPricing();
    expect(estimateModelCharge({ modelId: "gpt-4.1", tierId: "pro", inputTokens: 1000, outputTokens: 1000 }, config).amountNano).toBe("8000000");
    config.models["gpt-4.1"].pricing.input_per_million_usd = "0.000001";
    config.multipliers.pro = "0.0001";
    expect(estimateModelCharge({ modelId: "gpt-4.1", tierId: "pro", inputTokens: 5_000_000, outputTokens: 0 }, config).amountNano).toBe("1");
    expect(() => estimateModelCharge({ modelId: "gpt-4.1", tierId: "pro", inputTokens: -1, outputTokens: 0 }, config)).toThrow("token count");
    delete config.models["gpt-4.1"].pricing.cache_read_per_million_usd;
    expect(() => estimateModelCharge({ modelId: "gpt-4.1", tierId: "pro", inputTokens: 0, outputTokens: 0, cacheReadTokens: 1 }, config)).toThrow("Unconfigured cache price");
  });

  it("publishes atomically with permission, version and persistence checks", () => {
    const draft = createDefaultPricing();
    draft.models["gpt-4.1"].pricing.input_per_million_usd = "3";
    const store = useModelPricing.getState();
    expect(store.save(draft, "finance", "sample-1")).toEqual({ ok: false, reason: "forbidden" });
    const result = store.save(draft, ROLE_ADMIN, "sample-1");
    expect(result.ok).toBe(true);
    const persisted = JSON.parse(storage.get(PRICING_STORAGE_KEY)!);
    expect(validatePricing(persisted)).toBe(true);
    expect(getModelRate("gpt-4.1", "pro").input).toBe("2.4");
    expect(store.save(draft, ROLE_ADMIN, "sample-1")).toEqual({ ok: false, reason: "conflict" });
    // A retained snapshot continues quoting old prices after publication.
    expect(getModelRate("gpt-4.1", "pro", createDefaultPricing()).input).toBe("1.6");
  });

  it("rejects corrupt configurations, newer cross-tab versions and storage failure", () => {
    const draft = createDefaultPricing();
    draft.models["gpt-4.1"].pricing.input_per_million_usd = "-1";
    expect(useModelPricing.getState().save(draft, ROLE_ADMIN, "sample-1")).toEqual({ ok: false, reason: "invalid" });
    const other = createDefaultPricing(); other.revision = "other-tab-2";
    storage.set(PRICING_STORAGE_KEY, JSON.stringify(other));
    expect(useModelPricing.getState().save(createDefaultPricing(), ROLE_ADMIN, "sample-1")).toEqual({ ok: false, reason: "conflict" });
    vi.stubGlobal("localStorage", { getItem: () => JSON.stringify(other), setItem: () => { throw new Error("blocked"); } });
    expect(useModelPricing.getState().save(other, ROLE_ADMIN, "other-tab-2")).toEqual({ ok: false, reason: "storage" });
    expect(useModelPricing.getState().config.revision).toBe("other-tab-2");
  });
});

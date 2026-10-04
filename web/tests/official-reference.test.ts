import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  getOfficialReference,
  officialReferenceError,
  validateOfficialReference,
} from "../src/lib/official-reference";
import {
  createDefaultPricing,
  quoteModelCharge,
  useModelPricing,
  validatePricing,
  PRICING_STORAGE_KEY,
  migratePricing,
} from "../src/lib/model-pricing";
import { ROLE_ADMIN } from "../src/lib/system-access";
import type { OfficialPriceReference } from "../src/config/models";

let storage: Map<string, string>;
beforeEach(() => {
  storage = new Map();
  vi.stubGlobal("localStorage", {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => storage.set(key, value),
  });
  useModelPricing.setState({ config: createDefaultPricing() });
});

describe("independent official references", () => {
  it("has point-in-time sources and distinct Gemini reference and platform prices", () => {
    const config = createDefaultPricing();
    const reference = getOfficialReference("gemini-2.5-pro", config)!;
    expect(reference).toMatchObject({
      currency: "USD",
      status: "current",
      verifiedAt: "2026-09-28",
      rate: { mode: "tokens", pricing: { output_per_million_usd: "10" } },
    });
    expect(reference.scope.zh).toContain("200,000");
    expect(new URL(reference.sourceUrl).hostname).toBe("ai.google.dev");
    expect(config.models["gemini-2.5-pro"].pricing.output_per_million_usd).toBe(
      "5.00",
    );
    expect(
      getOfficialReference("claude-sonnet-4-20250514", config)?.status,
    ).toBe("legacy");
    expect(getOfficialReference("gpt-4.1", config)?.rate).toMatchObject({
      mode: "tokens",
      pricing: {
        input_per_million_usd: "2",
        output_per_million_usd: "8",
        cache_read_per_million_usd: "0.5",
      },
    });
  });
  it("does not invent official rates from base prices or the base source metadata", () => {
    const config = createDefaultPricing();
    config.models["deepseek-chat"].source = "manual-official";
    config.models["deepseek-chat"].sourceUrl =
      "https://example.com/old-reference";
    config.models["deepseek-chat"].verifiedAt = "2026-09-01";
    for (const id of [
      "deepseek-chat",
      "qwen-plus",
      "veyra-demo-image",
      "veyra-demo-video",
      "veyra-auto",
      "nonexistent",
    ])
      expect(getOfficialReference(id, config)).toBeNull();
    delete config.models["gpt-4.1"];
    expect(getOfficialReference("gpt-4.1", config)).toBeNull();
  });
  it("gives old snapshots verified display metadata without widening groups or changing quotes", () => {
    const config = createDefaultPricing();
    const before = structuredClone(config);
    const usage = { input_tokens: 1000, output_tokens: 500 };
    const quotation = quoteModelCharge(
      { modelId: "gpt-4.1", tierId: "pro", usage },
      config,
    );
    const reference = getOfficialReference("gpt-4.1", config)!;
    expect(config).toEqual(before);
    reference.sourceUrl = "https://altered.example";
    expect(getOfficialReference("gpt-4.1", config)?.sourceUrl).not.toBe(
      reference.sourceUrl,
    );
    expect(
      quoteModelCharge({ modelId: "gpt-4.1", tierId: "pro", usage }, config)
        .amountNano,
    ).toBe(quotation.amountNano);
  });
  it("persists explicit removal and overrides without seed resurrection or billing changes", () => {
    const config = createDefaultPricing();
    const originalQuote = quoteModelCharge(
      {
        modelId: "gpt-4.1",
        tierId: "pro",
        usage: { input_tokens: 1000, output_tokens: 500 },
      },
      config,
    );
    config.models["gpt-4.1"].officialReference = null;
    const ref = getOfficialReference("gemini-2.5-pro", config)!;
    ref.rate = {
      mode: "tokens",
      pricing: { input_per_million_usd: "99", output_per_million_usd: "100" },
    };
    config.models["gemini-2.5-pro"].officialReference = ref;
    const saved = useModelPricing
      .getState()
      .save(config, ROLE_ADMIN, "sample-1");
    expect(saved.ok).toBe(true);
    const restored = migratePricing(
      JSON.parse(storage.get(PRICING_STORAGE_KEY)!),
    )!;
    expect(getOfficialReference("gpt-4.1", restored)).toBeNull();
    expect(getOfficialReference("gemini-2.5-pro", restored)?.rate).toEqual(
      ref.rate,
    );
    expect(
      quoteModelCharge(
        {
          modelId: "gpt-4.1",
          tierId: "pro",
          usage: { input_tokens: 1000, output_tokens: 500 },
        },
        restored,
      ).amountNano,
    ).toBe(originalQuote.amountNano);
    expect(originalQuote.snapshot).not.toHaveProperty("officialReference");
  });
  it("rejects invalid sources, future/nonexistent dates, incomplete scope and guessed free prices", () => {
    const seed = getOfficialReference("gpt-4.1", createDefaultPricing())!;
    const cases = [
      { ...seed, sourceUrl: "javascript:alert(1)" },
      { ...seed, sourceUrl: "http://example.com/price" },
      { ...seed, sourceUrl: "https://user:secret@example.com/price" },
      { ...seed, verifiedAt: "2099-01-01" },
      { ...seed, verifiedAt: "2026-02-30" },
      { ...seed, scope: { zh: "", en: "", ar: "" } },
      { ...seed, currency: "CNY" },
      {
        ...seed,
        rate: {
          mode: "tokens",
          pricing: { input_per_million_usd: "", output_per_million_usd: "0" },
        },
      },
      {
        ...seed,
        rate: {
          mode: "tokens",
          pricing: {
            input_per_million_usd: "1",
            output_per_million_usd: "2",
            cache_read_per_million_usd: "-1",
          },
        },
      },
      { ...seed, rate: { mode: "expression", expression: "1+2" } },
    ];
    for (const entry of cases) {
      expect(validateOfficialReference(entry)).toBe(false);
      const config = createDefaultPricing();
      config.models["gpt-4.1"].officialReference =
        entry as OfficialPriceReference;
      expect(validatePricing(config)).toBe(false);
      expect(getOfficialReference("gpt-4.1", config)).toBeNull();
    }
  });
  it("keeps media units distinct, accepts verified zero, and leaves unquoted caches absent", () => {
    const seed = getOfficialReference("gpt-4.1", createDefaultPricing())!;
    expect(
      seed.rate.mode === "tokens" &&
        seed.rate.pricing.cache_write_per_million_usd,
    ).toBeUndefined();
    for (const unit of ["image", "second", "request"] as const) {
      const reference: OfficialPriceReference = {
        ...seed,
        rate: { mode: "unit", unit, priceUsd: "0" },
      };
      expect(officialReferenceError(reference)).toBeNull();
      expect(reference.rate).toEqual({ mode: "unit", unit, priceUsd: "0" });
    }
    expect(
      validateOfficialReference({
        ...seed,
        rate: { mode: "unit", unit: "million_tokens", priceUsd: "1" },
      }),
    ).toBe(false);
  });
  it("rejects publication by non-administrators and refuses incomplete reference drafts", () => {
    const config = createDefaultPricing();
    config.models["gpt-4.1"].officialReference = getOfficialReference(
      "gpt-4.1",
      config,
    );
    expect(
      useModelPricing.getState().save(config, "finance", "sample-1"),
    ).toEqual({ ok: false, reason: "forbidden" });
    config.models["gpt-4.1"].officialReference!.sourceUrl = "";
    expect(
      useModelPricing.getState().save(config, ROLE_ADMIN, "sample-1"),
    ).toEqual({ ok: false, reason: "invalid" });
    expect(storage.has(PRICING_STORAGE_KEY)).toBe(false);
  });
});

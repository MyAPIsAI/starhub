import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  catalogModels,
  getModelTierPrices,
  type PricingTimeWindow,
} from "../src/config/models";
import {
  createDefaultPricing,
  migratePricing,
  quoteModelCharge,
  readPricing,
  useModelPricing,
  validatePricing,
} from "../src/lib/model-pricing";
import {
  matchedPricingWindow,
  pricingRuleError,
  quotePricingRule,
} from "../src/lib/pricing-rules";
import {
  getKeyModelOptions,
  resolveKeyModelResult,
} from "../src/lib/key-model-policy";
import { useProductPreview } from "../src/lib/product-preview";
import { ROLE_ADMIN } from "../src/lib/system-access";
import { filterExportRequests, serializeRequestExport, serializeUsageExport } from "../src/lib/request-export";

const date = "2026-09-25T12:00:00Z";
const window = (
  overrides: Partial<PricingTimeWindow> = {},
): PricingTimeWindow => ({
  id: "offpeak",
  label: "夜间折扣（演示）",
  timezone: "Asia/Shanghai",
  days: [1],
  start: "23:00",
  end: "06:00",
  multiplier: "0.5",
  ...overrides,
});
beforeEach(() => {
  const storage = new Map<string, string>();
  vi.stubGlobal("localStorage", {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => storage.set(key, value),
    removeItem: (key: string) => storage.delete(key),
  });
  useModelPricing.setState({ config: createDefaultPricing() });
  useProductPreview.setState(useProductPreview.getInitialState(), true);
  useProductPreview.getState().resetScenario("enterprise");
});

describe("metered reference pricing", () => {
  it("quotes token buckets, requests, images, seconds and arithmetic with the group coefficient once", () => {
    const config = createDefaultPricing();
    const quote = (
      modelId: string,
      usage: Parameters<typeof quoteModelCharge>[0]["usage"],
    ) => quoteModelCharge({ modelId, tierId: "pro", usage, at: date }, config);
    expect(
      quote("gpt-4.1", {
        input_tokens: 1_000_000,
        output_tokens: 100_000,
        cache_read_tokens: 100_000,
        cache_write_tokens: 100_000,
      }).amountNano,
    ).toBe("2440000000");
    expect(quote("veyra-demo-image", { images: 3 }).amountNano).toBe(
      "96000000",
    );
    const seconds = quote("veyra-demo-video", { seconds: 1.1 });
    expect(seconds.amountNano).toBe("128000000");
    expect(seconds.snapshot.billedQuantity).toBe("2");
    config.models["gpt-4.1"].rule = {
      mode: "unit",
      unit: "request",
      priceUsd: "0.25",
      minimum: "1",
      ceilTo: "1",
    };
    expect(quote("gpt-4.1", { requests: 2 }).amountNano).toBe("400000000");
    config.models["gpt-4.1"].rule = {
      mode: "expression",
      expression:
        "max(0.01, input_tokens / 1000000 * 2 + output_tokens / 1000000 * 8) + ceil(seconds / 2) * 0.1",
    };
    expect(
      quote("gpt-4.1", { input_tokens: 1000, output_tokens: 1000, seconds: 3 })
        .amountNano,
    ).toBe("168000000");
  });

  it("keeps fractional precision through a sum and rounds only the final amount", () => {
    const config = createDefaultPricing();
    config.models["gpt-4.1"].rule = {
      mode: "expression",
      expression: "1 / 3 + 1 / 3 + 1 / 3",
    };
    config.multipliers.pro = "1";
    expect(
      quoteModelCharge({ modelId: "gpt-4.1", tierId: "pro", usage: {} }, config)
        .amountNano,
    ).toBe("1000000000");
    config.models["gpt-4.1"].rule = {
      mode: "expression",
      expression: "0.00000000025 + 0.00000000025",
    };
    expect(
      quoteModelCharge({ modelId: "gpt-4.1", tierId: "pro", usage: {} }, config)
        .amountNano,
    ).toBe("1");
  });

  it("rejects unsupported code, unknown variables, absent usage, negative values and zero division", () => {
    const config = createDefaultPricing(),
      entry = config.models["gpt-4.1"];
    for (const expression of [
      "globalThis.alert(1)",
      "constructor(1)",
      "input_tokens[0]",
      "Math.max(1,2)",
      "1;2",
      "unknown_variable",
      "fetch(1)",
      "2 ** 5",
      "(1",
    ])
      expect(
        pricingRuleError({ mode: "expression", expression }),
      ).not.toBeNull();
    const calculate = (expression: string, usage = {}) =>
      quotePricingRule(
        { mode: "expression", expression },
        entry.pricing,
        usage,
        "1",
        date,
      );
    expect(() => calculate("images * 0.1")).toThrow("缺少");
    expect(() => calculate("1 / (1 - 1)")).toThrow("零");
    expect(() => calculate("0 - 1")).toThrow("负数");
    expect(() => calculate("images", { images: -1 })).toThrow("无效");
    expect(() => calculate("images", { images: 1.5 })).toThrow("无效");
    expect(() => calculate("requests", { requests: Number.NaN })).toThrow(
      "无效",
    );
    expect(() =>
      quoteModelCharge(
        { modelId: "veyra-demo-video", tierId: "pro", usage: { requests: 1 } },
        config,
      ),
    ).toThrow("seconds");
  });

  it("matches overnight windows by their start weekday with an exclusive end and a base fallback", () => {
    const windows = [window()];
    // Monday 23:00 in Shanghai through Tuesday 06:00; these tests are UTC.
    expect(matchedPricingWindow(windows, "2026-09-21T14:59:59Z")).toBeNull();
    expect(matchedPricingWindow(windows, "2026-09-21T15:00:00Z")?.id).toBe(
      "offpeak",
    );
    expect(matchedPricingWindow(windows, "2026-09-21T21:59:59Z")?.id).toBe(
      "offpeak",
    );
    expect(matchedPricingWindow(windows, "2026-09-21T22:00:00Z")).toBeNull();
    expect(matchedPricingWindow(windows, "2026-09-22T15:00:00Z")).toBeNull();
    const config = createDefaultPricing();
    config.models["veyra-demo-image"].rule.timeWindows = windows;
    const offpeak = quoteModelCharge(
      {
        modelId: "veyra-demo-image",
        tierId: "pro",
        usage: { images: 1 },
        at: "2026-09-21T16:00:00Z",
      },
      config,
    );
    expect(offpeak.amountNano).toBe("16000000");
    expect(offpeak.snapshot).toMatchObject({
      groupMultiplier: "0.8000",
      timeMultiplier: "0.5",
      matchedWindow: { id: "offpeak" },
    });
    expect(
      quoteModelCharge(
        {
          modelId: "veyra-demo-image",
          tierId: "pro",
          usage: { images: 1 },
          at: date,
        },
        config,
      ).amountNano,
    ).toBe("32000000");
  });

  it("detects overlapping weekly windows across midnight and accepts adjacent boundaries", () => {
    const midnight = window({ days: [6], start: "23:00", end: "02:00" });
    const sunday = window({
      id: "sun",
      days: [0],
      start: "01:00",
      end: "03:00",
    });
    expect(
      pricingRuleError({ mode: "tokens", timeWindows: [midnight, sunday] }),
    ).toContain("重叠");
    sunday.start = "02:00";
    expect(
      pricingRuleError({ mode: "tokens", timeWindows: [midnight, sunday] }),
    ).toBeNull();
    expect(
      pricingRuleError({
        mode: "tokens",
        timeWindows: [window({ timezone: "Mars/Test" })],
      }),
    ).toContain("时区");
    expect(
      pricingRuleError({
        mode: "tokens",
        timeWindows: [
          window(),
          window({ id: "other", timezone: "UTC", days: [2] }),
        ],
      }),
    ).toContain("同一时区");
    expect(
      pricingRuleError({
        mode: "tokens",
        timeWindows: [window({ start: "00:00", end: "24:00" })],
      }),
    ).toBeNull();
  });

  it("migrates legacy snapshots without opening newly introduced media models", () => {
    const legacy: Record<string, unknown> = structuredClone(
      createDefaultPricing(),
    ) as unknown as Record<string, unknown>;
    delete legacy.groups;
    const models = legacy.models as Record<string, Record<string, unknown>>;
    delete models["veyra-demo-image"];
    delete models["veyra-demo-video"];
    for (const model of Object.values(models))
      for (const field of [
        "rule",
        "source",
        "sourceUrl",
        "verifiedAt",
        "effectiveAt",
        "version",
      ])
        delete model[field];
    const migrated = migratePricing(legacy)!;
    expect(validatePricing(migrated)).toBe(true);
    expect(migrated.models["gpt-4.1"].rule.mode).toBe("tokens");
    expect(migrated.models["veyra-demo-image"].tierAvailability.pro).toBe(
      false,
    );
    expect(migrated.multipliers.pro).toBe("0.8000");
    expect(migratePricing({ models: {}, multipliers: {} })).toBeNull();
  });

  it("keeps new models closed for saved old accounts that never published pricing", () => {
    localStorage.removeItem("veyra-platform-pricing-v1");
    localStorage.setItem(
      "veyra-billing-demo-v1",
      JSON.stringify({ version: 1, keys: [{ modelAccess: "all" }] }),
    );
    expect(readPricing().models["veyra-demo-image"].tierAvailability.pro).toBe(
      false,
    );
    expect(
      readPricing().models["veyra-demo-video"].tierAvailability.standard,
    ).toBe(false);
  });

  it("validates publication metadata and never claims an automatic source verification", () => {
    const config = createDefaultPricing(),
      entry = config.models["gpt-4.1"];
    expect(entry).toMatchObject({
      source: "sample",
      verifiedAt: null,
      sourceUrl: "",
    });
    entry.source = "manual-official";
    expect(
      useModelPricing.getState().save(config, ROLE_ADMIN, "sample-1"),
    ).toEqual({ ok: false, reason: "invalid" });
    entry.sourceUrl = "https://example.com/pricing";
    entry.verifiedAt = "2026-01-01";
    entry.effectiveAt = "2999-01-01";
    expect(
      useModelPricing.getState().save(config, ROLE_ADMIN, "sample-1"),
    ).toEqual({ ok: false, reason: "invalid" });
    entry.effectiveAt = "2026-01-01";
    const saved = useModelPricing
      .getState()
      .save(config, ROLE_ADMIN, "sample-1");
    expect(saved.ok).toBe(true);
    if (saved.ok)
      expect(saved.config.models["gpt-4.1"].version).toBe(
        saved.config.revision,
      );
  });
});

describe("settlement and authorization", () => {
  it("records metered calls and exact ledger deductions; historical snapshots survive edits", () => {
    const key = useProductPreview.getState().keys[0],
      before = useProductPreview.getState().balance;
    const call = useProductPreview
      .getState()
      .recordMeteredCall(key.id, "veyra-demo-image", { images: 3 });
    expect(call).toMatchObject({
      status: "success",
      cost_nano: "96000000",
      metered_demo: true,
      pricing_snapshot: {
        rule: { mode: "unit", unit: "image", priceUsd: "0.04" },
        usage: { images: 3 },
      },
    });
    const state = useProductPreview.getState();
    expect(BigInt(state.balance)).toBe(BigInt(before) - 96000000n);
    expect(state.ledger[0]).toMatchObject({
      trace_id: call.trace_id,
      amount_nano: "-96000000",
      balance_after_nano: state.balance,
    });
    const draft = structuredClone(useModelPricing.getState().config);
    draft.models["veyra-demo-image"].rule = {
      mode: "unit",
      unit: "image",
      priceUsd: "0.08",
      minimum: "0",
      ceilTo: "1",
    };
    expect(
      useModelPricing.getState().save(draft, ROLE_ADMIN, "sample-1").ok,
    ).toBe(true);
    const next = useProductPreview
      .getState()
      .recordMeteredCall(key.id, "veyra-demo-image", { images: 3 });
    expect(next.cost_nano).toBe("192000000");
    expect(call.cost_nano).toBe("96000000");
    expect(call.pricing_snapshot?.rule).toMatchObject({ priceUsd: "0.04" });
  });

  it("respects key allowlists, disabled groups, balance and quota in the metered verifier", () => {
    const state = useProductPreview.getState(),
      key = state.keys[0],
      restricted = state.keys[1];
    expect(
      state.recordMeteredCall(restricted.id, "veyra-demo-image", { images: 1 }),
    ).toMatchObject({ error_code: "model_unauthorized", cost_nano: "0" });
    state.updateKey(key.id, { quotaLimit_usd: 0.01 });
    expect(
      state.recordMeteredCall(key.id, "veyra-demo-image", { images: 1 }),
    ).toMatchObject({ error_code: "key_quota_exceeded", cost_nano: "0" });
    state.updateKey(key.id, { quotaLimit_usd: 50 });
    state.demoSetBalanceState("zero");
    expect(
      state.recordMeteredCall(key.id, "veyra-demo-image", { images: 1 }),
    ).toMatchObject({ error_code: "insufficient_balance", cost_nano: "0" });
    const config = createDefaultPricing();
    config.groups.pro.enabled = false;
    useModelPricing.setState({ config });
    expect(
      getModelTierPrices(catalogModels[0], config).find(
        (rate) => rate.tier.id === "pro",
      )?.available,
    ).toBe(false);
    expect(
      state.recordMeteredCall(key.id, "veyra-demo-image", { images: 1 }),
    ).toMatchObject({ error_code: "group_unauthorized", cost_nano: "0" });
  });

  it("does not let a simulated pricing timestamp rewind daily quotas", () => {
    const state = useProductPreview.getState(),
      key = state.keys[0];
    state.updateKey(key.id, {
      quotaPeriod: "daily",
      quotaLimit_usd: 0.04,
      currentPeriodUsed_nano: "0",
      totalUsed_nano: "0",
    });
    expect(
      state.recordMeteredCall(
        key.id,
        "veyra-demo-image",
        { images: 1 },
        "2026-01-01T00:00:00Z",
      ).status,
    ).toBe("success");
    const second = state.recordMeteredCall(
      key.id,
      "veyra-demo-image",
      { images: 1 },
      "2026-01-02T00:00:00Z",
    );
    expect(second).toMatchObject({
      error_code: "key_quota_exceeded",
      cost_nano: "0",
    });
    expect(second.time.slice(0, 4)).not.toBe("2025");
  });

  it("rejects missing media usage in text calls without crashing or charging, then allows recovery", () => {
    const key = useProductPreview.getState().keys[0],
      config = createDefaultPricing();
    config.models["gpt-4.1"].rule = {
      mode: "unit",
      unit: "image",
      priceUsd: "0.04",
      minimum: "0",
      ceilTo: "1",
    };
    useModelPricing.setState({ config });
    const failed = useProductPreview
      .getState()
      .recordCall(key.id, false, "gpt-4.1")!;
    expect(failed).toMatchObject({
      error_code: "pricing_invalid",
      cost_nano: "0",
      billing_status: "not_charged",
    });
    expect(failed.error_message).toContain("images");
    expect(failed.request?.model).toBe("gpt-4.1");
    expect(
      useProductPreview
        .getState()
        .recordMeteredCall(key.id, "gpt-4.1", { images: 1 }).status,
    ).toBe("success");
  });

  it("uses the same multi-mode quote for smart cost routing and filters incompatible pricing", () => {
    const key = useProductPreview.getState().keys[0], config = createDefaultPricing();
    config.models["gpt-4.1"].rule = { mode: "unit", unit: "request", priceUsd: "0.000001", minimum: "1", ceilTo: "1" };
    expect(resolveKeyModelResult(key, "veyra-auto", config).model?.id).toBe("gpt-4.1");
    config.models["gpt-4.1"].rule = { mode: "unit", unit: "image", priceUsd: "0.000001", minimum: "1", ceilTo: "1" };
    expect(resolveKeyModelResult(key, "veyra-auto", config).model?.id).not.toBe("gpt-4.1");
    expect(resolveKeyModelResult({ ...key, modelAccess: "custom", allowedModels: ["gpt-4.1"] }, "veyra-auto", config)).toEqual({ error: "pricing_invalid" });
  });

  it("ranks expression-priced candidates using the active time window", () => {
    const key = useProductPreview.getState().keys[0], config = createDefaultPricing();
    config.models["gpt-4.1"].rule = { mode: "expression", expression: "max(0.00001, requests * 0.00001)", timeWindows: [window({ timezone: "UTC", days: [5], start: "09:00", end: "10:00", multiplier: "100" })] };
    expect(resolveKeyModelResult(key, "veyra-auto", config, { at: "2026-09-25T08:59:59Z" }).model?.id).toBe("gpt-4.1");
    expect(resolveKeyModelResult(key, "veyra-auto", config, { at: "2026-09-25T09:00:00Z" }).model?.id).toBe("deepseek-chat");
    expect(resolveKeyModelResult(key, "veyra-auto", config, { at: "2026-09-25T10:00:00Z" }).model?.id).toBe("gpt-4.1");
  });

  it("exports actual request and aggregate counts, full metered snapshots, and escaped names", () => {
    const state = useProductPreview.getState(), key = state.keys[0];
    state.updateKey(key.id, { name: '图片 "报价"\n验证' });
    // Clear seed requests so the export only counts the 4 test calls below.
    useProductPreview.setState({ requests: [] });
    state.recordMeteredCall(key.id, "veyra-demo-image", { images: 1 });
    state.recordMeteredCall(key.id, "veyra-demo-image", { images: 2 });
    state.recordMeteredCall(key.id, "veyra-demo-video", { seconds: 1.1 });
    const smart = state.recordCall(key.id, false, "veyra-auto")!;
    const callsExport = state.requestExport("requests", 1), usageExport = state.requestExport("usage", 1);
    const rows = filterExportRequests(useProductPreview.getState().requests, callsExport);
    expect(callsExport.rowCount).toBe(4);
    expect(usageExport.rowCount).toBe(3);
    expect(callsExport.startDate).toBe(callsExport.endDate);
    const csv = serializeRequestExport(rows, useProductPreview.getState().keys);
    expect(callsExport.sizeKb).toBe(Number((new TextEncoder().encode(csv).byteLength / 1024).toFixed(2)));
    expect(usageExport.sizeKb).toBe(Number((new TextEncoder().encode(serializeUsageExport(rows)).byteLength / 1024).toFixed(2)));
    expect(csv).toContain('"图片 ""报价""\n验证"');
    expect(csv).toContain(`"veyra-auto","${smart.actual_model}"`);
    expect(csv).toContain('""images"":2');
    expect(csv).toContain('""seconds"":1.1');
    expect(csv).toContain('""billedQuantity"":""2""');
    expect(csv).toContain('""groupMultiplier"":""0.8000""');
    expect(csv).toContain('"0.064000000","64000000"');
    for (const invalid of [0, 32, 1.5]) expect(() => state.requestExport("usage", invalid)).toThrow(RangeError);
  });

  it("checks the actual expression amount even when a larger token ceiling would cost less", () => {
    const state = useProductPreview.getState(), key = state.keys[0], config = createDefaultPricing();
    config.models["gpt-4.1"].rule = { mode: "expression", expression: "max(0, 0.15 - output_tokens * 0.0002)" };
    useModelPricing.setState({ config });
    state.updateKey(key.id, { quotaLimit_usd: 0.05 });
    const before = state.balance;
    const call = useProductPreview.getState().recordCall(key.id, false, "gpt-4.1")!;
    expect(call).toMatchObject({ error_code: "key_quota_exceeded", cost_nano: "0" });
    expect(useProductPreview.getState().balance).toBe(before);
  });

  it("does not expose unimplemented media protocols or silently route text to media", () => {
    const key = useProductPreview.getState().keys[0];
    expect(
      getKeyModelOptions(key).some(
        (option) => option.id === "veyra-demo-image",
      ),
    ).toBe(false);
    expect(
      resolveKeyModelResult(key, "veyra-auto").model?.capabilities,
    ).toContain("text");
    expect(
      useProductPreview
        .getState()
        .recordCall(key.id, false, "veyra-demo-image"),
    ).toMatchObject({ error_code: "protocol_incompatible", cost_nano: "0" });
  });
});

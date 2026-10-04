import { afterEach, beforeEach, describe, expect, it } from "vitest";
import {
  getKeyModels,
  getKeyModelOptions,
  resolveKeyModel,
} from "../src/lib/key-model-policy";
import {
  createDefaultPricing,
  useModelPricing,
} from "../src/lib/model-pricing";
import { useProductPreview, type PreviewKey } from "../src/lib/product-preview";

const GPT = "gpt-4.1",
  CLAUDE = "claude-sonnet-4-20250514",
  DEEPSEEK = "deepseek-chat";
function createClientKey(overrides: Partial<PreviewKey> = {}) {
  return useProductPreview
    .getState()
    .createKeyDetailed({
      name: "客户端多模型测试",
      tier: "pro",
      model: GPT,
      modelAccess: "custom",
      allowedModels: [GPT, CLAUDE],
      ...overrides,
    });
}
function rejectWithoutCharge(
  keyId: string,
  model: string,
  error = "model_unauthorized",
) {
  const before = useProductPreview.getState();
  expect(before.recordCall(keyId, false, model)).toMatchObject({
    status: "failed",
    error_code: error,
    cost_nano: "0",
    billing_status: "not_charged",
  });
  const after = useProductPreview.getState();
  expect(after.balance).toBe(before.balance);
  expect(after.requests).toHaveLength(before.requests.length + 1);
  expect(after.ledger).toBe(before.ledger);
  expect(after.keys.find((k) => k.id === keyId)?.totalUsed_nano ?? "0").toBe(
    before.keys.find((k) => k.id === keyId)?.totalUsed_nano ?? "0",
  );
}
describe("credential permissions, per-request routing and exact actual-model billing", () => {
  beforeEach(() => {
    useProductPreview.setState(useProductPreview.getInitialState(), true);
    const config = createDefaultPricing();
    config.revision = "test-price-1";
    config.multipliers.pro = "0.5";
    config.models[GPT].pricing.input_per_million_usd = "2";
    config.models[GPT].pricing.output_per_million_usd = "10";
    config.models[CLAUDE].pricing.input_per_million_usd = "4";
    config.models[CLAUDE].pricing.output_per_million_usd = "20";
    useModelPricing.setState({ config });
  });
  afterEach(() => {
    useProductPreview.setState(useProductPreview.getInitialState(), true);
    useModelPricing.setState({ config: createDefaultPricing() });
  });
  it("A: alternates two exact models and smart routing with the same unchanged key", () => {
    const key = createClientKey(),
      before = useProductPreview.getState();
    const calls = [GPT, CLAUDE, "veyra-auto"].map((model) =>
      useProductPreview.getState().recordCall(key.id, false, model)!,
    );
    expect(calls.map((call) => call.status)).toEqual([
      "success",
      "success",
      "success",
    ]);
    expect(calls.map((call) => call.requested_model)).toEqual([
      GPT,
      CLAUDE,
      "veyra-auto",
    ]);
    expect(calls.map((call) => call.actual_model)).toEqual([GPT, CLAUDE, GPT]);
    calls.forEach((call) => {
      const expected =
        BigInt(call.input_tokens) * BigInt(call.input_price_nano) +
        BigInt(call.output_tokens) * BigInt(call.output_price_nano);
      expect(call.cost_nano).toBe(((expected + 500000n) / 1000000n).toString());
      expect(call.pricing_revision).toBe("test-price-1");
      expect(
        useProductPreview
          .getState()
          .ledger.filter((entry) => entry.trace_id === call.trace_id),
      ).toHaveLength(1);
    });
    const total = calls.reduce((sum, call) => sum + BigInt(call.cost_nano), 0n);
    expect(BigInt(useProductPreview.getState().balance)).toBe(
      BigInt(before.balance) - total,
    );
    expect(
      useProductPreview.getState().keys.find((item) => item.id === key.id),
    ).toMatchObject({
      type: key.type,
      modelAccess: key.modelAccess,
      allowedModels: key.allowedModels,
      smartAccess: true,
      totalUsed_nano: total.toString(),
    });
  });
  it("B: creation from a model example grants group defaults without a singleton allowlist", () => {
    const key = useProductPreview
      .getState()
      .createKeyDetailed({ name: "来自模型详情", tier: "pro", model: CLAUDE });
    expect(key).toMatchObject({
      model: CLAUDE,
      modelAccess: "all",
      smartAccess: true,
    });
    expect(key.allowedModels).toBeUndefined();
    expect(getKeyModelOptions(key).map((model) => model.id)).toEqual(
      expect.arrayContaining([GPT, CLAUDE, DEEPSEEK, "veyra-auto"]),
    );
  });
  it("records unknown and unauthorized failures without substitution or charge", () => {
    const key = createClientKey({ allowedModels: [GPT] });
    rejectWithoutCharge(key.id, CLAUDE);
    rejectWithoutCharge(key.id, "missing", "unknown_model");
    expect(
      useProductPreview.getState().recordCall(key.id, false, "veyra-auto")
        ?.actual_model,
    ).toBe(GPT);
  });
  it("preserves legacy specified and smart permissions without automatically widening either", () => {
    const created = createClientKey({ allowedModels: [GPT] });
    const {
      modelAccess: _a,
      allowedModels: _b,
      smartAccess: _c,
      ...rest
    } = created;
    const specified: PreviewKey = { ...rest, type: "specified" };
    useProductPreview.setState({ keys: [specified] });
    expect(getKeyModels(specified).map((model) => model.id)).toEqual([GPT]);
    rejectWithoutCharge(specified.id, CLAUDE);
    rejectWithoutCharge(specified.id, "veyra-auto");
    const smart: PreviewKey = {
      ...specified,
      type: "smart",
      allowedModels: [GPT, CLAUDE],
    };
    useProductPreview.setState({ keys: [smart] });
    rejectWithoutCharge(smart.id, GPT);
    expect(
      useProductPreview.getState().recordCall(smart.id, false, "veyra-auto")
        ?.actual_model,
    ).toBe(GPT);
  });
  it("preserves older multi-model allowlists and respects immediate group changes", () => {
    const key = createClientKey();
    const { modelAccess: _a, smartAccess: _b, ...legacy } = key;
    useProductPreview.setState({ keys: [legacy] });
    expect(
      useProductPreview.getState().recordCall(key.id, false, CLAUDE)
        ?.actual_model,
    ).toBe(CLAUDE);
    const config = structuredClone(useModelPricing.getState().config);
    config.models[CLAUDE].tierAvailability.pro = false;
    useModelPricing.setState({ config });
    rejectWithoutCharge(key.id, CLAUDE);
  });
  it("C: smart routing never escapes a whitelist even when another candidate is free", () => {
    const config = structuredClone(useModelPricing.getState().config);
    config.models[DEEPSEEK].pricing.input_per_million_usd = "0";
    config.models[DEEPSEEK].pricing.output_per_million_usd = "0";
    useModelPricing.setState({ config });
    const key = createClientKey();
    expect(resolveKeyModel(key, "veyra-auto")?.id).toBe(GPT);
    rejectWithoutCharge(key.id, DEEPSEEK);
    expect(
      useProductPreview.getState().recordCall(key.id, false, "veyra-auto"),
    ).toMatchObject({ status: "success", actual_model: GPT });
  });
  it("E: a published price affects only subsequent calls, with no repeated group multiplier", () => {
    const key = createClientKey({ allowedModels: [GPT] });
    const historic = useProductPreview
        .getState()
        .recordCall(key.id, false, GPT)!,
      original = structuredClone(historic);
    const config = structuredClone(useModelPricing.getState().config);
    config.revision = "test-price-2";
    config.models[GPT].pricing.input_per_million_usd = "4";
    config.models[GPT].pricing.output_per_million_usd = "20";
    useModelPricing.setState({ config });
    const next = useProductPreview.getState().recordCall(key.id, false, GPT)!;
    expect(BigInt(next.cost_nano)).toBe(BigInt(historic.cost_nano) * 2n);
    expect(next.input_rate_usd).toBe("2");
    expect(next.output_rate_usd).toBe("10");
    expect(historic).toEqual(original);
    expect(
      useProductPreview
        .getState()
        .requests.find((call) => call.trace_id === historic.trace_id),
    ).toEqual(original);
  });
  it("rejects unusable custom scopes and invalid quota amounts", () => {
    for (const allowedModels of [[], ["missing"]])
      expect(() => createClientKey({ allowedModels })).toThrow();
    expect(() =>
      createClientKey({ tier: "lite", allowedModels: [GPT] }),
    ).toThrow();
    expect(() => createClientKey({ quotaLimit_usd: -1 })).toThrow();
  });
});

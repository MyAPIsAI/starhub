import { afterEach, beforeEach, describe, expect, it } from "vitest";
import {
  getKeyModels,
  getKeyModelOptions,
  resolveKeyModel,
  resolveKeyModelResult,
} from "../src/lib/key-model-policy";
import {
  createDefaultPricing,
  useModelPricing,
} from "../src/lib/model-pricing";
import { useProductPreview, type PreviewKey } from "../src/lib/product-preview";
import { defaultDemoRequest } from "../src/lib/request-contract";

const GPT = "gpt-4.1",
  CLAUDE = "claude-sonnet-4-20250514",
  DEEPSEEK = "deepseek-chat";
function createClientKey(overrides: Partial<PreviewKey> = {}) {
  return useProductPreview.getState().createKeyDetailed({
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
  describe("default requests for legacy smart keys", () => {
    function legacySmartKey(overrides: Partial<PreviewKey> = {}) {
      const key: PreviewKey = {
        ...createClientKey({ allowedModels: [GPT] }),
        type: "smart",
        smartAccess: undefined,
        ...overrides,
      };
      useProductPreview.setState({ keys: [key] });
      return key;
    }
    it("defaults to the canonical entry and charges the authorized actual model once", () => {
      const key = legacySmartKey();
      const historic = useProductPreview
        .getState()
        .recordCall(key.id, false, "veyra-auto")!;
      const snapshot = structuredClone(historic);
      const before = useProductPreview.getState();
      const call = before.recordCall(key.id)!;
      expect(call).toMatchObject({
        status: "success",
        requested_model: "myapis-auto",
        actual_model: GPT,
        billing_status: "charged",
        input_rate_usd: "1",
        output_rate_usd: "5",
        request: { model: "myapis-auto" },
      });
      const after = useProductPreview.getState();
      expect(BigInt(call.cost_nano)).toBeGreaterThan(0n);
      expect(BigInt(after.balance)).toBe(
        BigInt(before.balance) - BigInt(call.cost_nano),
      );
      expect(after.ledger).toHaveLength(before.ledger.length + 1);
      expect(after.requests).toHaveLength(before.requests.length + 1);
      expect(
        after.ledger.filter((entry) => entry.trace_id === call.trace_id),
      ).toHaveLength(1);
      expect(after.keys[0]).toMatchObject({
        model: key.model,
        type: "smart",
        smartAccess: undefined,
        modelAccess: key.modelAccess,
        allowedModels: key.allowedModels,
      });
      expect(BigInt(after.keys[0].totalUsed_nano!)).toBe(
        BigInt(before.keys[0].totalUsed_nano!) + BigInt(call.cost_nano),
      );
      expect(
        after.requests.find((row) => row.trace_id === historic.trace_id),
      ).toEqual(snapshot);
    });
    it.each(["myapis-auto", "veyra-auto"])(
      "preserves an explicit %s request",
      (model) => {
        const key = legacySmartKey();
        expect(
          useProductPreview.getState().recordCall(key.id, false, model),
        ).toMatchObject({
          status: "success",
          requested_model: model,
          actual_model: GPT,
          request: { model },
        });
      },
    );
    it("preserves a submitted legacy request when the model argument is omitted", () => {
      const key = legacySmartKey();
      const request = defaultDemoRequest("veyra-auto");
      const snapshot = structuredClone(request);
      expect(
        useProductPreview
          .getState()
          .recordCall(key.id, false, undefined, request),
      ).toMatchObject({
        status: "success",
        requested_model: "veyra-auto",
        actual_model: GPT,
        request: { model: "veyra-auto" },
      });
      expect(request).toEqual(snapshot);
    });
    it("keeps a modern client key's default explicit model", () => {
      const key = createClientKey({ allowedModels: [GPT] });
      expect(useProductPreview.getState().recordCall(key.id)).toMatchObject({
        status: "success",
        requested_model: GPT,
        actual_model: GPT,
      });
    });
    it("respects an explicit smart-access override on a legacy key", () => {
      const key = legacySmartKey({ smartAccess: false });
      expect(useProductPreview.getState().recordCall(key.id)).toMatchObject({
        status: "success",
        requested_model: GPT,
        actual_model: GPT,
      });
      rejectWithoutCharge(key.id, "myapis-auto");
    });
    it.each(["myapis-auto", "veyra-auto"])(
      "does not expand a %s-only allowlist for default routing",
      (entry) => {
        const key = legacySmartKey({ allowedModels: [entry] });
        const before = useProductPreview.getState();
        expect(before.recordCall(key.id)).toMatchObject({
          status: "failed",
          requested_model: "myapis-auto",
          actual_model: null,
          error_code: "model_unauthorized",
          cost_nano: "0",
          billing_status: "not_charged",
        });
        const after = useProductPreview.getState();
        expect(after.balance).toBe(before.balance);
        expect(after.ledger).toBe(before.ledger);
        expect(after.keys[0].totalUsed_nano).toBe(
          before.keys[0].totalUsed_nano,
        );
      },
    );
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
  describe.each(["myapis-auto", "veyra-auto"])("smart entry %s", (entry) => {
    it("preserves the requested entry and charges the allowed actual model once", () => {
      const key = createClientKey({ allowedModels: [GPT] });
      const before = useProductPreview.getState();
      const call = before.recordCall(key.id, false, entry)!;
      expect(call).toMatchObject({
        status: "success",
        requested_model: entry,
        actual_model: GPT,
        billing_status: "charged",
        input_rate_usd: "1",
        output_rate_usd: "5",
      });
      const after = useProductPreview.getState();
      expect(BigInt(call.cost_nano)).toBeGreaterThan(0n);
      expect(BigInt(after.balance)).toBe(
        BigInt(before.balance) - BigInt(call.cost_nano),
      );
      expect(
        after.keys.find((item) => item.id === key.id)?.totalUsed_nano,
      ).toBe(call.cost_nano);
      expect(after.requests).toHaveLength(before.requests.length + 1);
      expect(after.ledger).toHaveLength(before.ledger.length + 1);
    });
    it("requires explicit smart access", () => {
      const key = createClientKey({ smartAccess: false, allowedModels: [GPT] });
      rejectWithoutCharge(key.id, entry);
    });
    it("does not turn an entry-only allowlist into actual-model authorization", () => {
      const key = createClientKey({ allowedModels: [GPT] });
      useProductPreview.setState({
        keys: [{ ...key, allowedModels: [entry] }],
      });
      rejectWithoutCharge(key.id, entry);
    });
    it("keeps protocol, context and tool-chain constraints", () => {
      const key = createClientKey({ allowedModels: [GPT] });
      const config = useModelPricing.getState().config;
      expect(
        resolveKeyModelResult(key, entry, config, { protocol: "Anthropic" }),
      ).toEqual({ error: "protocol_incompatible" });
      expect(
        resolveKeyModelResult(key, entry, config, { inputTokens: 2_000_000 }),
      ).toEqual({ error: "capability_incompatible" });
      expect(
        resolveKeyModelResult(key, entry, config, { chainModel: CLAUDE }),
      ).toEqual({ error: "tool_chain_incompatible" });
    });
    it("rejects metered verification without request capabilities and leaves funds unchanged", () => {
      const key = createClientKey({ allowedModels: [GPT] });
      const before = useProductPreview.getState();
      expect(
        before.recordMeteredCall(key.id, entry, {
          input_tokens: 1000,
          output_tokens: 100,
        }),
      ).toMatchObject({
        status: "failed",
        error_code: "capability_incompatible",
        billing_status: "not_charged",
        cost_nano: "0",
      });
      const after = useProductPreview.getState();
      expect(after.balance).toBe(before.balance);
      expect(after.ledger).toBe(before.ledger);
      expect(
        after.keys.find((item) => item.id === key.id)?.totalUsed_nano,
      ).toBe("0");
    });
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

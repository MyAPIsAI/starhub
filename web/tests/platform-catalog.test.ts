import { beforeEach, describe, it, expect, vi } from "vitest";
import { catalogModels, type PricingRule } from "../src/config/models";
import {
  createDefaultPricing,
  useModelPricing,
  validatePricing,
  PRICING_STORAGE_KEY,
  readPricing,
} from "../src/lib/model-pricing";
import {
  applyResourceSnapshot,
  nextDemoSnapshot,
  materializeAvailability,
  visibleGroups,
  groupResources,
  resourceSnapshotError,
  upgradePlatform,
} from "../src/lib/platform-catalog";
import {
  addStandardModel,
  removeStandardModel,
  removeServiceGroup,
} from "../src/lib/catalog-mutations";
import {
  getKeyModels,
  resolveKeyModelResult,
} from "../src/lib/key-model-policy";
import { useProductPreview } from "../src/lib/product-preview";
import { readBillingStorage } from "../src/lib/billing";
import {
  directoryPrices,
  getDirectoryModel,
} from "../src/config/model-directory";
import { quotePricingRule, pricingRuleError } from "../src/lib/pricing-rules";
let storage: Map<string, string>;
beforeEach(() => {
  storage = new Map();
  vi.stubGlobal("localStorage", {
    getItem: (k: string) => storage.get(k) ?? null,
    setItem: (k: string, v: string) => storage.set(k, v),
    removeItem: (k: string) => storage.delete(k),
  });
  useModelPricing.setState({ config: createDefaultPricing() });
  useProductPreview.setState(useProductPreview.getInitialState(), true);
  useProductPreview.getState().resetScenario("enterprise");
});
const keyPolicy = {
  tier: "pro",
  model: "gpt-4.1",
  type: "client",
  modelAccess: "all",
  smartAccess: true,
} as const;
function commit(config: ReturnType<typeof createDefaultPricing>) {
  const current = useModelPricing.getState();
  const result = current.save(
    materializeAvailability(config),
    "superAdmin",
    current.config.revision,
  );
  expect(result.ok).toBe(true);
  return useModelPricing.getState().config;
}
describe("standard catalog and mapped resource ownership", () => {
  it("creates a standard identity with price but no automatic resource or authorization", () => {
    const base = createDefaultPricing();
    const model = {
      ...structuredClone(catalogModels[0]),
      id: "demo-text-next",
      name: "企业文本",
      vendor: "演示",
    };
    const config = addStandardModel(base, model, "0");
    expect(validatePricing(config)).toBe(true);
    expect(getKeyModels(keyPolicy, config).some((m) => m.id === model.id)).toBe(
      false,
    );
    expect(
      Object.values(config.placements!).some((p) => p.modelId === model.id),
    ).toBe(false);
    commit(config);
    expect(readPricing().catalog?.[model.id].name).toBe("企业文本");
    expect(() => addStandardModel(config, model, "0")).toThrow("已存在");
    expect(
      removeStandardModel(config, model.id, [], []).catalog?.[model.id],
    ).toBeUndefined();
  });
  it("migrates old authorization to instance placement without expansion", () => {
    const old = createDefaultPricing();
    old.models["gpt-4.1"].tierAvailability.pro = false;
    delete old.resources;
    delete old.placements;
    delete old.catalog;
    const config = upgradePlatform(old);
    expect(groupResources(config, "pro", "gpt-4.1")).toHaveLength(0);
    expect(groupResources(config, "standard", "gpt-4.1")).toHaveLength(2);
    expect(
      getKeyModels(keyPolicy, config).some((m) => m.id === "gpt-4.1"),
    ).toBe(false);
  });
  it("syncs new instances as unmapped and unplaced; rejects stale and malformed snapshots", () => {
    const base = createDefaultPricing(),
      incoming = nextDemoSnapshot(base, "add");
    const config = applyResourceSnapshot(base, incoming);
    expect(config.placements?.["resource-import-2"]).toEqual({
      modelId: null,
      groupIds: [],
      enabled: false,
    });
    expect(() => applyResourceSnapshot(config, incoming)).toThrow("过旧");
    expect(
      resourceSnapshotError({ ...incoming, instances: [null] }),
    ).not.toBeNull();
    expect(
      resourceSnapshotError({
        ...incoming,
        instances: [...incoming.instances, incoming.instances[0]],
      }),
    ).not.toBeNull();
    expect(base.resources?.sequence).toBe(1);
  });
  it("pauses an existing placement if its upstream identity changes, instead of silently calling another model", () => {
    const base = createDefaultPricing(),
      incoming = nextDemoSnapshot(base, "restore");
    incoming.instances[0].upstreamModel = "different-model";
    const next = applyResourceSnapshot(base, incoming);
    expect(next.placements?.[incoming.instances[0].id].enabled).toBe(false);
    expect(resolveKeyModelResult(keyPolicy, "gpt-4.1", next).resource?.id).toBe(
      "resource-1-b",
    );
  });
  it("supports one instance in multiple dynamic groups and derives group models", () => {
    let c = createDefaultPricing();
    c.groups["enterprise-a"] = {
      name: "企业 A",
      description: "",
      enabled: true,
      visibility: "custom",
      accountIds: ["account-demo"],
    };
    c.multipliers["enterprise-a"] = "0.65";
    c.placements!["resource-1-a"].groupIds.push("enterprise-a");
    c = materializeAvailability(c);
    expect(validatePricing(c)).toBe(true);
    expect(visibleGroups(c).some((g) => g.id === "enterprise-a")).toBe(true);
    expect(
      visibleGroups(c, "account-partner").some((g) => g.id === "enterprise-a"),
    ).toBe(false);
    expect(
      getKeyModels({ ...keyPolicy, tier: "enterprise-a" }, c).map((m) => m.id),
    ).toEqual(["gpt-4.1"]);
    expect(groupResources(c, "pro", "gpt-4.1")).toHaveLength(2);
    commit(c);
    expect(readPricing().multipliers["enterprise-a"]).toBe("0.65");
  });
  it("enforces custom account authorization for old keys and prevents routing beyond allowlists", () => {
    const c = createDefaultPricing();
    c.groups.pro.visibility = "custom";
    c.groups.pro.accountIds = ["account-partner"];
    expect(resolveKeyModelResult(keyPolicy, "veyra-auto", c).error).toBe(
      "group_unauthorized",
    );
    expect(
      resolveKeyModelResult(
        {
          ...keyPolicy,
          accountId: "account-partner",
          modelAccess: "custom",
          allowedModels: ["gpt-4.1"],
        },
        "veyra-auto",
        c,
      ).model?.id,
    ).toBe("gpt-4.1");
    expect(
      resolveKeyModelResult(
        {
          ...keyPolicy,
          accountId: "account-partner",
          modelAccess: "custom",
          allowedModels: ["deepseek-chat"],
        },
        "gpt-4.1",
        c,
      ).error,
    ).toBe("model_unauthorized");
  });
  it("uses another instance of the same requested model and fails without changing the model when all are down", () => {
    const c = createDefaultPricing();
    c.resources!.instances.find((r) => r.id === "resource-1-a")!.status =
      "unavailable";
    expect(resolveKeyModelResult(keyPolicy, "gpt-4.1", c).resource?.id).toBe(
      "resource-1-b",
    );
    c.resources!.instances.find((r) => r.id === "resource-1-b")!.status =
      "unavailable";
    expect(resolveKeyModelResult(keyPolicy, "gpt-4.1", c).error).toBe(
      "resource_unavailable",
    );
    expect(
      resolveKeyModelResult(
        { ...keyPolicy, modelAccess: "custom", allowedModels: ["gpt-4.1"] },
        "veyra-auto",
        c,
      ).error,
    ).toBe("resource_unavailable");
  });
  it("filters resources by protocol and capability even when standard metadata supports them", () => {
    const c = createDefaultPricing();
    for (const r of c.resources!.instances.filter(
      (r) => r.canonicalModel === "gpt-4.1",
    ))
      r.protocols = [];
    expect(
      resolveKeyModelResult(keyPolicy, "gpt-4.1", c, { protocol: "OpenAI" })
        .error,
    ).toBe("resource_unavailable");
  });
  it("rejects referenced deletions, allows unreferenced group CRUD and preserves history", () => {
    let c = createDefaultPricing();
    expect(() => removeServiceGroup(c, "pro", [], [])).toThrow("引用");
    expect(() => removeStandardModel(c, "gpt-4.1", [], [])).toThrow("引用");
    c.groups["unused"] = {
      name: "临时分组",
      description: "",
      enabled: true,
      visibility: "common",
      accountIds: [],
    };
    c.multipliers.unused = "1";
    c = materializeAvailability(c);
    commit(c);
    const removed = removeServiceGroup(c, "unused", [], []);
    expect(removed.groups.unused).toBeUndefined();
    expect(validatePricing(removed)).toBe(true);
  });
  it("records resource provenance once with a bill, retains historical snapshots and recovers after outage without charge", () => {
    const store = useProductPreview.getState(),
      key = store.keys[0];
    const first = store.recordCall(key.id, false, "gpt-4.1")!;
    expect(first).toMatchObject({
      status: "success",
      resource_id: "resource-1-a",
      channel_id: "channel-1",
      resource_snapshot_id: "resource-sample-1",
    });
    const before = useProductPreview.getState().balance;
    commit(
      applyResourceSnapshot(
        useModelPricing.getState().config,
        nextDemoSnapshot(useModelPricing.getState().config, "outage"),
      ),
    );
    const failed = store.recordCall(key.id, false, "gpt-4.1")!;
    expect(failed).toMatchObject({
      status: "failed",
      billing_status: "not_charged",
      cost_nano: "0",
      error_code: "resource_unavailable",
    });
    expect(useProductPreview.getState().balance).toBe(before);
    commit(
      applyResourceSnapshot(
        useModelPricing.getState().config,
        nextDemoSnapshot(useModelPricing.getState().config, "restore"),
      ),
    );
    expect(store.recordCall(key.id, false, "gpt-4.1")?.status).toBe("success");
    expect(
      useProductPreview
        .getState()
        .requests.find((r) => r.trace_id === first.trace_id),
    ).toEqual(first);
    expect(
      useProductPreview
        .getState()
        .ledger.filter((l) => l.trace_id === first.trace_id),
    ).toHaveLength(1);
  });
  it("restores dynamic group keys and their exact bills without resetting source configuration", () => {
    let c = createDefaultPricing();
    c.groups["enterprise-persist"] = {
      name: "自定义组",
      description: "",
      enabled: true,
      visibility: "common",
      accountIds: [],
    };
    c.multipliers["enterprise-persist"] = "0.5";
    c.placements!["resource-1-a"].groupIds.push("enterprise-persist");
    c = commit(c);
    const key = useProductPreview
      .getState()
      .createKeyDetailed({
        name: "持久化",
        tier: "enterprise-persist",
        model: "gpt-4.1",
      });
    const call = useProductPreview
      .getState()
      .recordCall(key.id, false, "gpt-4.1")!;
    const saved = readBillingStorage();
    expect(saved?.keys?.find((k) => k.id === key.id)?.tier).toBe(
      "enterprise-persist",
    );
    expect(
      saved?.liveRequests.find((r) => r.trace_id === call.trace_id)?.cost_nano,
    ).toBe(call.cost_nano);
    expect(readPricing().groups["enterprise-persist"].name).toBe("自定义组");
  });
  it("persists resource settings atomically with revision and role checks", () => {
    const c = createDefaultPricing();
    expect(
      useModelPricing.getState().save(c, "customer", c.revision),
    ).toMatchObject({ ok: false, reason: "forbidden" });
    const old = structuredClone(c);
    commit(c);
    expect(
      useModelPricing.getState().save(old, "superAdmin", old.revision),
    ).toMatchObject({ ok: false, reason: "conflict" });
    expect(
      JSON.parse(storage.get(PRICING_STORAGE_KEY)!).resources.sequence,
    ).toBe(1);
  });
});
describe("conditional media pricing", () => {
  const rule: PricingRule = {
    mode: "unit",
    unit: "second",
    priceUsd: "0.08",
    minimum: "1",
    ceilTo: "1",
    conditionRates: [
      {
        id: "hd-audio",
        label: "高清含音频",
        when: { resolution: "1080p", audio: true },
        priceUsd: "0.2",
      },
      {
        id: "hd-silent",
        label: "高清静音",
        when: { resolution: "1080p", audio: false },
        priceUsd: "0.12",
      },
    ],
  };
  it("quotes by declared attributes, uses one group multiplier, and requires price-driving dimensions", () => {
    expect(
      quotePricingRule(
        rule,
        catalogModels[0].pricing,
        { seconds: 5.2 },
        "0.5",
        "2026-09-26T12:00:00Z",
        { resolution: "1080p", audio: true },
      ),
    ).toMatchObject({
      amountNano: "600000000",
      billedQuantity: "6",
      matchedCondition: { id: "hd-audio" },
    });
    expect(
      quotePricingRule(
        rule,
        catalogModels[0].pricing,
        { seconds: 5 },
        "1",
        "2026-09-26T12:00:00Z",
        { resolution: "720p", audio: false },
      ),
    ).toMatchObject({ amountNano: "400000000", matchedCondition: null });
    expect(() =>
      quotePricingRule(
        rule,
        catalogModels[0].pricing,
        { seconds: 5 },
        "1",
        "2026-09-26T12:00:00Z",
        {},
      ),
    ).toThrow("缺少条件");
  });
  it("uses explicit specification assumptions for directory and price impact examples", () => {
    const c = createDefaultPricing();
    c.models["veyra-demo-video"].rule = rule;
    const prices = directoryPrices(
      getDirectoryModel("veyra-demo-video", c)!,
      "pro",
      c,
    );
    expect(prices.canEstimate).toBe(true);
    expect(prices.rates[0].attributes).toEqual({
      resolution: "1080p",
      audio: true,
    });
    expect(prices.rates[0].quote?.snapshot.matchedCondition?.id).toBe(
      "hd-audio",
    );
    expect(prices.minAmountNano).toBe("160000000");
  });
  it("rejects overlapping, empty, duplicate and negative condition prices", () => {
    if (rule.mode !== "unit") return;
    expect(
      pricingRuleError({
        ...rule,
        conditionRates: [
          ...rule.conditionRates!,
          {
            id: "all",
            label: "重叠",
            when: { resolution: "1080p" },
            priceUsd: "0.3",
          },
        ],
      }),
    ).toContain("重叠");
    expect(
      pricingRuleError({
        ...rule,
        conditionRates: [{ id: "x", label: "x", when: {}, priceUsd: "-1" }],
      }),
    ).not.toBeNull();
  });
  it("settles media attributes and snapshots without rewriting past fees; missing attributes fail without charge", () => {
    const c = createDefaultPricing();
    c.models["veyra-demo-video"].rule = rule;
    commit(c);
    const store = useProductPreview.getState(),
      key = store.keys[0];
    const before = useProductPreview.getState().balance;
    const failed = store.recordMeteredCall(key.id, "veyra-demo-video", {
      seconds: 5,
    });
    expect(failed).toMatchObject({
      status: "failed",
      cost_nano: "0",
      error_code: "pricing_invalid",
    });
    expect(useProductPreview.getState().balance).toBe(before);
    const success = store.recordMeteredCall(
      key.id,
      "veyra-demo-video",
      { seconds: 5 },
      undefined,
      { resolution: "1080p", audio: true },
    );
    expect(success).toMatchObject({
      status: "success",
      cost_nano: "800000000",
      metered_attributes: { resolution: "1080p", audio: true },
    });
    const next = structuredClone(useModelPricing.getState().config);
    if (next.models["veyra-demo-video"].rule.mode === "unit")
      next.models["veyra-demo-video"].rule.conditionRates![0].priceUsd = "0.4";
    commit(next);
    expect(
      useProductPreview
        .getState()
        .requests.find((r) => r.trace_id === success.trace_id)?.pricing_snapshot
        ?.matchedCondition?.priceUsd,
    ).toBe("0.2");
  });
});

describe("off-peak token bucket rates", () => {
  it("supports asymmetric input/output/cache prices with one group multiplier and exact rate snapshots", () => {
    const c = createDefaultPricing();
    c.models["gpt-4.1"].rule = {
      mode: "tokens",
      timeWindows: [
        {
          id: "night",
          label: "低峰独立价",
          timezone: "UTC",
          days: [0, 1, 2, 3, 4, 5, 6],
          start: "00:00",
          end: "08:00",
          multiplier: "1",
          tokenPrices: {
            input_per_million_usd: "0.5",
            output_per_million_usd: "4",
            cache_read_per_million_usd: "0.1",
          },
        },
      ],
    };
    commit(c);
    const store = useProductPreview.getState(),
      call = store.recordMeteredCall(
        store.keys[0].id,
        "gpt-4.1",
        { input_tokens: 1000, output_tokens: 500, cache_read_tokens: 1000 },
        "2026-09-26T01:00:00Z",
      );
    expect(call.cost_nano).toBe("2080000");
    expect(call.input_rate_usd).toBe("0.4");
    expect(call.output_rate_usd).toBe("3.2");
    expect(call.pricing_snapshot?.pricing.input_per_million_usd).toBe("0.5");
    expect(call.pricing_snapshot?.timeMultiplier).toBe("1");
    const outside = store.recordMeteredCall(
      store.keys[0].id,
      "gpt-4.1",
      { input_tokens: 1000, output_tokens: 500, cache_read_tokens: 1000 },
      "2026-09-26T12:00:00Z",
    );
    expect(outside.cost_nano).toBe("5200000");
  });
  it("rejects double time multiplication and invalid overrides", () => {
    const window = {
      id: "night",
      label: "night",
      timezone: "UTC",
      days: [0, 1],
      start: "00:00",
      end: "08:00",
      multiplier: "0.5",
      tokenPrices: { input_per_million_usd: "1" },
    };
    expect(
      pricingRuleError({ mode: "tokens", timeWindows: [window] }),
    ).toContain("不能再次叠加");
    expect(
      pricingRuleError({
        mode: "unit",
        unit: "second",
        priceUsd: "1",
        minimum: "0",
        ceilTo: "1",
        timeWindows: [{ ...window, multiplier: "1" }],
      }),
    ).toContain("只适用于");
  });
});

import { describe, expect, it } from "vitest";
import { directoryPrices, getDirectoryModel } from "../config/model-directory";
import type { PricingCatalogConfig } from "../config/models";
import { getPublicCatalogPrice } from "./catalog-public-pricing";
import { createDefaultPricing, quoteModelCharge } from "./model-pricing";

const at = "2026-09-28T03:00:00Z";
const model = (config: PricingCatalogConfig, id = "gpt-4.1") =>
  getDirectoryModel(id, config);

describe("public catalog example pricing", () => {
  it("selects the lowest eligible public group and keeps its entire quoted rate and assumptions", () => {
    const config = createDefaultPricing();
    const result = getPublicCatalogPrice(model(config), config, at);
    expect(result).toMatchObject({
      status: "ready",
      mode: "tokens",
      groupId: "standard",
      comparison: "example-total",
      quotedAt: at,
    });
    expect(result.price).toEqual(
      directoryPrices(model(config)!, "standard", config, at, ""),
    );
    const rate = result.price!.rates[0];
    expect(rate).toMatchObject({
      input: "1.2",
      output: "4.8",
      cacheRead: "0.3",
      cacheWrite: "1.2",
    });
    expect(rate.usage).toMatchObject({
      input_tokens: 1000,
      output_tokens: 300,
      cache_read_tokens: 0,
    });
    expect(rate.quote).toEqual(
      quoteModelCharge(
        {
          modelId: rate.model.id,
          tierId: "standard",
          usage: rate.usage,
          attributes: rate.attributes,
          at,
        },
        config,
      ),
    );
  });

  it("never advertises a cheaper custom group even when the legacy demo account can use it", () => {
    const config = createDefaultPricing();
    config.groups.standard.visibility = "custom";
    config.groups.standard.accountIds = ["account-demo", "account-current"];
    config.multipliers.standard = "0.0001";
    const result = getPublicCatalogPrice(model(config), config, at);
    expect(result).toMatchObject({
      status: "ready",
      groupId: "pro",
      publicGroupCount: 3,
    });
    expect(result.price?.rates[0].input).toBe("1.6");
  });

  it("keeps the lowest exact rate when tiny example totals round to the same nano-dollar", () => {
    const config = createDefaultPricing();
    config.models["gpt-4.1"].pricing.input_per_million_usd = "0.000001";
    config.models["gpt-4.1"].pricing.output_per_million_usd = "0.000001";
    expect(
      directoryPrices(model(config)!, "pro", config, at, "").minAmountNano,
    ).toBe("1");
    const result = getPublicCatalogPrice(model(config), config, at);
    expect(result).toMatchObject({ status: "ready", groupId: "standard" });
    expect(result.price?.minAmountNano).toBe("1");
    expect(result.price?.rates[0].input).toBe("0.0000006");
  });

  it("honors disabled groups and treats absent historical visibility as common", () => {
    const config = createDefaultPricing();
    config.groups.standard.enabled = false;
    delete config.groups.pro.visibility;
    expect(getPublicCatalogPrice(model(config), config, at).groupId).toBe(
      "pro",
    );
    for (const group of Object.values(config.groups)) group.enabled = false;
    expect(getPublicCatalogPrice(model(config), config, at)).toMatchObject({
      status: "no-public-group",
      price: null,
    });
  });

  it.each([
    "no-placement",
    "disabled-placement",
    "unavailable-resource",
    "disabled-channel",
  ])("does not publish a cheap rate with %s", (reason) => {
    const config = createDefaultPricing();
    const resources = config.resources!.instances.filter(
      (resource) => resource.canonicalModel === "gpt-4.1",
    );
    // Leave one usable pro resource on channel 2, while standard depends on channel 1.
    for (const resource of resources)
      config.placements![resource.id].groupIds =
        resource.channelId === "channel-1" ? ["standard"] : ["pro"];
    const resource = resources.find((item) => item.channelId === "channel-1")!;
    if (reason === "no-placement") delete config.placements![resource.id];
    if (reason === "disabled-placement")
      config.placements![resource.id].enabled = false;
    if (reason === "unavailable-resource") resource.status = "unavailable";
    if (reason === "disabled-channel")
      config.resources!.channels.find(
        (channel) => channel.id === resource.channelId,
      )!.enabled = false;
    expect(getPublicCatalogPrice(model(config), config, at)).toMatchObject({
      status: "ready",
      groupId: "pro",
    });
  });

  it("requires a usable mapped instance rather than trusting a stale availability matrix", () => {
    const config = createDefaultPricing();
    for (const placement of Object.values(config.placements!))
      if (placement.modelId === "gpt-4.1") placement.modelId = "qwen-plus";
    expect(getPublicCatalogPrice(model(config), config, at)).toMatchObject({
      status: "no-resource",
      price: null,
    });
    delete config.resources;
    expect(getPublicCatalogPrice(model(config), config, at).status).toBe(
      "no-resource",
    );
  });

  it("distinguishes unknown, deleted and offline models without reusing another model's quote", () => {
    const config = createDefaultPricing();
    const stale = model(config)!;
    expect(getPublicCatalogPrice(undefined, config, at).status).toBe("unknown");
    expect(
      getPublicCatalogPrice({ ...stale, id: "does-not-exist" }, config, at)
        .status,
    ).toBe("unknown");
    config.catalog![stale.id].online = false;
    expect(getPublicCatalogPrice(stale, config, at)).toMatchObject({
      status: "offline",
      price: null,
    });
    delete config.catalog![stale.id];
    expect(getPublicCatalogPrice(stale, config, at).status).toBe("unknown");
  });

  it.each([
    { id: "veyra-demo-image", field: "images", amount: "16000000" },
    { id: "veyra-demo-video", field: "seconds", amount: "32000000" },
  ])(
    "uses actual $field units for $id, not unused zero token prices",
    ({ id, field, amount }) => {
      const config = createDefaultPricing();
      const result = getPublicCatalogPrice(model(config, id), config, at);
      expect(result).toMatchObject({
        status: "ready",
        mode: "unit",
        groupId: "lite",
      });
      expect(result.price!.rates[0].usage).toHaveProperty(field, 1);
      expect(result.price!.minAmountNano).toBe(amount);
    },
  );

  it("labels expressions as example charges and never converts them into fixed unit prices", () => {
    const config = createDefaultPricing();
    config.models["gpt-4.1"].rule = {
      mode: "expression",
      expression: "requests * 0.1 + input_tokens * 0.00001",
    };
    const result = getPublicCatalogPrice(model(config), config, at);
    expect(result).toMatchObject({
      status: "ready",
      mode: "expression",
      groupId: "standard",
    });
    expect(result.price!.minAmountNano).toBe("66000000");
    expect(result.price!.rates[0].usage).toMatchObject({
      requests: 1,
      input_tokens: 1000,
      output_tokens: 300,
    });
  });

  it("keeps the declared image specification instead of searching unrelated conditions for a headline bargain", () => {
    const config = createDefaultPricing();
    config.models["veyra-demo-image"].rule = {
      mode: "unit",
      unit: "image",
      priceUsd: "0.04",
      minimum: "0",
      ceilTo: "1",
      conditionRates: [
        {
          id: "large",
          label: "高清",
          when: { resolution: "1024x1024", quality: "high" },
          priceUsd: "0.1",
        },
        {
          id: "small",
          label: "小图",
          when: { resolution: "512x512", quality: "low" },
          priceUsd: "0.01",
        },
      ],
    };
    const result = getPublicCatalogPrice(
      model(config, "veyra-demo-image"),
      config,
      at,
    );
    expect(result).toMatchObject({
      status: "ready",
      mode: "unit",
      groupId: "lite",
    });
    expect(result.price!.rates[0].attributes).toEqual({
      resolution: "1024x1024",
      quality: "high",
    });
    expect(result.price!.rates[0].quote?.snapshot.matchedCondition?.id).toBe(
      "large",
    );
    expect(result.price!.minAmountNano).toBe("40000000");
  });

  it("preserves minimum billable quantity with the example instead of displaying its smaller raw usage", () => {
    const config = createDefaultPricing();
    config.models["veyra-demo-video"].rule = {
      mode: "unit",
      unit: "second",
      priceUsd: "0.08",
      minimum: "5",
      ceilTo: "1",
    };
    const result = getPublicCatalogPrice(
      model(config, "veyra-demo-video"),
      config,
      at,
    );
    expect(result.price!.rates[0].usage.seconds).toBe(1);
    expect(result.price!.rates[0].quote?.snapshot.billedQuantity).toBe("5");
    expect(result.price!.minAmountNano).toBe("160000000");
  });

  it("uses one explicit instant and applies time and group factors exactly once", () => {
    const config = createDefaultPricing();
    config.models["gpt-4.1"].rule = {
      mode: "tokens",
      timeWindows: [
        {
          id: "night",
          label: "夜间",
          timezone: "UTC",
          days: [0, 1, 2, 3, 4, 5, 6],
          start: "00:00",
          end: "08:00",
          multiplier: "0.5",
        },
      ],
    };
    const night = getPublicCatalogPrice(model(config), config, at);
    const day = getPublicCatalogPrice(
      model(config),
      config,
      "2026-09-28T12:00:00Z",
    );
    expect(night.price?.rates[0]).toMatchObject({
      input: "0.6",
      output: "2.4",
    });
    expect(night.price?.minAmountNano).toBe("1320000");
    expect(day.price?.minAmountNano).toBe("2640000");
    expect(
      night.price?.rates.every((rate) => rate.quote?.snapshot.quotedAt === at),
    ).toBe(true);
    expect(
      getPublicCatalogPrice(model(config), config, "invalid-date"),
    ).toMatchObject({ status: "quote-error", price: null });
  });

  it("defines routing as the selected public group's candidate interval, excluding media and private prices", () => {
    const config = createDefaultPricing();
    config.groups.standard.visibility = "custom";
    config.groups.standard.accountIds = ["account-demo"];
    config.multipliers.standard = "0.0001";
    const result = getPublicCatalogPrice(
      model(config, "veyra-auto"),
      config,
      at,
    );
    expect(result).toMatchObject({
      status: "ready",
      mode: "smart-group-estimate",
      groupId: "lite",
    });
    expect(result.price).toEqual(
      directoryPrices(model(config, "veyra-auto")!, "lite", config, at, ""),
    );
    expect(result.price!.rates.map((rate) => rate.model.id)).not.toContain(
      "gpt-4.1",
    );
    expect(
      result.price!.rates.every(
        (rate) =>
          rate.model.capabilities.includes("text") &&
          rate.model.protocols.length > 0,
      ),
    ).toBe(true);
    expect(
      result.price!.rates.every(
        (rate) => rate.quote!.snapshot.tierId === "lite",
      ),
    ).toBe(true);
  });

  it("does not misrepresent a partial routing quote as a verified minimum or zero-price fallback", () => {
    const config = createDefaultPricing();
    config.models["gpt-4.1"].rule = {
      mode: "expression",
      expression: "requests / images",
    };
    const specified = getPublicCatalogPrice(model(config), config, at);
    expect(specified).toMatchObject({
      status: "quote-error",
      price: null,
      groupId: null,
    });
    expect(specified.errors[0].message).toContain("除数不能为零");
    const smart = getPublicCatalogPrice(
      model(config, "veyra-auto"),
      config,
      at,
    );
    expect(smart).toMatchObject({ status: "quote-error", price: null });
    expect(smart.quotedGroupCount).toBeGreaterThan(0);
    expect(smart.errors.every((error) => error.modelId === "gpt-4.1")).toBe(
      true,
    );
  });

  it("does not mutate pricing, resource placements or the model being browsed", () => {
    const config = createDefaultPricing();
    const currentModel = model(config)!;
    const before = structuredClone(config);
    const beforeModel = structuredClone(currentModel);
    getPublicCatalogPrice(currentModel, config, at);
    getPublicCatalogPrice(model(config, "veyra-auto"), config, at);
    expect(config).toEqual(before);
    expect(currentModel).toEqual(beforeModel);
  });
});

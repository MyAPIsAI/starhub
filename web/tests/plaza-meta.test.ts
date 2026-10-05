import { describe, expect, it } from "vitest";
import {
  metaToModelItem,
  modelItemToMeta,
  type ModelMeta,
} from "../src/lib/api/plaza-meta";

const meta = (patch: Partial<ModelMeta> = {}): ModelMeta => ({
  model: "gpt-4o",
  name: "GPT-4o Chat",
  vendor: "openai",
  vendor_display: "OpenAI",
  description: "Chat model",
  tags: "text,vision,function_call",
  context_length: 128000,
  max_output: 4096,
  official_input_per_m: 2.500000123,
  official_output_per_m: null,
  official_cache_read_per_m: 0,
  official_cache_write_per_m: null,
  official_per_request: 0.02,
  per_request_usd: 0.01,
  score: 85,
  compatible_groups: "default,vip",
  sort_weight: 12,
  created_at: 1791170443,
  ...patch,
});

describe("model metadata identity", () => {
  it("keeps model names, vendor IDs and display names independent", () => {
    const item = metaToModelItem(meta());
    expect(item).toMatchObject({
      id: "gpt-4o",
      name: "GPT-4o Chat",
      vendor: "openai",
      vendorDisplay: "OpenAI",
    });
    item.name = "GPT-4o Renamed";
    expect(modelItemToMeta(item)).toEqual(meta({ name: item.name }));
  });

  it("falls back to the model ID for legacy metadata without a model name", () => {
    const item = metaToModelItem(meta({ name: undefined }));
    expect(item.name).toBe("gpt-4o");
    expect(item.name).not.toBe(item.vendorDisplay);
    expect(modelItemToMeta(item).vendor_display).toBe("OpenAI");
  });

  it("edits the vendor label without clearing prices, tags or hidden fields", () => {
    const original = meta({ vendor_display: "OpenAI GPT-4o" });
    const item = structuredClone(metaToModelItem(original));
    item.vendorDisplay = "OpenAI";
    expect(modelItemToMeta(item)).toEqual(meta());
    expect(original.vendor_display).toBe("OpenAI GPT-4o");
  });

  it("preserves unknown capabilities while applying edits to exposed fields", () => {
    const item = metaToModelItem(meta());
    item.capabilities = ["text", "reasoning"];
    item.pricing.output_per_million_usd = "10";
    item.vendorDisplay = "";
    expect(modelItemToMeta(item)).toEqual(
      meta({
        tags: "function_call,text,reasoning",
        official_output_per_m: 10,
        vendor_display: "openai",
      }),
    );
  });
});

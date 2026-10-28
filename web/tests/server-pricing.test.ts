import { describe, expect, it } from "vitest";
import {
  ownPrice,
  readServerPricing,
  replaceServerModelRatios,
  serverPricingMode,
  validServerRatio,
} from "../src/lib/server-pricing";
const fixture = {
  version: 3,
  loaded_version: 2,
  atomic_batch: true,
  options: [
    { key: "model_ratio", value: '{"actual":2.5,"unrelated":0}' },
    { key: "completion_ratio", value: '{"actual":4,"unrelated":0}' },
    { key: "model_price", value: '{"flat":0.1}' },
    { key: "model_price_expr", value: '{"dynamic":"requests * 0.1"}' },
    { key: "cache_ratio", value: "0.5" },
    { key: "audio_ratio", value: "40" },
  ],
};
describe("server token pricing", () => {
  it("preserves unrelated exact server entries and explicit zero when replacing a paired model rate", () => {
    const state = readServerPricing(fixture),
      options = replaceServerModelRatios(state, "actual", "3", "5");
    expect(options).toHaveLength(2);
    expect(JSON.parse(options[0].value)).toEqual({ actual: 3, unrelated: 0 });
    expect(JSON.parse(options[1].value)).toEqual({ actual: 5, unrelated: 0 });
    expect(state.input.actual).toBe(2.5);
    expect(ownPrice(state.input, "missing")).toBeUndefined();
    expect(ownPrice(state.input, "unrelated")).toBe(0);
    expect(state.version).toBe(3);
    expect(state.loaded_version).toBe(2);
  });
  it("handles prototype-like model IDs without inheriting a configured rate", () => {
    const state = readServerPricing(fixture);
    expect(ownPrice(state.input, "constructor")).toBeUndefined();
    const options = replaceServerModelRatios(state, "__proto__", "1", "2");
    expect(Object.hasOwn(JSON.parse(options[0].value), "__proto__")).toBe(true);
    expect(Object.getPrototypeOf(state.input)).toBe(Object.prototype);
  });
  it("keeps per-request and expression modes outside the token editor", () => {
    const state = readServerPricing(fixture);
    expect(serverPricingMode(state, "flat")).toBe("per_request");
    expect(serverPricingMode(state, "dynamic")).toBe("expression");
    for (const id of ["flat", "dynamic"])
      expect(() => replaceServerModelRatios(state, id, "1", "2")).toThrow();
  });
  it("rejects missing, malformed and unsupported snapshots instead of inventing prices", () => {
    for (const value of [
      { ...fixture, atomic_batch: undefined },
      { ...fixture, version: Number.MAX_SAFE_INTEGER + 1 },
      { ...fixture, loaded_version: -1 },
      { ...fixture, options: fixture.options.slice(1) },
      {
        ...fixture,
        options: [...fixture.options.slice(1), fixture.options[1]],
      },
      {
        ...fixture,
        options: fixture.options.map((o) =>
          o.key === "model_ratio" ? { ...o, value: "null" } : o,
        ),
      },
      {
        ...fixture,
        options: fixture.options.map((o) =>
          o.key === "model_ratio" ? { ...o, value: '{"bad":-1}' } : o,
        ),
      },
      {
        ...fixture,
        options: fixture.options.map((o) =>
          o.key === "audio_ratio" ? { ...o, value: "1001" } : o,
        ),
      },
    ])
      expect(() => readServerPricing(value)).toThrow();
  });
  it.each(["0", ".5", "1e-7", "1000000000"])(
    "accepts finite configured ratio %s",
    (value) => expect(validServerRatio(value)).toBe(true),
  );
  it.each(["", "-1", "NaN", "Infinity", "0x10", "1e999", "1000000001"])(
    "rejects invalid ratio %s",
    (value) => expect(validServerRatio(value)).toBe(false),
  );
});

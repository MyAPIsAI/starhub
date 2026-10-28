import { describe, expect, it } from "vitest";
import {
  ownPrice,
  readServerPricing,
  replaceServerModelRatios,
  serverPricingMode,
  validServerRatio,
  validServerRequestPrice,
  replaceServerRequestPrice,
  replaceServerExpression,
  validServerExpression,
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
  it("edits only an existing request price while preserving unrelated prices and inactive zero entries", () => {
    const state = readServerPricing(fixture);
    state.perRequest = { flat: 0.1, other: 0.0000001, inactive: 0 };
    const before = structuredClone(state);
    expect(replaceServerRequestPrice(state, "flat", "2.5e-4")).toEqual([
      {
        key: "model_price",
        value: JSON.stringify({ flat: 0.00025, other: 0.0000001, inactive: 0 }),
      },
    ]);
    expect(state).toEqual(before);
    for (const id of ["actual", "dynamic", "missing", "inactive"])
      expect(() => replaceServerRequestPrice(state, id, "1")).toThrow();
    state.expressions.flat = "requests * 0.2";
    expect(() => replaceServerRequestPrice(state, "flat", "1")).toThrow();
  });
  it.each(["0.000000001", "1e-7", "10000"])(
    "accepts request price %s",
    (value) => {
      expect(validServerRequestPrice(value)).toBe(true);
    },
  );
  it.each(["", "0", "-1", "1e-999", "NaN", "Infinity", "0x10", "10000.01"])(
    "rejects request price or mode change %s",
    (value) => {
      expect(validServerRequestPrice(value)).toBe(false);
      expect(() =>
        replaceServerRequestPrice(readServerPricing(fixture), "flat", value),
      ).toThrow();
    },
  );
  it("preserves expression maps and fallback prices while editing only the selected existing rule", () => {
    const state = readServerPricing(fixture);
    state.expressions.other = "0";
    const before = structuredClone(state),
      expression = "total_tokens <= 1000 ? 0 : 0.001";
    expect(replaceServerExpression(state, "dynamic", expression)).toEqual([
      {
        key: "model_price_expr",
        value: JSON.stringify({ dynamic: expression, other: "0" }),
      },
    ]);
    expect(state).toEqual(before);
    for (const id of ["flat", "actual", "missing"])
      expect(() => replaceServerExpression(state, id, expression)).toThrow();
  });
  it("matches the engine byte limit and prevents empty mode changes without duplicating server grammar", () => {
    expect(validServerExpression("0")).toBe(true);
    expect(validServerExpression("1".repeat(512))).toBe(true);
    for (const expression of ["", " \n ", "1".repeat(513), "界".repeat(171)]) {
      expect(validServerExpression(expression)).toBe(false);
      expect(() =>
        replaceServerExpression(
          readServerPricing(fixture),
          "dynamic",
          expression,
        ),
      ).toThrow();
    }
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

import { describe, expect, it } from "vitest";
import {
  emptyTokenTimeWindow,
  readServerTokenTimePricing,
  readTokenTimeAcknowledgement,
  replaceTokenTimeSchedule,
  tokenPriceKeys,
  tokenTimeScheduleError,
  validTokenWindowPrice,
} from "../src/lib/server-time-token-pricing";

const fixture = () => ({
  version: 3,
  loaded_version: 2,
  revision_guards: true,
  schema: "weekly_token_prices_v1",
  billing_active: true,
  token_only: true,
  asynchronous_tasks: false,
  multiplier_behavior: "stacked",
  rules: {
    model: {
      timezone: "UTC",
      windows: [
        {
          id: "night",
          label: "Night",
          days: [0],
          start: "23:00",
          end: "06:00",
          prices: {
            input_per_m: "1.230000000",
            output_per_m: "4.56",
            cache_read_per_m: "0",
            cache_write_per_m: "2.5",
            audio_input_per_m: "0.000000001",
            audio_output_per_m: "1000000",
          },
        },
      ],
    },
  },
});

describe("actual independent token schedule contracts", () => {
  it("retains all six exact prices and returns detached windows", () => {
    const input = fixture(),
      state = readServerTokenTimePricing(input);
    expect(state.rules.model.windows[0].prices.input_per_m).toBe("1.230000000");
    input.rules.model.windows[0].prices.cache_write_per_m = "99";
    input.rules.model.windows[0].days.push(2);
    expect(state.rules.model.windows[0].prices.cache_write_per_m).toBe("2.5");
    expect(state.rules.model.windows[0].days).toEqual([0]);
    const edited = replaceTokenTimeSchedule(
      state,
      "another",
      state.rules.model,
    );
    edited.model.windows[0].prices.input_per_m = "999";
    edited.another.windows[0].days.push(4);
    expect(state.rules.model.windows[0].prices.input_per_m).toBe("1.230000000");
    expect(state.rules.model.windows[0].days).toEqual([0]);
    expect(edited.model.windows[0].days).toEqual([0]);
    expect(replaceTokenTimeSchedule(state, "model", null)).toEqual({});
    expect(state.version).toBe(3);
    expect(state.loaded_version).toBe(2);
    for (const model of ["__proto__", "constructor", "toString"]) {
      const special = replaceTokenTimeSchedule(state, model, state.rules.model);
      expect(Object.hasOwn(special, model)).toBe(true);
      expect(special[model].windows[0].prices.cache_write_per_m).toBe("2.5");
      expect(Object.getPrototypeOf(special)).toBe(Object.prototype);
      expect(Object.hasOwn(state.rules, model)).toBe(false);
    }
  });
  it("checks exact decimal boundaries without Number conversion or exceptions", () => {
    for (const price of [
      "0",
      "1.230000000",
      "0.000000001",
      "999999.999999999",
      "1000000.000000000",
    ])
      expect(validTokenWindowPrice(price)).toBe(true);
    for (const price of [
      "",
      null,
      1,
      "1e2",
      "-1",
      "+1",
      "01",
      ".5",
      "1.",
      " 1",
      "1\n",
      "0.0000000001",
      "1000000.000000001",
      "9999999",
      "NaN",
    ])
      expect(validTokenWindowPrice(price)).toBe(false);
    const input = fixture();
    for (const key of tokenPriceKeys)
      input.rules.model.windows[0].prices[key] = "0";
    expect(
      readServerTokenTimePricing(input).rules.model.windows[0].prices,
    ).toEqual(input.rules.model.windows[0].prices);
    const empty = emptyTokenTimeWindow();
    expect(tokenPriceKeys.every((key) => empty.prices[key] === "")).toBe(true);
  });
  it("refuses absent capabilities, unsupported billing and malformed price classes", () => {
    for (const patch of [
      { schema: "weekly_multiplier_v1" },
      { revision_guards: false },
      { billing_active: false },
      { token_only: false },
      { asynchronous_tasks: true },
      { asynchronous_tasks: undefined },
      { multiplier_behavior: "replaced" },
      { version: -1 },
      { loaded_version: NaN },
    ]) {
      expect(() =>
        readServerTokenTimePricing({ ...fixture(), ...patch }),
      ).toThrow();
      expect(() =>
        readTokenTimeAcknowledgement({ ...fixture(), ...patch }),
      ).toThrow();
    }
    for (const key of tokenPriceKeys) {
      const input: any = fixture();
      delete input.rules.model.windows[0].prices[key];
      expect(() => readServerTokenTimePricing(input)).toThrow();
    }
    for (const patch of [
      { input_per_m: 1 },
      { input_per_m: null },
      { unknown: "1" },
    ]) {
      const input = fixture();
      Object.assign(input.rules.model.windows[0].prices, patch);
      expect(() => readServerTokenTimePricing(input)).toThrow();
    }
    const input = fixture();
    Object.assign(input.rules.model.windows[0], { multiplier: 1 });
    expect(() => readServerTokenTimePricing(input)).toThrow();
  });
  it("shares weekly validation and rejects virtual models, overlapping windows and control suffixes", () => {
    for (const patch of [
      { days: [] },
      { days: [7] },
      { id: "night\n" },
      { start: "23:00\n" },
      { end: "06:00\n" },
      { label: "" },
    ]) {
      const input = fixture();
      Object.assign(input.rules.model.windows[0], patch);
      expect(() => readServerTokenTimePricing(input)).toThrow();
    }
    for (const model of ["myapis-auto", "veyra-auto"]) {
      const input = fixture();
      expect(() =>
        readServerTokenTimePricing({
          ...input,
          rules: { [model]: input.rules.model },
        }),
      ).toThrow();
    }
    const input = fixture();
    input.rules.model.windows.push({
      ...structuredClone(input.rules.model.windows[0]),
      id: "overlap",
    });
    expect(() => readServerTokenTimePricing(input)).toThrow();
    const state = readServerTokenTimePricing(fixture());
    const draft = structuredClone(state.rules.model);
    draft.windows[0].prices.input_per_m = "";
    expect(tokenTimeScheduleError(draft)).toBe("prices");
    expect(() => replaceTokenTimeSchedule(state, "model", draft)).toThrow();
  });
});

import { describe, expect, it } from "vitest";
import {
  parseServiceLevelTargets,
  readServiceLevel,
  successPercent,
} from "../src/lib/service-level";

describe("published service-level targets", () => {
  it.each([0, 1, 999999, 1000000, 995000])(
    "round-trips exact ppm %s",
    (ppm) => {
      expect(parseServiceLevelTargets(successPercent(ppm), "")).toEqual({
        success_rate_ppm: ppm,
        p95_latency_ms: null,
      });
    },
  );
  it.each([
    ["", ""],
    ["100.0001", ""],
    ["-1", ""],
    ["1e2", ""],
    ["99.99999", ""],
    ["1,2", ""],
    ["NaN", "1"],
    ["", "0"],
    ["", "1.2"],
    ["", "86400001"],
    ["", "1e3"],
  ])("rejects invalid publication %s/%s", (success, latency) => {
    expect(parseServiceLevelTargets(success, latency)).toBeUndefined();
  });
  it("supports a single metric and preserves zero rather than absence", () => {
    expect(parseServiceLevelTargets(" 0 ", " 1 ")).toEqual({
      success_rate_ppm: 0,
      p95_latency_ms: 1,
    });
    expect(parseServiceLevelTargets("", "86400000")).toEqual({
      success_rate_ppm: null,
      p95_latency_ms: 86400000,
    });
    const value = {
      revision: 1,
      updated_at: 1,
      targets: { success_rate_ppm: 0, p95_latency_ms: null },
    };
    expect(readServiceLevel(value)).toEqual(value);
    expect(
      readServiceLevel({ revision: 0, updated_at: 0, targets: null })?.targets,
    ).toBeNull();
  });
  it.each([
    undefined,
    null,
    {},
    {
      revision: 0,
      updated_at: 0,
      targets: { success_rate_ppm: 0, p95_latency_ms: 1 },
    },
    { revision: 1, updated_at: 1, targets: {} },
    {
      revision: 1,
      updated_at: 1,
      targets: { success_rate_ppm: null, p95_latency_ms: null },
    },
    {
      revision: 1,
      updated_at: 1,
      targets: { success_rate_ppm: 1000001, p95_latency_ms: null },
    },
    {
      revision: 1,
      updated_at: 1,
      targets: { success_rate_ppm: 0, p95_latency_ms: 0 },
    },
    { revision: Number.MAX_SAFE_INTEGER + 1, updated_at: 1, targets: null },
  ])("does not manufacture editable metadata from %j", (value) => {
    expect(readServiceLevel(value)).toBeUndefined();
  });
});

import { describe, expect, it } from "vitest";
import { catalogModels } from "../config/models";
import {
  createQualityTrend,
  createQualityUnits,
  filterQualityUnits,
  getQualityMonths,
  serializeQualityCsv,
  summarizeQualityUnits,
} from "./service-quality-demo";

describe("service quality demonstration data", () => {
  it("respects catalog model/group availability and request-type metric boundaries", () => {
    const units = createQualityUnits("2026-08");
    for (const unit of units) {
      expect(
        catalogModels.find((model) => model.id === unit.model)
          ?.tierAvailability[unit.group],
      ).toBe(true);
      expect(unit.buckets.length).toBe(unit.stream ? 3 : 0);
      if (!unit.stream) expect(unit.performanceStatus).toBeNull();
      expect(unit.successes).toBeLessThanOrEqual(unit.requests);
    }
    expect(units.some((unit) => unit.group === "lite")).toBe(true);
  });
  it("combines filters without leaking other customers and represents every natural day", () => {
    const units = createQualityUnits("2026-08");
    const filtered = filterQualityUnits(units, {
      userId: 1001,
      group: "ultra",
      model: "gpt-4.1",
      stream: true,
    });
    expect(filtered).toHaveLength(1);
    expect(filterQualityUnits(units, { userId: 9999 })).toEqual([]);
    expect(
      filterQualityUnits(units, { group: "lite", model: "gpt-4.1" }),
    ).toEqual([]);
    const trend = createQualityTrend(filtered[0], "2026-08");
    expect(trend).toHaveLength(31);
    expect(trend[0].date).toBe("2026-08-01");
    expect(trend[30].date).toBe("2026-08-31");
    const current = createQualityUnits(getQualityMonths()[0])[0];
    expect(current.requests).not.toBe(units[0].requests);
    expect(
      summarizeQualityUnits(
        filterQualityUnits(createQualityUnits(getQualityMonths()[0]), {
          userId: 1001,
        }),
      ),
    ).toEqual({
      total: 8,
      successAttention: 2,
      performanceAttention: 3,
      logicalRequests: 143060,
    });
    expect(summarizeQualityUnits([])).toEqual({
      total: 0,
      successAttention: 0,
      performanceAttention: 0,
      logicalRequests: 0,
    });
  });
  it("preserves CSV cells containing quotes, commas and line breaks, including an empty report", () => {
    expect(
      serializeQualityCsv([
        ["Model", "Note"],
        ["gpt-4.1", 'a,"b"\nnext'],
      ]),
    ).toBe('\uFEFF"Model","Note"\r\n"gpt-4.1","a,""b""\nnext"');
    expect(serializeQualityCsv([["Model", "Note"]])).toBe(
      '\uFEFF"Model","Note"',
    );
  });
});

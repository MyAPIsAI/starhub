import { describe, expect, it } from "vitest";
import {
  dubaiDate,
  filterRequestHistory,
  requestTimeBounds,
  type RequestHistoryFilters,
} from "../src/lib/request-history";
import type { PreviewRequest } from "../src/lib/product-preview";
const now = Date.parse("2026-09-27T10:30:00.000Z");
const defaults: RequestHistoryFilters = {
  range: "all",
  start: "",
  end: "",
  query: "",
  status: "all",
};
const row = (time: string, overrides: Partial<PreviewRequest> = {}) =>
  ({
    time,
    trace_id: time,
    model: "gpt-4.1",
    requested_model: "veyra-auto",
    actual_model: "gpt-4.1",
    status: "success",
    ...overrides,
  }) as PreviewRequest;

describe("request history Dubai date semantics", () => {
  it("changes date at Dubai midnight rather than UTC midnight", () => {
    expect(dubaiDate("2026-09-26T19:59:59.999Z")).toBe("2026-09-26");
    expect(dubaiDate("2026-09-26T20:00:00.000Z")).toBe("2026-09-27");
  });
  it("today includes both Dubai day boundaries and excludes adjacent days", () => {
    const rows = [
      "2026-09-26T19:59:59.999Z",
      "2026-09-26T20:00:00.000Z",
      "2026-09-27T19:59:59.999Z",
      "2026-09-27T20:00:00.000Z",
    ].map((time) => row(time));
    expect(
      filterRequestHistory(rows, { ...defaults, range: "today" }, now).rows.map(
        (r) => r.time,
      ),
    ).toEqual([rows[2].time, rows[1].time]);
  });
  it("last seven days means today plus six complete calendar dates", () => {
    expect(requestTimeBounds({ ...defaults, range: "7d" }, now)).toEqual({
      from: Date.parse("2026-09-20T20:00:00Z"),
      until: Date.parse("2026-09-27T20:00:00Z"),
    });
  });
  it("last thirty calendar days crosses month boundaries", () => {
    expect(requestTimeBounds({ ...defaults, range: "30d" }, now).from).toBe(
      Date.parse("2026-08-28T20:00:00Z"),
    );
  });
  it("custom end date includes the complete final day", () => {
    const bounds = requestTimeBounds(
      { range: "custom", start: "2026-09-01", end: "2026-09-02" },
      now,
    );
    expect(bounds).toEqual({
      from: Date.parse("2026-08-31T20:00:00Z"),
      until: Date.parse("2026-09-02T20:00:00Z"),
    });
    const final = row("2026-09-02T19:59:59.999Z");
    expect(
      filterRequestHistory([final, row("2026-09-02T20:00:00Z")], {
        ...defaults,
        range: "custom",
        start: "2026-09-01",
        end: "2026-09-02",
      }).rows,
    ).toEqual([final]);
  });
  it("a same-day custom range is valid", () => {
    const bounds = requestTimeBounds({
      range: "custom",
      start: "2026-09-01",
      end: "2026-09-01",
    });
    expect(bounds.error).toBeUndefined();
    expect(bounds.until - bounds.from).toBe(86400000);
  });
  it("rejects missing, impossible and reversed dates without silently filtering another range", () => {
    expect(
      requestTimeBounds({ range: "custom", start: "", end: "2026-09-01" })
        .error,
    ).toBe("missing_dates");
    expect(
      requestTimeBounds({
        range: "custom",
        start: "2026-02-30",
        end: "2026-09-01",
      }).error,
    ).toBe("invalid_date");
    expect(
      requestTimeBounds({
        range: "custom",
        start: "2026-09-02",
        end: "2026-09-01",
      }).error,
    ).toBe("reversed_dates");
    expect(
      filterRequestHistory([row("2026-09-27T08:00:00Z")], {
        ...defaults,
        range: "custom",
      }).rows,
    ).toEqual([]);
  });
  it("all-time includes old history independent of overview ranges", () => {
    const old = row("2020-01-01T00:00:00Z");
    expect(filterRequestHistory([old], defaults, now).rows).toEqual([old]);
  });
  it("combines case-insensitive search, status and date restrictions", () => {
    const expected = row("2026-09-27T05:00:00Z", {
      status: "failed",
      error_code: "MODEL_NOT_AUTHORIZED",
    });
    const rows = [
      expected,
      row("2026-09-27T05:00:00Z"),
      row("2026-08-01T05:00:00Z", {
        status: "failed",
        error_code: "MODEL_NOT_AUTHORIZED",
      }),
    ];
    expect(
      filterRequestHistory(
        rows,
        {
          ...defaults,
          range: "today",
          query: "  authorized ",
          status: "failed",
        },
        now,
      ).rows,
    ).toEqual([expected]);
  });
  it("searches requested and actual models as well as legacy model IDs", () => {
    const request = row("2026-09-27T05:00:00Z");
    expect(
      filterRequestHistory([request], { ...defaults, query: "veyra-auto" })
        .rows,
    ).toEqual([request]);
    expect(
      filterRequestHistory([request], { ...defaults, query: "GPT-4.1" }).rows,
    ).toEqual([request]);
    const legacy = row("2026-09-27T05:00:00Z", {
      requested_model: undefined,
      actual_model: undefined,
    });
    expect(
      filterRequestHistory([legacy], { ...defaults, query: "gpt-4.1" }).rows,
    ).toEqual([legacy]);
  });
  it("keeps all matching records, sorts newest first, and does not mutate the source", () => {
    const rows = Array.from({ length: 125 }, (_, index) =>
      row(new Date(now + index * 1000).toISOString()),
    );
    const before = [...rows];
    const filtered = filterRequestHistory(rows, defaults).rows;
    expect(filtered).toHaveLength(125);
    expect(filtered[0]).toBe(rows[124]);
    expect(rows).toEqual(before);
  });
  it("does not treat an invalid timestamp as a matching request", () => {
    expect(filterRequestHistory([row("not-a-date")], defaults).rows).toEqual(
      [],
    );
  });
});

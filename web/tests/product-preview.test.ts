import { beforeEach, describe, expect, it } from "vitest";
import {
  activityFor,
  breakdownByModel,
  makePreviewRequests,
  periodRequests,
  summarizeRequests,
  useProductPreview,
} from "../src/lib/product-preview";
import { formatMoney, sumNano } from "../src/lib/money";

describe("preview flow consistency", () => {
  beforeEach(() => {
    useProductPreview.setState(useProductPreview.getInitialState(), true);
    useProductPreview.getState().updateKey("pk_1", { ipWhitelist: [] });
  });
  it("uses the same Dubai calendar period for totals, charts and rows", () => {
    const records = makePreviewRequests();
    for (const days of [7, 30]) {
      const rows = periodRequests(records, days),
        summary = summarizeRequests(rows),
        chart = activityFor(rows, days);
      expect(chart.reduce((sum, p) => sum + p.requests, 0)).toBe(summary.count);
      expect(summary.cost).toBe(sumNano(rows.map((r) => r.cost_nano)));
      expect(chart).toHaveLength(days);
    }
    expect(periodRequests(records, 7)).toHaveLength(1095);
    expect(periodRequests(records, 30).length).toBeGreaterThan(1095);
  });
  it("reconciles the per-model breakdown with the summary cards", () => {
    const records = makePreviewRequests();
    for (const days of [7, 30]) {
      const rows = periodRequests(records, days),
        summary = summarizeRequests(rows),
        breakdown = breakdownByModel(rows);
      // 调用数按模型分组后必须正好回到周期总量。
      expect(breakdown.reduce((sum, m) => sum + m.count, 0)).toBe(
        summary.count,
      );
      // 各模型消费按显示精度配平后，加总必须等于「累计消费」卡片。
      expect(
        formatMoney(
          breakdown.reduce((sum, m) => sum + BigInt(m.cost), 0n).toString(),
          "total",
        ),
      ).toBe(formatMoney(summary.cost, "total"));
      // 有调用的模型才有百分比；没有的行显示破折号。
      for (const model of breakdown) {
        expect(model.count).toBeGreaterThan(0);
        expect(model.success).toMatch(/^\d+(?:\.\d+)?$/);
      }
    }
  });
  it("creates a key and records a successful request with exact balance reconciliation", () => {
    const before = useProductPreview.getState();
    const key = before.createKey("客服助手", "pro", "gpt-4.1");
    const row = useProductPreview.getState().recordCall(key.id, false)!;
    const after = useProductPreview.getState();
    expect(row.source).toBe("playground");
    expect(after.selectedKey).toBe(key.id);
    expect(after.requests[0]).toBe(row);
    expect(BigInt(after.balance)).toBe(
      BigInt(before.balance) - BigInt(row.cost_nano),
    );
    expect(BigInt(after.opening)).toBe(
      BigInt(after.balance) +
        BigInt(sumNano(after.requests.map((r) => r.cost_nano))),
    );
  });
  it("records failure for diagnosis without charging", () => {
    const before = useProductPreview.getState();
    const row = before.recordCall("pk_1", true)!;
    expect(row.cost_nano).toBe("0");
    expect(row.error_code).toBe("upstream_unavailable");
    expect(useProductPreview.getState().balance).toBe(before.balance);
    expect(useProductPreview.getState().requests.length).toBe(
      before.requests.length + 1,
    );
  });
  it("refuses requests through disabled keys", () => {
    const state = useProductPreview.getState();
    state.toggleKey("pk_1");
    expect(
      useProductPreview.getState().recordCall("pk_1", false),
    ).toMatchObject({
      status: "failed",
      error_code: "key_disabled",
      cost_nano: "0",
    });
    expect(useProductPreview.getState().requests).toHaveLength(
      state.requests.length + 1,
    );
    expect(useProductPreview.getState().balance).toBe(state.balance);
  });
});

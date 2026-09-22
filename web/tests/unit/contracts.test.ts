import { describe, expect, it } from "vitest";
import { fillDays, integer, quota, usdToQuota } from "@/lib/format";
import {
  buildPayload,
  makeDraft,
  resources,
} from "@/features/resources/schema";
import { optionDraft, serializeOption } from "@/features/options/validation";
import { canView, canEdit, safeRedirect } from "@/router/capabilities";
import en from "@/locales/en-US.json";
import zh from "@/locales/zh-CN.json";

describe("billing display and integer input", () => {
  it("preserves the minimum billable quota and rejects silent rounding", () => {
    expect(quota(1, "en-US")).toBe("$0.000002");
    expect(usdToQuota("0.000002")).toBe(1);
    expect(usdToQuota("1.234568")).toBe(617284);
    expect(() => usdToQuota("0.000001")).toThrow();
    for (const value of ["1e6", "1.5", "9007199254740992", ""])
      expect(() => integer(value)).toThrow();
    expect(integer("0")).toBe(0);
    expect(quota(null)).toBe("—");
  });
  it("fills UTC days without mixing shortfalls into collected quota", () => {
    const rows = fillDays(
      [
        {
          date: "2026-09-21",
          requests: 2,
          quota: 1,
          unfunded_quota: 3,
          prompt_tokens: 0,
          completion_tokens: 0,
        },
      ],
      3,
      new Date("2026-09-22T23:59:00Z"),
    );
    expect(rows.map((r) => r.date)).toEqual([
      "2026-09-20",
      "2026-09-21",
      "2026-09-22",
    ]);
    expect(rows.map((r) => r.quota)).toEqual([0, 1, 0]);
    expect(rows[1]?.unfunded_quota).toBe(3);
  });
});

describe("management form contracts", () => {
  it("sends explicit zero, false, empty fields, and expected balances", () => {
    const row = {
      id: 1,
      name: "key",
      remain_quota: 10,
      unlimited_quota: true,
      model_limits: "model",
      group: "default",
      allow_ips: "127.0.0.1",
    };
    const draft = {
      ...makeDraft("tokens", row),
      remain_quota: "0",
      unlimited_quota: false,
      model_limits: "",
      group: "",
      allow_ips: "",
    };
    expect(buildPayload("tokens", draft, row)).toEqual({
      id: 1,
      remain_quota: 0,
      expected_quota: 10,
      unlimited_quota: false,
      model_limits: "",
      group: "",
      allow_ips: "",
    });
  });
  it("does not re-submit masked provider keys or unchanged balances", () => {
    const row = {
      id: 3,
      name: "provider",
      key: "sk-***",
      type: 1,
      models: "m",
      group: "default",
      base_url: "https://example.com",
    };
    const draft = { ...makeDraft("channels", row), base_url: "" };
    expect(makeDraft("channels", row).key).toBe("");
    expect(buildPayload("channels", draft, row)).toEqual({
      id: 3,
      base_url: "",
    });
    const user = { id: 2, quota: 123, group: "default", status: 1 };
    expect(
      buildPayload("users", { ...makeDraft("users", user), status: "2" }, user),
    ).toEqual({ id: 2, status: 2 });
  });
  it("validates auto groups and ignores custom batch redemption codes", () => {
    expect(() =>
      buildPayload("tokens", { ...makeDraft("tokens"), group: "auto" }),
    ).toThrow();
    expect(
      buildPayload("redemptions", {
        ...makeDraft("redemptions"),
        count: "2",
        code: "custom",
      }),
    ).not.toHaveProperty("code");
  });
  it("validates option structure and keeps expressions as strings", () => {
    expect(() => serializeOption("group_ratio", '{"default":-1}')).toThrow();
    expect(() =>
      serializeOption("group_group_ratio", '{"default":2}'),
    ).toThrow();
    expect(() => serializeOption("model_price", '{"m":10001}')).toThrow();
    expect(() => serializeOption("aff_rebate_ratio", "1.01")).toThrow();
    expect(() => serializeOption("model_price_expr", '{"m":12}')).toThrow();
    expect(serializeOption("model_price_expr", '{"m":"p * 2"}')).toBe(
      '{"m":"p * 2"}',
    );
    expect(serializeOption("alert_webhook_url", "")).toBe('""');
    expect(optionDraft("alert_webhook_url", '"https://example.com"')).toBe(
      "https://example.com",
    );
  });
});

describe("permissions and translations", () => {
  it("limits member navigation and root-only account edits", () => {
    expect(canView(1, "channels")).toBe(false);
    expect(canView(10, "users")).toBe(true);
    expect(canEdit(10, "users")).toBe(false);
    expect(canEdit(100, "users")).toBe(true);
    expect(canEdit(1, "tokens")).toBe(true);
    expect(safeRedirect("//evil.example")).toBe("/console/overview");
    expect(safeRedirect("/console/tokens")).toBe("/console/tokens");
  });
  it("has matching locale keys and translates every resource field", () => {
    expect(Object.keys(en).sort()).toEqual(Object.keys(zh).sort());
    for (const resource of Object.values(resources))
      for (const key of [
        ...resource.columns,
        ...resource.fields.map((f) => f.key),
      ]) {
        if (key !== "actor") expect(en).toHaveProperty(key);
      }
  });
});

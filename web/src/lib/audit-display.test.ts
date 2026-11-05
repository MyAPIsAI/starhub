import { describe, expect, it } from "vitest";
import {
  auditActionLabel,
  auditAuthResult,
  auditChanges,
  auditObjectLabel,
  auditRoleLabel,
  auditSearchActions,
  auditSummary,
} from "./audit-display";

const zh = (value: string) => value;
const en = (_: string, value: string) => value;
const ar = (_: string, __: string, value: string) => value;

describe("audit presentation", () => {
  it("localizes staff events, roles and verification outcomes", () => {
    expect(auditActionLabel("staff_2fa_verified", zh)).toBe("登录后台");
    expect(auditAuthResult("staff_2fa_verified", zh)).toContain("2FA）通过");
    expect(auditObjectLabel("staff", zh)).toBe("后台账号");
    expect(auditRoleLabel(100, zh)).toBe("超级管理员");
    expect(auditSearchActions("登录", zh)).toContain("staff_2fa_verified");
    expect(auditSearchActions(" SIGN IN ", en)).toContain("staff_2fa_verified");
    expect(auditActionLabel("staff_2fa_verified", ar)).toBe(
      "تسجيل الدخول إلى الإدارة",
    );
  });

  it("formats creation without translating user identifiers", () => {
    const changes = auditChanges(
      "",
      JSON.stringify({
        username: "staff_created",
        role: 5,
        status: 1,
        two_factor_bound: false,
      }),
      zh,
    );
    expect(changes).toEqual([
      "用户名: staff_created",
      "角色: 财务",
      "状态: 已启用",
      "双重验证（2FA）: 未绑定",
    ]);
  });

  it("shows changed fields only and distinguishes zero, null and false", () => {
    const before = JSON.stringify({
      username: "root",
      status: 1,
      two_factor_bound: false,
      limit: 0,
      price: null,
    });
    const after = JSON.stringify({
      username: "root",
      status: 2,
      two_factor_bound: true,
      limit: 10,
      price: 0,
    });
    expect(auditChanges(before, after, zh)).toEqual([
      "状态: 已启用 → 已禁用",
      "双重验证（2FA）: 未绑定 → 已绑定",
      "limit: 0 → 10",
      "price: — → 0",
    ]);
    expect(auditChanges(before, before, zh)).toEqual([]);
    expect(auditChanges("", "", zh)).toEqual([]);
  });

  it("combines price snapshots without repeating the component", () => {
    expect(
      auditChanges(
        '{"component":"group_ratio","value":0.75}',
        '{"component":"group_ratio","value":0.7}',
        zh,
      ),
    ).toEqual(["售价倍率: 0.75 → 0.7"]);
    expect(
      auditChanges(
        '{"reference":{"official_input_per_m":0.28}}',
        '{"reference":{"official_input_per_m":0.27}}',
        zh,
      )[0],
    ).toContain("0.28 → 0.27");
  });

  it("isolates before/after numeric sequences in RTL layouts", () => {
    expect(
      auditChanges(
        '{"component":"group_ratio","value":0.75}',
        '{"component":"group_ratio","value":0.7}',
        ar,
      ),
    ).toEqual(["معامل سعر البيع: \u20660.75 → 0.7\u2069"]);
    expect(auditSummary('{"amount_nano":"200000000000"}', ar)).toContain(
      "\u2066$200.0000\u2069",
    );
  });

  it("preserves exact integer amounts and safely handles legacy or future data", () => {
    expect(auditSummary('{"amount_nano":"9007199254740993000"}', zh)).toContain(
      "9,007,199,254.7410",
    );
    expect(auditChanges("legacy <script>", "updated", zh)).toEqual([
      "legacy <script> → updated",
    ]);
    expect(auditSummary('{"constructor":false,"__proto__":{"x":0}}', zh)).toBe(
      "constructor: 否; __proto__: x: 0",
    );
    expect(auditActionLabel("constructor", zh)).toBe(
      "未识别操作 (constructor)",
    );
    expect(auditRoleLabel(777, zh)).toContain("777");
    expect(auditChanges('{"listed":true}', "", zh)).toEqual([
      "上架状态: 已上架 → —",
    ]);
  });
});

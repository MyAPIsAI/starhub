import { describe, expect, it } from "vitest";
import {
  validateSpendableWallet,
  validateMoneyLedger,
  type SpendableWallet,
  type MoneyLedgerPage,
} from "./money-wallet";
import { decimalUsdToNano, quotaMoneyNano, formatMoney } from "../money";
import { tokenToPreviewKey } from "./key-adapter";
import {
  remainingNanoForLifetimeLimit,
  nanoForPeriodLimit,
} from "../key-spending-limit";
import type { Token } from "./tokens";

const wallet: SpendableWallet = {
  schema: "wallet_v2",
  amount_unit: "usd_nano",
  user_id: 1,
  balance_nano: "18446744073709551613999",
  reserved_nano: "1",
  total_nano: "18446744073709551614000",
  spent_nano: "1",
  revision: 2,
  api_quota_linked: true,
  simulation_conversion_available: false,
};
describe("canonical money contracts", () => {
  it("keeps a nano and balances beyond int64 exact", () => {
    expect(validateSpendableWallet(wallet, 1).balance_nano).toBe(
      wallet.balance_nano,
    );
    expect(formatMoney("1", "detail")).toBe("$0.000000001");
    expect(decimalUsdToNano("9007199254.740993001")).toBe(
      "9007199254740993001",
    );
    expect(quotaMoneyNano("1", 0)).toBe("1");
    expect(() =>
      quotaMoneyNano(undefined, Number.MAX_SAFE_INTEGER + 1),
    ).toThrow();
    expect(() => quotaMoneyNano("01", 0)).toThrow();
  });
  it("rejects foreign ownership, simulation linkage and inconsistent balances", () => {
    expect(() => validateSpendableWallet(wallet, 2)).toThrow();
    expect(() =>
      validateSpendableWallet(
        {
          ...wallet,
          simulation_conversion_available: true,
        } as unknown as SpendableWallet,
        1,
      ),
    ).toThrow();
    expect(() =>
      validateSpendableWallet({ ...wallet, total_nano: "1" }, 1),
    ).toThrow();
    expect(() =>
      validateSpendableWallet(
        { ...wallet, balance_nano: 1 } as unknown as SpendableWallet,
        1,
      ),
    ).toThrow();
  });
  it("validates exact journal pages and negative deltas", () => {
    const page: MoneyLedgerPage = {
      schema: "money_ledger_v2",
      amount_unit: "usd_nano",
      user_id: 1,
      page: 1,
      page_size: 10,
      total: 1,
      items: [
        {
          id: 2,
          user_id: 1,
          sequence: 2,
          operation_id: "n2:test:settle",
          type: "consumption",
          available_delta_nano: "-1",
          reserved_delta_nano: "-1",
          spent_delta_nano: "2",
          available_after_nano: "0",
          reserved_after_nano: "0",
          spent_after_nano: "2",
          reason: "Usage",
          funding_source: "system_quota",
          created_at: 1,
        },
      ],
    };
    expect(validateMoneyLedger(page, 1, 1, 10)).toBe(page);
    expect(() => validateMoneyLedger(page, 1, 2, 10)).toThrow();
    expect(() =>
      validateMoneyLedger(
        { ...page, items: [{ ...page.items[0], user_id: 2 }] },
        1,
        1,
        10,
      ),
    ).toThrow();
    expect(() =>
      validateMoneyLedger(
        { ...page, items: [{ ...page.items[0], spent_delta_nano: "1.5" }] },
        1,
        1,
        10,
      ),
    ).toThrow();
  });
  it("edits lifetime budgets without rounding spent or pending fractions", () => {
    const key = tokenToPreviewKey({
      id: 1,
      user_id: 1,
      name: "nano",
      key: "masked",
      status: 1,
      group: "default",
      expired_time: -1,
      remain_quota: 0,
      remain_quota_nano: "1998",
      used_quota: 0,
      used_quota_nano: "1",
      pending_quota: 0,
      pending_quota_nano: "1",
      unlimited_quota: false,
      model_limits: "model",
      allow_ips: "",
      auto_groups: "",
      created_at: 1,
    } satisfies Token);
    expect(key.quotaLimit_nano).toBe("2000");
    expect(key.totalUsed_nano).toBe("1");
    expect(remainingNanoForLifetimeLimit("0.000002001", key)).toBe("2000");
    expect(() => remainingNanoForLifetimeLimit("0.000000001", key)).toThrow(
      "below_used",
    );
    expect(
      nanoForPeriodLimit("0.000000003", {
        ...key,
        quotaPeriod: "daily",
        currentPeriodUsed_nano: "1",
        currentPeriodReserved_nano: "1",
      }),
    ).toBe("3");
    expect(() =>
      nanoForPeriodLimit("0.000000001", {
        ...key,
        quotaPeriod: "daily",
        currentPeriodUsed_nano: "1",
        currentPeriodReserved_nano: "1",
      }),
    ).toThrow("below_used");
  });
});

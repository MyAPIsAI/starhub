import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  parseFinanceSummary,
  type FinanceSummary,
} from "../src/lib/api/finance";
import {
  canReadFinance,
  isAdmin,
  useSystemAccess,
  ROLE_USER,
  ROLE_ROOT,
  ROLE_FINANCE,
} from "../src/lib/system-access";
import { useProductPreview } from "../src/lib/product-preview";
import { FinanceOverview } from "../src/features/finance-overview";
import { FinanceRoleControl } from "../src/features/finance-role-control";
import type { AdminUser } from "../src/lib/api/admin";
import "../src/i18n";
import { formatFinanceAmount } from "../src/lib/finance-format";

// SSR uses Zustand's initial snapshot; selectors here read the current role
// fixture so these tests exercise the component's permission boundary.
vi.mock("../src/lib/system-access", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../src/lib/system-access")>();
  const useRole = Object.assign(
    <T,>(
      selector: (
        state: ReturnType<typeof actual.useSystemAccess.getState>,
      ) => T,
    ) => selector(actual.useSystemAccess.getState()),
    actual.useSystemAccess,
  );
  return { ...actual, useSystemAccess: useRole };
});

const summary: FinanceSummary = {
  since: 100,
  until: 200,
  wallet_balance_nano: "18446744073709551614",
  paid_recharge_nano: "100000000000",
  paid_recharge_count: 1,
  paid_orders_without_timestamp: 0,
  ledger_credits_nano: "100000000000",
  ledger_debits_nano: "2000000000",
  api_collected_nano: "1234000",
  api_unfunded_nano: "8000",
  api_request_count: 3,
  api_success_count: 1,
  api_failed_count: 1,
  api_cancelled_count: 1,
  wallet_and_api_quota_linked: false,
};
const user: AdminUser = {
  id: 7,
  username: "finance-target",
  email: "",
  role: ROLE_USER,
  status: 1,
  quota: 1000,
  used_quota: 0,
  group: "default",
  aff_code: "test",
  inviter_id: 0,
  created_at: 100,
};
const initialRole = useSystemAccess.getState().role;
afterEach(() => {
  useSystemAccess.getState().setRole(initialRole);
  vi.restoreAllMocks();
});

describe("finance precision and role boundaries", () => {
  it("distinguishes small charged amounts from zero", () => {
    expect(formatFinanceAmount("0")).toBe("$0.0000");
    expect(formatFinanceAmount("8000")).toBe("$0.000008");
    expect(formatFinanceAmount("-8000")).toBe("−$0.000008");
    expect(formatFinanceAmount("1")).toBe("<$0.000001");
  });
  it("preserves exact backend money strings beyond safe integers", () => {
    expect(parseFinanceSummary(summary).wallet_balance_nano).toBe(
      "18446744073709551614",
    );
  });
  it.each([
    null,
    {},
    { ...summary, wallet_balance_nano: 18446744073709551614 },
    { ...summary, api_collected_nano: "1e9" },
    { ...summary, api_request_count: -1 },
    { ...summary, until: 0 },
    { ...summary, wallet_and_api_quota_linked: undefined },
  ])("rejects incomplete or lossy finance responses", (value) =>
    expect(() => parseFinanceSummary(value)).toThrow(),
  );
  it.each([0, 1, 4, 5, 10, 11, 99, 100, 101])(
    "allows exact server finance roles only: %s",
    (role) => {
      expect(canReadFinance(role)).toBe([5, 10, 100].includes(role));
      expect(isAdmin(role)).toBe([10, 100].includes(role));
    },
  );
  it("never renders cached finance figures for an unauthorized role", () => {
    const client = new QueryClient();
    client.setQueryData(
      ["finance-summary", useProductPreview.getState().user.id, 30],
      summary,
    );
    useSystemAccess.getState().setRole(ROLE_USER);
    const html = renderToStaticMarkup(
      <QueryClientProvider client={client}>
        <MemoryRouter>
          <FinanceOverview />
        </MemoryRouter>
      </QueryClientProvider>,
    );
    expect(html).not.toContain('data-metric="wallet"');
    expect(html).not.toContain(summary.wallet_balance_nano);
    client.clear();
  });
  it("formats authorized huge totals with integer arithmetic", () => {
    const client = new QueryClient();
    client.setQueryData(
      ["finance-summary", useProductPreview.getState().user.id, 30],
      summary,
    );
    useSystemAccess.getState().setRole(ROLE_FINANCE);
    const html = renderToStaticMarkup(
      <QueryClientProvider client={client}>
        <MemoryRouter>
          <FinanceOverview />
        </MemoryRouter>
      </QueryClientProvider>,
    );
    expect(html).toContain("$18,446,744,073.7096");
    expect(html).toContain(summary.wallet_balance_nano);
    client.clear();
  });
  it.each([ROLE_USER, ROLE_FINANCE, 10, ROLE_ROOT])(
    "exposes grants only to Root: %s",
    (role) => {
      useSystemAccess.getState().setRole(role);
      const client = new QueryClient();
      const html = renderToStaticMarkup(
        <QueryClientProvider client={client}>
          <FinanceRoleControl user={user} onUpdated={() => {}} />
        </QueryClientProvider>,
      );
      expect(html.includes("<button")).toBe(role === ROLE_ROOT);
      client.clear();
    },
  );
  it.each([10, 100])(
    "does not offer finance-role changes for privileged target %s",
    (role) => {
      useSystemAccess.getState().setRole(ROLE_ROOT);
      const client = new QueryClient();
      const html = renderToStaticMarkup(
        <QueryClientProvider client={client}>
          <FinanceRoleControl user={{ ...user, role }} onUpdated={() => {}} />
        </QueryClientProvider>,
      );
      expect(html).toBe("");
      client.clear();
    },
  );
});

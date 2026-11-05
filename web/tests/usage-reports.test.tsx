import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
const fake = vi.hoisted(() => ({ query: {} as any }));
vi.mock("../src/lib/api/usage-report", () => ({
  useUsageReport: () => fake.query,
}));
vi.mock("../src/lib/api/exports", () => ({
  useCreateExport: () => ({ mutate: vi.fn(), isPending: false }),
}));
vi.mock("../src/lib/product-preview", () => {
  throw Error("Usage reports must not import demo data");
});
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ i18n: { language: "zh" } }),
}));
vi.mock("recharts", () => ({
  ResponsiveContainer: ({ children }: any) => <div>{children}</div>,
  AreaChart: ({ children }: any) => <div>{children}</div>,
  Area: () => null,
  CartesianGrid: () => null,
  Tooltip: () => null,
  XAxis: () => null,
  YAxis: () => null,
}));
import UsageReports from "../src/features/usage-reports";
const stats = (id: string, amount: string) => ({
  id,
  name: id,
  requests: 1,
  successful: 0,
  input_tokens: 2,
  output_tokens: 1,
  cache_read_tokens: 1,
  cache_write_tokens: 0,
  amount_nano: amount,
});
beforeEach(() => {
  fake.query = {
    data: {
      timezone: "Asia/Dubai",
      start_date: "2026-11-01",
      end_date: "2026-11-03",
      group_by: "key",
      granularity: "day",
      totals: { ...stats("total", "3000000"), requests: 2 },
      rows: [stats("Key one", "1000000"), stats("Key two", "2000000")],
      by_key: [stats("Key one", "1000000"), stats("Key two", "2000000")],
      by_model: [stats("Model", "3000000")],
      series: [],
      options: {},
    },
    isError: false,
    isFetching: false,
    refetch: vi.fn(),
  };
});
const render = () =>
  renderToStaticMarkup(
    <MemoryRouter>
      <UsageReports />
    </MemoryRouter>,
  );
describe("real usage report rendering", () => {
  it("shows real Key totals and zero success instead of 100 percent", () => {
    const text = render();
    expect(text).toContain("Key one");
    expect(text).toContain("Key two");
    expect(text).toContain("$0.0030");
    expect(text).toContain("0.00%");
    expect(text).not.toContain("100.00%");
    expect(text).toContain("缓存读 / 写 Token");
  });
  it("does not show stale data on a backend error", () => {
    fake.query.isError = true;
    fake.query.error = Error("report unavailable");
    const text = render();
    expect(text).toContain('role="alert"');
    expect(text).toContain("report unavailable");
    expect(text).not.toContain("Key one");
  });
  it("renders an empty report without fabricated samples", () => {
    Object.assign(fake.query.data, {
      totals: { ...stats("total", "0"), requests: 0 },
      rows: [],
      by_key: [],
      by_model: [],
    });
    const text = render();
    expect(text).toContain("当前筛选条件下没有调用");
    expect(text).toContain("$0.0000");
    expect(text).not.toContain("Key one");
  });
  it("keeps the page visible when a legacy server returns a user list", () => {
    fake.query.data = { items: [{ username: "private-account" }], total: 1 };
    const text = render();
    expect(text).toContain('role="alert"');
    expect(text).toContain("报表接口与当前后端版本不兼容");
    expect(text).toContain("用量报表与多维分析");
    expect(text).not.toContain("private-account");
    expect(text).not.toContain("usage-summary-value");
  });
  it("does not render incomplete cached report data", () => {
    delete fake.query.data.options;
    const text = render();
    expect(text).toContain("报表接口与当前后端版本不兼容");
    expect(text).not.toContain("Key one");
  });
});

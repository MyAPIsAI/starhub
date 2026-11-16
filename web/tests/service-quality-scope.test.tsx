import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  afterEach,
  beforeAll,
  afterAll,
  describe,
  expect,
  it,
  vi,
} from "vitest";
import ServiceQuality from "../src/features/service-quality";
import {
  useSystemAccess,
  sessionQueryIdentity,
} from "../src/lib/system-access";
import type { ServiceQualitySummary } from "../src/lib/api/service-quality";
import * as http from "../src/lib/api/http";
import "../src/i18n";

vi.mock("../src/lib/preferences", () => ({
  usePreferences: Object.assign(
    (selector: (state: { language: string }) => unknown) =>
      selector({ language: "en" }),
    { getState: () => ({ language: "en" }) },
  ),
}));

vi.mock("../src/lib/system-access", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../src/lib/system-access")>();
  return {
    ...actual,
    useSystemAccess: Object.assign(
      <T,>(
        selector: (
          state: ReturnType<typeof actual.useSystemAccess.getState>,
        ) => T,
      ) => selector(actual.useSystemAccess.getState()),
      actual.useSystemAccess,
    ),
    useSessionQueryIdentity: () =>
      actual.sessionQueryIdentity(actual.useSystemAccess.getState()),
  };
});

const metric = {
  model: "glm-5.2",
  total_requests: 4,
  success_count: 4,
  failed_count: 0,
  success_rate: 1,
  avg_duration_ms: 8000,
  p95_duration_ms: 12000,
  total_quota: 1,
  error_rate: 0,
};
const currentMonth = new Date().toISOString().slice(0, 7);
const monthStart = Date.parse(`${currentMonth}-01T00:00:00Z`) / 1000;
const summary: ServiceQualitySummary = {
  window_hours: 168,
  window_start: monthStart,
  window_end: Math.floor(Date.now() / 1000) + 1,
  overall: metric,
  by_model: [metric],
  by_day: [],
};
const clients: QueryClient[] = [];

function render(
  role: number,
  search = "",
  data: ServiceQualitySummary | null = summary,
  trendData?: ServiceQualitySummary,
) {
  const access = useSystemAccess.getState();
  if (access.session.accountId !== 7 || access.role !== role) {
    access.setSession({ id: 7, group: "default", role });
  }
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Infinity, gcTime: Infinity },
    },
  });
  clients.push(client);
  const identity = sessionQueryIdentity(useSystemAccess.getState());
  const params = new URLSearchParams(search);
  const customer = params.get("customer");
  const month = params.get("month") || currentMonth;
  const key: unknown[] = ["service-quality", identity, { month }];
  if ((role === 10 || role === 100) && customer)
    key.push({ userId: Number(customer) });
  if (data) client.setQueryData(key, data);
  if (trendData) {
    const model = data?.by_model[Number(params.get("unit")?.slice(3))]?.model;
    const trendKey: unknown[] = ["service-quality", identity, { month }, model];
    if ((role === 10 || role === 100) && customer)
      trendKey.push({ userId: Number(customer) });
    client.setQueryData(trendKey, trendData);
  }
  const markup = () =>
    renderToStaticMarkup(
      createElement(
        QueryClientProvider,
        { client },
        createElement(
          MemoryRouter,
          { initialEntries: [`/console/service-quality${search}`] },
          createElement(ServiceQuality),
        ),
      ),
    );
  const html = markup();
  return { html, client, query: client.getQueryCache().getAll()[0], markup };
}

beforeAll(() => {
  vi.spyOn(http, "apiGet");
  const originalError = console.error;
  vi.spyOn(console, "error").mockImplementation((...args) => {
    // Browser checks cover layout; these assertions exercise account data selection.
    if (String(args[0]).includes("useLayoutEffect does nothing on the server"))
      return;
    originalError(...args);
  });
});
afterEach(() => {
  clients.splice(0).forEach((client) => client.clear());
  vi.mocked(http.apiGet).mockReset();
  useSystemAccess.getState().setSession(null);
});
afterAll(() => vi.restoreAllMocks());

describe("service quality account scope", () => {
  it.each([1, 5, 7, 11, 101])(
    "role %s keeps its own real metrics despite forged view and customer",
    async (role) => {
      const { html, query } = render(role, "?view=admin&customer=99");
      expect(html).toContain("glm-5.2");
      expect(html).toMatch(
        /aria-label="User ID"[^>]*disabled=""[^>]*value="7"/,
      );
      expect(html).toContain("#7");
      expect(html).not.toContain("#99");
      expect(html).not.toContain("#1001");
      expect(html).not.toContain("Demo view");
      const get = vi.mocked(http.apiGet).mockResolvedValue(summary);
      await query.fetch();
      expect(get.mock.calls.at(-1)?.[0]).toBe(
        `/api/service-quality?month=${currentMonth}`,
      );
    },
  );

  it.each([10, 100])(
    "role %s can select another user or clear to all users",
    async (role) => {
      const all = render(role);
      expect(all.html).toContain("glm-5.2");
      expect(all.html).toContain("All users");
      expect(all.html).not.toMatch(/aria-label="User ID"[^>]*disabled/);
      expect(all.html).not.toContain("#0");
      const other = render(role, "?customer=99");
      expect(other.html).toMatch(/aria-label="User ID"[^>]*value="99"/);
      expect(other.html).toContain("#99");
      expect(other.query.queryHash).not.toBe(all.query.queryHash);
      const get = vi.mocked(http.apiGet).mockResolvedValue(summary);
      await other.query.fetch();
      expect(get.mock.calls.at(-1)?.[0]).toBe(
        `/api/service-quality?month=${currentMonth}&user_id=99`,
      );
      await all.query.fetch();
      expect(get.mock.calls.at(-1)?.[0]).toBe(
        `/api/service-quality?month=${currentMonth}`,
      );
    },
  );

  it("keeps details accessible for a real account", () => {
    const month = new Date().toISOString().slice(0, 7);
    const { html } = render(
      1,
      `?month=${month}&unit=sq_0&view=admin&customer=99`,
    );
    expect(html).toContain('class="sq-detail-identity"');
    expect(html).toContain("#7");
    expect(html).not.toContain("#99");
  });

  it("shows no demo records while the selected account is loading or empty", () => {
    const loading = render(10, "?customer=99", null);
    expect(loading.html).toContain("Loading request metrics");
    expect(loading.html).not.toContain("sq-row-view");
    const empty = render(10, "?customer=99", {
      ...summary,
      by_model: [],
      by_day: [],
    });
    expect(empty.html).toContain("No matching accounting units");
    expect(empty.html).not.toContain("sq-row-view");
  });

  it("queries separate calendar months and rejects rolling-window responses", async () => {
    const january = render(10, "?month=2024-01&customer=99", null);
    const february = render(10, "?month=2024-02&customer=99", null);
    expect(january.query.queryHash).not.toBe(february.query.queryHash);
    const januaryData = {
      ...summary,
      window_hours: 744,
      window_start: Date.parse("2024-01-01T00:00:00Z") / 1000,
      window_end: Date.parse("2024-02-01T00:00:00Z") / 1000,
    };
    const get = vi.mocked(http.apiGet).mockResolvedValue(januaryData);
    await january.query.fetch();
    expect(get.mock.calls.at(-1)?.[0]).toBe(
      "/api/service-quality?month=2024-01&user_id=99",
    );
    expect(january.markup()).toContain("2024-01-31");
    expect(january.markup()).not.toContain("last 7 days");
    await expect(february.query.fetch()).rejects.toThrow(
      "Invalid monthly service quality window",
    );
    expect(february.markup()).not.toContain('class="sq-row-view"');
  });

  it("fetches the selected model trend without falling back to account totals", async () => {
    const allModels = {
      ...summary,
      by_model: [metric, { ...metric, model: "other-model", success_rate: 0 }],
      by_day: [
        {
          date: `${currentMonth}-01`,
          total_requests: 8,
          success_count: 4,
          failed_count: 4,
          success_rate: 0.5,
          avg_duration_ms: 8000,
          p95_duration_ms: 12000,
        },
      ],
    };
    const view = render(
      10,
      `?month=${currentMonth}&customer=99&unit=sq_0`,
      allModels,
    );
    expect(view.html).toContain("Loading model trend");
    expect(view.html).not.toContain('class="sq-trend-current"');
    const trendQuery = view.client
      .getQueryCache()
      .getAll()
      .find((query) => query.queryKey[3] === metric.model)!;
    const modelData = {
      ...summary,
      by_day: [
        {
          ...allModels.by_day[0],
          success_rate: 1,
          total_requests: 4,
          failed_count: 0,
        },
      ],
    };
    const get = vi.mocked(http.apiGet).mockResolvedValue(modelData);
    await trendQuery.fetch();
    expect(get.mock.calls.at(-1)?.[0]).toBe(
      `/api/service-quality?month=${currentMonth}&model=glm-5.2&user_id=99`,
    );
    expect(view.markup()).toMatch(/sq-trend-current[\s\S]*?100\.00%/);
    expect(view.markup()).not.toContain("50.00%");
    get.mockRejectedValue(new Error("trend unavailable"));
    await expect(trendQuery.fetch()).rejects.toThrow("trend unavailable");
    expect(view.markup()).toContain("Could not load this model&#x27;s trend");
    expect(view.markup()).not.toContain('class="sq-trend-current"');
    get.mockResolvedValue({ ...summary, by_day: [] });
    await trendQuery.fetch();
    expect(view.markup()).toContain("No trend data for this model");
  });
});

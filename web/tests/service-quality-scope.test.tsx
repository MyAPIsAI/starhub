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
const summary: ServiceQualitySummary = {
  window_hours: 168,
  overall: metric,
  by_model: [metric],
  by_day: [],
};
const clients: QueryClient[] = [];

function render(
  role: number,
  search = "",
  data: ServiceQualitySummary | null = summary,
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
  const customer = new URLSearchParams(search).get("customer");
  const key: unknown[] = ["service-quality", identity, 168];
  if ((role === 10 || role === 100) && customer)
    key.push({ userId: Number(customer) });
  if (data) client.setQueryData(key, data);
  const html = renderToStaticMarkup(
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
  return { html, client, query: client.getQueryCache().getAll()[0] };
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
      expect(get.mock.calls.at(-1)?.[0]).toBe("/api/service-quality?hours=168");
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
        "/api/service-quality?hours=168&user_id=99",
      );
      await all.query.fetch();
      expect(get.mock.calls.at(-1)?.[0]).toBe("/api/service-quality?hours=168");
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
});

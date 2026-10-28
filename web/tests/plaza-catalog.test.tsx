import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeAll, afterAll, describe, expect, it, vi } from "vitest";
import {
  plazaToDirectoryModel,
  resolvePlazaVendor,
  type PlazaFiltersResponse,
  type PlazaRow,
} from "../src/lib/api/plaza";
import { ApiError } from "../src/lib/api/http";
import ModelSquare from "../src/pages/models/index";
import ModelDetail from "../src/pages/models/detail";
import { useProductPreview } from "../src/lib/product-preview";
import { useSystemAccess, ROLE_USER } from "../src/lib/system-access";
import { useModelPricing } from "../src/lib/model-pricing";
import type { ServiceQualitySummary } from "../src/lib/api/service-quality";

vi.mock("../src/components/product-shell", () => ({
  PublicHeader: () => null,
}));
vi.mock("../src/components/demo-control-bar", () => ({
  DemoControlBar: () => null,
}));

const row: PlazaRow = {
  id: "gpt-4o",
  name: "gpt-4o",
  vendor: "openai",
  vendor_display: "OpenAI",
  description: "Backend model metadata",
  tags: ["text", "vision"],
  context_length: 128000,
  max_output: 4096,
  protocols: ["OpenAI", "Anthropic"],
  pricing: {
    mode: "per_token",
    input_per_m: 2.5,
    output_per_m: 10,
    cache_read_per_m: null,
    cache_write_per_m: null,
    per_request: null,
    official_input_per_m: null,
    official_output_per_m: null,
    official_cache_read_per_m: null,
    official_cache_write_per_m: null,
    official_per_request: null,
    group_ratio: 1,
    explicit: true,
    currency: "USD",
  },
  available_groups: ["default"],
  is_available: true,
  unavailable_reason: null,
  score: null,
  calls_7d: null,
  channel_count: 1,
  created_at: 0,
  sort_weight: 0,
};

const filters: PlazaFiltersResponse = {
  vendors: [
    { value: "openai", label: "OpenAI", count: 1 },
    { value: "anthropic", label: "Anthropic", count: 1 },
  ],
  tags: [],
  price_histogram: [],
  total: 2,
  group: "default",
};

type RenderOptions = {
  route?: string;
  items?: PlazaRow[];
  pending?: boolean;
  error?: ApiError;
  group?: string;
  otherIdentityOnly?: boolean;
  quality?: ServiceQualitySummary;
  groupSla?: unknown;
};

function renderPage({
  route = "/models?view=cards",
  items = [row],
  pending = false,
  error,
  group = "default",
  otherIdentityOnly = false,
  quality,
  groupSla = null,
}: RenderOptions = {}) {
  const client = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        retryOnMount: false,
        staleTime: Infinity,
        gcTime: Infinity,
      },
    },
  });
  const identity = useSystemAccess.getState().role;
  client.setQueryData(["plaza-filters", identity], filters);
  const quoteIdentity = otherIdentityOnly ? "other-user" : identity;
  const url = new URL(route, "http://localhost");
  const isDetail = url.pathname !== "/models";
  const key = isDetail
    ? [
        "plaza-model",
        quoteIdentity,
        decodeURIComponent(url.pathname.slice("/models/".length)),
      ]
    : [
        "plaza-models",
        quoteIdentity,
        {
          q: url.searchParams.get("q") || undefined,
          vendors:
            resolvePlazaVendor(
              url.searchParams.get("vendor") ?? "",
              filters.vendors,
            ) || undefined,
          page_size: 200,
        },
      ];
  if (error) {
    client.getQueryCache().build(client, { queryKey: key }).setState({
      status: "error",
      error,
      fetchStatus: "idle",
    });
  } else if (!pending) {
    client.setQueryData(
      key,
      isDetail
        ? {
            group,
            group_sla: groupSla,
            model: {
              ...(items[0] ?? row),
              id: decodeURIComponent(url.pathname.slice("/models/".length)),
            },
          }
        : {
            items,
            total: items.length,
            page: 1,
            page_size: 200,
            group,
          },
    );
  }
  if (quality && isDetail) {
    client.setQueryData(
      [
        "service-quality",
        168,
        decodeURIComponent(url.pathname.slice("/models/".length)),
      ],
      quality,
    );
  }
  const html = renderToStaticMarkup(
    createElement(
      QueryClientProvider,
      { client },
      createElement(
        MemoryRouter,
        { initialEntries: [route] },
        createElement(
          Routes,
          null,
          createElement(Route, {
            path: "/models",
            element: createElement(ModelSquare),
          }),
          createElement(Route, {
            path: "/models/:id",
            element: createElement(ModelDetail),
          }),
        ),
      ),
    ),
  );
  client.clear();
  return html;
}

describe("backend-only plaza catalog", () => {
  const originalError = console.error;
  beforeAll(() => {
    vi.spyOn(console, "error").mockImplementation((...args) => {
      // SSR verifies data selection; layout effects are verified in the browser.
      if (
        String(args[0]).includes("useLayoutEffect does nothing on the server")
      )
        return;
      originalError(...args);
    });
  });
  afterAll(() => vi.restoreAllMocks());

  it("keeps vendor identifiers separate from display names", () => {
    const model = plazaToDirectoryModel(row);
    expect(model.vendorId).toBe("openai");
    expect(model.vendor).toBe("OpenAI");
    expect(model.protocols).toEqual(["OpenAI", "Anthropic"]);
    expect(resolvePlazaVendor("OpenAI", filters.vendors)).toBe("openai");
    expect(resolvePlazaVendor("openai", filters.vendors)).toBe("openai");
  });

  it.each(["openai", "OpenAI"])(
    "uses the slug query for vendor=%s and retains the backend model",
    (vendor) => {
      const html = renderPage({ route: `/models?view=cards&vendor=${vendor}` });
      expect(html).toContain('data-model-id="gpt-4o"');
      expect(html).toContain(
        'data-catalog-vendor="openai" class="is-active" aria-pressed="true"',
      );
      expect(html).not.toContain('data-model-id="gpt-4.1"');
    },
  );

  it("shows only backend results in list view", () => {
    const html = renderPage({ route: "/models?view=list" });
    expect(html).toContain('class="catalog-list-row"');
    expect((html.match(/data-model-id=/g) ?? []).length).toBe(1);
    expect(html).not.toContain("Veyra Auto");
  });

  it.each(["cards", "list"])(
    "uses backend token prices and the existing comparison table in %s view",
    (view) => {
      const html = renderPage({
        route: `/models?view=${view}`,
        items: [
          {
            ...row,
            pricing: {
              ...row.pricing,
              official_input_per_m: 3,
              official_output_per_m: 12,
            },
          },
        ],
      });
      expect(html).toContain('class="catalog-price-summary-table"');
      expect(html).toContain("$2.5");
      expect(html).toContain("$10");
      expect(html).toContain("$3");
      expect(html).toContain("$12");
      expect(html).toContain("接入此模型");
      expect(html).not.toContain("模型不存在");
    },
  );

  it("keeps zero prices distinct from missing official values", () => {
    const html = renderPage({
      items: [
        {
          ...row,
          pricing: {
            ...row.pricing,
            input_per_m: 0,
            output_per_m: 0,
            official_input_per_m: 0,
          },
        },
      ],
    });
    expect(html).toContain(
      'data-price-kind="official"><bdi dir="ltr">$0</bdi>',
    );
    expect(html).toContain('data-price-kind="official"><bdi dir="ltr">—</bdi>');
    expect(html).toContain('data-price-kind="public"><bdi dir="ltr">$0</bdi>');
  });

  it("does not substitute local official references for backend nulls", () => {
    const html = renderPage({
      items: [
        {
          ...row,
          id: "gpt-4.1",
          pricing: { ...row.pricing, input_per_m: 7, output_per_m: 17 },
        },
      ],
    });
    expect(html).toContain("$7");
    expect(html).toContain("$17");
    expect(html).not.toContain(
      'data-price-kind="official"><bdi dir="ltr">$2</bdi>',
    );
  });

  it("shows per-request and conditional pricing without a fake token rate", () => {
    const perRequest = renderPage({
      items: [
        {
          ...row,
          pricing: {
            ...row.pricing,
            mode: "per_request",
            per_request: 0.04,
            official_per_request: 0.08,
          },
        },
      ],
    });
    expect(perRequest).toContain("$0.04");
    expect(perRequest).toContain("$0.08");
    expect(perRequest).toContain("美元 / 次");
    expect(perRequest).not.toContain("catalog-price-summary-table");
    const tiered = renderPage({
      items: [{ ...row, pricing: { ...row.pricing, mode: "tiered" } }],
    });
    expect(tiered).toContain("按用量计价");
    expect(tiered).not.toContain("$2.5");
  });

  it("does not connect unavailable models or models without protocols", () => {
    for (const item of [
      { ...row, is_available: false },
      { ...row, protocols: [] },
    ]) {
      const html = renderPage({ items: [item] });
      expect(html).not.toContain("接入此模型");
      expect(html).toContain("查看状态");
    }
  });

  it("shows backend detail quotes in the original components without inventing other group prices", () => {
    const html = renderPage({
      route: "/models/gpt-4o",
      group: "vip",
      items: [
        {
          ...row,
          available_groups: ["default", "vip"],
          pricing: {
            ...row.pricing,
            input_per_m: 1.5,
            output_per_m: 6,
            cache_read_per_m: 0,
            group_ratio: 0.6,
          },
        },
      ],
    });
    expect(html).toContain('class="price-comparison-table"');
    expect(html).toContain('class="directory-price"');
    expect(html).toContain("OpenAI / Anthropic");
    expect(html).toContain("$1.5");
    expect(html).toContain("$6");
    expect(html).toContain('value="vip"');
    expect(html).not.toContain('value="default"');
    expect(html).not.toContain('value="ultra"');
    expect(html).not.toContain("undefined×");
    expect(html).not.toContain("示例费用");
  });

  it.each(["tiered", "expression"])(
    "shows the backend expression for %s detail pricing",
    (mode) => {
      const expression = "prompt_tokens * 0.000001";
      const html = renderPage({
        route: "/models/gpt-4o",
        items: [{ ...row, pricing: { ...row.pricing, mode, expression } }],
      });
      expect(html).toContain("表达式定价");
      expect(html).toContain(expression);
    },
  );

  it.each(["tiered", "expression", "per_request"])(
    "does not show fallback token ratios as fixed detail prices for %s",
    (mode) => {
      const html = renderPage({
        route: "/models/gpt-4o",
        items: [
          {
            ...row,
            pricing: {
              ...row.pricing,
              mode,
              per_request: 0.04,
              expression: "prompt_tokens * 0.000001",
            },
          },
        ],
      });
      expect(html).not.toContain("单价（美元/百万 Token）");
    },
  );

  it("does not present local demo time windows as backend billing rules", () => {
    const original = useModelPricing.getState().config;
    const config = structuredClone(original);
    config.models["gpt-4.1"].rule.timeWindows = [
      {
        id: "local-only",
        label: "Local-only schedule",
        timezone: "UTC",
        days: [1],
        start: "00:00",
        end: "23:59",
        multiplier: "0.5",
      },
    ];
    useModelPricing.setState({ config });
    try {
      const html = renderPage({ route: "/models/gpt-4.1" });
      expect(html).not.toContain("Local-only schedule");
      expect(html).not.toContain("分时定价");
      expect(html).toContain("$2.5");
    } finally {
      useModelPricing.setState({ config: original });
    }
  });

  it("uses scoped API model metrics instead of an unscoped local cache", () => {
    vi.stubGlobal("localStorage", {
      getItem: (key: string) =>
        key === "sq-gpt-4o" ? JSON.stringify({ success: 1, p95: 99 }) : null,
      setItem: () => {},
    });
    try {
      const metric = {
        model: "gpt-4o",
        total_requests: 10,
        success_count: 7,
        failed_count: 3,
        success_rate: 0.7,
        avg_duration_ms: 400,
        p95_duration_ms: 1234,
        total_quota: 100,
        error_rate: 0.3,
      };
      const quality = {
        window_hours: 168,
        overall: metric,
        by_model: [metric],
        by_day: [],
      };
      const html = renderPage({ route: "/models/gpt-4o", quality });
      expect(html).toContain("70.0%");
      expect(html).toContain("1234ms");
      expect(html).not.toContain("99ms");
      expect(html).toContain("模型7天成功率/P95");
      const empty = renderPage({ route: "/models/gpt-4o" });
      expect(empty).not.toContain("99ms");
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("does not publish demo SLA targets for a real backend group", () => {
    const html = renderPage({ route: "/models/gpt-4o", group: "pro" });
    expect(html).toContain("目标未发布");
  });

  it("keeps published zero targets separate from measured model quality", () => {
    const html = renderPage({
      route: "/models/gpt-4o",
      groupSla: {
        revision: 2,
        updated_at: 123,
        targets: { success_rate_ppm: 0, p95_latency_ms: 1200 },
      },
    });
    expect(html).toContain("成功率目标");
    expect(html).toContain("0%</bdi>");
    expect(html).toContain("1200 ms");
    expect(html).toContain("不代表合同保证");
    const unavailable = renderPage({
      route: "/models/gpt-4o",
      items: [{ ...row, is_available: false }],
      groupSla: {
        revision: 2,
        updated_at: 123,
        targets: { success_rate_ppm: 0, p95_latency_ms: 1200 },
      },
    });
    expect(unavailable).not.toContain("1200 ms");
  });

  it("reports missing backend prices without calling the model nonexistent", () => {
    const html = renderPage({
      items: [
        {
          ...row,
          pricing: { ...row.pricing, input_per_m: null, output_per_m: null },
        },
      ],
    });
    expect(html).toContain("报价暂不可用");
    expect(html).not.toContain("模型不存在");
  });

  it("does not reuse another account's cached prices or protocols", () => {
    for (const route of ["/models?view=cards", "/models/gpt-4o"]) {
      const html = renderPage({ route, otherIdentityOnly: true });
      expect(html).toContain("正在加载模型");
      expect(html).not.toContain("$2.5");
      expect(html).not.toContain("OpenAI / Anthropic");
    }
  });

  it("retains global backend vendor facets for an empty search result", () => {
    const html = renderPage({
      route: "/models?view=cards&q=absent",
      items: [],
    });
    expect(html).toContain("没有符合条件的模型");
    expect(html).toContain('data-catalog-vendor="openai"');
    expect(html).toContain('data-catalog-vendor="anthropic"');
    expect(html).not.toContain("data-model-id=");
  });

  it("does not populate a successful empty catalog with local samples", () => {
    const html = renderPage({ items: [] });
    expect(html).toContain("没有符合条件的模型");
    expect(html).not.toContain("data-model-id=");
    expect(html).not.toContain("GPT-4.1");
  });

  it("shows loading rather than samples or an empty result", () => {
    const html = renderPage({ pending: true });
    expect(html).toContain("正在加载模型");
    expect(html).not.toContain("没有符合条件的模型");
    expect(html).not.toContain("data-model-id=");
  });

  it("shows a retryable failure without local samples", () => {
    const html = renderPage({ error: new ApiError(500, "failed") });
    expect(html).toContain("模型加载失败");
    expect(html).toContain("重试");
    expect(html).not.toContain("data-model-id=");
  });

  it("respects a backend 404 even when the local demo contains the model", () => {
    const html = renderPage({
      route: "/models/deepseek-chat",
      error: new ApiError(404, "model not found"),
    });
    expect(html).toContain("模型不存在");
    expect(html).not.toContain("access-detail-heading");
    expect(html).not.toContain("重试");
  });

  it("does not show a local detail during loading or a server failure", () => {
    for (const options of [
      { pending: true },
      { error: new ApiError(500, "failed") },
    ]) {
      const html = renderPage({ route: "/models/deepseek-chat", ...options });
      expect(html).toContain(
        "pending" in options ? "正在加载模型" : "模型加载失败",
      );
      expect(html).not.toContain("access-detail-heading");
      expect(html).not.toContain("模型不存在");
    }
  });
});

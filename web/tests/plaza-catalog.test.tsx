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
};

function renderPage({
  route = "/models?view=cards",
  items = [row],
  pending = false,
  error,
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
  client.setQueryData(["plaza-filters"], filters);
  const url = new URL(route, "http://localhost");
  const isDetail = url.pathname !== "/models";
  const key = isDetail
    ? ["plaza-model", decodeURIComponent(url.pathname.slice("/models/".length))]
    : [
        "plaza-models",
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
            model: {
              ...row,
              id: decodeURIComponent(url.pathname.slice("/models/".length)),
            },
          }
        : {
            items,
            total: items.length,
            page: 1,
            page_size: 200,
            group: "default",
          },
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

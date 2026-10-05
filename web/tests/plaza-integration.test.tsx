import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createElement, type ReactElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import type {
  PlazaDetailResponse,
  PlazaFiltersResponse,
  PlazaListResponse,
  PlazaRow,
} from "../src/lib/api/plaza";
import { plazaToDirectoryModel } from "../src/lib/api/plaza";
import { ApiError } from "../src/lib/api/http";
import ModelSquare from "../src/pages/models/index";
import ModelDetail from "../src/pages/models/detail";
import { PlazaPrice } from "../src/components/plaza-price";

interface Query<T> {
  data?: T;
  isPending: boolean;
  isSuccess: boolean;
  isError: boolean;
  error: Error | null;
  refetch: () => void;
}

const state = vi.hoisted(() => ({
  list: {} as Query<PlazaListResponse>,
  detail: {} as Query<PlazaDetailResponse>,
  filters: {} as Query<PlazaFiltersResponse>,
  query: {} as Record<string, unknown>,
}));

vi.mock("../src/lib/api/plaza", async (original) => ({
  ...(await original<typeof import("../src/lib/api/plaza")>()),
  usePlazaModels: (query: Record<string, unknown>) => {
    state.query = query;
    return state.list;
  },
  usePlazaModel: () => state.detail,
  usePlazaFilters: () => state.filters,
}));
vi.mock("../src/components/product-shell", () => ({
  PublicHeader: () => null,
}));
vi.mock("../src/components/demo-control-bar", () => ({
  DemoControlBar: () => null,
}));

const row = (id = "backend-only-model"): PlazaRow => ({
  id,
  name: id,
  vendor: "openai",
  vendor_display: "OpenAI",
  description: "description-only-needle",
  tags: ["text", "function_call"],
  context_length: 1500000,
  max_output: 4096,
  protocols: ["OpenAI", "Anthropic"],
  available_groups: ["default", "vip"],
  is_available: true,
  unavailable_reason: null,
  score: null,
  calls_7d: null,
  channel_count: 2,
  created_at: 1,
  sort_weight: 0,
  pricing: {
    mode: "per_token",
    input_per_m: 123,
    output_per_m: 246,
    cache_read_per_m: 0,
    cache_write_per_m: null,
    per_request: null,
    official_input_per_m: 456,
    official_output_per_m: 912,
    official_cache_read_per_m: null,
    official_cache_write_per_m: null,
    official_per_request: null,
    group_ratio: 0.6,
    explicit: true,
    currency: "USD",
  },
});
const success = <T,>(data: T): Query<T> => ({
  data,
  isPending: false,
  isSuccess: true,
  isError: false,
  error: null,
  refetch: vi.fn(),
});
const render = (element: ReactElement, path = "/models") =>
  renderToStaticMarkup(
    createElement(MemoryRouter, { initialEntries: [path] }, element),
  );
const detail = () =>
  createElement(
    Routes,
    null,
    createElement(Route, {
      path: "/models/:id",
      element: createElement(ModelDetail),
    }),
  );

beforeEach(() => {
  state.list = success({
    items: [row()],
    total: 1,
    page: 1,
    page_size: 24,
    group: "default",
  });
  state.detail = success({
    model: row(),
    group: "default",
    related: { same_vendor: [], same_price: [] },
  });
  state.filters = success({
    vendors: [
      { value: "openai", label: "OpenAI", count: 101 },
      { value: "google", label: "Google", count: 100 },
    ],
    tags: [],
    price_histogram: [],
    total: 201,
    group: "default",
  });
  vi.spyOn(console, "error").mockImplementation(() => {});
});
afterEach(() => vi.restoreAllMocks());

describe("plaza API and catalog integration", () => {
  it("preserves backend identifiers, prices, protocols and group coverage without local tiers", () => {
    const model = plazaToDirectoryModel(row());
    expect(model).toMatchObject({
      vendorId: "openai",
      vendor: "OpenAI",
      contextDisplay: "1.5M",
      maxOutput: 4096,
      protocols: ["OpenAI", "Anthropic"],
      availableGroups: ["default", "vip"],
      isAvailable: true,
    });
    expect(model.pricing).toEqual(row().pricing);
    expect(model.capabilities).toContain("function_call");
    expect(model).not.toHaveProperty("tierAvailability");
  });

  it("sends vendor IDs and keeps backend matches found only in descriptions", () => {
    const html = render(
      createElement(ModelSquare),
      "/models?vendor=openai&q=description-only-needle",
    );
    expect(state.query).toMatchObject({
      vendors: "openai",
      q: "description-only-needle",
      page: 1,
      page_size: 24,
    });
    expect(html).toContain('data-model-id="backend-only-model"');
    expect(html).toContain('data-catalog-vendor="google"');
    expect(html).toContain('data-catalog-vendor="openai"');
    expect(html).toContain("101</span>");
    expect(html).toContain("$123");
    expect(html).toContain("$456");
    expect(html).toContain("1.5M");
    expect(html).toContain("/console/connect?model=backend-only-model");
  });

  it("honors successful empty catalogs without showing local sample models", () => {
    state.list = success({
      items: [],
      total: 0,
      page: 1,
      page_size: 24,
      group: "default",
    });
    const html = render(createElement(ModelSquare));
    expect(html).not.toContain("data-model-id=");
    expect(html).toContain('class="catalog-empty"');
  });

  it("distinguishes loading, transport errors and missing model details", () => {
    state.list = {
      ...state.list,
      data: undefined,
      isPending: true,
      isSuccess: false,
    };
    expect(render(createElement(ModelSquare))).not.toContain("data-model-id=");
    state.list = {
      ...state.list,
      isPending: false,
      isError: true,
      error: new ApiError(500, "failed"),
    };
    expect(render(createElement(ModelSquare))).toContain('role="alert"');
    state.detail = {
      ...state.detail,
      data: undefined,
      isError: true,
      isSuccess: false,
      error: new ApiError(404, "missing"),
    };
    const html = render(detail(), "/models/gpt-4.1");
    expect(html).toContain('role="alert"');
    expect(html).not.toContain("GPT-4.1");
    expect(html).not.toContain("$1.2");
  });

  it("uses API pagination beyond the first 200 models", () => {
    state.list = success({
      items: [row("second-page-model")],
      total: 201,
      page: 2,
      page_size: 24,
      group: "default",
    });
    const html = render(createElement(ModelSquare), "/models?page=2&view=list");
    expect(state.query.page).toBe(2);
    expect(html).toContain('class="plaza-pagination"');
    expect(html).toContain('data-model-id="second-page-model"');
  });

  it("uses backend detail prices and actual coverage while preserving return filters", () => {
    const html = render(
      detail(),
      "/models/backend-only-model?from=%2Fmodels%3Fvendor%3Dopenai%26page%3D2",
    );
    expect(html).toContain('href="/models?vendor=openai&amp;page=2"');
    expect(html).toContain("$123");
    expect(html).toContain("$456");
    expect(html).toContain('data-current-group="true"');
    expect(html).toContain("vip");
    expect(html).not.toContain("ultra");
    expect(html).toContain("/console/connect?model=backend-only-model");
  });

  it("keeps an unavailable group's quote separate from other groups' coverage", () => {
    state.detail.data!.model.is_available = false;
    state.detail.data!.model.available_groups = ["vip"];
    state.detail.data!.model.unavailable_reason = "group_not_enabled";
    const html = render(detail(), "/models/backend-only-model");
    expect(html).not.toContain("/console/connect?model=");
    expect(html).not.toContain('data-current-group="true"');
    expect(html).toContain("vip");
  });
});

describe("backend price rendering", () => {
  it("preserves zero prices and absent rates on separate axes", () => {
    const html = renderToStaticMarkup(
      createElement(PlazaPrice, { pricing: row().pricing, detailed: true }),
    );
    expect(html).toMatch(/data-price-axis="cache-read".*?\$0/);
    expect(html).toMatch(/data-price-axis="cache-write".*?—/);
  });

  it("renders per-request platform and official prices without token prices", () => {
    const pricing = {
      ...row().pricing,
      mode: "per_request",
      per_request: 0.04,
      official_per_request: 0.08,
    };
    const html = renderToStaticMarkup(createElement(PlazaPrice, { pricing }));
    expect(html).toContain("$0.04");
    expect(html).toContain("$0.08");
    expect(html).not.toContain("$123");
  });

  it("does not present base ratios as fixed quotes for conditional billing", () => {
    const pricing = { ...row().pricing, mode: "tiered" };
    const html = renderToStaticMarkup(
      createElement(PlazaPrice, { pricing, detailed: true }),
    );
    expect(html).not.toContain("$123");
    expect(html).not.toContain("$246");
    expect(html).toContain("$456");
  });
});

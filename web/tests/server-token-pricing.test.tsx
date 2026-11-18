import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ServerTokenPricing } from "../src/features/server-token-pricing";
import { readServerPricing } from "../src/lib/server-pricing";
import { emptyModelMetadata } from "../src/lib/model-metadata";
import {
  useSystemAccess,
  sessionQueryIdentity,
} from "../src/lib/system-access";
import "../src/i18n";
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
  return {
    ...actual,
    useSystemAccess: useRole,
    useSessionQueryIdentity: () =>
      actual.sessionQueryIdentity(actual.useSystemAccess.getState()),
  };
});
const initial = useSystemAccess.getState();
afterEach(() => useSystemAccess.setState(initial, true));
function render(
  role = 100,
  error = false,
  guest = false,
  catalogError = false,
  emptyCatalog = false,
) {
  useSystemAccess
    .getState()
    .setSession(guest ? null : { id: 7, role, group: "default" });
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Infinity, gcTime: Infinity },
    },
  });
  const key = [
    "server-pricing",
    sessionQueryIdentity(useSystemAccess.getState()),
  ];
  client.setQueryData(
    key,
    readServerPricing({
      version: 3,
      loaded_version: 2,
      atomic_batch: true,
      options: [
        {
          key: "model_ratio",
          value: '{"actual-only":2.5,"zero":0,"claude-3-5-sonnet-20241022":3}',
        },
        { key: "completion_ratio", value: '{"actual-only":4,"zero":0}' },
        { key: "model_price", value: "{}" },
        { key: "model_price_expr", value: "{}" },
        { key: "cache_ratio", value: "0.5" },
        { key: "audio_ratio", value: "40" },
      ],
    }),
  );
  const catalogKey = [
    "plaza-meta",
    sessionQueryIdentity(useSystemAccess.getState()),
  ];
  client.setQueryData(catalogKey, {
    revision: 1,
    revision_guards: true,
    models: (emptyCatalog ? [] : ["awaiting-price", "actual-only", "zero"]).map(
      (model) => ({
        ...emptyModelMetadata(),
        model,
      }),
    ),
  });
  if (catalogError)
    client
      .getQueryCache()
      .find({ queryKey: catalogKey })!
      .setState({
        status: "error",
        error: new Error("Unavailable"),
        fetchStatus: "idle",
      });
  if (error)
    client
      .getQueryCache()
      .find({ queryKey: key })!
      .setState({
        status: "error",
        error: new Error("Unavailable"),
        fetchStatus: "idle",
      });
  const html = renderToStaticMarkup(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <ServerTokenPricing />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  client.clear();
  return html;
}
describe("actual server token pricing", () => {
  it("shows registered models and explicit zero without listing unregistered default prices", () => {
    const html = render();
    expect(html).toContain('data-server-priced-model="actual-only"');
    expect(html).toContain('data-server-priced-model="zero"');
    expect(html).toContain("2.5");
    expect(html).not.toContain('data-server-priced-model="gpt-4.1"');
    expect(html).not.toContain(
      'data-server-priced-model="claude-3-5-sonnet-20241022"',
    );
    expect(html.match(/data-server-priced-model="actual-only"/g)).toHaveLength(
      1,
    );
    const row = html.match(
      /<tr[^>]*data-server-priced-model="awaiting-price"[^>]*>(.*?)<\/tr>/s,
    )?.[1];
    expect(row).toContain("待定价");
    expect(row).toContain("设置价格");
    expect(row).not.toContain("<bdi>0</bdi>");
    const zero = html.match(
      /<tr[^>]*data-server-priced-model="zero"[^>]*>(.*?)<\/tr>/s,
    )?.[1];
    expect(zero).not.toContain("待定价");
  });
  it("does not fall back to default prices when the model catalog is unavailable", () => {
    const html = render(100, false, false, true);
    expect(html).not.toContain('data-server-priced-model="awaiting-price"');
    expect(html).not.toContain("data-server-priced-model");
    expect(html).toContain("模型目录读取失败");
  });
  it("hides cached server prices when the authoritative read fails", () => {
    const html = render(100, true);
    expect(html).not.toContain("data-server-priced-model");
    expect(html).toContain('role="alert"');
  });
  it("keeps an empty model catalog empty despite seeded pricing options", () => {
    const html = render(100, false, false, false, true);
    expect(html).not.toContain("data-server-priced-model");
    expect(html).toContain("暂无匹配的已登记模型");
  });
  it.each([1, 5, 99, 101])(
    "requires actual administrator authority for role %s",
    (role) => {
      expect(render(role)).not.toContain("data-server-priced-model");
    },
  );
  it("requires a confirmed server session", () => {
    expect(render(100, false, true)).not.toContain("data-server-priced-model");
  });
});

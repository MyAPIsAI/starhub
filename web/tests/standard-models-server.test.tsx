import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StandardModels } from "../src/features/standard-models";
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
function render(role = 100, error = false, empty = false) {
  useSystemAccess.getState().setSession({ id: 7, role, group: "default" });
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Infinity, gcTime: Infinity },
    },
  });
  const key = ["plaza-meta", sessionQueryIdentity(useSystemAccess.getState())];
  client.setQueryData(key, {
    revision: 0,
    revision_guards: true,
    models: empty
      ? []
      : [
          {
            ...emptyModelMetadata(),
            model: "server-only-model",
            vendor_display: "Actual provider",
            tags: "unknown-server-tag",
            max_output: 2048,
          },
        ],
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
        <StandardModels role={100} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  client.clear();
  return html;
}
describe("server model metadata catalog", () => {
  it("renders a server model absent from the local Demo rule map without crashing", () => {
    const html = render();
    expect(html).toContain('data-server-model="server-only-model"');
    expect(html).toContain("unknown-server-tag");
    expect(html).toContain("2048");
    expect(html).not.toContain('data-server-model="gpt-4.1"');
  });
  it("keeps empty storage empty without sample models or resource counts", () => {
    const html = render(100, false, true);
    expect(html).not.toContain("data-server-model");
    expect(html).not.toContain("GPT-4.1");
  });
  it("hides stale cached metadata after a failed authoritative read", () => {
    const html = render(100, true);
    expect(html).not.toContain("data-server-model");
    expect(html).toContain('role="alert"');
  });
  it.each([1, 5, 99, 101])(
    "does not accept administrator props over actual role %s",
    (role) => {
      expect(render(role)).not.toContain("data-server-model");
    },
  );
});

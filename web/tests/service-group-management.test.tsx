import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ServiceGroups } from "../src/features/service-groups";
import { readServiceGroupCatalog } from "../src/lib/service-group-config";
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
const original = useSystemAccess.getState();
afterEach(() => useSystemAccess.setState(original, true));
const group = {
  id: 1,
  revision: 1,
  group_id: "database-vip",
  name: "Database VIP",
  description: "Stored definition",
  enabled: true,
  visibility: "common",
  account_ids: [],
  multiplier: "0.8",
  created_at: 0,
  updated_at: 1,
};
const snapshot = {
  groups: [group],
  group_ratios: { default: 1, "database-vip": 0.5, "legacy-only": 0 },
  option_version: 2,
  loaded_option_version: 1,
  revision_guards: true,
  option_version_guard: true,
  account_access_guard: true,
};
function render(role = 100, error = false, empty = false) {
  useSystemAccess.getState().setSession({ id: 7, group: "default", role });
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Infinity, gcTime: Infinity },
    },
  });
  const identity = sessionQueryIdentity(useSystemAccess.getState());
  const key = ["service-group-configuration", identity];
  const data = readServiceGroupCatalog(
    empty ? { ...snapshot, groups: [] } : snapshot,
  );
  client.setQueryData(key, data);
  client.setQueryData(["service-groups", identity], data.groups);
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
        <ServiceGroups role={100} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  client.clear();
  return html;
}
describe("server-backed group management", () => {
  it("shows only actual definitions and actual legacy ratio entries", () => {
    const html = render();
    expect(html).toContain('data-config-group="database-vip"');
    expect(html).toContain('data-config-group="legacy-only"');
    expect(html).toContain('data-config-group="default"');
    expect(html).toContain("Database VIP");
    expect(html).toContain("0.8");
    expect(html).toContain("0.5");
    expect(html).not.toContain("account-demo");
    expect(html).not.toContain("Ultra");
  });
  it("does not turn legacy ratios into fictitious definitions in an empty metadata store", () => {
    const html = render(100, false, true);
    expect(html).not.toContain("Database VIP");
    expect(html).not.toContain("data-target-group");
    expect(html).toContain('data-config-group="legacy-only"');
  });
  it("hides cached configuration after a failed authoritative read", () => {
    const html = render(100, true);
    expect(html).not.toContain("data-config-group");
    expect(html).toContain('role="alert"');
  });
  it.each([1, 5, 99, 101])(
    "does not trust an administrator prop over actual role %s",
    (role) => {
      const html = render(role);
      expect(html).not.toContain("data-config-group");
      expect(html).toContain('role="alert"');
    },
  );
});

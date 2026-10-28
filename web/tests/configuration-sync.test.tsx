import { renderToStaticMarkup } from "react-dom/server";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ConfigurationSync } from "../src/features/configuration-sync";
import { useSystemAccess, ROLE_ROOT } from "../src/lib/system-access";
import type { OptionStatus } from "../src/lib/api/option";
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
  return { ...actual, useSystemAccess: useRole };
});
const initial = useSystemAccess.getState().role;
afterEach(() => useSystemAccess.getState().setRole(initial));
const status: OptionStatus = {
  database_version: 3,
  loaded_version: 3,
  in_sync: true,
  instances: [
    { id: "peer-current", version: 3, last_seen: 1793145600 },
    { id: "peer-older", version: 2, last_seen: 1793145600 },
  ],
};
const render = (role: number, data: OptionStatus, error?: Error) => {
  useSystemAccess.getState().setRole(role);
  const client = new QueryClient();
  client.setQueryData(["option-status"], data);
  if (error)
    client
      .getQueryCache()
      .find({ queryKey: ["option-status"] })!
      .setState({ status: "error", error, fetchStatus: "idle" });
  const html = renderToStaticMarkup(
    <QueryClientProvider client={client}>
      <ConfigurationSync />
    </QueryClientProvider>,
  );
  client.clear();
  return html;
};
describe("configuration reload observation scope", () => {
  it("keeps peer versions independent of a successful local load", () => {
    const html = render(ROLE_ROOT, status);
    expect(html).toContain('data-local-sync="current"');
    expect(html.match(/data-peer-sync="different"/g)).toHaveLength(1);
    expect(html.match(/data-peer-sync="current"/g)).toHaveLength(1);
    expect(html).toContain("peer-older");
  });
  it("shows an unknown overall state when there are no heartbeats", () => {
    const html = render(ROLE_ROOT, { ...status, instances: [] });
    expect(html).toContain('class="configuration-sync-empty"');
    expect(html).not.toContain("<tbody");
  });
  it("hides cached observations after status reading fails", () => {
    const html = render(ROLE_ROOT, status, new Error("Unavailable"));
    expect(html).toContain('role="alert"');
    expect(html).not.toContain("peer-older");
    expect(html).not.toContain("data-local-sync");
  });
  it.each([1, 5, 99, 101])(
    "does not render admin configuration state for role %s",
    (role) => expect(render(role, status)).toBe(""),
  );
});

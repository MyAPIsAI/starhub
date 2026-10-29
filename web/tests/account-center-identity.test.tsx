import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import AccountCenter from "../src/features/account-center";
import { useSystemAccess } from "../src/lib/system-access";

vi.mock("../src/lib/product-preview", () => ({
  useProductPreview: (select: (state: unknown) => unknown) =>
    select({
      user: {
        id: "demo-impostor-id",
        username: "Demo impostor name",
        email: "",
        accountStatus: "normal",
        isLoggedIn: true,
        securityEvents: [],
      },
      applyAccountSecurity: () => {},
      logout: () => {},
    }),
}));
vi.mock("../src/lib/api/user", () => ({
  useSessions: () => ({ data: [], isLoading: false }),
  useTerminateOtherSessions: () => ({ isPending: false }),
  useUpdateSelf: () => ({ isPending: false }),
}));
vi.mock("../src/lib/preferences", () => ({
  usePreferences: (select: (state: { language: string }) => unknown) =>
    select({ language: "en" }),
}));
vi.mock("../src/lib/system-access", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../src/lib/system-access")>();
  return {
    ...actual,
    useSystemAccess: Object.assign(
      <T,>(
        select: (
          state: ReturnType<typeof actual.useSystemAccess.getState>,
        ) => T,
      ) => select(actual.useSystemAccess.getState()),
      actual.useSystemAccess,
    ),
  };
});
const initial = useSystemAccess.getState();
afterEach(() => useSystemAccess.setState(initial, true));
function render() {
  const client = new QueryClient();
  try {
    return renderToStaticMarkup(
      <QueryClientProvider client={client}>
        <MemoryRouter>
          <AccountCenter onDeactivate={() => {}} />
        </MemoryRouter>
      </QueryClientProvider>,
    );
  } finally {
    client.clear();
  }
}
describe("confirmed account center identifiers", () => {
  it("shows the server username and numeric user ID rather than the Demo profile", () => {
    useSystemAccess.getState().setSession({
      id: 7,
      group: "default",
      role: 1,
      username: "admin@enterprise.example",
    });
    const html = render();
    expect(html).toMatch(
      /id="account-username"[^>]+value="admin@enterprise.example"/,
    );
    expect(html).toMatch(/id="account-id"[^>]+value="7"/);
    expect(html).not.toContain("Demo impostor name");
    expect(html).not.toContain('value="demo-impostor-id"');
  });
  it("keeps unconfirmed identity neutral and disables identifier copying", () => {
    useSystemAccess.getState().setSession(null);
    const html = render();
    expect(html).toMatch(/id="account-username"[^>]+value="Account center"/);
    expect(html).toMatch(/id="account-id"[^>]+value="—"/);
    expect(
      html.match(/<button[^>]*aria-label="Copy user ID"[^>]*>/)?.[0],
    ).toContain('disabled=""');
    expect(html).not.toContain("Demo impostor name");
  });
});

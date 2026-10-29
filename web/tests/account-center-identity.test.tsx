import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import AccountCenter from "../src/features/account-center";
import { useSystemAccess } from "../src/lib/system-access";
import { ApiError } from "../src/lib/api/http";
const devices = vi.hoisted(() => ({
  error: false,
  reason: null as unknown,
  sessions: [] as {
    created_at: number;
    expires_at: number;
    is_current: boolean;
  }[],
}));
const profile = vi.hoisted(() => ({
  error: false,
  reason: null as unknown,
  data: undefined as
    | undefined
    | {
        id: number;
        username: string;
        email: string;
        role: number;
        status: number;
        created_at: number;
        can_update?: boolean;
      },
}));
const history = vi.hoisted(() => ({
  error: false,
  events: [] as {
    id: number;
    action: "password_changed" | "email_changed";
    created_at: number;
  }[],
}));

vi.mock("../src/lib/product-preview", () => ({
  useProductPreview: (select: (state: unknown) => unknown) =>
    select({
      user: {
        id: "demo-impostor-id",
        username: "Demo impostor name",
        email: "impostor@demo.invalid",
        createdAt: "1994-01-01T00:00:00.000Z",
        accountStatus: "normal",
        isLoggedIn: true,
        phone: "13812345678",
        passwordUpdatedAt: "1994-01-01T00:00:00.000Z",
        securityEvents: [
          {
            id: "fake-event",
            action: "password_changed",
            time: "1994-01-01T00:00:00.000Z",
          },
        ],
      },
      applyAccountSecurity: () => {},
      logout: () => {},
    }),
}));
vi.mock("../src/lib/api/user", () => ({
  useUserSecurityHistory: () => ({
    data: history.events,
    isError: history.error,
    isSuccess: !history.error,
    isLoading: false,
    isFetching: false,
  }),
  useUserProfile: () => ({
    data: profile.data,
    isError: profile.error,
    error: profile.reason,
  }),
  useSessions: () => ({
    data: devices.sessions,
    isLoading: false,
    isError: devices.error,
    error: devices.reason,
  }),
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
afterEach(() => {
  useSystemAccess.setState(initial, true);
  devices.error = false;
  devices.reason = null;
  devices.sessions = [];
  profile.data = undefined;
  profile.error = false;
  profile.reason = null;
  history.error = false;
  history.events = [];
});
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
  it("hides cached security events on failure without inventing an empty history", () => {
    useSystemAccess.getState().setSession({
      id: 7,
      group: "default",
      role: 1,
      username: "Server owner",
    });
    history.events = [
      { id: 1, action: "password_changed", created_at: 1735689600 },
    ];
    history.error = true;
    const html = render();
    expect(html).toContain(
      "Security history is unavailable. Previous records are hidden.",
    );
    expect(html).not.toContain('class="ac-events"');
    expect(html).not.toContain("No changes recorded in this scope.");
    expect(html).toContain("Reload security history");
  });
  it("opens security changes only with explicit server support and hides Demo contacts/history", () => {
    useSystemAccess.getState().setSession({
      id: 7,
      group: "default",
      role: 1,
      username: "Server owner",
    });
    profile.data = {
      id: 7,
      username: "Server owner",
      email: "server@actual.example",
      role: 1,
      status: 1,
      created_at: 1735689600,
      can_update: true,
    };
    let html = render();
    expect(
      html.match(/<button[^>]*data-account-action="password"[^>]*>/)?.[0],
    ).not.toContain('disabled=""');
    expect(
      html.match(/<button[^>]*data-account-action="phone"[^>]*>/)?.[0],
    ).toContain('disabled=""');
    expect(html).not.toContain("138");
    expect(html).not.toContain("1994");
    expect(html).toContain("No changes recorded in this scope.");
    expect(html).toContain(
      "Earlier actions, other entry points and device operations are outside this history.",
    );
    profile.data.can_update = false;
    html = render();
    expect(
      html.match(/<button[^>]*data-account-action="password"[^>]*>/)?.[0],
    ).toContain('disabled=""');
  });
  it("uses only the server profile for email, role, status and creation date", () => {
    useSystemAccess.getState().setSession({
      id: 7,
      group: "default",
      role: 5,
      username: "Server owner",
    });
    profile.data = {
      id: 7,
      username: "Server owner",
      email: "server@actual.example",
      role: 5,
      status: 1,
      created_at: 1735689600,
    };
    const html = render();
    expect(html).toContain('value="se***@actual.example"');
    expect(html).toContain("Finance");
    expect(html).toContain("Active");
    expect(html).toContain("2025");
    expect(html).not.toContain("1994");
    expect(html).not.toContain("demo.invalid");
    expect(html).toContain("Reload profile");
    expect(html).toContain("Email is a profile field.");
  });
  it("hides cached profile metadata on failure and offers an explicit reload", () => {
    useSystemAccess.getState().setSession({
      id: 7,
      group: "default",
      role: 5,
      username: "Server owner",
    });
    profile.data = {
      id: 7,
      username: "Server owner",
      email: "server@actual.example",
      role: 5,
      status: 1,
      created_at: 1735689600,
    };
    profile.error = true;
    const html = render();
    expect(html).toContain(
      "The account profile is unavailable. Previous details are hidden.",
    );
    expect(html).not.toContain("actual.example");
    expect(html).not.toContain("2025");
    expect(html).not.toContain("Finance");
    expect(html).toContain("Unconfirmed");
    expect(html).toContain("Reload profile");
  });
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
  it("hides cached device rows on a read failure and does not claim the list is empty", () => {
    useSystemAccess.getState().setSession({
      id: 7,
      group: "default",
      role: 1,
      username: "Server owner",
    });
    devices.sessions = [
      { created_at: 100, expires_at: 300, is_current: false },
    ];
    devices.error = true;
    const html = render();
    expect(html).toContain(
      "The device list is unavailable. Previous records are hidden.",
    );
    expect(html).not.toContain('class="ac-device"');
    expect(html).not.toContain("No active sessions.");
    expect(html).toMatch(
      /<button[^>]*disabled[^>]*>Sign out other devices<\/button>/,
    );
    expect(html).toContain("Reload devices");
  });
  it("offers account confirmation after a server-side owner mismatch", () => {
    useSystemAccess.getState().setSession({
      id: 7,
      group: "default",
      role: 1,
      username: "Previous owner",
    });
    devices.error = true;
    devices.reason = new ApiError(409, "session identity changed");
    const html = render();
    expect(html).toContain(
      "The signed-in account may have changed. Confirm the current account.",
    );
    expect(html).toContain("Confirm current account");
    expect(html).not.toContain("No active sessions.");
  });
});

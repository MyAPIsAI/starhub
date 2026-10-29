import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PublicHeader } from "../src/components/product-shell";
import { useSystemAccess } from "../src/lib/system-access";

const fake = vi.hoisted(() => ({ status: "ready" }));
vi.mock("../src/lib/product-preview", () => {
  throw Error("The public account entry must not import Demo identity");
});
vi.mock("../src/lib/session-restore", () => ({
  useSessionRestore: () => ({ status: fake.status, retry: () => {} }),
}));
vi.mock("../src/lib/preferences", () => ({
  usePreferences: (select?: (state: { language: string }) => unknown) => {
    const state = {
      language: "en",
      theme: "light",
      setTheme: () => {},
      setLanguage: () => {},
    };
    return select ? select(state) : state;
  },
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
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
  fake.status = "ready";
});
function render() {
  return renderToStaticMarkup(
    <MemoryRouter initialEntries={["/docs?article=responses"]}>
      <PublicHeader />
    </MemoryRouter>,
  );
}
describe("server session public account entry", () => {
  it("keeps an alias-like server username exact in desktop and mobile entries", () => {
    useSystemAccess.getState().setSession({
      id: 7,
      group: "default",
      role: 1,
      username: "admin@enterprise.example",
    });
    const html = render();
    expect(html.match(/href="\/console\/settings"/g)).toHaveLength(2);
    expect(html.match(/>admin@enterprise.example<\/a>/g)).toHaveLength(2);
    expect(html).not.toContain("veyra_demo");
    expect(html).not.toContain('href="/login"');
  });
  it("uses a neutral account label for a confirmed legacy identity with no username", () => {
    useSystemAccess.getState().setSession({ id: 7, group: "default", role: 1 });
    expect(render().match(/>Account center<\/a>/g)).toHaveLength(2);
  });
  it("never treats a role alone as a confirmed account and clears names on logout", () => {
    useSystemAccess
      .getState()
      .setSession({ id: 7, group: "default", role: 1, username: "Old owner" });
    useSystemAccess.getState().setSession(null);
    useSystemAccess.getState().setRole(100);
    const html = render();
    expect(html.match(/href="\/login"/g)).toHaveLength(2);
    expect(html).not.toContain("Old owner");
    expect(html).not.toContain("veyra-public-account");
  });
  it.each(["checking", "unavailable"])(
    "keeps %s session restoration neutral",
    (status) => {
      useSystemAccess.getState().setSession({
        id: 7,
        group: "default",
        role: 1,
        username: "Private username",
      });
      fake.status = status;
      const html = render();
      expect(html).not.toContain("Private username");
      expect(html).not.toContain('href="/console/settings"');
      expect(html).not.toContain('href="/login"');
      expect(html).toContain(
        status === "checking"
          ? "Checking your session"
          : "Unable to check your session. Retry",
      );
    },
  );
});

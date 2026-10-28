import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useSystemAccess } from "../src/lib/system-access";
import { useProductPreview } from "../src/lib/product-preview";
import { useModelPricing } from "../src/lib/model-pricing";

const access = useSystemAccess.getState();
const preview = useProductPreview.getState();
const pricing = useModelPricing.getState();
beforeEach(() => {
  const values = new Map<string, string>();
  vi.stubGlobal("localStorage", {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => values.set(key, value),
    removeItem: (key: string) => values.delete(key),
  });
  vi.stubGlobal("fetch", vi.fn());
});
afterEach(() => {
  useSystemAccess.setState(access, true);
  useProductPreview.setState(preview, true);
  useModelPricing.setState(pricing, true);
  vi.unstubAllGlobals();
});

describe("local Demo scenarios preserve server authority", () => {
  it.each([1, 5, 10, 99, 100])(
    "preserves actual profile and session for role %s",
    (role) => {
      useProductPreview
        .getState()
        .login("", "server-fixture", { id: "41", username: "server-fixture" });
      useSystemAccess.getState().setSession({ id: 41, role, group: "vip" });
      const user = structuredClone(useProductPreview.getState().user);
      const session = structuredClone(useSystemAccess.getState().session);
      for (const scenario of ["first", "enterprise", "pricing"] as const) {
        useProductPreview.getState().resetScenario(scenario);
        expect(useProductPreview.getState().user).toEqual(user);
        expect(useSystemAccess.getState().session).toEqual(session);
        expect(useSystemAccess.getState().role).toBe(role);
      }
      expect(fetch).not.toHaveBeenCalled();
    },
  );
  it("never establishes a server session for a guest scenario", () => {
    useSystemAccess.getState().setSession(null);
    const session = useSystemAccess.getState().session;
    useProductPreview.getState().resetScenario("pricing");
    expect(useProductPreview.getState().user.isLoggedIn).toBe(false);
    expect(useSystemAccess.getState().session).toEqual(session);
    expect(useSystemAccess.getState().role).toBe(1);
    expect(fetch).not.toHaveBeenCalled();
  });
});

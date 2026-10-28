import { afterEach, describe, expect, it, vi } from "vitest";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.resetModules();
});
function storageFixture(entries: [string, string][] = []) {
  const values = new Map(entries);
  const writes = vi.fn((key: string, value: string) => values.set(key, value));
  vi.stubGlobal("localStorage", {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: writes,
    removeItem: (key: string) => values.delete(key),
  });
  vi.stubGlobal("fetch", vi.fn());
  return { values, writes };
}
describe("explicit browser Demo initialization", () => {
  it("does not create Demo archives on import, pure reads or server profile changes", async () => {
    vi.resetModules();
    const { values, writes } = storageFixture();
    const pricing = await import("../src/lib/model-pricing");
    const preview = await import("../src/lib/product-preview");
    pricing.readPricing();
    preview.useProductPreview.getState().syncBillingStorage();
    preview.useProductPreview
      .getState()
      .login("server@example.invalid", "Server profile", {
        id: "42",
        username: "actual-server",
      });
    preview.useProductPreview.getState().logout();
    expect(writes).not.toHaveBeenCalled();
    expect(values.size).toBe(0);
    expect(fetch).not.toHaveBeenCalled();
  });
  it("starts persistence once on an explicit Demo entry and retains later Demo mutations", async () => {
    vi.resetModules();
    const { values, writes } = storageFixture();
    const { initializeDemoPreview, useProductPreview } =
      await import("../src/lib/product-preview");
    initializeDemoPreview();
    expect(values.has("veyra-platform-pricing-v1")).toBe(true);
    expect(values.has("veyra-billing-demo-v1")).toBe(true);
    const count = writes.mock.calls.length;
    initializeDemoPreview();
    expect(writes).toHaveBeenCalledTimes(count);
    useProductPreview.setState({ balance: "123" });
    expect(JSON.parse(values.get("veyra-billing-demo-v1")!).balance).toBe(
      "123",
    );
    expect(fetch).not.toHaveBeenCalled();
  });
  it("leaves corrupt archives untouched while ordinary modules and pages read", async () => {
    vi.resetModules();
    const { values, writes } = storageFixture([
      ["veyra-platform-pricing-v1", "{invalid-prices"],
      ["veyra-billing-demo-v1", "{invalid-wallet"],
    ]);
    const { readPricing } = await import("../src/lib/model-pricing");
    const { useProductPreview } = await import("../src/lib/product-preview");
    readPricing();
    expect(useProductPreview.getState().syncBillingStorage()).toBe(false);
    expect(values.get("veyra-platform-pricing-v1")).toBe("{invalid-prices");
    expect(values.get("veyra-billing-demo-v1")).toBe("{invalid-wallet");
    expect(writes).not.toHaveBeenCalled();
  });
  it("does not restore a Demo archive identity over a confirmed server session or guest", async () => {
    vi.resetModules();
    storageFixture();
    const { initializeDemoPreview, useProductPreview } =
      await import("../src/lib/product-preview");
    const { useSystemAccess } = await import("../src/lib/system-access");
    initializeDemoPreview();
    useSystemAccess
      .getState()
      .setSession({ id: 42, role: 100, group: "default" });
    useProductPreview
      .getState()
      .login("server@example.invalid", "Server profile", {
        id: "42",
        username: "actual-server",
      });
    const user = structuredClone(useProductPreview.getState().user);
    const raw = JSON.parse(localStorage.getItem("veyra-billing-demo-v1")!);
    raw.user = {
      ...raw.user,
      id: "stored-demo",
      username: "old-demo",
      isLoggedIn: true,
    };
    localStorage.setItem("veyra-billing-demo-v1", JSON.stringify(raw));
    expect(useProductPreview.getState().syncBillingStorage()).toBe(true);
    expect(useProductPreview.getState().user).toEqual(user);
    useSystemAccess.getState().setSession(null);
    useProductPreview.getState().logout();
    const guest = structuredClone(useProductPreview.getState().user);
    raw.user.enterpriseName = "Stored stale account";
    localStorage.setItem("veyra-billing-demo-v1", JSON.stringify(raw));
    expect(useProductPreview.getState().syncBillingStorage()).toBe(true);
    expect(useProductPreview.getState().user).toEqual(guest);
    expect(guest.isLoggedIn).toBe(false);
  });
});

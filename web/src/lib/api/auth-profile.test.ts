import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { authUserProfile, type AuthUser } from "./auth";
import { BILLING_STORAGE_KEY, parseBillingSnapshot } from "../billing";
import { useProductPreview } from "../product-preview";

const user: AuthUser = {
  id: 3,
  username: "no_phone_member",
  email: "",
  role: 1,
  status: 1,
  quota: 0,
  used_quota: 0,
  group: "default",
  aff_code: "",
  inviter_id: 0,
  created_at: 1,
};
const initialState = useProductPreview.getState();
let saved: Map<string, string>;

beforeEach(() => {
  saved = new Map();
  vi.stubGlobal("localStorage", {
    getItem: (key: string) => saved.get(key) ?? null,
    setItem: (key: string, value: string) => saved.set(key, value),
  });
  useProductPreview.getState().resetScenario("enterprise");
});

afterEach(() => {
  useProductPreview.setState(initialState, true);
  vi.unstubAllGlobals();
});

describe("backend account profiles", () => {
  it("does not invent an empty phone number for a backend account", () => {
    expect(authUserProfile(user)).toEqual({ id: "3", username: user.username });
    expect(authUserProfile(user)).not.toHaveProperty("phone");
  });

  it("recovers an older empty-phone snapshot without changing its financial or account data", () => {
    useProductPreview
      .getState()
      .login(user.email, user.username, authUserProfile(user));
    const raw = saved.get(BILLING_STORAGE_KEY)!;
    const legacy = JSON.parse(raw);
    legacy.user.phone = "";
    expect(parseBillingSnapshot(JSON.stringify(legacy))).toEqual(
      parseBillingSnapshot(raw),
    );
    legacy.balance = "1";
    expect(parseBillingSnapshot(JSON.stringify(legacy))).toBeNull();
  });

  it.each(["login", "register"] as const)(
    "keeps a %s profile valid when another tab reads the saved snapshot",
    (action) => {
      useProductPreview
        .getState()
        [action](user.email, user.username, authUserProfile(user));
      const raw = saved.get(BILLING_STORAGE_KEY)!;
      const snapshot = parseBillingSnapshot(raw);
      expect(snapshot).not.toBeNull();
      expect(snapshot?.user).toMatchObject({
        id: "3",
        username: user.username,
        isLoggedIn: true,
      });
      expect(snapshot?.user).not.toHaveProperty("phone");
      useProductPreview.setState(initialState, true);
      saved.set(BILLING_STORAGE_KEY, raw);
      expect(useProductPreview.getState().syncBillingStorage()).toBe(true);
      expect(useProductPreview.getState().user).toMatchObject({
        id: "3",
        username: user.username,
        isLoggedIn: true,
        phone: undefined,
      });
    },
  );
});

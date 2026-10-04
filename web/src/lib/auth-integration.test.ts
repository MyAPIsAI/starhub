import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useProductPreview } from "./product-preview";
import { BILLING_STORAGE_KEY, parseBillingSnapshot } from "./billing";
import { useModelPricing } from "./model-pricing";
import { demoAuth, DemoAuthService, DEMO_ACCOUNT } from "./demo-auth";
import { accountDisplayName, DEMO_ACCOUNT_PROFILE } from "./account-identity";

const initialState = useProductPreview.getState();
let savedValues: Map<string, string>;

beforeEach(() => {
  savedValues = new Map();
  vi.stubGlobal("localStorage", {
    getItem: (key: string) => savedValues.get(key) ?? null,
    setItem: (key: string, value: string) => savedValues.set(key, value),
    removeItem: (key: string) => savedValues.delete(key),
  });
  useProductPreview.getState().resetScenario("enterprise");
});

afterEach(() => {
  useProductPreview.setState(initialState, true);
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("authentication preserves prepaid account rules", () => {
  it("logging in does not unfreeze an account or change its wallet, ledger, calls, orders, or key permissions", () => {
    const store = useProductPreview;
    const key = store.getState().keys[0];
    const successfulCall = store
      .getState()
      .recordCall(key.id, false, "gpt-4.1");
    expect(successfulCall?.billing_status).toBe("charged");
    store.getState().createRechargeOrder("20.00");
    store.setState((state) => ({
      user: { ...state.user, accountStatus: "frozen", isLoggedIn: false },
    }));
    const before = store.getState();
    const financialAndPermissionState = {
      balance: before.balance,
      opening: before.opening,
      ledger: structuredClone(before.ledger),
      rechargeOrders: structuredClone(before.rechargeOrders),
      requests: structuredClone(before.requests),
      keys: structuredClone(before.keys),
      selectedKey: before.selectedKey,
    };

    before.login("", "demo-maker", {
      username: "demo-maker",
      phone: "+8613800001234",
    });
    const after = store.getState();
    expect(after.user.isLoggedIn).toBe(true);
    expect(after.user.accountStatus).toBe("frozen");
    expect(after.user).toMatchObject({
      username: "demo-maker",
      phone: "+8613800001234",
      email: "",
    });
    expect(after).toMatchObject(financialAndPermissionState);
    const blockedCall = after.recordCall(key.id, false, "gpt-4.1");
    expect(blockedCall?.error_code).toBe("account_unavailable");
    expect(blockedCall?.billing_status).toBe("not_charged");
    expect(store.getState().balance).toBe(financialAndPermissionState.balance);
    expect(store.getState().ledger).toEqual(financialAndPermissionState.ledger);
  });

  it("registration starts with zero prepaid balance and no inherited keys or free credit", () => {
    const store = useProductPreview;
    expect(BigInt(store.getState().balance)).toBeGreaterThan(0n);
    expect(store.getState().keys.length).toBeGreaterThan(0);
    store.getState().register("new-team@example.com", "New demo company");
    const registered = store.getState();
    expect(registered).toMatchObject({
      scenario: "first",
      balance: "0",
      opening: "0",
      requests: [],
      keys: [],
      selectedKey: "",
      ledger: [],
      rechargeOrders: [],
      exports: [],
      toolChains: {},
      revealedKeyIds: {},
      demoMode: { accountState: "new", balanceState: "zero" },
      user: {
        email: "new-team@example.com",
        enterpriseName: "New demo company",
        isLoggedIn: true,
        onboardingCompleted: false,
        accountStatus: "normal",
      },
    });

    const key = registered.createKeyDetailed({
      name: "New company first key",
      tier: "pro",
      model: "gpt-4.1",
      quotaLimit_usd: 100,
      quotaPeriod: "never",
    });
    const unpaidCall = store.getState().recordCall(key.id, false, "gpt-4.1");
    expect(unpaidCall?.error_code).toBe("insufficient_balance");
    expect(unpaidCall?.billing_status).toBe("not_charged");
    expect(unpaidCall?.cost_nano).toBe("0");
    expect(store.getState().balance).toBe("0");
    expect(store.getState().ledger).toEqual([]);
    store.getState().logout();
    store.getState().login("new-team@example.com", "New demo company");
    expect(store.getState().balance).toBe("0");
    expect(store.getState().keys.map((item) => item.id)).toEqual([key.id]);
  });

  it("restores a phone account profile and zero-balance wallet from the persisted snapshot without inventing an email", () => {
    const store = useProductPreview;
    const profile = { username: "demo-maker", phone: "+8613800001234" };
    store.getState().register("", profile.username, profile);
    const order = store.getState().createRechargeOrder("20.00");
    expect(order.ok).toBe(true);
    const original = store.getState();
    const raw = savedValues.get(BILLING_STORAGE_KEY)!;
    const parsed = parseBillingSnapshot(raw);
    expect(parsed?.user).toMatchObject({ ...profile, email: "" });
    expect(parsed?.balance).toBe("0");
    expect(parsed?.ledger).toEqual([]);

    store.setState(initialState, true);
    savedValues.set(BILLING_STORAGE_KEY, raw);
    expect(store.getState().syncBillingStorage()).toBe(true);
    expect(store.getState().user).toMatchObject({ ...profile, email: "" });
    expect(store.getState().balance).toBe("0");
    expect(store.getState().rechargeOrders).toEqual(original.rechargeOrders);
    store.getState().logout();
    store.getState().login("", profile.username, profile);
    expect(store.getState().balance).toBe("0");
    expect(store.getState().ledger).toEqual([]);
    expect(store.getState().rechargeOrders).toEqual(original.rechargeOrders);
  });

  it("restores a legacy enterprise snapshot without retaining a previously viewed personal identity", () => {
    const store = useProductPreview;
    const snapshot = JSON.parse(savedValues.get(BILLING_STORAGE_KEY)!);
    delete snapshot.user.username;
    delete snapshot.user.phone;
    snapshot.user.enterpriseName = "Existing custom company";
    snapshot.user.accountStatus = "frozen";
    const enterpriseRaw = JSON.stringify(snapshot);
    expect(parseBillingSnapshot(enterpriseRaw)?.user?.username).toBeUndefined();
    store.getState().register("", "demo-maker", {
      username: "demo-maker",
      phone: "+8613800001234",
    });
    savedValues.set(BILLING_STORAGE_KEY, enterpriseRaw);
    expect(store.getState().syncBillingStorage()).toBe(true);
    expect(store.getState().user.email).toBe("admin@enterprise.example");
    expect(store.getState().user.username).toBeUndefined();
    expect(store.getState().user.phone).toBeUndefined();
    expect(store.getState().user.enterpriseName).toBe(
      "Existing custom company",
    );
    expect(store.getState().user.accountStatus).toBe("frozen");
    expect(store.getState().balance).toBe(snapshot.balance);
    expect(store.getState().ledger).toEqual(snapshot.ledger);
    expect(
      store.getState().keys.map(({ keySecret: _secret, ...key }) => key),
    ).toEqual(snapshot.keys);
    expect(accountDisplayName(store.getState().user)).toBe("veyra_demo");
  });

  it.each(["first", "enterprise", "pricing"] as const)(
    "restores the built-in demo account when resetting the %s scenario",
    (scenario) => {
      const store = useProductPreview;
      store.getState().register("", "demo-maker", {
        username: "demo-maker",
        phone: "+8613800001234",
      });
      store.getState().resetScenario(scenario);
      expect(store.getState().user).toMatchObject(DEMO_ACCOUNT_PROFILE);
      const saved = parseBillingSnapshot(savedValues.get(BILLING_STORAGE_KEY)!);
      expect(saved?.user).toMatchObject(DEMO_ACCOUNT_PROFILE);
    },
  );

  it("rejects corrupt profile fields while accepting an unbound email", () => {
    useProductPreview.getState().register("", "demo-maker", {
      username: "demo-maker",
      phone: "+8613800001234",
    });
    const saved = JSON.parse(savedValues.get(BILLING_STORAGE_KEY)!);
    expect(parseBillingSnapshot(JSON.stringify(saved))).not.toBeNull();
    saved.user.phone = 13800001234;
    expect(parseBillingSnapshot(JSON.stringify(saved))).toBeNull();
    saved.user.phone = "+8613800001234";
    saved.user.username = {};
    expect(parseBillingSnapshot(JSON.stringify(saved))).toBeNull();
  });
});

describe("account display identity compatibility", () => {
  it("uses the username ahead of legacy company or email fields without modifying saved data", () => {
    const profile = Object.freeze({
      username: "demo-maker",
      phone: "+8613800001234",
      email: "legacy@example.com",
      enterpriseName: "Custom historical company",
    });
    expect(accountDisplayName(profile)).toBe("demo-maker");
    expect(profile.enterpriseName).toBe("Custom historical company");
  });

  it("keeps old email identities and never invents a username from a company name", () => {
    expect(
      accountDisplayName({
        email: "legacy@example.com",
        enterpriseName: "Company",
      }),
    ).toBe("legacy@example.com");
    expect(
      accountDisplayName({
        email: "admin@enterprise.example",
        enterpriseName: "Old company",
      }),
    ).toBe("veyra_demo");
    expect(accountDisplayName({ enterpriseName: "Company" })).toBe("演示账号");
    expect(
      accountDisplayName({ username: " ", email: " " }, "Demo account"),
    ).toBe("Demo account");
  });
});

describe("account center identity and business integration", () => {
  it("keeps stable ID, wallet and key authorization when verified contacts change and persists security state", () => {
    const store = useProductPreview;
    const before = store.getState();
    const identity = {
      id: before.user.id,
      username: before.user.username!,
      phone: "+8613900005678",
      email: "bound@example.test",
    };
    store.getState().applyAccountSecurity(identity, "email_changed");
    const after = store.getState();
    expect(after.user.id).toBe(before.user.id);
    expect(after.balance).toBe(before.balance);
    expect(after.ledger).toBe(before.ledger);
    expect(after.keys).toBe(before.keys);
    expect(after.user.email).toBe(identity.email);
    expect(after.user.securityEvents[0].action).toBe("email_changed");
    expect(after.user.sessions).toHaveLength(1);
    const raw = savedValues.get(BILLING_STORAGE_KEY)!;
    const saved = parseBillingSnapshot(raw)!;
    expect(saved.user?.id).toBe(before.user.id);
    expect(saved.user?.securityEvents).toEqual(after.user.securityEvents);
    store.setState(initialState, true);
    savedValues.set(BILLING_STORAGE_KEY, raw);
    store.getState().syncBillingStorage();
    expect(store.getState().user.id).toBe(before.user.id);
    expect(store.getState().user.securityEvents).toEqual(
      after.user.securityEvents,
    );
  });

  it("keeps profile history attached to each ID when moving between local demo identities", () => {
    const store = useProductPreview;
    const a = {
      id: "account-user-a",
      username: "user_a",
      phone: "+8613900001111",
    };
    const b = {
      id: "account-user-b",
      username: "user_b",
      phone: "+8613900002222",
    };
    store.getState().register("", a.username, a);
    store
      .getState()
      .applyAccountSecurity({ ...a, email: "a@example.test" }, "email_changed");
    const original = store.getState().user;
    store.getState().register("", b.username, b);
    expect(store.getState().user.securityEvents).toEqual([]);
    expect(store.getState().user.passwordUpdatedAt).toBeUndefined();
    store.getState().login("a@example.test", a.username, a);
    expect(store.getState().user.securityEvents).toEqual(
      original.securityEvents,
    );
    expect(store.getState().user.createdAt).toBe(original.createdAt);
    const raw = savedValues.get(BILLING_STORAGE_KEY)!;
    store.setState(initialState, true);
    savedValues.set(BILLING_STORAGE_KEY, raw);
    store.getState().syncBillingStorage();
    store.getState().login("", b.username, b);
    expect(store.getState().user.securityEvents).toEqual([]);
    const unchanged = store.getState();
    expect(() =>
      unchanged.applyAccountSecurity({ ...a }, "password_changed"),
    ).toThrow("ACCOUNT_CHANGED");
    expect(store.getState()).toBe(unchanged);
  });

  it("uses the current password to reveal a key, invalidates old password and cached reveals after change", async () => {
    const authValues = new Map<string, string>();
    const auth = new DemoAuthService({
      getItem: (key) => authValues.get(key) ?? null,
      setItem: (key, value) => {
        authValues.set(key, value);
      },
    });
    vi.spyOn(demoAuth, "verifyPassword").mockImplementation((id, password) =>
      auth.verifyPassword(id, password),
    );
    const store = useProductPreview;
    const key = store.getState().keys[0];
    expect(
      await store
        .getState()
        .verifyPasswordAndRevealKey(key.id, "arbitrary-eight-characters"),
    ).toBe(false);
    expect(
      await store
        .getState()
        .verifyPasswordAndRevealKey(key.id, DEMO_ACCOUNT.password),
    ).toBe(true);
    const identity = await auth.changePassword({
      accountId: store.getState().user.id,
      currentPassword: DEMO_ACCOUNT.password,
      newPassword: "ChangedPassword123",
    });
    store.getState().applyAccountSecurity(identity, "password_changed");
    expect(store.getState().revealedKeyIds).toEqual({});
    expect(store.getState().user.passwordUpdatedAt).toBeTruthy();
    expect(store.getState().keys[0]).toEqual(key);
    expect(
      await store
        .getState()
        .verifyPasswordAndRevealKey(key.id, DEMO_ACCOUNT.password),
    ).toBe(false);
    expect(
      await store
        .getState()
        .verifyPasswordAndRevealKey(key.id, "ChangedPassword123"),
    ).toBe(true);
    store.getState().logout();
    expect(store.getState().revealedKeyIds).toEqual({});
    expect(
      await store
        .getState()
        .verifyPasswordAndRevealKey(key.id, "ChangedPassword123"),
    ).toBe(false);
  });

  it("attributes newly created keys to the stable user ID", () => {
    const store = useProductPreview;
    store
      .getState()
      .register("", "another", {
        id: "account-new-owner",
        username: "another",
        phone: "+8613900003333",
      });
    const key = store
      .getState()
      .createKeyDetailed({
        name: "New owner key",
        tier: "pro",
        model: "gpt-4.1",
      });
    expect(key.accountId).toBe("account-new-owner");
    store
      .getState()
      .applyAccountSecurity(
        {
          id: "account-new-owner",
          username: "another",
          phone: "+8613900004444",
        },
        "phone_changed",
      );
    expect(store.getState().keys[0].accountId).toBe("account-new-owner");
  });

  it("does not grant a newly registered ID access to a demo-account-only custom group", () => {
    const pricing = useModelPricing.getState().config;
    const custom = structuredClone(pricing);
    custom.groups.pro = {
      ...custom.groups.pro,
      visibility: "custom",
      accountIds: ["account-demo"],
    };
    useModelPricing.setState({ config: custom });
    try {
      useProductPreview
        .getState()
        .register("", "restricted_user", {
          id: "account-new-restricted",
          username: "restricted_user",
          phone: "+8613900005555",
        });
      expect(() =>
        useProductPreview
          .getState()
          .createKeyDetailed({
            name: "Private group",
            tier: "pro",
            model: "gpt-4.1",
          }),
      ).toThrow("No available authorized model");
      expect(useProductPreview.getState().keys).toEqual([]);
      expect(useProductPreview.getState().balance).toBe("0");
    } finally {
      useModelPricing.setState({ config: pricing });
    }
  });

  it("persists revocation of simulated sessions and rejects malformed security histories", () => {
    const store = useProductPreview;
    store.setState((s) => ({
      user: {
        ...s.user,
        sessions: [
          ...s.user.sessions.filter((session) => session.isCurrent),
          {
            id: "other",
            device: "Simulated browser",
            ip: "127.0.0.2",
            lastActive: "Demo",
            isCurrent: false,
          },
        ],
      },
    }));
    store.getState().terminateOtherSessions();
    const saved = JSON.parse(savedValues.get(BILLING_STORAGE_KEY)!);
    expect(saved.user.sessions).toHaveLength(1);
    expect(saved.user.securityEvents[0].action).toBe("sessions_revoked");
    saved.user.securityEvents[0].action = "invented";
    expect(parseBillingSnapshot(JSON.stringify(saved))).toBeNull();
  });
});

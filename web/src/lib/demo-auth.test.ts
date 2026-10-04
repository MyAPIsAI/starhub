import { createHash } from "node:crypto";
import { describe, expect, it } from "vitest";
import { accountIdForLegacy } from "./account-identity";
import {
  DEMO_ACCOUNT,
  DemoAuthService,
  normalizeDemoPhone,
  normalizeDemoUsername,
} from "./demo-auth";

class MemoryStorage {
  data = new Map<string, string>();
  getItem(key: string) {
    return this.data.get(key) ?? null;
  }
  setItem(key: string, value: string) {
    this.data.set(key, value);
  }
}

function fixture() {
  const storage = new MemoryStorage();
  let time = Date.UTC(2026, 8, 27);
  const now = () => time;
  return {
    storage,
    service: new DemoAuthService(storage, now),
    now,
    advance(ms: number) {
      time += ms;
    },
    refresh() {
      return new DemoAuthService(storage, now);
    },
  };
}

const registration = {
  username: "demo_team",
  phone: "13900139000",
  password: "demoPassword789",
  code: DEMO_ACCOUNT.code,
};
const registeredIdentity = {
  id: expect.stringMatching(/^account-[a-f0-9-]{36}$/),
  username: registration.username,
  phone: "+8613900139000",
};
const builtinIdentity = {
  id: "account-demo",
  username: DEMO_ACCOUNT.username,
  phone: DEMO_ACCOUNT.phone,
  email: DEMO_ACCOUNT.email,
  enterpriseName: DEMO_ACCOUNT.enterpriseName,
};

function legacyCredential(email: string, password: string) {
  const salt = "12".repeat(16);
  return {
    email,
    enterpriseName: "Legacy enterprise",
    salt,
    digest: createHash("sha256").update(`${salt}:${password}`).digest("hex"),
  };
}

function legacySnapshot(now: number) {
  return {
    version: 1,
    accounts: {
      [DEMO_ACCOUNT.email]: legacyCredential(
        DEMO_ACCOUNT.email,
        "legacyDemo456",
      ),
      "legacy@example.com": legacyCredential(
        "legacy@example.com",
        "legacyUser789",
      ),
    },
    challenges: {
      "phone-login": {
        purpose: "phone-login",
        target: DEMO_ACCOUNT.phone,
        code: DEMO_ACCOUNT.code,
        expiresAt: now + 600_000,
        resendAt: now + 60_000,
        attemptsRemaining: 3,
      },
      register: {
        purpose: "register",
        target: "old-pending@example.com",
        code: DEMO_ACCOUNT.code,
        expiresAt: now + 600_000,
        resendAt: now + 60_000,
        attemptsRemaining: 5,
      },
    },
    resendAt: now + 60_000,
    failures: 0,
    lockedUntil: 0,
  };
}

describe("local demo authentication", () => {
  it("accepts the exact built-in password by username, phone, and retained email alias", async () => {
    const { service } = fixture();
    await expect(
      service.loginPassword(DEMO_ACCOUNT.username, "anyPassword123"),
    ).rejects.toMatchObject({ code: "INVALID_CREDENTIALS" });
    await expect(
      service.loginPassword("unknown@example.com", DEMO_ACCOUNT.password),
    ).rejects.toMatchObject({ code: "INVALID_CREDENTIALS" });
    for (const alias of [
      " VEYRA_DEMO ",
      "13800138000",
      "+86 13800138000",
      " ADMIN@ENTERPRISE.EXAMPLE ",
    ]) {
      await expect(
        service.loginPassword(alias, DEMO_ACCOUNT.password),
      ).resolves.toEqual(builtinIdentity);
    }
  });

  it("normalizes supported phone formats and does not create unknown accounts during login", () => {
    const { service } = fixture();
    expect(normalizeDemoPhone("138 0013 8000")).toBe(DEMO_ACCOUNT.phone);
    expect(normalizeDemoPhone("0086-13800138000")).toBe(DEMO_ACCOUNT.phone);
    expect(() => service.issueCode("phone-login", "+12025550123")).toThrow(
      "INVALID_PHONE",
    );
    expect(() => service.issueCode("phone-login", registration.phone)).toThrow(
      "ACCOUNT_NOT_FOUND",
    );
    expect(() =>
      service.loginPhone(registration.phone, DEMO_ACCOUNT.code),
    ).toThrow("ACCOUNT_NOT_FOUND");
  });

  it("validates multilingual usernames and normalizes compatibility characters and case", () => {
    expect(normalizeDemoUsername(" 小明 ")).toBe("小明");
    expect(normalizeDemoUsername("مستخدم_١٢")).toBe("مستخدم_١٢");
    expect(normalizeDemoUsername(" Demo_User-2 ")).toBe("demo_user-2");
    expect(normalizeDemoUsername("Ｄｅｍｏ＿Ｕｓｅｒ")).toBe("demo_user");
    expect(normalizeDemoUsername("明".repeat(24))).toHaveLength(24);
    for (const invalid of [
      "a",
      "明".repeat(25),
      "13800138000",
      "1user",
      "_user",
      "has space",
      "user@example.com",
      "🙂user",
    ])
      expect(() => normalizeDemoUsername(invalid)).toThrow("INVALID_USERNAME");
  });

  it("requires sending the matching phone challenge before login", () => {
    const { service } = fixture();
    expect(() => service.loginPhone(DEMO_ACCOUNT.phone, "")).toThrow(
      "CODE_REQUIRED",
    );
    expect(() =>
      service.loginPhone(DEMO_ACCOUNT.phone, DEMO_ACCOUNT.code),
    ).toThrow("CODE_NOT_ISSUED");
    service.issueCode("phone-login", "13800138000");
    expect(service.loginPhone("+86 13800138000", DEMO_ACCOUNT.code)).toEqual(
      builtinIdentity,
    );
  });

  it("binds every code purpose to a phone and never accepts email challenges for phone operations", async () => {
    const { service } = fixture();
    expect(() => service.issueCode("register", "legacy@example.com")).toThrow(
      "INVALID_PHONE",
    );
    expect(() => service.issueCode("reset", DEMO_ACCOUNT.email)).toThrow(
      "INVALID_PHONE",
    );
    service.issueCode("register", registration.phone);
    expect(service.challenge("register", "+86 13900139000")?.target).toBe(
      registeredIdentity.phone,
    );
    expect(service.challenge("register", "13700137000")).toBeNull();
    expect(service.challenge("reset", registration.phone)).toBeNull();
    await expect(
      service.register({ ...registration, phone: "13700137000" }),
    ).rejects.toMatchObject({ code: "CODE_NOT_ISSUED" });
    expect(() =>
      service.loginPhone(DEMO_ACCOUNT.phone, DEMO_ACCOUNT.code),
    ).toThrow("CODE_NOT_ISSUED");
    await expect(
      service.resetPassword(
        DEMO_ACCOUNT.phone,
        DEMO_ACCOUNT.code,
        "newPassword123",
      ),
    ).rejects.toMatchObject({ code: "CODE_NOT_ISSUED" });
  });

  it("keeps a shared absolute resend cooldown across targets, purposes, and refresh", () => {
    const f = fixture();
    const challenge = f.service.issueCode("register", registration.phone);
    expect(challenge.resendAt).toBe(f.now() + 60_000);
    expect(() => f.service.issueCode("register", "13700137000")).toThrow(
      "CODE_COOLDOWN",
    );
    expect(() =>
      f.service.issueCode("phone-login", DEMO_ACCOUNT.phone),
    ).toThrow("CODE_COOLDOWN");
    f.advance(59_000);
    const refreshed = f.refresh();
    expect(refreshed.cooldownUntil() - f.now()).toBe(1_000);
    expect(() => refreshed.issueCode("register", registration.phone)).toThrow(
      "CODE_COOLDOWN",
    );
    f.advance(1_000);
    expect(refreshed.cooldownUntil()).toBe(0);
    expect(refreshed.issueCode("register", "13700137000").target).toBe(
      "+8613700137000",
    );
    expect(refreshed.challenge("register", registration.phone)).toBeNull();
  });

  it("expires at ten minutes but retains metadata for an explanatory interface", () => {
    const f = fixture();
    const challenge = f.service.issueCode("phone-login", DEMO_ACCOUNT.phone);
    expect(challenge.expiresAt).toBe(f.now() + 600_000);
    f.advance(600_000);
    const refreshed = f.refresh();
    expect(
      refreshed.challenge("phone-login", DEMO_ACCOUNT.phone)?.expiresAt,
    ).toBe(f.now());
    expect(() =>
      refreshed.loginPhone(DEMO_ACCOUNT.phone, DEMO_ACCOUNT.code),
    ).toThrow("CODE_EXPIRED");
    refreshed.issueCode("phone-login", DEMO_ACCOUNT.phone);
    expect(refreshed.loginPhone(DEMO_ACCOUNT.phone, DEMO_ACCOUNT.code)).toEqual(
      builtinIdentity,
    );
  });

  it("allows correction after one code typo and rejects replay after successful use", () => {
    const { service } = fixture();
    service.issueCode("phone-login", DEMO_ACCOUNT.phone);
    expect(() => service.loginPhone(DEMO_ACCOUNT.phone, "000000")).toThrow(
      "CODE_INVALID",
    );
    expect(
      service.challenge("phone-login", DEMO_ACCOUNT.phone)?.attemptsRemaining,
    ).toBe(4);
    service.loginPhone(DEMO_ACCOUNT.phone, DEMO_ACCOUNT.code);
    expect(() =>
      service.loginPhone(DEMO_ACCOUNT.phone, DEMO_ACCOUNT.code),
    ).toThrow("CODE_NOT_ISSUED");
    expect(() => service.issueCode("phone-login", DEMO_ACCOUNT.phone)).toThrow(
      "CODE_COOLDOWN",
    );
  });

  it("invalidates a code after five mistakes and persists that state until a new code is sent", () => {
    const f = fixture();
    f.service.issueCode("phone-login", DEMO_ACCOUNT.phone);
    for (let attempt = 0; attempt < 4; attempt++)
      expect(() => f.service.loginPhone(DEMO_ACCOUNT.phone, "000000")).toThrow(
        "CODE_INVALID",
      );
    expect(() => f.refresh().loginPhone(DEMO_ACCOUNT.phone, "000000")).toThrow(
      "CODE_EXHAUSTED",
    );
    expect(() =>
      f.refresh().loginPhone(DEMO_ACCOUNT.phone, DEMO_ACCOUNT.code),
    ).toThrow("CODE_EXHAUSTED");
    f.advance(60_000);
    const retry = f.refresh();
    retry.issueCode("phone-login", DEMO_ACCOUNT.phone);
    expect(retry.loginPhone(DEMO_ACCOUNT.phone, DEMO_ACCOUNT.code)).toEqual(
      builtinIdentity,
    );
  });

  it("locks password login for fifteen minutes after five errors without alias or refresh bypass", async () => {
    const f = fixture();
    for (let attempt = 0; attempt < 4; attempt++)
      await expect(
        f.service.loginPassword(DEMO_ACCOUNT.email, "bad-password"),
      ).rejects.toMatchObject({ code: "INVALID_CREDENTIALS" });
    const refreshed = f.refresh();
    await expect(
      refreshed.loginPassword(DEMO_ACCOUNT.username, "bad-password"),
    ).rejects.toMatchObject({
      code: "TEMPORARILY_LOCKED",
      retryAt: f.now() + 900_000,
    });
    await expect(
      f.refresh().loginPassword(DEMO_ACCOUNT.phone, DEMO_ACCOUNT.password),
    ).rejects.toMatchObject({ code: "TEMPORARILY_LOCKED" });
    f.advance(899_999);
    expect(f.refresh().lockedUntil()).toBe(f.now() + 1);
    f.advance(1);
    await expect(
      f.refresh().loginPassword(DEMO_ACCOUNT.username, DEMO_ACCOUNT.password),
    ).resolves.toEqual(builtinIdentity);
  });

  it("can recover a password lock using independent phone verification", async () => {
    const { service } = fixture();
    for (let attempt = 0; attempt < 5; attempt++)
      await service
        .loginPassword(DEMO_ACCOUNT.email, "bad-password")
        .catch(() => {});
    service.issueCode("phone-login", DEMO_ACCOUNT.phone);
    service.loginPhone(DEMO_ACCOUNT.phone, DEMO_ACCOUNT.code);
    expect(service.lockedUntil()).toBe(0);
    await expect(
      service.loginPassword(DEMO_ACCOUNT.username, DEMO_ACCOUNT.password),
    ).resolves.toEqual(builtinIdentity);
  });

  it("validates username, phone, password, and prior verification before registration", async () => {
    const { service } = fixture();
    await expect(
      service.register({ ...registration, username: "1invalid" }),
    ).rejects.toMatchObject({ code: "INVALID_USERNAME" });
    await expect(
      service.register({ ...registration, phone: "123" }),
    ).rejects.toMatchObject({ code: "INVALID_PHONE" });
    await expect(
      service.register({ ...registration, password: "abcdefgh" }),
    ).rejects.toMatchObject({ code: "WEAK_PASSWORD" });
    await expect(service.register(registration)).rejects.toMatchObject({
      code: "CODE_NOT_ISSUED",
    });
    expect(() => service.issueCode("register", DEMO_ACCOUNT.phone)).toThrow(
      "PHONE_EXISTS",
    );
  });

  it("registers only the submitted identity, persists salted hashes, and supports all valid account aliases", async () => {
    const f = fixture();
    f.service.issueCode("register", registration.phone);
    const identity = await f.service.register({
      ...registration,
      username: " DEMO_TEAM ",
    });
    expect(identity).toEqual(registeredIdentity);
    expect(identity).not.toHaveProperty("email");
    expect(identity).not.toHaveProperty("enterpriseName");
    const saved = [...f.storage.data.values()].join("");
    expect(saved).not.toContain(registration.password);
    expect(saved).not.toContain(DEMO_ACCOUNT.password);
    expect(saved).toContain('"salt"');
    expect(saved).toContain('"digest"');
    expect(f.service.challenge("register", registration.phone)).toBeNull();
    const refreshed = f.refresh();
    for (const alias of [
      registration.username,
      "Ｄｅｍｏ＿Ｔｅａｍ",
      registration.phone,
      "+86 13900139000",
    ])
      await expect(
        refreshed.loginPassword(alias, registration.password),
      ).resolves.toEqual(registeredIdentity);
    await expect(
      refreshed.loginPassword(DEMO_ACCOUNT.username, DEMO_ACCOUNT.password),
    ).resolves.toEqual(builtinIdentity);
    await expect(
      refreshed.loginPassword(registration.username, registration.password),
    ).resolves.toEqual(registeredIdentity);
  });

  it("rejects duplicate normalized usernames and bound phone numbers with distinct reasons", async () => {
    const f = fixture();
    f.service.issueCode("register", registration.phone);
    await f.service.register(registration);
    await expect(
      f.service.register({
        ...registration,
        username: "Ｄｅｍｏ＿Ｔｅａｍ",
        phone: "13700137000",
      }),
    ).rejects.toMatchObject({ code: "USERNAME_EXISTS" });
    await expect(
      f.service.register({
        ...registration,
        username: "other_team",
        phone: "+86 13900139000",
      }),
    ).rejects.toMatchObject({ code: "PHONE_EXISTS" });
    await expect(
      f.service.register({
        ...registration,
        username: "VEYRA_DEMO",
        phone: "13700137000",
      }),
    ).rejects.toMatchObject({ code: "USERNAME_EXISTS" });
    f.advance(60_000);
    expect(() => f.service.issueCode("register", registration.phone)).toThrow(
      "PHONE_EXISTS",
    );
  });

  it("supports Chinese and Arabic registration and password login", async () => {
    const f = fixture();
    for (const [username, phone] of [
      ["小明", "13700137000"],
      ["مستخدم_١٢", "13600136000"],
    ]) {
      f.service.issueCode("register", phone);
      const identity = await f.service.register({
        ...registration,
        username,
        phone,
      });
      expect(identity).toEqual({
        id: expect.stringMatching(/^account-[a-f0-9-]{36}$/),
        username,
        phone: `+86${phone}`,
      });
      await expect(
        f.refresh().loginPassword(username, registration.password),
      ).resolves.toEqual(identity);
      f.advance(60_000);
    }
  });

  it("routes a newly registered phone-code login to its own account rather than the built-in demo", async () => {
    const f = fixture();
    f.service.issueCode("register", registration.phone);
    await f.service.register(registration);
    f.advance(60_000);
    const refreshed = f.refresh();
    refreshed.issueCode("phone-login", registration.phone);
    expect(refreshed.loginPhone(registration.phone, DEMO_ACCOUNT.code)).toEqual(
      registeredIdentity,
    );
    f.advance(60_000);
    refreshed.issueCode("phone-login", DEMO_ACCOUNT.phone);
    expect(refreshed.loginPhone(DEMO_ACCOUNT.phone, DEMO_ACCOUNT.code)).toEqual(
      builtinIdentity,
    );
    expect(() =>
      refreshed.loginPhone("13700137000", DEMO_ACCOUNT.code),
    ).toThrow("ACCOUNT_NOT_FOUND");
  });

  it("does not consume registration verification when correctable form validation fails", async () => {
    const { service } = fixture();
    service.issueCode("register", registration.phone);
    await expect(
      service.register({ ...registration, password: "short" }),
    ).rejects.toMatchObject({ code: "WEAK_PASSWORD" });
    await expect(service.register(registration)).resolves.toEqual(
      registeredIdentity,
    );
  });

  it("resets the account actually bound to the verified phone and rejects replay", async () => {
    const f = fixture();
    f.service.issueCode("register", registration.phone);
    await f.service.register(registration);
    f.advance(60_000);
    expect(() => f.service.issueCode("reset", "13700137000")).toThrow(
      "ACCOUNT_NOT_FOUND",
    );
    f.service.issueCode("reset", registration.phone);
    await expect(
      f.service.resetPassword(registration.phone, "000000", "replacement456"),
    ).rejects.toMatchObject({ code: "CODE_INVALID" });
    await f.service.resetPassword(
      registration.phone,
      DEMO_ACCOUNT.code,
      "replacement456",
    );
    const refreshed = f.refresh();
    await expect(
      refreshed.loginPassword(registration.username, registration.password),
    ).rejects.toMatchObject({ code: "INVALID_CREDENTIALS" });
    await expect(
      refreshed.loginPassword(registration.username, "replacement456"),
    ).resolves.toEqual(registeredIdentity);
    await expect(
      refreshed.loginPassword(registration.phone, "replacement456"),
    ).resolves.toEqual(registeredIdentity);
    await expect(
      refreshed.loginPassword(DEMO_ACCOUNT.username, DEMO_ACCOUNT.password),
    ).resolves.toEqual(builtinIdentity);
    await expect(
      refreshed.resetPassword(
        registration.phone,
        DEMO_ACCOUNT.code,
        "again1234",
      ),
    ).rejects.toMatchObject({ code: "CODE_NOT_ISSUED" });
    expect([...f.storage.data.values()].join("")).not.toContain(
      "replacement456",
    );
  });

  it("retains a changed built-in password and its aliases after refresh", async () => {
    const f = fixture();
    f.service.issueCode("reset", DEMO_ACCOUNT.phone);
    await f.service.resetPassword(
      DEMO_ACCOUNT.phone,
      DEMO_ACCOUNT.code,
      "changedDemo456",
    );
    await expect(
      f.refresh().loginPassword(DEMO_ACCOUNT.email, DEMO_ACCOUNT.password),
    ).rejects.toMatchObject({ code: "INVALID_CREDENTIALS" });
    for (const alias of [
      DEMO_ACCOUNT.email,
      DEMO_ACCOUNT.username,
      DEMO_ACCOUNT.phone,
    ])
      await expect(
        f.refresh().loginPassword(alias, "changedDemo456"),
      ).resolves.toEqual(builtinIdentity);
  });

  it("uses different salts for two accounts with the same password", async () => {
    const f = fixture();
    f.service.issueCode("register", registration.phone);
    await f.service.register(registration);
    f.advance(60_000);
    f.service.issueCode("register", "13700137000");
    await f.service.register({
      ...registration,
      username: "second_user",
      phone: "13700137000",
    });
    const saved = JSON.parse(f.storage.getItem("veyra.demo-auth.v2")!);
    expect(saved.accounts["user:demo_team"].salt).not.toBe(
      saved.accounts["user:second_user"].salt,
    );
    expect(saved.accounts["user:demo_team"].digest).not.toBe(
      saved.accounts["user:second_user"].digest,
    );
  });

  it("allows only one concurrent registration for a phone even with different usernames", async () => {
    const f = fixture();
    f.service.issueCode("register", registration.phone);
    const results = await Promise.allSettled([
      f.service.register(registration),
      f.service.register({ ...registration, username: "second_user" }),
    ]);
    expect(
      results.filter((result) => result.status === "fulfilled"),
    ).toHaveLength(1);
    const rejected = results.find((result) => result.status === "rejected");
    expect(rejected?.status === "rejected" && rejected.reason.code).toBe(
      "PHONE_EXISTS",
    );
    const saved = JSON.parse(f.storage.getItem("veyra.demo-auth.v2")!);
    expect(Object.values(saved.accounts)).toHaveLength(1);
  });

  it("migrates legacy salted passwords and preserves email-only accounts without inventing a phone", async () => {
    const f = fixture();
    const legacy = legacySnapshot(f.now());
    f.storage.setItem("veyra.demo-auth.v1", JSON.stringify(legacy));
    const migrated = f.refresh();
    expect(migrated.cooldownUntil()).toBe(legacy.resendAt);
    expect(
      migrated.challenge("phone-login", DEMO_ACCOUNT.phone)?.attemptsRemaining,
    ).toBe(3);
    expect(migrated.challenge("register", "13700137000")).toBeNull();
    await expect(
      migrated.loginPassword("LEGACY@EXAMPLE.COM", "legacyUser789"),
    ).resolves.toEqual({
      id: accountIdForLegacy({ email: "legacy@example.com" }),
      username: "legacy@example.com",
      email: "legacy@example.com",
      enterpriseName: "Legacy enterprise",
    });
    await expect(
      migrated.loginPassword(DEMO_ACCOUNT.username, "legacyDemo456"),
    ).resolves.toEqual(builtinIdentity);
    await expect(
      migrated.loginPassword(DEMO_ACCOUNT.username, DEMO_ACCOUNT.password),
    ).rejects.toMatchObject({ code: "INVALID_CREDENTIALS" });
    const saved = JSON.parse(f.storage.getItem("veyra.demo-auth.v2")!);
    expect(saved.version).toBe(2);
    expect(saved.accounts["legacy@example.com"].digest).toBe(
      legacy.accounts["legacy@example.com"].digest,
    );
    expect(saved.accounts["legacy@example.com"].salt).toBe(
      legacy.accounts["legacy@example.com"].salt,
    );
    expect(saved.accounts["legacy@example.com"]).not.toHaveProperty("phone");
    expect(saved.accounts[DEMO_ACCOUNT.email].digest).toBe(
      legacy.accounts[DEMO_ACCOUNT.email].digest,
    );
    expect(f.storage.getItem("veyra.demo-auth.v1")).toBe(
      JSON.stringify(legacy),
    );
    await expect(
      f.refresh().loginPassword("legacy@example.com", "legacyUser789"),
    ).resolves.toHaveProperty("email", "legacy@example.com");
    f.advance(60_000);
    expect(() => f.refresh().issueCode("reset", "13700137000")).toThrow(
      "ACCOUNT_NOT_FOUND",
    );
    expect(() => f.refresh().issueCode("reset", "legacy@example.com")).toThrow(
      "INVALID_PHONE",
    );
  });

  it("does not remove legacy lock or resend cooldown while migrating to phone registration", async () => {
    const f = fixture();
    const legacy = {
      ...legacySnapshot(f.now()),
      failures: 5,
      lockedUntil: f.now() + 900_000,
    };
    f.storage.setItem("veyra.demo-auth.v1", JSON.stringify(legacy));
    const migrated = f.refresh();
    expect(migrated.lockedUntil()).toBe(legacy.lockedUntil);
    expect(migrated.cooldownUntil()).toBe(legacy.resendAt);
    await expect(
      migrated.loginPassword(DEMO_ACCOUNT.username, "legacyDemo456"),
    ).rejects.toMatchObject({
      code: "TEMPORARILY_LOCKED",
      retryAt: legacy.lockedUntil,
    });
    expect(() => migrated.issueCode("register", registration.phone)).toThrow(
      "CODE_COOLDOWN",
    );
    f.advance(60_000);
    migrated.issueCode("register", registration.phone);
    const saved = JSON.parse(f.storage.getItem("veyra.demo-auth.v2")!);
    expect(saved.failures).toBe(5);
    expect(saved.lockedUntil).toBe(legacy.lockedUntil);
    expect(f.refresh().lockedUntil()).toBe(legacy.lockedUntil);
  });

  it("fails explicitly when session storage cannot persist an authentication result", async () => {
    const unavailable = new DemoAuthService({
      getItem: () => null,
      setItem: () => {
        throw new Error("Storage blocked");
      },
    });
    expect(() =>
      unavailable.issueCode("phone-login", DEMO_ACCOUNT.phone),
    ).toThrow("STORAGE_UNAVAILABLE");
    expect(unavailable.challenge("phone-login", DEMO_ACCOUNT.phone)).toBeNull();
    expect(unavailable.cooldownUntil()).toBe(0);
    const absent = new DemoAuthService(null);
    expect(() => absent.issueCode("phone-login", DEMO_ACCOUNT.phone)).toThrow(
      "STORAGE_UNAVAILABLE",
    );
    await expect(
      absent.loginPassword(DEMO_ACCOUNT.username, DEMO_ACCOUNT.password),
    ).rejects.toMatchObject({ code: "STORAGE_UNAVAILABLE" });
  });
});

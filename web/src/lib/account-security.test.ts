import { createHash } from "node:crypto";
import { describe, expect, it } from "vitest";
import { accountIdForLegacy } from "./account-identity";
import { DEMO_ACCOUNT, DemoAuthService, type DemoIdentity } from "./demo-auth";

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
    advance(ms = 60_000) {
      time += ms;
    },
    refresh() {
      return new DemoAuthService(storage, now);
    },
  };
}
const builtin = {
  accountId: DEMO_ACCOUNT.id,
  currentPassword: DEMO_ACCOUNT.password,
};
const code = DEMO_ACCOUNT.code;
async function register(
  f: ReturnType<typeof fixture>,
  username = "alice",
  phone = "13900139000",
) {
  f.service.issueCode("register", phone);
  const identity = await f.service.register({
    username,
    phone,
    password: "secret456",
    code,
  });
  f.advance();
  return identity;
}

// These test state transitions and the recovery/alias boundaries, not presentation text.
describe("local account security", () => {
  it("persists email verification independently of activity history and keeps it through password changes", async () => {
    const f = fixture();
    expect(f.service.identity(DEMO_ACCOUNT.id)?.emailVerified).not.toBe(true);
    const input = {
      ...builtin,
      purpose: "bind-email" as const,
      target: "verified@example.com",
    };
    await f.service.issueSecurityCode(input);
    expect(
      (await f.service.changeContact({ ...input, code })).emailVerified,
    ).toBe(true);
    const refreshed = f.refresh();
    expect(refreshed.identity(DEMO_ACCOUNT.id)).toMatchObject({
      email: input.target,
      emailVerified: true,
    });
    await refreshed.changePassword({
      ...builtin,
      newPassword: "updatedPassword456",
    });
    expect(f.refresh().identity(DEMO_ACCOUNT.id)?.emailVerified).toBe(true);
  });

  it("assigns immutable account IDs, migrates old identities consistently, and never invents missing credentials", async () => {
    const f = fixture();
    expect(f.service.identity(DEMO_ACCOUNT.id)?.id).toBe("account-demo");
    expect(f.service.identity("account-unknown")).toBeNull();
    await expect(
      f.service.verifyPassword("account-unknown", "password123"),
    ).rejects.toMatchObject({ code: "ACCOUNT_NOT_FOUND" });
    const a = await register(f);
    const b = await register(f, "bob", "13700137000");
    expect(a.id).not.toBe(b.id);
    expect(a.id).not.toContain(a.username);
    expect(f.refresh().identity(a.id)).toEqual(a);
    expect(accountIdForLegacy({ username: "Ａｌｉｃｅ" })).toBe(
      accountIdForLegacy({ username: "alice" }),
    );
    expect(accountIdForLegacy({ email: DEMO_ACCOUNT.email })).toBe(
      DEMO_ACCOUNT.id,
    );
    const saved = JSON.parse(f.storage.getItem("veyra.demo-auth.v2")!);
    delete saved.accounts["user:alice"].id;
    f.storage.setItem("veyra.demo-auth.v2", JSON.stringify(saved));
    const migrated = await f.refresh().loginPassword("alice", "secret456");
    expect(migrated.id).toBe(accountIdForLegacy({ username: "alice" }));
    expect((await f.refresh().loginPassword("alice", "secret456")).id).toBe(
      migrated.id,
    );
  });

  it("reauthenticates the exact account and rejects incorrect passwords without modifying contacts", async () => {
    const f = fixture();
    const alice = await register(f);
    await expect(
      f.service.verifyPassword(alice.id, DEMO_ACCOUNT.password),
    ).rejects.toMatchObject({ code: "INVALID_CREDENTIALS" });
    await expect(
      f.service.issueSecurityCode({
        ...builtin,
        currentPassword: "incorrect123",
        purpose: "bind-email",
        target: "demo@example.com",
      }),
    ).rejects.toMatchObject({ code: "INVALID_CREDENTIALS" });
    expect(f.service.identity(DEMO_ACCOUNT.id)?.email).toBe(DEMO_ACCOUNT.email);
    expect(
      f.service.challenge("bind-email", "demo@example.com", DEMO_ACCOUNT.id),
    ).toBeNull();
    expect(await f.service.verifyPassword(alice.id, "secret456")).toEqual(
      alice,
    );
  });

  it("requires a verified target, scopes codes by account and purpose, and persists one-time binding", async () => {
    const f = fixture();
    const alice = await register(f);
    const input = {
      ...builtin,
      purpose: "bind-email" as const,
      target: " New-Demo@Example.com ",
    };
    const challenge = await f.service.issueSecurityCode(input);
    expect(challenge).toMatchObject({
      accountId: DEMO_ACCOUNT.id,
      target: "new-demo@example.com",
      code,
    });
    expect(f.service.identity(DEMO_ACCOUNT.id)?.email).toBe(DEMO_ACCOUNT.email);
    expect(
      f.service.challenge("bind-email", input.target, alice.id),
    ).toBeNull();
    await expect(
      f.service.changeContact({ ...input, target: "other@example.com", code }),
    ).rejects.toMatchObject({ code: "CODE_NOT_ISSUED" });
    await expect(
      f.service.changeContact({
        ...input,
        accountId: alice.id,
        currentPassword: "secret456",
        code,
      }),
    ).rejects.toMatchObject({ code: "CODE_NOT_ISSUED" });
    await expect(
      f.service.changeContact({
        ...builtin,
        purpose: "change-phone",
        target: "13700137000",
        code,
      }),
    ).rejects.toMatchObject({ code: "CODE_NOT_ISSUED" });
    await expect(
      f.service.changeContact({ ...input, code: "000000" }),
    ).rejects.toMatchObject({ code: "CODE_INVALID" });
    const changed = await f.refresh().changeContact({ ...input, code });
    expect(changed).toMatchObject({
      id: DEMO_ACCOUNT.id,
      username: DEMO_ACCOUNT.username,
      email: "new-demo@example.com",
    });
    const refreshed = f.refresh();
    expect(
      refreshed.challenge("bind-email", input.target, DEMO_ACCOUNT.id),
    ).toBeNull();
    await expect(
      refreshed.loginPassword(DEMO_ACCOUNT.email, DEMO_ACCOUNT.password),
    ).rejects.toMatchObject({ code: "INVALID_CREDENTIALS" });
    expect(
      await refreshed.loginPassword(
        "NEW-DEMO@EXAMPLE.COM",
        DEMO_ACCOUNT.password,
      ),
    ).toEqual(changed);
    await expect(
      refreshed.changeContact({ ...input, target: "other@example.com", code }),
    ).rejects.toMatchObject({ code: "CODE_NOT_ISSUED" });
  });

  it("changes built-in phone persistently, removes the old login/recovery alias, and keeps the account ID", async () => {
    const f = fixture();
    f.service.issueCode("reset", DEMO_ACCOUNT.phone);
    f.advance();
    const input = {
      ...builtin,
      purpose: "change-phone" as const,
      target: "13900139000",
    };
    await f.service.issueSecurityCode(input);
    expect(f.service.identity(DEMO_ACCOUNT.id)?.phone).toBe(DEMO_ACCOUNT.phone);
    const changed = await f.service.changeContact({ ...input, code });
    expect(changed).toMatchObject({
      id: DEMO_ACCOUNT.id,
      phone: "+8613900139000",
      email: DEMO_ACCOUNT.email,
    });
    expect(f.service.challenge("reset", DEMO_ACCOUNT.phone)).toBeNull();
    const refreshed = f.refresh();
    await expect(
      refreshed.loginPassword(DEMO_ACCOUNT.phone, DEMO_ACCOUNT.password),
    ).rejects.toMatchObject({ code: "INVALID_CREDENTIALS" });
    expect(
      await refreshed.loginPassword(input.target, DEMO_ACCOUNT.password),
    ).toEqual(changed);
    f.advance();
    expect(() => refreshed.issueCode("reset", DEMO_ACCOUNT.phone)).toThrow(
      "ACCOUNT_NOT_FOUND",
    );
    refreshed.issueCode("phone-login", input.target);
    expect(refreshed.loginPhone(input.target, code)).toEqual(changed);
  });

  it("rejects unchanged or already-owned contacts and rechecks ownership at submission", async () => {
    const f = fixture();
    const alice = await register(f);
    await expect(
      f.service.issueSecurityCode({
        ...builtin,
        purpose: "bind-email",
        target: DEMO_ACCOUNT.email,
      }),
    ).rejects.toMatchObject({ code: "CONTACT_UNCHANGED" });
    await expect(
      f.service.issueSecurityCode({
        ...builtin,
        purpose: "change-phone",
        target: alice.phone!,
      }),
    ).rejects.toMatchObject({ code: "PHONE_EXISTS" });
    const phone = "13700137000";
    await f.service.issueSecurityCode({
      ...builtin,
      purpose: "change-phone",
      target: phone,
    });
    f.advance();
    await register(f, "bob", phone);
    await expect(
      f.service.changeContact({
        ...builtin,
        purpose: "change-phone",
        target: phone,
        code,
      }),
    ).rejects.toMatchObject({ code: "PHONE_EXISTS" });
    const email = "alice@example.com";
    const aliceInput = {
      accountId: alice.id,
      currentPassword: "secret456",
      purpose: "bind-email" as const,
      target: email,
    };
    await f.service.issueSecurityCode(aliceInput);
    await f.service.changeContact({ ...aliceInput, code });
    f.advance();
    await expect(
      f.service.issueSecurityCode({
        ...builtin,
        purpose: "bind-email",
        target: email,
      }),
    ).rejects.toMatchObject({ code: "EMAIL_EXISTS" });
  });

  it("applies the same persistent cooldown, expiry, and attempt limits to email verification", async () => {
    const f = fixture();
    const input = {
      ...builtin,
      purpose: "bind-email" as const,
      target: "hello@example.com",
    };
    const issued = await f.service.issueSecurityCode(input);
    await expect(
      f.service.issueSecurityCode({ ...input, target: "other@example.com" }),
    ).rejects.toMatchObject({
      code: "CODE_COOLDOWN",
      retryAt: issued.resendAt,
    });
    expect(() =>
      f.refresh().issueCode("phone-login", DEMO_ACCOUNT.phone),
    ).toThrow("CODE_COOLDOWN");
    for (let n = 0; n < 4; n++)
      await expect(
        f.service.changeContact({ ...input, code: "000000" }),
      ).rejects.toMatchObject({ code: "CODE_INVALID" });
    await expect(
      f.service.changeContact({ ...input, code: "000000" }),
    ).rejects.toMatchObject({ code: "CODE_EXHAUSTED" });
    await expect(
      f.refresh().changeContact({ ...input, code }),
    ).rejects.toMatchObject({ code: "CODE_EXHAUSTED" });
    f.advance();
    await f.service.issueSecurityCode(input);
    f.advance(600_000);
    await expect(
      f.refresh().changeContact({ ...input, code }),
    ).rejects.toMatchObject({ code: "CODE_EXPIRED" });
  });

  it("changes passwords by current credentials and invalidates every pending account challenge", async () => {
    const f = fixture();
    f.service.issueCode("reset", DEMO_ACCOUNT.phone);
    f.advance();
    f.service.issueCode("phone-login", DEMO_ACCOUNT.phone);
    f.advance();
    const email = {
      ...builtin,
      purpose: "bind-email" as const,
      target: "new@example.com",
    };
    await f.service.issueSecurityCode(email);
    await expect(
      f.service.changePassword({
        ...builtin,
        currentPassword: "incorrect123",
        newPassword: "replacement456",
      }),
    ).rejects.toMatchObject({ code: "INVALID_CREDENTIALS" });
    await expect(
      f.service.changePassword({
        ...builtin,
        newPassword: DEMO_ACCOUNT.password,
      }),
    ).rejects.toMatchObject({ code: "PASSWORD_UNCHANGED" });
    await expect(
      f.service.changePassword({ ...builtin, newPassword: "weak" }),
    ).rejects.toMatchObject({ code: "WEAK_PASSWORD" });
    const changed = await f.service.changePassword({
      ...builtin,
      newPassword: "replacement456",
    });
    expect(changed.id).toBe(DEMO_ACCOUNT.id);
    expect(f.service.challenge("reset", DEMO_ACCOUNT.phone)).toBeNull();
    expect(f.service.challenge("phone-login", DEMO_ACCOUNT.phone)).toBeNull();
    expect(
      f.service.challenge("bind-email", email.target, DEMO_ACCOUNT.id),
    ).toBeNull();
    const refreshed = f.refresh();
    await expect(
      refreshed.loginPassword(DEMO_ACCOUNT.username, DEMO_ACCOUNT.password),
    ).rejects.toMatchObject({ code: "INVALID_CREDENTIALS" });
    expect(
      await refreshed.loginPassword(DEMO_ACCOUNT.username, "replacement456"),
    ).toEqual(changed);
    await expect(
      refreshed.resetPassword(DEMO_ACCOUNT.phone, code, "obsolete123"),
    ).rejects.toMatchObject({ code: "CODE_NOT_ISSUED" });
    const persisted = [...f.storage.data.values()].join("");
    expect(persisted).not.toContain("replacement456");
    expect(persisted).not.toContain(DEMO_ACCOUNT.password);
  });

  it("shares password failure lock with account mutations and permits phone recovery", async () => {
    const f = fixture();
    for (let n = 0; n < 5; n++)
      await f.service
        .verifyPassword(DEMO_ACCOUNT.id, "wrong123")
        .catch(() => {});
    await expect(
      f.refresh().changePassword({ ...builtin, newPassword: "replacement456" }),
    ).rejects.toMatchObject({ code: "TEMPORARILY_LOCKED" });
    await expect(
      f.service.issueSecurityCode({
        ...builtin,
        purpose: "bind-email",
        target: "test@example.com",
      }),
    ).rejects.toMatchObject({ code: "TEMPORARILY_LOCKED" });
    f.service.issueCode("phone-login", DEMO_ACCOUNT.phone);
    f.service.loginPhone(DEMO_ACCOUNT.phone, code);
    expect(
      await f.service.verifyPassword(DEMO_ACCOUNT.id, DEMO_ACCOUNT.password),
    ).toMatchObject({ id: DEMO_ACCOUNT.id });
  });

  it("rejects concurrent stale-password replacement rather than overwriting the first change", async () => {
    const f = fixture();
    await f.service.verifyPassword(DEMO_ACCOUNT.id, DEMO_ACCOUNT.password);
    const passwords = ["firstPassword123", "secondPassword456"];
    const results = await Promise.allSettled(
      passwords.map((newPassword) =>
        f.service.changePassword({ ...builtin, newPassword }),
      ),
    );
    expect(
      results.filter((result) => result.status === "fulfilled"),
    ).toHaveLength(1);
    const losing = results.find((result) => result.status === "rejected");
    expect(losing?.status === "rejected" && losing.reason.code).toBe(
      "AUTH_STATE_CHANGED",
    );
    const winning =
      passwords[results.findIndex((result) => result.status === "fulfilled")];
    expect(
      await f.refresh().loginPassword(DEMO_ACCOUNT.username, winning),
    ).toMatchObject({ id: DEMO_ACCOUNT.id });
  });

  it("retains the original legacy ID when an email-only account replaces its login alias", async () => {
    const f = fixture();
    const salt = "12".repeat(16);
    const legacyEmail = "legacy@example.com";
    const identity: DemoIdentity = {
      id: accountIdForLegacy({ email: legacyEmail }),
      username: legacyEmail,
      email: legacyEmail,
    };
    f.storage.setItem(
      "veyra.demo-auth.v1",
      JSON.stringify({
        version: 1,
        accounts: {
          [legacyEmail]: {
            email: legacyEmail,
            enterpriseName: "Legacy name",
            salt,
            digest: createHash("sha256")
              .update(`${salt}:legacyPassword123`)
              .digest("hex"),
          },
        },
        challenges: {},
        resendAt: 0,
        failures: 0,
        lockedUntil: 0,
      }),
    );
    const service = f.refresh();
    const input = {
      accountId: identity.id,
      currentPassword: "legacyPassword123",
      purpose: "bind-email" as const,
      target: "replacement@example.com",
    };
    await service.issueSecurityCode(input);
    const changed = await service.changeContact({ ...input, code });
    expect(changed).toMatchObject({
      id: identity.id,
      username: input.target,
      email: input.target,
    });
    await expect(
      f.refresh().loginPassword(legacyEmail, input.currentPassword),
    ).rejects.toMatchObject({ code: "INVALID_CREDENTIALS" });
    expect(
      await f.refresh().loginPassword(input.target, input.currentPassword),
    ).toEqual(changed);
    expect(changed.id).not.toBe(accountIdForLegacy(changed));
  });

  it("fails without a successful state mutation if storage cannot save a security change", async () => {
    const f = fixture();
    await f.service.verifyPassword(DEMO_ACCOUNT.id, DEMO_ACCOUNT.password);
    const blocked = new DemoAuthService({
      getItem: (key) => f.storage.getItem(key),
      setItem: () => {
        throw new Error("blocked");
      },
    });
    await expect(
      blocked.changePassword({ ...builtin, newPassword: "replacement456" }),
    ).rejects.toMatchObject({ code: "STORAGE_UNAVAILABLE" });
    expect(
      await f
        .refresh()
        .loginPassword(DEMO_ACCOUNT.username, DEMO_ACCOUNT.password),
    ).toMatchObject({ id: DEMO_ACCOUNT.id });
  });
});

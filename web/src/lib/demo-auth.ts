import { accountIdForLegacy, DEMO_ACCOUNT_PROFILE } from "./account-identity";

/** Local interaction demo only. This client-side state is not production authentication. */
export const DEMO_ACCOUNT = {
  ...DEMO_ACCOUNT_PROFILE,
  password: "securepass123",
  code: "829104",
} as const;

export type DemoPublicAuthPurpose = "phone-login" | "register" | "reset";
export type DemoSecurityPurpose = "bind-email" | "change-phone";
export type DemoAuthPurpose = DemoPublicAuthPurpose | DemoSecurityPurpose;
export type DemoAuthErrorCode =
  | "INVALID_EMAIL"
  | "INVALID_PHONE"
  | "INVALID_USERNAME"
  | "INVALID_CREDENTIALS"
  | "ACCOUNT_NOT_FOUND"
  | "ACCOUNT_EXISTS"
  | "USERNAME_EXISTS"
  | "PHONE_EXISTS"
  | "EMAIL_EXISTS"
  | "CONTACT_UNCHANGED"
  | "PASSWORD_UNCHANGED"
  | "AUTH_STATE_CHANGED"
  | "ENTERPRISE_REQUIRED"
  | "WEAK_PASSWORD"
  | "CODE_REQUIRED"
  | "CODE_NOT_ISSUED"
  | "CODE_INVALID"
  | "CODE_EXPIRED"
  | "CODE_EXHAUSTED"
  | "CODE_COOLDOWN"
  | "TEMPORARILY_LOCKED"
  | "STORAGE_UNAVAILABLE"
  | "CRYPTO_UNAVAILABLE";

export class DemoAuthError extends Error {
  constructor(
    public readonly code: DemoAuthErrorCode,
    public readonly retryAt?: number,
  ) {
    super(code);
    this.name = "DemoAuthError";
  }
}

export interface DemoIdentity {
  id: string;
  username: string;
  phone?: string;
  email?: string;
  emailVerified?: boolean;
  enterpriseName?: string;
}

export interface DemoChallenge {
  purpose: DemoAuthPurpose;
  target: string;
  accountId?: string;
  code: string;
  expiresAt: number;
  resendAt: number;
  attemptsRemaining: number;
}

type DemoStorage = Pick<Storage, "getItem" | "setItem">;
interface Credential extends DemoIdentity {
  salt: string;
  digest: string;
}
interface AccountReference {
  key: string;
  identity: DemoIdentity;
  credential?: Credential;
}
interface DemoAuthState {
  version: 2;
  accounts: Record<string, Credential>;
  challenges: Partial<Record<DemoAuthPurpose, DemoChallenge>>;
  resendAt: number;
  failures: number;
  lockedUntil: number;
}

const STORAGE_KEY = "veyra.demo-auth.v2";
const LEGACY_STORAGE_KEY = "veyra.demo-auth.v1";
const BUILTIN_KEY = DEMO_ACCOUNT.email;
const COOLDOWN_MS = 60_000;
const CODE_LIFETIME_MS = 10 * 60_000;
const PASSWORD_LOCK_MS = 15 * 60_000;
const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
const USERNAME_PATTERN = /^\p{L}[\p{L}\p{N}_-]+$/u;
const purposes: DemoAuthPurpose[] = [
  "phone-login",
  "register",
  "reset",
  "bind-email",
  "change-phone",
];
const builtinIdentity: DemoIdentity = {
  id: "account-demo",
  username: DEMO_ACCOUNT.username,
  phone: DEMO_ACCOUNT.phone,
  email: DEMO_ACCOUNT.email,
  enterpriseName: DEMO_ACCOUNT.enterpriseName,
};
const emptyState = (): DemoAuthState => ({
  version: 2,
  accounts: {},
  challenges: {},
  resendAt: 0,
  failures: 0,
  lockedUntil: 0,
});

function sessionStorageForDemo(): DemoStorage | null {
  try {
    return typeof window === "undefined" ? null : window.sessionStorage;
  } catch {
    return null;
  }
}

export function normalizeDemoEmail(email: string): string {
  const normalized = email.trim().toLowerCase();
  if (!EMAIL_PATTERN.test(normalized)) throw new DemoAuthError("INVALID_EMAIL");
  return normalized;
}

export function normalizeDemoPhone(phone: string): string {
  const compact = phone
    .normalize("NFKC")
    .trim()
    .replace(/[\s()-]/g, "");
  const national = compact.replace(/^(?:\+86|0086)/, "");
  if (!/^1[3-9]\d{9}$/.test(national)) throw new DemoAuthError("INVALID_PHONE");
  return `+86${national}`;
}

export function normalizeDemoUsername(username: string): string {
  const normalized = username.normalize("NFKC").trim().toLowerCase();
  if (!USERNAME_PATTERN.test(normalized) || [...normalized].length > 24)
    throw new DemoAuthError("INVALID_USERNAME");
  return normalized;
}

function identityOf(credential: DemoIdentity): DemoIdentity {
  return {
    id: credential.id,
    username: credential.username,
    ...(credential.phone ? { phone: credential.phone } : {}),
    ...(credential.email ? { email: credential.email } : {}),
    ...(credential.emailVerified === true ? { emailVerified: true } : {}),
    ...(credential.enterpriseName
      ? { enterpriseName: credential.enterpriseName }
      : {}),
  };
}

function normalizeTarget(purpose: DemoAuthPurpose, target: string) {
  return purpose === "bind-email"
    ? normalizeDemoEmail(target)
    : normalizeDemoPhone(target);
}

function validTimestamp(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value) && value >= 0;
}

function object(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === "object" && !Array.isArray(value);
}

/** Migrate the first demo format without replacing its hashes, lock, or cooldown. */
function parseState(saved: unknown): DemoAuthState | null {
  if (
    !object(saved) ||
    (saved.version !== 1 && saved.version !== 2) ||
    !object(saved.accounts) ||
    !object(saved.challenges) ||
    !validTimestamp(saved.resendAt) ||
    !validTimestamp(saved.lockedUntil) ||
    typeof saved.failures !== "number" ||
    !Number.isInteger(saved.failures) ||
    saved.failures < 0 ||
    saved.failures > 5
  )
    return null;
  const state = emptyState();
  for (const [key, value] of Object.entries(saved.accounts)) {
    if (
      !object(value) ||
      typeof value.salt !== "string" ||
      typeof value.digest !== "string" ||
      !/^[a-f0-9]{32}$/.test(value.salt) ||
      !/^[a-f0-9]{64}$/.test(value.digest)
    )
      continue;
    let identity: DemoIdentity;
    if (saved.version === 1) {
      if (
        !EMAIL_PATTERN.test(key) ||
        value.email !== key ||
        typeof value.enterpriseName !== "string"
      )
        continue;
      // Legacy email accounts stay email-only. Never infer a phone binding or username.
      identity =
        key === BUILTIN_KEY
          ? builtinIdentity
          : {
              id: accountIdForLegacy({ email: key }),
              username: key,
              email: key,
              enterpriseName: value.enterpriseName,
            };
    } else {
      if (typeof value.username !== "string") continue;
      const email =
        typeof value.email === "string" && EMAIL_PATTERN.test(value.email)
          ? value.email
          : undefined;
      let username: string;
      try {
        // Email is retained as the display identity for existing email-only accounts.
        username =
          email === value.username
            ? email
            : normalizeDemoUsername(value.username);
      } catch {
        continue;
      }
      let phone: string | undefined;
      try {
        phone =
          typeof value.phone === "string"
            ? normalizeDemoPhone(value.phone)
            : undefined;
      } catch {
        continue;
      }
      if (!phone && !email) continue;
      identity = {
        id:
          key === BUILTIN_KEY
            ? "account-demo"
            : typeof value.id === "string" &&
                /^account-[a-z0-9-]+$/.test(value.id)
              ? value.id
              : accountIdForLegacy({ username, email }),
        username,
        ...(phone ? { phone } : {}),
        ...(email ? { email } : {}),
        ...(email && value.emailVerified === true
          ? { emailVerified: true }
          : {}),
        ...(typeof value.enterpriseName === "string"
          ? { enterpriseName: value.enterpriseName }
          : {}),
      };
    }
    state.accounts[key] = {
      ...identity,
      salt: value.salt,
      digest: value.digest,
    };
  }
  for (const purpose of purposes) {
    const challenge = saved.challenges[purpose];
    if (
      !object(challenge) ||
      challenge.purpose !== purpose ||
      typeof challenge.target !== "string" ||
      challenge.code !== DEMO_ACCOUNT.code ||
      !validTimestamp(challenge.expiresAt) ||
      !validTimestamp(challenge.resendAt) ||
      typeof challenge.attemptsRemaining !== "number" ||
      !Number.isInteger(challenge.attemptsRemaining) ||
      challenge.attemptsRemaining < 0 ||
      challenge.attemptsRemaining > 5
    )
      continue;
    try {
      // Old email challenges cannot authorize a phone operation. Their global cooldown remains.
      const accountId =
        typeof challenge.accountId === "string"
          ? challenge.accountId
          : undefined;
      if (
        (purpose === "bind-email" || purpose === "change-phone") &&
        !accountId
      )
        continue;
      state.challenges[purpose] = {
        purpose,
        target: normalizeTarget(purpose, challenge.target),
        ...(accountId ? { accountId } : {}),
        code: challenge.code,
        expiresAt: challenge.expiresAt,
        resendAt: challenge.resendAt,
        attemptsRemaining: challenge.attemptsRemaining,
      };
    } catch {
      continue;
    }
  }
  state.resendAt = saved.resendAt;
  state.lockedUntil = saved.lockedUntil;
  state.failures = saved.failures;
  return state;
}

function readState(storage: DemoStorage | null): DemoAuthState {
  for (const key of [STORAGE_KEY, LEGACY_STORAGE_KEY]) {
    try {
      const raw = storage?.getItem(key);
      const parsed = raw ? parseState(JSON.parse(raw)) : null;
      if (parsed) return parsed;
    } catch {
      // Try the retained legacy state if the current snapshot cannot be read.
    }
  }
  return emptyState();
}

function assertStrongPassword(password: string) {
  if (password.length < 8 || !/[A-Za-z]/.test(password) || !/\d/.test(password))
    throw new DemoAuthError("WEAK_PASSWORD");
}

function webCrypto(): Crypto {
  if (!globalThis.crypto?.subtle) throw new DemoAuthError("CRYPTO_UNAVAILABLE");
  return globalThis.crypto;
}

async function passwordDigest(password: string, salt: string): Promise<string> {
  // A salted digest avoids storing plaintext. Client-side hashing is not an auth boundary.
  const bytes = new TextEncoder().encode(`${salt}:${password}`);
  const digest = await webCrypto().subtle.digest("SHA-256", bytes);
  return Array.from(new Uint8Array(digest), (byte) =>
    byte.toString(16).padStart(2, "0"),
  ).join("");
}

async function credentialFor(
  identity: DemoIdentity,
  password: string,
): Promise<Credential> {
  const salt = Array.from(
    webCrypto().getRandomValues(new Uint8Array(16)),
    (byte) => byte.toString(16).padStart(2, "0"),
  ).join("");
  return {
    ...identityOf(identity),
    salt,
    digest: await passwordDigest(password, salt),
  };
}

export class DemoAuthService {
  private state: DemoAuthState;
  private builtinPending: Promise<void> | null = null;

  constructor(
    private readonly storage: DemoStorage | null = sessionStorageForDemo(),
    private readonly now: () => number = Date.now,
  ) {
    this.state = readState(storage);
  }

  /** Absolute deadline, shared across targets and purposes to prevent resend bypass. */
  cooldownUntil(): number {
    return this.state.resendAt > this.now() ? this.state.resendAt : 0;
  }

  lockedUntil(): number {
    return this.state.lockedUntil > this.now() ? this.state.lockedUntil : 0;
  }

  identity(accountId: string): DemoIdentity | null {
    const reference = this.findAccount((identity) => identity.id === accountId);
    return reference ? { ...reference.identity } : null;
  }

  challenge(
    purpose: DemoAuthPurpose,
    target: string,
    accountId?: string,
  ): DemoChallenge | null {
    let normalized: string;
    try {
      normalized = normalizeTarget(purpose, target);
    } catch {
      return null;
    }
    const challenge = this.state.challenges[purpose];
    return challenge &&
      challenge.target === normalized &&
      (!accountId || challenge.accountId === accountId)
      ? { ...challenge }
      : null;
  }

  issueCode(purpose: DemoPublicAuthPurpose, target: string): DemoChallenge {
    const normalized = normalizeDemoPhone(target);
    if (this.cooldownUntil())
      throw new DemoAuthError("CODE_COOLDOWN", this.state.resendAt);
    const account = this.findAccount(
      (identity) => identity.phone === normalized,
    );
    if (purpose === "register" && account)
      throw new DemoAuthError("PHONE_EXISTS");
    if (purpose !== "register" && !account)
      throw new DemoAuthError("ACCOUNT_NOT_FOUND");
    const issuedAt = this.now();
    const challenge: DemoChallenge = {
      purpose,
      target: normalized,
      ...(account ? { accountId: account.identity.id } : {}),
      code: DEMO_ACCOUNT.code,
      expiresAt: issuedAt + CODE_LIFETIME_MS,
      resendAt: issuedAt + COOLDOWN_MS,
      attemptsRemaining: 5,
    };
    this.commit({
      ...this.state,
      challenges: { ...this.state.challenges, [purpose]: challenge },
      resendAt: challenge.resendAt,
    });
    return { ...challenge };
  }

  async loginPassword(
    account: string,
    password: string,
  ): Promise<DemoIdentity> {
    this.assertUnlocked();
    await this.ensureBuiltinCredential();
    this.assertUnlocked();
    const reference = this.findByAlias(account);
    const credential = reference?.credential;
    const digest = credential
      ? await passwordDigest(password, credential.salt)
      : null;
    if (reference) this.assertCredentialUnchanged(reference);
    if (!credential || digest !== credential.digest)
      return this.authenticationFailure("INVALID_CREDENTIALS");
    this.assertUnlocked();
    this.succeed();
    return identityOf(credential);
  }

  loginPhone(phone: string, code: string): DemoIdentity {
    const normalized = normalizeDemoPhone(phone);
    const account = this.findAccount(
      (identity) => identity.phone === normalized,
    );
    if (!account) throw new DemoAuthError("ACCOUNT_NOT_FOUND");
    this.verifyCode("phone-login", normalized, code);
    this.succeed("phone-login");
    return { ...account.identity };
  }

  async register(input: {
    username: string;
    phone: string;
    password: string;
    code: string;
  }): Promise<DemoIdentity> {
    const username = normalizeDemoUsername(input.username);
    const phone = normalizeDemoPhone(input.phone);
    assertStrongPassword(input.password);
    this.assertUniqueRegistration(username, phone);
    this.verifyCode("register", phone, input.code);
    const identity: DemoIdentity = {
      id: `account-${webCrypto().randomUUID()}`,
      username,
      phone,
    };
    const credential = await credentialFor(identity, input.password);
    // Hashing yields: recheck both uniqueness and one-time authorization before committing.
    this.assertUniqueRegistration(username, phone);
    this.verifyCode("register", phone, input.code);
    this.succeed("register", {
      ...this.state.accounts,
      [`user:${username}`]: credential,
    });
    return identity;
  }

  async resetPassword(
    phone: string,
    code: string,
    password: string,
  ): Promise<void> {
    const normalized = normalizeDemoPhone(phone);
    assertStrongPassword(password);
    const account = this.findAccount(
      (identity) => identity.phone === normalized,
    );
    if (!account) throw new DemoAuthError("ACCOUNT_NOT_FOUND");
    this.verifyCode("reset", normalized, code);
    const credential = await credentialFor(account.identity, password);
    this.assertCredentialUnchanged(account);
    this.verifyCode("reset", normalized, code);
    this.commitSecurityChange(account, credential);
  }

  async verifyPassword(
    accountId: string,
    password: string,
  ): Promise<DemoIdentity> {
    const account = await this.verifyCurrentCredential(accountId, password);
    this.assertCredentialUnchanged(account);
    this.succeed();
    return { ...account.identity };
  }

  async changePassword(input: {
    accountId: string;
    currentPassword: string;
    newPassword: string;
  }): Promise<DemoIdentity> {
    assertStrongPassword(input.newPassword);
    if (input.currentPassword === input.newPassword)
      throw new DemoAuthError("PASSWORD_UNCHANGED");
    const account = await this.verifyCurrentCredential(
      input.accountId,
      input.currentPassword,
    );
    const credential = await credentialFor(account.identity, input.newPassword);
    this.assertCredentialUnchanged(account);
    this.commitSecurityChange(account, credential);
    return identityOf(credential);
  }

  async issueSecurityCode(input: {
    accountId: string;
    currentPassword: string;
    purpose: DemoSecurityPurpose;
    target: string;
  }): Promise<DemoChallenge> {
    const target = normalizeTarget(input.purpose, input.target);
    const account = await this.verifyCurrentCredential(
      input.accountId,
      input.currentPassword,
    );
    this.assertContactAvailable(account, input.purpose, target);
    if (this.cooldownUntil())
      throw new DemoAuthError("CODE_COOLDOWN", this.state.resendAt);
    const issuedAt = this.now();
    const challenge: DemoChallenge = {
      purpose: input.purpose,
      target,
      accountId: account.identity.id,
      code: DEMO_ACCOUNT.code,
      expiresAt: issuedAt + CODE_LIFETIME_MS,
      resendAt: issuedAt + COOLDOWN_MS,
      attemptsRemaining: 5,
    };
    this.assertCredentialUnchanged(account);
    this.commit({
      ...this.state,
      challenges: { ...this.state.challenges, [input.purpose]: challenge },
      resendAt: challenge.resendAt,
      failures: 0,
      lockedUntil: 0,
    });
    return { ...challenge };
  }

  async changeContact(input: {
    accountId: string;
    currentPassword: string;
    purpose: DemoSecurityPurpose;
    target: string;
    code: string;
  }): Promise<DemoIdentity> {
    const target = normalizeTarget(input.purpose, input.target);
    const account = await this.verifyCurrentCredential(
      input.accountId,
      input.currentPassword,
    );
    this.assertContactAvailable(account, input.purpose, target);
    this.verifyCode(input.purpose, target, input.code, input.accountId);
    const identity: DemoIdentity =
      input.purpose === "bind-email"
        ? {
            ...account.identity,
            email: target,
            emailVerified: true,
            // Email-only legacy accounts use their new email as their login identity.
            ...(account.identity.username === account.identity.email
              ? { username: target }
              : {}),
          }
        : { ...account.identity, phone: target };
    const credential = { ...account.credential!, ...identity };
    this.commitSecurityChange(account, credential);
    return identityOf(credential);
  }

  private async verifyCurrentCredential(
    accountId: string,
    password: string,
  ): Promise<AccountReference> {
    this.assertUnlocked();
    await this.ensureBuiltinCredential();
    this.assertUnlocked();
    const account = this.findAccount((identity) => identity.id === accountId);
    if (!account?.credential) throw new DemoAuthError("ACCOUNT_NOT_FOUND");
    const digest = await passwordDigest(password, account.credential.salt);
    this.assertCredentialUnchanged(account);
    if (digest !== account.credential.digest)
      return this.authenticationFailure("INVALID_CREDENTIALS");
    this.assertUnlocked();
    return account;
  }

  private assertCredentialUnchanged(account: AccountReference) {
    if (this.state.accounts[account.key] !== account.credential)
      throw new DemoAuthError("AUTH_STATE_CHANGED");
  }

  private assertContactAvailable(
    account: AccountReference,
    purpose: DemoSecurityPurpose,
    target: string,
  ) {
    const field = purpose === "bind-email" ? "email" : "phone";
    if (account.identity[field] === target)
      throw new DemoAuthError("CONTACT_UNCHANGED");
    if (
      this.findAccount(
        (identity) =>
          identity.id !== account.identity.id && identity[field] === target,
      )
    )
      throw new DemoAuthError(
        field === "email" ? "EMAIL_EXISTS" : "PHONE_EXISTS",
      );
  }

  private commitSecurityChange(
    account: AccountReference,
    credential: Credential,
  ) {
    this.assertCredentialUnchanged(account);
    const challenges = { ...this.state.challenges };
    for (const purpose of purposes) {
      const challenge = challenges[purpose];
      if (
        challenge &&
        (challenge.accountId === account.identity.id ||
          (purpose !== "register" &&
            !challenge.accountId &&
            challenge.target === account.identity.phone))
      )
        delete challenges[purpose];
    }
    this.commit({
      ...this.state,
      accounts: { ...this.state.accounts, [account.key]: credential },
      challenges,
      failures: 0,
      lockedUntil: 0,
    });
  }

  private findAccount(
    match: (identity: DemoIdentity) => boolean,
  ): AccountReference | undefined {
    for (const [key, credential] of Object.entries(this.state.accounts)) {
      if (match(credential))
        return { key, identity: identityOf(credential), credential };
    }
    if (
      !Object.hasOwn(this.state.accounts, BUILTIN_KEY) &&
      match(builtinIdentity)
    )
      return { key: BUILTIN_KEY, identity: { ...builtinIdentity } };
    return undefined;
  }

  private findByAlias(account: string): AccountReference | undefined {
    const alias = account.normalize("NFKC").trim().toLowerCase();
    try {
      const phone = normalizeDemoPhone(alias);
      return this.findAccount((identity) => identity.phone === phone);
    } catch {
      return this.findAccount(
        (identity) =>
          identity.username.normalize("NFKC").toLowerCase() === alias ||
          identity.email?.toLowerCase() === alias,
      );
    }
  }

  private assertUniqueRegistration(username: string, phone: string) {
    if (
      this.findAccount(
        (identity) =>
          identity.username.normalize("NFKC").toLowerCase() === username,
      )
    )
      throw new DemoAuthError("USERNAME_EXISTS");
    if (this.findAccount((identity) => identity.phone === phone))
      throw new DemoAuthError("PHONE_EXISTS");
  }

  private commit(next: DemoAuthState) {
    if (!this.storage) throw new DemoAuthError("STORAGE_UNAVAILABLE");
    try {
      this.storage.setItem(STORAGE_KEY, JSON.stringify(next));
    } catch {
      throw new DemoAuthError("STORAGE_UNAVAILABLE");
    }
    this.state = next;
  }

  private async ensureBuiltinCredential(): Promise<void> {
    if (this.state.accounts[BUILTIN_KEY]) return;
    if (!this.builtinPending) {
      this.builtinPending = credentialFor(
        builtinIdentity,
        DEMO_ACCOUNT.password,
      )
        .then((credential) => {
          if (!this.state.accounts[BUILTIN_KEY])
            this.commit({
              ...this.state,
              accounts: { ...this.state.accounts, [BUILTIN_KEY]: credential },
            });
        })
        .finally(() => {
          this.builtinPending = null;
        });
    }
    await this.builtinPending;
  }

  private assertUnlocked() {
    if (this.lockedUntil())
      throw new DemoAuthError("TEMPORARILY_LOCKED", this.state.lockedUntil);
    if (this.state.lockedUntil && this.state.lockedUntil <= this.now())
      this.commit({ ...this.state, failures: 0, lockedUntil: 0 });
  }

  private authenticationFailure(code: "INVALID_CREDENTIALS"): never {
    const failures = Math.min(5, this.state.failures + 1);
    const lockedUntil = failures >= 5 ? this.now() + PASSWORD_LOCK_MS : 0;
    this.commit({ ...this.state, failures, lockedUntil });
    if (lockedUntil) throw new DemoAuthError("TEMPORARILY_LOCKED", lockedUntil);
    throw new DemoAuthError(code);
  }

  private verifyCode(
    purpose: DemoAuthPurpose,
    target: string,
    code: string,
    accountId?: string,
  ) {
    if (!code.trim()) throw new DemoAuthError("CODE_REQUIRED");
    const challenge = this.state.challenges[purpose];
    if (
      !challenge ||
      challenge.target !== target ||
      (accountId && challenge.accountId !== accountId)
    )
      throw new DemoAuthError("CODE_NOT_ISSUED");
    if (challenge.expiresAt <= this.now())
      throw new DemoAuthError("CODE_EXPIRED");
    if (challenge.attemptsRemaining <= 0)
      throw new DemoAuthError("CODE_EXHAUSTED");
    if (challenge.code !== code.trim()) {
      const attemptsRemaining = challenge.attemptsRemaining - 1;
      this.commit({
        ...this.state,
        challenges: {
          ...this.state.challenges,
          [purpose]: { ...challenge, attemptsRemaining },
        },
      });
      throw new DemoAuthError(
        attemptsRemaining ? "CODE_INVALID" : "CODE_EXHAUSTED",
      );
    }
  }

  private succeed(
    purpose?: DemoAuthPurpose,
    accounts: Record<string, Credential> = this.state.accounts,
  ) {
    const challenges = { ...this.state.challenges };
    if (purpose) delete challenges[purpose];
    this.commit({
      ...this.state,
      accounts,
      challenges,
      failures: 0,
      lockedUntil: 0,
    });
  }
}

export const demoAuth = new DemoAuthService();

/** Built-in local demo identity; enterpriseName is retained only for old snapshots. */
export const DEMO_ACCOUNT_PROFILE = {
  id: "account-demo",
  username: "veyra_demo",
  phone: "+8613800138000",
  email: "admin@enterprise.example",
  enterpriseName: "演示账号",
} as const;

interface AccountIdentity {
  username?: string;
  email?: string;
  enterpriseName?: string;
}

/** Present an account identifier without treating a legacy company name as a username. */
export function accountDisplayName(
  account: AccountIdentity,
  fallback = "演示账号",
): string {
  const identifier = account.username?.trim() || account.email?.trim();
  if (!identifier) return fallback;
  return identifier.toLowerCase() === DEMO_ACCOUNT_PROFILE.email
    ? DEMO_ACCOUNT_PROFILE.username
    : identifier;
}

/** Stable compatibility ID, shared by auth and saved business snapshots. */
export function accountIdForLegacy(account: AccountIdentity): string {
  const alias = (account.username?.trim() || account.email?.trim() || "")
    .normalize("NFKC")
    .toLowerCase();
  if (
    !alias ||
    alias === DEMO_ACCOUNT_PROFILE.username ||
    alias === DEMO_ACCOUNT_PROFILE.email
  )
    return "account-demo";
  // Two deterministic hashes keep legacy emails out of the visible account ID.
  const bytes = new TextEncoder().encode(alias);
  const hash = (seed: number) => {
    let value = seed;
    for (const byte of bytes) value = Math.imul(value ^ byte, 16777619) >>> 0;
    return value.toString(16).padStart(8, "0");
  };
  return `account-legacy-${hash(2166136261)}${hash(3335557771)}`;
}

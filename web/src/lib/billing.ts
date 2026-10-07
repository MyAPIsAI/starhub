import type {
  LedgerEntry,
  PreviewKey,
  ProductPreviewState,
  AdminTopUpRecord,
} from "./product-preview";
import type { RequestRecord } from "../mocks/service";

/** Local payment simulation only. No payment credentials are collected or stored. */
export type RechargeStatus =
  | "pending"
  | "processing"
  | "paid"
  | "expired"
  | "failed"
  | "cancelled"
  | "disputed";

export interface RechargeOrder {
  id: string;
  amount_nano: string;
  amount_usd: string;
  currency: "USD";
  status: RechargeStatus;
  channel: "card";
  created_at: string;
  expires_at: string;
  provider_order_id: string;
  paid_at?: string;
  late_payment?: boolean;
  failure_reason?: "payment_failed" | "payment_cancelled" | "payment_expired";
}

export type RechargeResult =
  | { ok: true; order: RechargeOrder; alreadyProcessed?: boolean }
  | { ok: false; error: string };

export type RechargeAmountResult =
  | { ok: true; amount_usd: string; amount_nano: string }
  | { ok: false; error: "required" | "format" | "min" | "max" };

export const RECHARGE_EXPIRY_MS = 30 * 60 * 1000;
export const BILLING_STORAGE_KEY = "veyra-billing-demo-v1";

/** Parse decimal dollars through integer cents, never through floating point. */
export function validateRechargeAmount(input: string): RechargeAmountResult {
  if (typeof input !== "string" || !input.trim())
    return { ok: false, error: "required" };
  const text = input.trim();
  if (!/^\d{1,12}(?:\.\d{1,2})?$/.test(text))
    return { ok: false, error: "format" };
  const [whole, decimal = ""] = text.split(".");
  const cents = BigInt(whole) * 100n + BigInt(decimal.padEnd(2, "0"));
  if (cents < 1000n) return { ok: false, error: "min" };
  if (cents > 1000000n) return { ok: false, error: "max" };
  return {
    ok: true,
    amount_usd: `${cents / 100n}.${(cents % 100n).toString().padStart(2, "0")}`,
    amount_nano: (cents * 10000000n).toString(),
  };
}

export function newBillingId(prefix: string): string {
  const suffix =
    typeof globalThis.crypto?.randomUUID === "function"
      ? globalThis.crypto.randomUUID()
      : `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}-${++fallbackId}`;
  return `${prefix}_${suffix}`;
}
let fallbackId = 0;

export function expireRechargeOrder(
  order: RechargeOrder,
  now: number,
): RechargeOrder {
  if (
    (order.status === "pending" || order.status === "processing") &&
    now >= Date.parse(order.expires_at)
  ) {
    return { ...order, status: "expired", failure_reason: "payment_expired" };
  }
  return order;
}

export function transitionRechargeOrder(
  existing: RechargeOrder,
  outcome: "success" | "failed" | "cancelled",
  now: number,
): RechargeResult {
  if (!Number.isFinite(now)) return { ok: false, error: "invalid_time" };
  const order = expireRechargeOrder(existing, now);
  // Successful accounting is irreversible through a later failure notification.
  if (order.status === "paid" || order.status === "disputed") {
    return { ok: true, order, alreadyProcessed: true };
  }
  if (outcome === "success") {
    if (order.status === "failed")
      return { ok: false, error: "new_order_required" };
    return {
      ok: true,
      order: {
        ...order,
        status: "paid",
        paid_at: new Date(now).toISOString(),
        late_payment:
          order.status === "expired" ||
          order.status === "cancelled" ||
          now >= Date.parse(order.expires_at),
        failure_reason: undefined,
      },
    };
  }
  if (
    order.status === "expired" ||
    order.status === "cancelled" ||
    order.status === "failed"
  ) {
    return { ok: true, order, alreadyProcessed: true };
  }
  return {
    ok: true,
    order: {
      ...order,
      status: outcome,
      failure_reason:
        outcome === "failed" ? "payment_failed" : "payment_cancelled",
    },
  };
}

export interface BillingSnapshot {
  version: 1;
  accountProfiles?: ProductPreviewState["accountProfiles"];
  balance: string;
  ledger: LedgerEntry[];
  rechargeOrders: RechargeOrder[];
  liveRequests: RequestRecord[];
  sampleRequestsEnabled: boolean;
  keys?: Omit<PreviewKey, "keySecret">[];
  selectedKey?: string;
  scenario?: "first" | "enterprise" | "pricing";
  toolChains?: Record<string, string>;
  // 手动加款记录（#105）：随快照持久化，刷新后可还原。校验与 AdminTopUpRecord 字段一一对应。
  adminTopUps?: AdminTopUpRecord[];
  user?: Pick<
    ProductPreviewState["user"],
    | "enterpriseName"
    | "email"
    | "username"
    | "phone"
    | "isLoggedIn"
    | "onboardingCompleted"
    | "accountStatus"
  > &
    Partial<
      Pick<
        ProductPreviewState["user"],
        | "id"
        | "createdAt"
        | "passwordUpdatedAt"
        | "securityEvents"
        | "sessions"
        | "emailVerified"
      >
    >;
}

type StorageLike = Pick<Storage, "getItem" | "setItem">;
function availableStorage(): StorageLike | undefined {
  try {
    return globalThis.localStorage;
  } catch {
    return undefined;
  }
}
const record = (value: unknown): value is Record<string, unknown> =>
  !!value && typeof value === "object" && !Array.isArray(value);
const text = (value: unknown, limit = 500): value is string =>
  typeof value === "string" && value.length > 0 && value.length <= limit;
const timestamp = (value: unknown): value is string =>
  text(value, 50) && Number.isFinite(Date.parse(value));
const nano = (value: unknown): value is string =>
  typeof value === "string" &&
  /^-?\d{1,19}$/.test(value) &&
  BigInt(value) >= -9223372036854775808n &&
  BigInt(value) <= 9223372036854775807n;
const optionalText = (value: unknown) => value === undefined || text(value);

function validLedger(value: unknown): value is LedgerEntry {
  if (!record(value)) return false;
  return (
    text(value.id) &&
    timestamp(value.time) &&
    [
      "recharge",
      "call_summary",
      "adjustment",
      // 保留以兼容历史快照与 M4 §5.1 的线上退款规划；首发无生产者。
      "refund",
    ].includes(String(value.type)) &&
    nano(value.amount_nano) &&
    nano(value.balance_after_nano) &&
    text(value.description, 2000) &&
    optionalText(value.order_id) &&
    optionalText(value.trace_id) &&
    optionalText(value.idempotency_key)
  );
}

function validOrder(value: unknown): value is RechargeOrder {
  if (!record(value) || typeof value.amount_usd !== "string") return false;
  const amount = validateRechargeAmount(value.amount_usd);
  if (
    !amount.ok ||
    amount.amount_usd !== value.amount_usd ||
    amount.amount_nano !== value.amount_nano
  )
    return false;
  return (
    text(value.id) &&
    value.currency === "USD" &&
    value.channel === "card" &&
    [
      "pending",
      "processing",
      "paid",
      "expired",
      "failed",
      "cancelled",
      "disputed",
    ].includes(String(value.status)) &&
    timestamp(value.created_at) &&
    timestamp(value.expires_at) &&
    Date.parse(value.expires_at) - Date.parse(value.created_at) ===
      RECHARGE_EXPIRY_MS &&
    text(value.provider_order_id) &&
    (value.paid_at === undefined || timestamp(value.paid_at)) &&
    (!(value.status === "paid" || value.status === "disputed") ||
      timestamp(value.paid_at)) &&
    (value.late_payment === undefined ||
      typeof value.late_payment === "boolean") &&
    (value.failure_reason === undefined ||
      ["payment_failed", "payment_cancelled", "payment_expired"].includes(
        String(value.failure_reason),
      ))
  );
}

function validRequest(value: unknown): value is RequestRecord {
  if (!record(value)) return false;
  return (
    ["trace_id", "model", "vendor"].every((key) => text(value[key])) &&
    typeof value.tier === "string" &&
    /^[a-z0-9][a-z0-9_-]{1,59}$/.test(value.tier) &&
    ["success", "failed", "interrupted"].includes(String(value.status)) &&
    timestamp(value.time) &&
    value.source === "playground" &&
    ["cost_nano", "input_price_nano", "output_price_nano"].every(
      (key) => nano(value[key]) && BigInt(value[key] as string) >= 0n,
    ) &&
    [
      "input_tokens",
      "output_tokens",
      "cache_tokens",
      "duration_ms",
      "ttft_ms",
    ].every(
      (key) =>
        typeof value[key] === "number" &&
        Number.isFinite(value[key]) &&
        value[key] >= 0,
    ) &&
    typeof value.fallback === "boolean" &&
    (value.error_code === null || text(value.error_code))
  );
}

/** Reject corrupt snapshots atomically instead of hydrating a partial wallet. */
export function parseBillingSnapshot(raw: string): BillingSnapshot | null {
  try {
    if (raw.length > 4_000_000) return null;
    const value: unknown = JSON.parse(raw);
    if (
      !record(value) ||
      value.version !== 1 ||
      !nano(value.balance) ||
      typeof value.sampleRequestsEnabled !== "boolean"
    )
      return null;
    // Older backend profiles used an empty string for an unavailable phone.
    // Treat only that legacy value as absent before validating the whole snapshot.
    if (record(value.user) && value.user.phone === "") delete value.user.phone;
    if (
      !Array.isArray(value.ledger) ||
      value.ledger.length > 20000 ||
      !value.ledger.every(validLedger)
    )
      return null;
    if (
      !Array.isArray(value.rechargeOrders) ||
      value.rechargeOrders.length > 5000 ||
      !value.rechargeOrders.every(validOrder)
    )
      return null;
    if (
      !Array.isArray(value.liveRequests) ||
      value.liveRequests.length > 10000 ||
      !value.liveRequests.every(validRequest)
    )
      return null;
    if (
      value.keys !== undefined &&
      (!Array.isArray(value.keys) ||
        !value.keys.every(
          (key) =>
            record(key) &&
            text(key.id) &&
            typeof key.name === "string" &&
            text(key.model) &&
            typeof key.tier === "string" &&
            /^[a-z0-9][a-z0-9_-]{1,59}$/.test(key.tier) &&
            ["specified", "client", "smart"].includes(String(key.type)) &&
            typeof key.active === "boolean" &&
            text(key.keyMasked) &&
            (key.smartAccess === undefined ||
              typeof key.smartAccess === "boolean") &&
            (key.modelAccess === undefined ||
              ["all", "custom"].includes(String(key.modelAccess))) &&
            (key.allowedModels === undefined ||
              (Array.isArray(key.allowedModels) &&
                key.allowedModels.every((model) => text(model)))) &&
            (key.quotaLimit_usd === undefined ||
              key.quotaLimit_usd === null ||
              (typeof key.quotaLimit_usd === "number" &&
                Number.isFinite(key.quotaLimit_usd) &&
                key.quotaLimit_usd >= 0)) &&
            (key.quotaPeriod === undefined ||
              ["never", "daily", "monthly"].includes(
                String(key.quotaPeriod),
              )) &&
            (key.currentPeriodUsed_nano === undefined ||
              nano(key.currentPeriodUsed_nano)) &&
            (key.totalUsed_nano === undefined || nano(key.totalUsed_nano)) &&
            (key.usagePeriodStartedAt === undefined ||
              timestamp(key.usagePeriodStartedAt)),
        ))
    )
      return null;
    if (
      value.selectedKey !== undefined &&
      typeof value.selectedKey !== "string"
    )
      return null;
    if (
      value.scenario !== undefined &&
      !["first", "enterprise", "pricing"].includes(String(value.scenario))
    )
      return null;
    if (
      value.toolChains !== undefined &&
      (!record(value.toolChains) ||
        !Object.values(value.toolChains).every((item) => text(item)))
    )
      return null;
    if (
      value.adminTopUps !== undefined &&
      (!Array.isArray(value.adminTopUps) ||
        value.adminTopUps.length > 5000 ||
        !value.adminTopUps.every(
          (item) =>
            record(item) &&
            text(item.id) &&
            text(item.enterpriseName) &&
            text(item.amount_usd) &&
            nano(item.amount_nano) &&
            BigInt(item.amount_nano) >= 0n &&
            ["gift", "compensation", "offline_payment", "clawback"].includes(
              String(item.type),
            ) &&
            text(item.reason, 2000) &&
            text(item.operator) &&
            timestamp(item.created_at),
        ))
    )
      return null;
    if (
      value.accountProfiles !== undefined &&
      (!record(value.accountProfiles) ||
        Object.keys(value.accountProfiles).length > 1000 ||
        !Object.entries(value.accountProfiles).every(
          ([id, profile]) =>
            text(id, 128) &&
            record(profile) &&
            (profile.createdAt === undefined || timestamp(profile.createdAt)) &&
            (profile.passwordUpdatedAt === undefined ||
              timestamp(profile.passwordUpdatedAt)) &&
            Array.isArray(profile.securityEvents) &&
            profile.securityEvents.length <= 30 &&
            profile.securityEvents.every(
              (event) =>
                record(event) &&
                text(event.id, 100) &&
                timestamp(event.time) &&
                [
                  "password_changed",
                  "email_changed",
                  "phone_changed",
                  "sessions_revoked",
                ].includes(String(event.action)),
            ) &&
            Array.isArray(profile.sessions) &&
            profile.sessions.length <= 20 &&
            profile.sessions.every(
              (session) =>
                record(session) &&
                text(session.id, 100) &&
                text(session.device, 200) &&
                text(session.ip, 100) &&
                text(session.lastActive, 100) &&
                typeof session.isCurrent === "boolean",
            ),
        ))
    )
      return null;
    if (
      value.user !== undefined &&
      (!record(value.user) ||
        (value.user.emailVerified !== undefined &&
          typeof value.user.emailVerified !== "boolean") ||
        (value.user.id !== undefined && !text(value.user.id, 128)) ||
        (value.user.createdAt !== undefined &&
          !timestamp(value.user.createdAt)) ||
        (value.user.passwordUpdatedAt !== undefined &&
          !timestamp(value.user.passwordUpdatedAt)) ||
        (value.user.securityEvents !== undefined &&
          (!Array.isArray(value.user.securityEvents) ||
            value.user.securityEvents.length > 30 ||
            !value.user.securityEvents.every(
              (event) =>
                record(event) &&
                text(event.id, 100) &&
                timestamp(event.time) &&
                [
                  "password_changed",
                  "email_changed",
                  "phone_changed",
                  "sessions_revoked",
                ].includes(String(event.action)),
            ))) ||
        (value.user.sessions !== undefined &&
          (!Array.isArray(value.user.sessions) ||
            value.user.sessions.length > 20 ||
            !value.user.sessions.every(
              (session) =>
                record(session) &&
                text(session.id, 100) &&
                text(session.device, 200) &&
                text(session.ip, 100) &&
                text(session.lastActive, 100) &&
                typeof session.isCurrent === "boolean",
            ))) ||
        typeof value.user.enterpriseName !== "string" ||
        typeof value.user.email !== "string" ||
        (value.user.username !== undefined && !text(value.user.username, 80)) ||
        (value.user.phone !== undefined &&
          (typeof value.user.phone !== "string" ||
            !/^\+?[0-9]{7,15}$/.test(value.user.phone))) ||
        typeof value.user.isLoggedIn !== "boolean" ||
        typeof value.user.onboardingCompleted !== "boolean" ||
        !["normal", "frozen", "deactivated"].includes(
          String(value.user.accountStatus),
        ))
    )
      return null;
    const ledger = value.ledger as LedgerEntry[];
    const orders = value.rechargeOrders as RechargeOrder[];
    const calls = value.liveRequests as RequestRecord[];
    if (
      new Set(ledger.map((entry) => entry.id)).size !== ledger.length ||
      new Set(orders.map((order) => order.id)).size !== orders.length ||
      new Set(orders.map((order) => order.provider_order_id)).size !==
        orders.length ||
      new Set(calls.map((call) => call.trace_id)).size !== calls.length
    )
      return null;
    const keys = ledger
      .map((entry) => entry.idempotency_key)
      .filter((key): key is string => !!key);
    if (new Set(keys).size !== keys.length) return null;
    let balance = 0n;
    for (const entry of [...ledger].reverse()) {
      balance += BigInt(entry.amount_nano);
      if (balance !== BigInt(entry.balance_after_nano)) return null;
    }
    if (balance !== BigInt(value.balance)) return null;
    for (const order of orders) {
      const entries = ledger.filter(
        (entry) => entry.type === "recharge" && entry.order_id === order.id,
      );
      if (order.status === "paid" || order.status === "disputed") {
        if (
          entries.length !== 1 ||
          entries[0].amount_nano !== order.amount_nano ||
          entries[0].idempotency_key !== `payment:${order.provider_order_id}`
        )
          return null;
      } else if (entries.length) return null;
    }
    for (const call of calls) {
      const entries = ledger.filter(
        (entry) =>
          entry.trace_id === call.trace_id && entry.type === "call_summary",
      );
      const cost = BigInt(call.cost_nano);
      if (cost > 0n) {
        if (
          entries.length !== 1 ||
          BigInt(entries[0].amount_nano) !== -cost ||
          entries[0].idempotency_key !== `call:${call.trace_id}`
        )
          return null;
      } else if (entries.length) return null;
    }
    return {
      version: 1,
      ...(value.accountProfiles !== undefined
        ? {
            accountProfiles:
              value.accountProfiles as BillingSnapshot["accountProfiles"],
          }
        : {}),
      balance: value.balance,
      ledger,
      rechargeOrders: orders,
      liveRequests: calls,
      sampleRequestsEnabled: value.sampleRequestsEnabled,
      ...(value.keys !== undefined
        ? {
            keys: value.keys as BillingSnapshot["keys"],
            selectedKey: value.selectedKey as string | undefined,
          }
        : {}),
      ...(value.scenario !== undefined
        ? { scenario: value.scenario as BillingSnapshot["scenario"] }
        : {}),
      ...(value.toolChains !== undefined
        ? { toolChains: value.toolChains as Record<string, string> }
        : {}),
      ...(value.adminTopUps !== undefined
        ? { adminTopUps: value.adminTopUps as AdminTopUpRecord[] }
        : {}),
      ...(value.user !== undefined
        ? { user: value.user as BillingSnapshot["user"] }
        : {}),
    };
  } catch {
    return null;
  }
}

export function readBillingStorage(
  storage = availableStorage(),
): BillingSnapshot | null {
  try {
    const raw = storage?.getItem(BILLING_STORAGE_KEY);
    return raw ? parseBillingSnapshot(raw) : null;
  } catch {
    return null;
  }
}

export function writeBillingStorage(
  snapshot: BillingSnapshot,
  storage = availableStorage(),
): boolean {
  try {
    if (!storage) return false;
    storage.setItem(BILLING_STORAGE_KEY, JSON.stringify(snapshot));
    return true;
  } catch {
    return false;
  }
}

/** A storage event is delivered to other tabs, never the tab that wrote it. */
export function listenBillingStorage(
  sync: () => void,
  target: EventTarget | undefined = typeof window === "undefined"
    ? undefined
    : window,
): () => void {
  if (!target) return () => {};
  const listener = (event: Event) => {
    const change = event as StorageEvent;
    if (change.key !== BILLING_STORAGE_KEY) return;
    if (change.storageArea && change.storageArea !== availableStorage()) return;
    // Read the latest validated snapshot instead of a possibly queued old event.
    sync();
  };
  target.addEventListener("storage", listener);
  return () => target.removeEventListener("storage", listener);
}

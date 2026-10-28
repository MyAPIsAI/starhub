import type { PricingAttributes } from "../config/models";
import { create } from "zustand";
import { useSystemAccess } from "./system-access";
import { DEMO_ACCOUNT_PROFILE, accountIdForLegacy } from "./account-identity";
import { writtenLanguage } from "./localize";
import { demoAuth, type DemoIdentity } from "./demo-auth";
import {
  getKeyModels,
  SMART_MODEL_ID,
  isSmartModelId,
  resolveKeyModelResult,
} from "./key-model-policy";
import {
  defaultDemoRequest,
  validateDemoRequest,
  demoInputTokens,
  simulateDemoResponse,
  type DemoRequest,
} from "./request-contract";
import {
  estimateModelCharge,
  quoteModelCharge,
  type PricingQuoteSnapshot,
  type MeteredUsage,
  getPricingSnapshot,
  createDefaultPricing,
  useModelPricing,
  PRICING_STORAGE_KEY,
} from "./model-pricing";
import { sampleRequests } from "../mocks/service";
import type { RequestRecord } from "./request-record";
import { sumNano, type NanoString } from "./money";
import {
  billingDate,
  filterLedger,
  serializeLedgerCsv,
} from "./billing-export";
import {
  filterExportRequests,
  aggregateRequestUsage,
  serializeRequestExport,
  serializeUsageExport,
} from "./request-export";
import { usePreferences } from "./preferences";
import type { TierId } from "../config/placeholders";
import {
  validateRechargeAmount,
  newBillingId,
  expireRechargeOrder,
  transitionRechargeOrder,
  RECHARGE_EXPIRY_MS,
  readBillingStorage,
  writeBillingStorage,
  listenBillingStorage,
  type BillingSnapshot,
  type RechargeOrder,
  type RechargeResult,
} from "./billing";
export type { RechargeOrder } from "./billing";

export type PreviewRequest = Omit<RequestRecord, "source"> & {
  source: "api" | "playground";
  pricing_revision?: string;
  resource_id?: string;
  channel_id?: string;
  resource_snapshot_id?: string;
  upstream_model?: string;
  pricing_snapshot?: PricingQuoteSnapshot;
  metered_usage?: MeteredUsage;
  metered_demo?: boolean;
  metered_attributes?: PricingAttributes;
  input_rate_usd?: string;
  output_rate_usd?: string;
  requested_model?: string;
  actual_model?: string | null;
  key_id?: string;
  request?: DemoRequest;
  response_text?: string;
  billing_status?: "charged" | "not_charged";
  error_message?: string;
  recovery_action?: string;
  protocol?: DemoRequest["protocol"];
  outcome?: "success" | "failed" | "cancelled";
  chain_id?: string;
};

export interface PreviewKey {
  accountId?: string;
  id: string;
  name: string;
  model: string;
  tier: TierId;
  active: boolean;
  quota?: number; // Legacy persisted hint; displays derive quota from exact period amounts.
  keyMasked: string;
  keySecret: string;
  type: "specified" | "client" | "smart";
  modelAccess?: "all" | "custom";
  smartAccess?: boolean;
  smartStrategy?: "cost" | "latency" | "stability";
  allowedModels?: string[];
  quotaLimit_usd?: number | null;
  quotaPeriod?: "never" | "daily" | "monthly";
  currentPeriodUsed_nano?: string;
  usagePeriodStartedAt?: string;
  credentialVersion?: string;
  totalUsed_nano?: string;
  validUntil?: string | null;
  ipWhitelist?: string[];
  lastUsedAt?: string | null;
}

export interface LedgerEntry {
  id: string;
  time: string;
  type:
    | "recharge"
    | "call_summary"
    | "adjustment"
    // 前向兼容：M4 §5.1 规划了渠道侧退款流水，但 §5.3 明确首发只做线下退款
    // （由财务做「手动调整·扣回」）。当前无生产者，保留此类型与持久化校验白名单。
    | "refund";
  amount_nano: string; // "+100000000000" or "-523000000"
  balance_after_nano: string;
  description: string;
  order_id?: string;
  trace_id?: string;
  idempotency_key?: string;
}

export interface ExportRecord {
  id: string;
  type: "requests" | "ledger" | "usage";
  rangeDays: number;
  startDate: string;
  endDate: string;
  createdAt: string;
  expiresAt: string;
  rowCount: number;
  sizeKb: number;
  status: "completed" | "processing";
  filename: string;
}

/**
 * 手动加款的留痕记录（#105）。
 * 首发只有一名后台管理员，加款提交即入账，**没有待复核状态**；
 * 这张表只作审计追溯用，字段与 `M4` §5.2 一致。
 */
export interface AdminTopUpRecord {
  id: string;
  enterpriseName: string;
  amount_usd: string;
  amount_nano: string;
  type: "gift" | "compensation" | "offline_payment" | "clawback";
  reason: string;
  operator: string;
  created_at: string;
}

export interface IncidentAnnouncement {
  id: string;
  title: { en: string; zh: string; ar: string };
  impactedVendor: string;
  status: "investigating" | "identified" | "monitoring" | "resolved";
  updatedAt: string;
  content: { en: string; zh: string; ar: string };
}

export const previewDay =
  Math.floor((Date.now() + 4 * 3600000) / 86400000) * 86400000 - 4 * 3600000;

export function makePreviewRequests(): PreviewRequest[] {
  const rows: PreviewRequest[] = [];
  for (let day = 0; day < 30; day++) {
    const count = 124 + ((day * 37) % 110);
    for (let i = 0; i < count; i++) {
      const base = sampleRequests[(i + day) % sampleRequests.length];
      const failed = (i + day * 17) % 143 === 5;
      const input = 620 + ((i * 137) % 1700);
      const output = 148 + ((i * 53) % 710);
      const cost = failed
        ? 0n
        : (BigInt(input) * BigInt(base.input_price_nano) +
            BigInt(output) * BigInt(base.output_price_nano) +
            500000n) /
          1000000n;
      rows.push({
        ...base,
        trace_id: `tr_preview_${day.toString(16).padStart(2, "0")}${i.toString(16).padStart(4, "0")}`,
        time: new Date(
          previewDay -
            day * 86400000 +
            (i / count) * (day === 0 ? 52000000 : 86000000),
        ).toISOString(),
        input_tokens: failed ? 0 : input,
        output_tokens: failed ? 0 : output,
        cost_nano: cost.toString(),
        status: failed ? "failed" : "success",
        error_code: failed ? "upstream_unavailable" : null,
        duration_ms: failed ? 60000 : 1034 + ((i * 83) % 2300),
        ttft_ms: failed ? 0 : 240 + ((i * 13) % 300),
        source: "api",
      });
    }
  }
  return rows.sort((a, b) => b.time.localeCompare(a.time));
}

export function periodRequests(rows: PreviewRequest[], days: number) {
  const start = previewDay - (days - 1) * 86400000;
  return rows.filter(
    (row) =>
      Date.parse(row.time) >= start &&
      Date.parse(row.time) < previewDay + 86400000,
  );
}

export function summarizeRequests(rows: PreviewRequest[]) {
  return {
    count: rows.length,
    cost: sumNano(rows.map((r) => r.cost_nano)),
    success: rows.length
      ? (
          (rows.filter((r) => r.status === "success").length / rows.length) *
          100
        ).toFixed(2)
      : "—",
  };
}

export interface ModelBreakdown {
  model: string;
  vendor: string;
  count: number;
  /** nano-USD，交给 formatMoney 决定显示精度，不进浮点。 */
  cost: NanoString;
  /** 百分比字符串；该模型无调用时为 "—"。 */
  success: string;
}

/** 按模型聚合当前周期内的调用，供「按模型分布」表格使用。 */
export function breakdownByModel(rows: PreviewRequest[]): ModelBreakdown[] {
  const buckets = new Map<string, PreviewRequest[]>();
  for (const row of rows) {
    const key = row.model;
    const bucket = buckets.get(key);
    if (bucket) bucket.push(row);
    else buckets.set(key, [row]);
  }
  const entries = [...buckets.entries()]
    .map(([model, bucket]) => {
      const summary = summarizeRequests(bucket);
      return {
        model,
        vendor: bucket[0].vendor,
        count: summary.count,
        cost: summary.cost,
        success: summary.success,
      };
    })
    .sort((a, b) => b.count - a.count);
  // 各模型消费各自四舍五入到 4 位后，相加会比合计多／少最后一位
  // （本例：各桶相加 $5.1242，而合计 $5.1241）。这里按显示精度配平：
  // 目标取「合计 nano 值」四舍五入的结果，与「累计消费」卡片同源同算法，
  // 再用最大余数法把差额摊到各桶上，进位／退位都做。
  const scale = 10n ** 5n; // nano-USD 保留 4 位小数后的最小显示单位（10^9 / 10^4）
  const round4 = (value: bigint) =>
    value / scale + ((value % scale) * 2n >= scale ? 1n : 0n);
  const shares = entries.map((entry) => {
    const value = BigInt(entry.cost);
    return { floor: value / scale, remainder: value % scale };
  });
  const total = entries.reduce((acc, entry) => acc + BigInt(entry.cost), 0n);
  let residual = round4(total) - shares.reduce((acc, s) => acc + s.floor, 0n);
  const byRemainder = shares
    .map((share, index) => ({ index, remainder: share.remainder }))
    .sort((a, b) => (b.remainder > a.remainder ? 1 : -1));
  for (const { index } of residual > 0n
    ? byRemainder
    : [...byRemainder].reverse()) {
    if (residual === 0n) break;
    if (residual > 0n) {
      shares[index].floor += 1n;
      residual -= 1n;
    } else {
      shares[index].floor -= 1n;
      residual += 1n;
    }
  }
  return entries.map((entry, index) => ({
    ...entry,
    cost: (shares[index].floor * scale).toString() as NanoString,
  }));
}

export function activityFor(rows: PreviewRequest[], days: number) {
  return Array.from({ length: days }, (_, i) => {
    const start = previewDay - (days - 1 - i) * 86400000;
    const matching = rows.filter(
      (r) =>
        Date.parse(r.time) >= start && Date.parse(r.time) < start + 86400000,
    );
    const totalCostNano = matching.reduce(
      (acc, cur) => acc + BigInt(cur.cost_nano),
      0n,
    );
    const totalTokens = matching.reduce(
      (acc, cur) => acc + (cur.input_tokens + cur.output_tokens),
      0,
    );
    const avgLatency =
      matching.length > 0
        ? Math.round(
            matching.reduce((acc, cur) => acc + cur.duration_ms, 0) /
              matching.length,
          )
        : 185;
    return {
      date: new Date(start + 4 * 3600000)
        .toISOString()
        .slice(5, 10)
        .replace("-", "/"),
      requests: matching.length,
      cost: Number((Number(totalCostNano) / 1e9).toFixed(4)),
      tokens: totalTokens,
      latency: avgLatency,
    };
  });
}

const initialRequests = makePreviewRequests();
/** 供测试与场景重置引用的种子调用记录条数（first 场景为空，其余场景恢复该数量）。 */
export const seedRequestCount = initialRequests.length;
const seedRequestsCost = BigInt(
  sumNano(initialRequests.map((r) => r.cost_nano)),
);
/** 种子场景的期初余额：当前余额加上种子记录已产生的消费，保证「期初 − 消费 = 余额」。 */
export const seedOpeningNano = (128400000000n + seedRequestsCost).toString();

export type AccountSecurityAction =
  "password_changed" | "email_changed" | "phone_changed";
export interface AccountSecurityEvent {
  id: string;
  time: string;
  action: AccountSecurityAction | "sessions_revoked";
}

export type AccountProfileMetadata = Pick<
  ProductPreviewState["user"],
  "createdAt" | "passwordUpdatedAt" | "securityEvents" | "sessions"
>;
const profileMetadata = (
  user: ProductPreviewState["user"],
): AccountProfileMetadata => ({
  createdAt: user.createdAt,
  passwordUpdatedAt: user.passwordUpdatedAt,
  securityEvents: user.securityEvents,
  sessions: user.sessions,
});
const currentDemoSession = () => ({
  id: `sess_current_${Date.now()}`,
  device: "Current browser · Demo",
  ip: "127.0.0.1",
  lastActive: new Date().toISOString(),
  isCurrent: true,
});

export interface ProductPreviewState {
  // Authentication & Profile
  user: {
    id: string;
    createdAt?: string;
    passwordUpdatedAt?: string;
    securityEvents: AccountSecurityEvent[];
    enterpriseName: string;
    email: string;
    username?: string;
    phone?: string;
    emailVerified?: boolean;
    isLoggedIn: boolean;
    onboardingCompleted: boolean;
    accountStatus: "normal" | "frozen" | "deactivated";
    concurrencyLimit: number;
    sessions: Array<{
      id: string;
      device: string;
      ip: string;
      lastActive: string;
      isCurrent: boolean;
    }>;
  };
  accountProfiles: Record<string, AccountProfileMetadata>;
  // Key state & requests
  requests: PreviewRequest[];
  keys: PreviewKey[];
  selectedKey: string;
  opening: string;
  balance: string; // Nano-USD string
  revealedKeyIds: Record<string, number>; // keyId -> expiry timestamp
  scenario: "first" | "enterprise" | "pricing";
  toolChains: Record<string, string>;

  // Billing & Ledger
  ledger: LedgerEntry[];
  rechargeOrders: RechargeOrder[];

  // Exports
  exports: ExportRecord[];

  // Admin Portal state
  adminReviews: AdminTopUpRecord[];
  incidents: IncidentAnnouncement[];

  // Demo Control States
  demoMode: {
    accountState: "mature" | "new";
    balanceState: "normal" | "low" | "zero";
    upstreamOutage: boolean;
    uiState: "normal" | "loading" | "empty" | "error";
  };

  // Actions
  createKey: (name: string, tier: TierId, model: string) => PreviewKey;
  createKeyDetailed: (params: {
    name: string;
    tier: TierId;
    model: string;
    type?: "specified" | "client" | "smart";
    modelAccess?: "all" | "custom";
    smartAccess?: boolean;
    smartStrategy?: "cost" | "latency" | "stability";
    allowedModels?: string[];
    quotaLimit_usd?: number | null;
    quotaPeriod?: "never" | "daily" | "monthly";
    ipWhitelist?: string[];
  }) => PreviewKey;
  updateKey: (id: string, updates: Partial<PreviewKey>) => void;
  regenerateKey: (id: string) => { oldKey: string; newKey: string };
  deleteKey: (id: string) => void;
  toggleKey: (id: string) => void;
  chooseKey: (id: string) => void;
  verifyPasswordAndRevealKey: (
    keyId: string,
    password: string,
  ) => Promise<boolean>;

  recordCall: (
    id: string,
    failed: boolean,
    requestedModel?: string,
    request?: DemoRequest,
  ) => PreviewRequest | null;
  recordMeteredCall: (
    id: string,
    modelId: string,
    usage: MeteredUsage,
    at?: string,
    attributes?: PricingAttributes,
  ) => PreviewRequest;
  createRechargeOrder: (amountUsd: string, channel?: "card") => RechargeResult;
  beginRechargePayment: (id: string) => RechargeResult;
  resolveRechargeOrder: (
    id: string,
    outcome: "success" | "failed" | "cancelled",
    options?: { now?: number },
  ) => RechargeResult;
  expireRechargeOrders: (now?: number) => void;
  syncBillingStorage: () => boolean;
  rechargeAccount: (amount_usd: number) => RechargeOrder;
  requestExport: (
    type: "requests" | "ledger" | "usage",
    rangeDays: number,
  ) => ExportRecord;

  login: (
    email: string,
    enterpriseName?: string,
    profile?: {
      id?: string;
      username?: string;
      phone?: string;
      emailVerified?: boolean;
    },
  ) => void;
  register: (
    email: string,
    enterpriseName: string,
    profile?: {
      id?: string;
      username?: string;
      phone?: string;
      emailVerified?: boolean;
    },
  ) => void;
  applyAccountSecurity: (
    identity: DemoIdentity,
    action: AccountSecurityAction,
  ) => void;
  logout: () => void;
  completeOnboarding: () => void;
  updateEnterpriseName: (name: string) => void;
  terminateOtherSessions: () => void;
  deactivateAccount: () => { cleared_nano: string };

  // #105：加款单人即可生效，不再有审批／驳回两个动作（原 adminApproveTopUp / adminRejectTopUp）。
  adminCreateTopUp: (params: {
    amount_usd: number;
    type: "gift" | "compensation" | "offline_payment" | "clawback";
    reason: string;
    operator: string;
  }) => AdminTopUpRecord;
  adminUpdateConcurrency: (concurrency: number) => void;
  adminToggleAccountStatus: () => void;
  adminPublishIncident: (incident: IncidentAnnouncement) => void;

  resetScenario: (scenario: "first" | "enterprise" | "pricing") => void;
  demoSetAccountState: (mode: "mature" | "new") => void;
  demoSetBalanceState: (mode: "normal" | "low" | "zero") => void;
  demoSetUpstreamOutage: (outage: boolean) => void;
  demoSetDataState: (state: "normal" | "loading" | "empty" | "error") => void;
}

export function keyPeriodUsed(
  key: PreviewKey,
  now = new Date().toISOString(),
): bigint {
  if (!key.quotaPeriod || key.quotaPeriod === "never")
    return BigInt(key.totalUsed_nano ?? key.currentPeriodUsed_nano ?? "0");
  const previous = key.usagePeriodStartedAt ?? key.lastUsedAt;
  if (!previous) return BigInt(key.currentPeriodUsed_nano ?? "0");
  const date = billingDate(now),
    old = billingDate(previous);
  const matches =
    key.quotaPeriod === "daily"
      ? date === old
      : date.slice(0, 7) === old.slice(0, 7);
  return matches ? BigInt(key.currentPeriodUsed_nano ?? "0") : 0n;
}
function ipv4(value: string): number | null {
  const pieces = value.split(".");
  if (
    pieces.length !== 4 ||
    pieces.some((piece) => !/^\d{1,3}$/.test(piece) || Number(piece) > 255)
  )
    return null;
  return (
    pieces.reduce((address, piece) => address * 256 + Number(piece), 0) >>> 0
  );
}
function validIpRule(rule: string): boolean {
  const [address, mask, ...extra] = rule.split("/");
  return (
    !extra.length &&
    ipv4(address) !== null &&
    (mask === undefined ||
      (/^\d{1,2}$/.test(mask) && Number(mask) >= 0 && Number(mask) <= 32))
  );
}
function matchesIpRule(address: string, rule: string): boolean {
  if (!validIpRule(rule)) return false;
  const [network, prefix = "32"] = rule.split("/");
  const mask =
    Number(prefix) === 0 ? 0 : (0xffffffff << (32 - Number(prefix))) >>> 0;
  return (ipv4(address)! & mask) >>> 0 === (ipv4(network)! & mask) >>> 0;
}
export function decimalUsdNano(value: number): bigint {
  if (!Number.isFinite(value) || value < 0) return 0n;
  const [whole, fraction = ""] = value.toFixed(9).split(".");
  return BigInt(whole + fraction);
}
function rateUsdToNano(value = "0"): string {
  const [whole, fraction = ""] = value.split(".");
  const denominator = 10n ** BigInt(fraction.length);
  return (
    (BigInt(whole + fraction) * 1_000_000_000n * 2n + denominator) /
    (denominator * 2n)
  ).toString();
}
function demoKeySecret(id: string, version = "legacy") {
  return `sk-veyra-demo-${id}-${version}`;
}
const requestErrors: Record<string, [string, string]> = {
  group_unauthorized: [
    "账号无权使用此分组或分组已停用，本次未计费。",
    "请恢复分组授权或选择有权限的分组密钥。",
  ],
  resource_unavailable: [
    "本分组没有符合请求能力的可用资源实例，本次未计费。",
    "检查资源快照、实例映射和分组投放后重试。",
  ],
  pricing_invalid: [
    "计价规则与本次用量不匹配；本次未计费。",
    "查看计价错误详情，补齐计费用量或请系统管理员调整模型计费规则后重试。",
  ],
  unknown_model: [
    "模型标识不存在；本次未计费。",
    "从模型目录选择准确的模型标识后重试。",
  ],
  model_unavailable: [
    "模型已下架；本次未计费。",
    "选择当前分组开放的其他模型后重试。",
  ],
  model_unauthorized: [
    "此密钥未授权请求的模型或智能入口；本次未计费。",
    "选择授权范围内模型，或在密钥高级设置中明确调整授权。",
  ],
  protocol_incompatible: [
    "请求协议与授权模型不兼容；本次未计费。",
    "在接入中心选择匹配的工具或兼容模型。",
  ],
  capability_incompatible: [
    "授权模型无法满足能力或上下文要求；本次未计费。",
    "选择支持所需能力的模型，或缩短输入和输出上限。",
  ],
  tool_chain_incompatible: [
    "工具调用链已固定模型，当前请求无法兼容；本次未计费。",
    "恢复原模型及兼容参数，或明确开启新的调用链。",
  ],
  ip_unauthorized: [
    "模拟请求来源 127.0.0.1 不在密钥网络地址白名单；本次未计费。",
    "在密钥高级设置中允许 127.0.0.1，或清空网络地址限制后重试。",
  ],
  key_disabled: [
    "密钥已停用；本次未计费。",
    "在密钥管理中启用此密钥，或选择其他可用密钥。",
  ],
  key_expired: ["密钥已过期；本次未计费。", "更新有效期或换用可用密钥。"],
  key_not_found: ["密钥不存在；本次未计费。", "创建或选择已有密钥后重试。"],
  key_quota_exceeded: [
    "本次最大预估费用超过密钥剩余额度；本次未计费。",
    "降低输出上限、提高此密钥额度，或等待额度周期重置。",
  ],
  insufficient_balance: [
    "账户余额不足以覆盖本次最大预估费用；本次未计费。",
    "模拟充值完成到账后，保留当前输入重试。",
  ],
  upstream_unavailable: [
    "模拟上游故障；本次未计费。",
    "关闭故障演示后重试，或主动选择另一兼容模型。",
  ],
  account_unavailable: [
    "账户当前不可调用；本次未计费。",
    "在账户页面确认状态，或联系运营处理。",
  ],
  invalid_request: [
    "请求参数无效或模型标识不一致；本次未计费。",
    "检查消息、温度（0–2）和输出上限（1–32768）后重试。",
  ],
};

const initialKeys: PreviewKey[] = [
  {
    id: "pk_1",
    name: "production-app",
    model: "gpt-4.1",
    tier: "pro",
    active: true,
    keyMasked: "sk-xy-••••9f2d",
    keySecret: "sk-xy-live-8f92a1c0d4e59f2d",
    type: "specified",
    allowedModels: ["gpt-4.1", "claude-sonnet-4-20250514"],
    quotaLimit_usd: 500,
    quotaPeriod: "monthly",
    currentPeriodUsed_nano: "28450000000",
    totalUsed_nano: "84210000000",
    quota: 38,
    ipWhitelist: ["192.168.1.0/24", "10.0.4.15"],
    lastUsedAt: "2026-09-18T14:41:00+04:00",
  },
  {
    id: "pk_2",
    name: "content-assistant",
    model: "claude-sonnet-4-20250514",
    tier: "ultra",
    active: true,
    keyMasked: "sk-xy-••••7b4a",
    keySecret: "sk-xy-live-3c18b7e21a957b4a",
    type: "smart",
    smartStrategy: "cost",
    allowedModels: ["claude-sonnet-4-20250514", "deepseek-chat"],
    quotaLimit_usd: 100,
    quotaPeriod: "monthly",
    currentPeriodUsed_nano: "92150000000",
    totalUsed_nano: "92150000000",
    quota: 92,
    ipWhitelist: [],
    lastUsedAt: "2026-09-18T13:20:00+04:00",
  },
  {
    id: "pk_3",
    name: "data-batch-crawler",
    model: "deepseek-chat",
    tier: "standard",
    active: true,
    keyMasked: "sk-xy-••••3e81",
    keySecret: "sk-xy-live-4d99f018a7c23e81",
    type: "specified",
    allowedModels: ["deepseek-chat", "qwen-plus"],
    quotaLimit_usd: null,
    quotaPeriod: "never",
    currentPeriodUsed_nano: "14200000000",
    totalUsed_nano: "48900000000",
    quota: 14,
    ipWhitelist: [],
    lastUsedAt: "2026-09-18T11:05:00+04:00",
  },
];

const initialLedger: LedgerEntry[] = [
  {
    id: "tx_1001",
    time: "2026-09-01T09:00:00+04:00",
    type: "recharge",
    amount_nano: "200000000000",
    balance_after_nano: "200000000000",
    description: "Online Top-up (Corporate Card · Stripe)",
    order_id: "ord_918204",
    idempotency_key: "payment:demo_pay_918204",
  },
  {
    id: "tx_1002",
    time: "2026-09-10T02:00:00+04:00",
    type: "call_summary",
    amount_nano: "-42500000000",
    balance_after_nano: "157500000000",
    description: "API Daily Usage Settlement (2026-09-09)",
  },
  {
    id: "tx_1003",
    time: "2026-09-15T15:30:00+04:00",
    type: "adjustment",
    amount_nano: "10000000000",
    balance_after_nano: "167500000000",
    description: "Enterprise Onboarding Credit Incentive",
  },
  {
    id: "tx_1004",
    time: "2026-09-17T02:00:00+04:00",
    type: "call_summary",
    amount_nano: "-39100000000",
    balance_after_nano: "128400000000",
    description: "API Daily Usage Settlement (2026-09-16)",
  },
];

initialLedger.reverse();

const initialRechargeOrders: RechargeOrder[] = [
  {
    id: "ord_918204",
    amount_nano: "200000000000",
    amount_usd: "200.00",
    status: "paid",
    channel: "card",
    currency: "USD",
    provider_order_id: "demo_pay_918204",
    expires_at: "2026-09-01T09:28:30+04:00",
    created_at: "2026-09-01T08:58:30+04:00",
    paid_at: "2026-09-01T09:00:00+04:00",
  },
];

const initialExports: ExportRecord[] = [
  {
    id: "exp_01",
    type: "requests",
    rangeDays: 7,
    startDate: "2026-09-11",
    endDate: "2026-09-18",
    createdAt: "2026-09-18T10:15:00+04:00",
    expiresAt: "2026-09-25T10:15:00+04:00",
    rowCount: 1095,
    sizeKb: 142,
    status: "completed",
    filename: "requests_20260911_20260918.csv",
  },
];

// #105：加款即入账的留痕（单人操作，无复核状态），入账金额已包含在 opening 里，
// 因此这里只作审计展示，不再需要 status / reviewer。
const initialAdminReviews: AdminTopUpRecord[] = [
  {
    id: "rev_801",
    enterpriseName: "Apex AI Studio",
    amount_usd: "2500.00",
    amount_nano: "2500000000000",
    type: "offline_payment",
    reason: "线上下单合同 #XY-2026-0915（银行转账）",
    operator: "陈闻（超级管理员）",
    created_at: "2026-09-18T11:30:00+04:00",
  },
  {
    id: "rev_802",
    enterpriseName: "CyberPulse Tech",
    amount_usd: "500.00",
    amount_nano: "500000000000",
    type: "compensation",
    reason: "上游故障时长补偿",
    operator: "林思（超级管理员）",
    created_at: "2026-09-17T16:20:00+04:00",
  },
];

const initialIncidents: IncidentAnnouncement[] = [
  {
    id: "inc_01",
    title: {
      en: "Intermittent elevated latencies on Anthropic Claude 3.5 Sonnet upstream",
      zh: "Anthropic Claude 3.5 Sonnet 上游出现偶发延迟升高",
      ar: "ارتفاع متقطع في زمن الاستجابة على مزود أنثروبيك كلود",
    },
    impactedVendor: "Anthropic",
    status: "resolved",
    updatedAt: "2026-09-18T12:00:00+04:00",
    content: {
      en: "Upstream provider resolved network packet drops. Intelligent routing successfully redirected 99.4% of traffic to healthy fallbacks during the window.",
      zh: "上游供应商已完成网络丢包修复。Veyra智能路由在此期间成功将 99.4% 的流量无缝分流至健康节点。",
      ar: "تم حل المشكلة من قبل المزود. قام التوجيه الذكي بتحويل 99.4% من حركة المرور بنجاح.",
    },
  },
];

let receivingBillingSnapshot = false;
let appliedBillingSignature: string | undefined;

function billingSnapshot(state: ProductPreviewState): BillingSnapshot {
  return {
    version: 1,
    accountProfiles: state.accountProfiles,
    balance: state.balance,
    ledger: state.ledger,
    rechargeOrders: state.rechargeOrders,
    liveRequests: state.requests
      .filter((row) => row.source === "playground")
      .map(
        ({ request: _request, response_text: _response, ...row }) =>
          row as RequestRecord,
      ),
    // enterprise / pricing 场景始终携带种子数据，first 场景为空（新用户首屏）。
    // 不再由当前 requests 推导，避免 resetScenario 后刷新丢失种子数据。
    sampleRequestsEnabled: state.requests.some((row) => row.source === "api"),
    keys: state.keys.map(({ keySecret: _secret, ...key }) => key),
    selectedKey: state.selectedKey,
    scenario: state.scenario,
    toolChains: state.toolChains,
    adminTopUps: state.adminReviews,
    user: {
      id: state.user.id,
      createdAt: state.user.createdAt,
      passwordUpdatedAt: state.user.passwordUpdatedAt,
      securityEvents: state.user.securityEvents,
      sessions: state.user.sessions,
      enterpriseName: state.user.enterpriseName,
      email: state.user.email,
      emailVerified: state.user.emailVerified,
      username: state.user.username,
      phone: state.user.phone,
      isLoggedIn: state.user.isLoggedIn,
      onboardingCompleted: state.user.onboardingCompleted,
      accountStatus: state.user.accountStatus,
    },
  };
}

export const useProductPreview = create<ProductPreviewState>((set, get) => ({
  accountProfiles: {},
  user: {
    ...DEMO_ACCOUNT_PROFILE,
    securityEvents: [],
    isLoggedIn: false,
    onboardingCompleted: true,
    accountStatus: "normal",
    concurrencyLimit: 50,
    sessions: [
      {
        id: "sess_curr",
        device: "Chrome 128 · macOS (Current)",
        ip: "194.67.210.45",
        lastActive: "Just now",
        isCurrent: true,
      },
      {
        id: "sess_ci",
        device: "GitHub Actions CI Runner",
        ip: "52.148.12.90",
        lastActive: "35 minutes ago",
        isCurrent: false,
      },
    ],
  },
  requests: initialRequests,
  keys: initialKeys,
  selectedKey: "pk_1",
  opening: seedOpeningNano,
  balance: "128400000000",
  revealedKeyIds: {},
  scenario: "enterprise",
  toolChains: {},
  ledger: initialLedger,
  rechargeOrders: initialRechargeOrders,
  exports: initialExports,
  adminReviews: initialAdminReviews,
  incidents: initialIncidents,
  demoMode: {
    accountState: "mature",
    balanceState: "normal",
    upstreamOutage: false,
    uiState: "normal",
  },

  createKey: (name, tier, model) => {
    return get().createKeyDetailed({ name, tier, model });
  },

  createKeyDetailed: (params) => {
    if (!params.name.trim()) throw new Error("密钥名称不能为空");
    if (params.ipWhitelist?.some((rule) => !validIpRule(rule)))
      throw new Error("IP（网络地址）白名单格式无效");
    if (
      params.quotaLimit_usd !== undefined &&
      params.quotaLimit_usd !== null &&
      (!Number.isFinite(params.quotaLimit_usd) ||
        params.quotaLimit_usd < 0 ||
        params.quotaLimit_usd > 9_000_000_000)
    )
      throw new Error("密钥额度须在 0 至 90 亿美元之间");
    const policy = {
      ...params,
      accountId: get().user.id,
      type: params.type ?? "client",
      modelAccess: params.modelAccess ?? "all",
      smartAccess: params.smartAccess ?? true,
    } as const;
    const allowed = getKeyModels(policy);
    if (!allowed.length) throw new Error("No available authorized model");
    const id = newBillingId("pk");
    const randHex = Math.random().toString(16).substring(2, 6);
    const credentialVersion = newBillingId("v");
    const keySecret = demoKeySecret(id, credentialVersion);
    const keyMasked = `sk-xy-••••${randHex}`;

    const newKey: PreviewKey = {
      id,
      accountId: get().user.id,
      name: params.name.trim(),
      model: allowed.some((item) => item.id === params.model)
        ? params.model
        : allowed[0].id,
      tier: params.tier,
      active: true,
      keyMasked,
      keySecret,
      credentialVersion,
      type: params.type || "client",
      smartAccess: params.smartAccess ?? true,
      smartStrategy: params.smartStrategy || "cost",
      modelAccess: params.modelAccess ?? "all",
      allowedModels:
        (params.modelAccess ?? "all") === "all"
          ? undefined
          : allowed.map((item) => item.id),
      quotaLimit_usd: params.quotaLimit_usd,
      quotaPeriod: params.quotaPeriod || "never",
      currentPeriodUsed_nano: "0",
      usagePeriodStartedAt: new Date().toISOString(),
      totalUsed_nano: "0",
      quota: 0,
      ipWhitelist: params.ipWhitelist || [],
      lastUsedAt: null,
    };

    set((s) => ({
      keys: [...s.keys, newKey],
      selectedKey: newKey.id,
      revealedKeyIds: { ...s.revealedKeyIds, [newKey.id]: Date.now() + 300000 },
    }));

    return newKey;
  },

  updateKey: (id, updates) => {
    if (
      updates.quotaLimit_usd !== undefined &&
      updates.quotaLimit_usd !== null &&
      (!Number.isFinite(updates.quotaLimit_usd) ||
        updates.quotaLimit_usd < 0 ||
        updates.quotaLimit_usd > 9_000_000_000)
    )
      throw new Error("密钥额度须在 0 至 90 亿美元之间");
    if (updates.ipWhitelist?.some((rule) => !validIpRule(rule)))
      throw new Error("IP（网络地址）白名单格式无效");
    set((s) => ({
      keys: s.keys.map((key) => {
        if (key.id !== id) return key;
        const updated = { ...key, ...updates };
        if (updates.quotaPeriod && updates.quotaPeriod !== key.quotaPeriod) {
          const now = new Date().toISOString();
          const date = billingDate(now);
          // New calls are attributable to this credential, so changing a cycle
          // recomputes the correct bucket rather than discarding past usage.
          const records = s.requests.filter(
            (call) => call.key_id === id && call.status === "success",
          );
          const recordedTotal = records.reduce(
            (sum, call) => sum + BigInt(call.cost_nano),
            0n,
          );
          const legacyUnattributed =
            BigInt(key.totalUsed_nano ?? "0") - recordedTotal;
          const used =
            updates.quotaPeriod === "never"
              ? BigInt(key.totalUsed_nano ?? "0")
              : records
                  .filter((call) =>
                    updates.quotaPeriod === "daily"
                      ? billingDate(call.time) === date
                      : billingDate(call.time).slice(0, 7) === date.slice(0, 7),
                  )
                  .reduce(
                    (sum, call) => sum + BigInt(call.cost_nano),
                    legacyUnattributed > 0n ? legacyUnattributed : 0n,
                  );
          updated.currentPeriodUsed_nano = used.toString();
          updated.usagePeriodStartedAt = now;
        }
        return updated;
      }),
    }));
  },

  regenerateKey: (id) => {
    const target = get().keys.find((k) => k.id === id);
    if (!target) return { oldKey: "", newKey: "" };
    const randHex = Math.random().toString(16).substring(2, 6);
    const credentialVersion = newBillingId("v");
    const newKeySecret = demoKeySecret(id, credentialVersion);
    const newMasked = `sk-xy-••••${randHex}`;

    set((s) => ({
      keys: s.keys.map((k) =>
        k.id === id
          ? {
              ...k,
              keySecret: newKeySecret,
              credentialVersion,
              keyMasked: newMasked,
            }
          : k,
      ),
      revealedKeyIds: { ...s.revealedKeyIds, [id]: Date.now() + 300000 },
    }));

    return { oldKey: target.keySecret, newKey: newKeySecret };
  },

  deleteKey: (id) =>
    set((s) => ({
      keys: s.keys.filter((k) => k.id !== id),
      selectedKey:
        s.selectedKey === id
          ? s.keys.find((k) => k.id !== id)?.id || ""
          : s.selectedKey,
    })),

  toggleKey: (id) =>
    set((s) => ({
      keys: s.keys.map((k) => (k.id === id ? { ...k, active: !k.active } : k)),
    })),

  chooseKey: (selectedKey) => set({ selectedKey }),

  verifyPasswordAndRevealKey: async (keyId, password) => {
    const owner = get().user.id;
    if (
      !get().user.isLoggedIn ||
      get().user.accountStatus === "deactivated" ||
      !get().keys.some((key) => key.id === keyId)
    )
      return false;
    // Verify the password through the demo auth service. The real
    // security boundary is the backend session; this UI-level gate
    // prevents casual shoulder-surfing. Console-preview.tsx calls
    // POST /api/user/login instead, but this method is used by
    // tests and the demo flow.
    const ok = await demoAuth
      .verifyPassword(owner, password)
      .catch(() => false);
    if (!ok) return false;
    set((s) => ({
      revealedKeyIds: { ...s.revealedKeyIds, [keyId]: Date.now() + 300000 },
    }));
    return true;
  },

  recordCall: (id, failed, requestedModel, submittedRequest) => {
    get().syncBillingStorage();
    const state = get();
    const key = state.keys.find((k) => k.id === id);
    const fallbackModel =
      key?.type === "smart" && key.smartAccess === undefined
        ? SMART_MODEL_ID
        : (key?.model ?? "gpt-4.1");
    const request = structuredClone(
      submittedRequest ?? defaultDemoRequest(requestedModel ?? fallbackModel),
    );
    // The passed request is the canonical source. Conflicting call-site values are errors.
    const valid =
      validateDemoRequest(request) &&
      (!requestedModel || requestedModel === request.model);
    const nowIso = new Date().toISOString();
    const pricing = getPricingSnapshot();
    let error: string | undefined = !valid
      ? "invalid_request"
      : !key
        ? "key_not_found"
        : !key.active
          ? "key_disabled"
          : state.user.accountStatus !== "normal"
            ? "account_unavailable"
            : key.validUntil && Date.parse(key.validUntil) <= Date.now()
              ? "key_expired"
              : undefined;
    if (
      !error &&
      key?.ipWhitelist?.length &&
      !key.ipWhitelist.some((rule) => matchesIpRule("127.0.0.1", rule))
    )
      error = "ip_unauthorized";
    const chainKey = request.chainId ? `${id}:${request.chainId}` : undefined;
    const resolution =
      key && !error
        ? resolveKeyModelResult(key, request.model, pricing, {
            protocol: request.protocol,
            capability: request.capability,
            chainModel: chainKey ? state.toolChains[chainKey] : undefined,
            inputTokens: demoInputTokens(request),
            maxTokens: request.max_tokens,
            at: nowIso,
          })
        : undefined;
    error ??= resolution?.error;
    const chosenModel = resolution?.model;
    const simulation = chosenModel
      ? simulateDemoResponse(request, chosenModel.id)
      : undefined;
    let charge: ReturnType<typeof estimateModelCharge> | undefined;
    let estimate: ReturnType<typeof estimateModelCharge> | undefined;
    let pricingError: string | undefined;
    if (chosenModel && simulation && key) {
      try {
        charge = estimateModelCharge(
          {
            modelId: chosenModel.id,
            tierId: key.tier,
            inputTokens: simulation.inputTokens,
            outputTokens: simulation.outputTokens,
            at: nowIso,
          },
          pricing,
        );
        estimate = estimateModelCharge(
          {
            modelId: chosenModel.id,
            tierId: key.tier,
            inputTokens: simulation.inputTokens,
            // M4 §3 (BIL-3): 预扣输出按 min(最大输出, 4096) 估算；未指定最大输出时按 4096。
            outputTokens: Math.min(request.max_tokens, 4096),
            at: nowIso,
          },
          pricing,
        );
        // Arbitrary arithmetic need not grow with output count: always cover the actual simulated amount.
        if (BigInt(charge.amountNano) > BigInt(estimate.amountNano))
          estimate = charge;
      } catch (problem) {
        error = "pricing_invalid";
        pricingError = (problem as Error).message;
      }
    }
    const currentUsed = key ? keyPeriodUsed(key, nowIso) : 0n;
    const limit =
      key?.quotaLimit_usd === undefined || key.quotaLimit_usd === null
        ? null
        : decimalUsdNano(key.quotaLimit_usd);
    if (
      !error &&
      limit !== null &&
      currentUsed + BigInt(estimate?.amountNano ?? "0") > limit
    )
      error = "key_quota_exceeded";
    if (
      !error &&
      (BigInt(state.balance) <= 0n ||
        BigInt(state.balance) < BigInt(estimate?.amountNano ?? "0"))
    )
      error = "insufficient_balance";
    if (!error && (failed || state.demoMode.upstreamOutage))
      error = "upstream_unavailable";
    const cost = error ? 0n : BigInt(charge?.amountNano ?? "0");
    const rate =
      charge?.snapshot.rule.mode === "tokens" ? charge.rate : undefined;
    const call: PreviewRequest = {
      trace_id: newBillingId("tr_demo"),
      time: nowIso,
      source: "playground",
      tier: key?.tier ?? "pro",
      key_id: id,
      requested_model: request.model,
      actual_model: !error ? (chosenModel?.id ?? null) : null,
      model: chosenModel?.id ?? request.model,
      vendor: chosenModel?.vendor ?? "—",
      status: error ? "failed" : "success",
      error_code: error ?? null,
      error_message: error
        ? `${requestErrors[error]?.[0] ?? "请求未完成。"}${pricingError ? ` ${pricingError}` : ""}`
        : undefined,
      recovery_action: error
        ? (requestErrors[error]?.[1] ?? "保留当前输入后重试。")
        : undefined,
      cost_nano: cost.toString(),
      billing_status: error ? "not_charged" : "charged",
      input_tokens: error ? 0 : simulation!.inputTokens,
      output_tokens: error ? 0 : simulation!.outputTokens,
      cache_tokens: 0,
      input_price_nano: rateUsdToNano(rate?.input),
      output_price_nano: rateUsdToNano(rate?.output),
      resource_id: resolution?.resource?.id,
      channel_id: resolution?.resource?.channelId,
      upstream_model: resolution?.resource?.upstreamModel,
      resource_snapshot_id: pricing.resources?.id,
      pricing_revision: pricing.revision,
      pricing_snapshot: charge?.snapshot,
      metered_usage: charge?.snapshot.usage,
      input_rate_usd: rate?.input,
      output_rate_usd: rate?.output,
      duration_ms: error ? 0 : 180 + (simulation?.inputTokens ?? 0),
      ttft_ms: error ? 0 : 80,
      fallback: false,
      protocol: request.protocol,
      chain_id: request.chainId,
      request,
      response_text: !error ? simulation?.text : undefined,
    };
    const newBalance = (BigInt(state.balance) - cost).toString();
    const nextUsed = currentUsed + cost;
    set({
      requests: [call, ...state.requests],
      balance: newBalance,
      toolChains:
        !error && chainKey && chosenModel
          ? { ...state.toolChains, [chainKey]: chosenModel.id }
          : state.toolChains,
      ledger:
        cost > 0n
          ? [
              {
                id: newBillingId("tx_call"),
                time: nowIso,
                type: "call_summary",
                amount_nano: (-cost).toString(),
                balance_after_nano: newBalance,
                description: `模拟调用 · ${request.model} → ${chosenModel?.id}`,
                trace_id: call.trace_id,
                idempotency_key: `call:${call.trace_id}`,
              },
              ...state.ledger,
            ]
          : state.ledger,
      keys: key
        ? state.keys.map((k) =>
            k.id === id
              ? {
                  ...k,
                  currentPeriodUsed_nano: nextUsed.toString(),
                  usagePeriodStartedAt: nowIso,
                  totalUsed_nano: (
                    BigInt(k.totalUsed_nano ?? "0") + cost
                  ).toString(),
                  quota:
                    limit && limit > 0n
                      ? Math.min(100, Number((nextUsed * 100n) / limit))
                      : limit === 0n
                        ? 100
                        : 0,
                  lastUsedAt: nowIso,
                }
              : k,
          )
        : state.keys,
    });
    return call;
  },

  recordMeteredCall: (id, modelId, usage, at, attributes) => {
    get().syncBillingStorage();
    const state = get(),
      key = state.keys.find((item) => item.id === id);
    const nowIso = new Date().toISOString(),
      pricing = getPricingSnapshot();
    // Quote timestamps can exercise time windows, but cannot move the key's quota period backwards.
    let error: string | undefined = !key
      ? "key_not_found"
      : !key.active
        ? "key_disabled"
        : state.user.accountStatus !== "normal"
          ? "account_unavailable"
          : key.validUntil && Date.parse(key.validUntil) <= Date.now()
            ? "key_expired"
            : undefined;
    if (
      !error &&
      key?.ipWhitelist?.length &&
      !key.ipWhitelist.some((rule) => matchesIpRule("127.0.0.1", rule))
    )
      error = "ip_unauthorized";
    const resolution =
      key && !error ? resolveKeyModelResult(key, modelId, pricing) : undefined;
    error ??= resolution?.error;
    // This explicit settlement verifier never silently performs smart routing with missing capabilities.
    if (!error && isSmartModelId(modelId)) error = "capability_incompatible";
    const chosen = resolution?.model;
    let quote: ReturnType<typeof quoteModelCharge> | undefined,
      pricingError: string | undefined;
    if (!error && chosen && key) {
      try {
        quote = quoteModelCharge(
          {
            modelId: chosen.id,
            tierId: key.tier,
            usage,
            attributes,
            at: at ?? nowIso,
          },
          pricing,
        );
      } catch (problem) {
        error = "pricing_invalid";
        pricingError = (problem as Error).message;
      }
    }
    const currentUsed = key ? keyPeriodUsed(key, nowIso) : 0n;
    const limit =
      key?.quotaLimit_usd === undefined || key.quotaLimit_usd === null
        ? null
        : decimalUsdNano(key.quotaLimit_usd);
    const calculated = BigInt(quote?.amountNano ?? "0");
    if (!error && limit !== null && currentUsed + calculated > limit)
      error = "key_quota_exceeded";
    if (
      !error &&
      (BigInt(state.balance) <= 0n || BigInt(state.balance) < calculated)
    )
      error = "insufficient_balance";
    if (!error && state.demoMode.upstreamOutage) error = "upstream_unavailable";
    const cost = error ? 0n : calculated;
    const call: PreviewRequest = {
      trace_id: newBillingId("tr_metered"),
      time: nowIso,
      source: "playground",
      tier: key?.tier ?? "pro",
      key_id: id,
      requested_model: modelId,
      actual_model: error ? null : chosen!.id,
      model: chosen?.id ?? modelId,
      vendor: chosen?.vendor ?? "—",
      status: error ? "failed" : "success",
      error_code: error ?? null,
      error_message: error
        ? `${requestErrors[error]?.[0] ?? "计费验证失败；未计费。"}${pricingError ? ` ${pricingError}` : ""}`
        : undefined,
      recovery_action: error
        ? (requestErrors[error]?.[1] ?? "保留用量输入，修正后重试。")
        : undefined,
      cost_nano: cost.toString(),
      billing_status: error ? "not_charged" : "charged",
      input_tokens: error ? 0 : (usage.input_tokens ?? 0),
      output_tokens: error ? 0 : (usage.output_tokens ?? 0),
      cache_tokens: error
        ? 0
        : (usage.cache_read_tokens ?? 0) + (usage.cache_write_tokens ?? 0),
      input_price_nano:
        quote?.snapshot.rule.mode === "tokens"
          ? rateUsdToNano(quote.rate.input)
          : "0",
      output_price_nano:
        quote?.snapshot.rule.mode === "tokens"
          ? rateUsdToNano(quote.rate.output)
          : "0",
      input_rate_usd:
        quote?.snapshot.rule.mode === "tokens" ? quote.rate.input : undefined,
      output_rate_usd:
        quote?.snapshot.rule.mode === "tokens" ? quote.rate.output : undefined,
      resource_id: resolution?.resource?.id,
      channel_id: resolution?.resource?.channelId,
      upstream_model: resolution?.resource?.upstreamModel,
      resource_snapshot_id: pricing.resources?.id,
      pricing_revision: pricing.revision,
      pricing_snapshot: quote?.snapshot,
      metered_usage: structuredClone(usage),
      metered_demo: true,
      metered_attributes: structuredClone(attributes),
      duration_ms: 0,
      ttft_ms: 0,
      fallback: false,
      response_text: error
        ? undefined
        : "【本地计费验证】按输入用量生成演示账单；未调用模型，未生成文字、图片或视频。",
    };
    const newBalance = (BigInt(state.balance) - cost).toString(),
      nextUsed = currentUsed + cost;
    set({
      requests: [call, ...state.requests],
      balance: newBalance,
      ledger:
        cost > 0n
          ? [
              {
                id: newBillingId("tx_call"),
                time: nowIso,
                type: "call_summary",
                amount_nano: (-cost).toString(),
                balance_after_nano: newBalance,
                description: `模拟计费验证 · ${modelId}（未生成内容）`,
                trace_id: call.trace_id,
                idempotency_key: `call:${call.trace_id}`,
              },
              ...state.ledger,
            ]
          : state.ledger,
      keys: key
        ? state.keys.map((item) =>
            item.id === id
              ? {
                  ...item,
                  currentPeriodUsed_nano: nextUsed.toString(),
                  usagePeriodStartedAt: nowIso,
                  totalUsed_nano: (
                    BigInt(item.totalUsed_nano ?? "0") + cost
                  ).toString(),
                  quota:
                    limit && limit > 0n
                      ? Math.min(100, Number((nextUsed * 100n) / limit))
                      : limit === 0n
                        ? 100
                        : 0,
                  lastUsedAt: nowIso,
                }
              : item,
          )
        : state.keys,
    });
    return call;
  },

  createRechargeOrder: (amountUsd, channel = "card") => {
    get().syncBillingStorage();
    const amount = validateRechargeAmount(amountUsd);
    if (!amount.ok) return amount;
    if (channel !== "card") return { ok: false, error: "unsupported_channel" };
    const now = Date.now();
    const order: RechargeOrder = {
      id: newBillingId("ord"),
      amount_usd: amount.amount_usd,
      amount_nano: amount.amount_nano,
      currency: "USD",
      status: "pending",
      channel,
      created_at: new Date(now).toISOString(),
      expires_at: new Date(now + RECHARGE_EXPIRY_MS).toISOString(),
      provider_order_id: newBillingId("demo_pay"),
    };
    set((s) => ({ rechargeOrders: [order, ...s.rechargeOrders] }));
    return { ok: true, order };
  },

  beginRechargePayment: (id) => {
    get().expireRechargeOrders();
    const order = get().rechargeOrders.find((item) => item.id === id);
    if (!order) return { ok: false, error: "order_not_found" };
    if (order.status === "processing")
      return { ok: true, order, alreadyProcessed: true };
    if (order.status !== "pending")
      return { ok: false, error: "order_not_payable" };
    const next: RechargeOrder = { ...order, status: "processing" };
    set((s) => ({
      rechargeOrders: s.rechargeOrders.map((item) =>
        item.id === id ? next : item,
      ),
    }));
    return { ok: true, order: next };
  },

  resolveRechargeOrder: (id, outcome, options) => {
    get().syncBillingStorage();
    const state = get();
    const order = state.rechargeOrders.find((item) => item.id === id);
    if (!order) return { ok: false, error: "order_not_found" };
    const result = transitionRechargeOrder(
      order,
      outcome,
      options?.now ?? Date.now(),
    );
    if (!result.ok) return result;
    const paymentKey = `payment:${order.provider_order_id}`;
    const alreadyCredited = state.ledger.some(
      (item) =>
        item.idempotency_key === paymentKey ||
        (item.type === "recharge" && item.order_id === order.id),
    );
    const shouldCredit =
      outcome === "success" &&
      result.order.status === "paid" &&
      !alreadyCredited &&
      !result.alreadyProcessed;
    const amount = shouldCredit ? BigInt(result.order.amount_nano) : 0n;
    const newBalance = (BigInt(state.balance) + amount).toString();
    const entry: LedgerEntry | undefined = shouldCredit
      ? {
          id: newBillingId("tx"),
          time: result.order.paid_at!,
          type: "recharge",
          amount_nano: amount.toString(),
          balance_after_nano: newBalance,
          description: "Demo online recharge",
          order_id: order.id,
          idempotency_key: paymentKey,
        }
      : undefined;
    set({
      rechargeOrders: state.rechargeOrders.map((item) =>
        item.id === id ? result.order : item,
      ),
      balance: newBalance,
      opening: (BigInt(state.opening) + amount).toString(),
      ledger: entry ? [entry, ...state.ledger] : state.ledger,
      demoMode: {
        ...state.demoMode,
        balanceState:
          BigInt(newBalance) <= 0n
            ? "zero"
            : BigInt(newBalance) < 5000000000n
              ? "low"
              : "normal",
      },
    });
    return {
      ...result,
      alreadyProcessed:
        result.alreadyProcessed || (outcome === "success" && alreadyCredited),
    };
  },

  expireRechargeOrders: (now = Date.now()) => {
    if (!Number.isFinite(now)) return;
    get().syncBillingStorage();
    const current = get().rechargeOrders;
    const next = current.map((order) => expireRechargeOrder(order, now));
    if (next.some((order, index) => order !== current[index])) {
      set({ rechargeOrders: next });
    }
  },

  syncBillingStorage: () => {
    const snapshot = readBillingStorage();
    if (!snapshot) return false;
    const signature = JSON.stringify(snapshot);
    if (signature === appliedBillingSignature) return true;
    const requests: PreviewRequest[] = [
      ...snapshot.liveRequests,
      ...(snapshot.sampleRequestsEnabled ? initialRequests : []),
    ];
    // Receiving another tab's wallet must not echo it back to localStorage.
    // Expiration is applied separately by expireRechargeOrders after this read.
    receivingBillingSnapshot = true;
    try {
      set({
        balance: snapshot.balance,
        accountProfiles: snapshot.accountProfiles ?? {},
        ledger: snapshot.ledger,
        rechargeOrders: snapshot.rechargeOrders,
        requests,
        ...(snapshot.keys
          ? {
              keys: snapshot.keys.map((key) => ({
                ...key,
                keySecret: demoKeySecret(key.id, key.credentialVersion),
              })),
              selectedKey: snapshot.selectedKey ?? "",
            }
          : {}),
        ...(snapshot.user
          ? {
              user: {
                ...get().user,
                ...snapshot.user,
                id: snapshot.user.id ?? accountIdForLegacy(snapshot.user),
                createdAt: snapshot.user.createdAt,
                passwordUpdatedAt: snapshot.user.passwordUpdatedAt,
                securityEvents: snapshot.user.securityEvents ?? [],
                sessions: snapshot.user.sessions ?? [currentDemoSession()],
                username: snapshot.user.username,
                phone: snapshot.user.phone,
                emailVerified: snapshot.user.emailVerified,
              },
            }
          : {}),
        scenario: snapshot.scenario ?? get().scenario,
        toolChains: snapshot.toolChains ?? {},
        ...(snapshot.adminTopUps !== undefined
          ? { adminReviews: snapshot.adminTopUps }
          : {}),
        opening: (
          BigInt(snapshot.balance) +
          BigInt(sumNano(requests.map((row) => row.cost_nano)))
        ).toString(),
        demoMode: {
          ...get().demoMode,
          accountState: snapshot.sampleRequestsEnabled ? "mature" : "new",
          balanceState:
            BigInt(snapshot.balance) <= 0n
              ? "zero"
              : BigInt(snapshot.balance) < 5000000000n
                ? "low"
                : "normal",
        },
      });
      appliedBillingSignature = signature;
    } finally {
      receivingBillingSnapshot = false;
    }
    return true;
  },

  // Compatibility entry point for older demo screens. Both steps use the same
  // validated order and callback path as the explicit checkout simulation.
  rechargeAccount: (amount_usd) => {
    const created = get().createRechargeOrder(String(amount_usd));
    if (!created.ok) throw new Error(created.error);
    const result = get().resolveRechargeOrder(created.order.id, "success");
    if (!result.ok) throw new Error(result.error);
    return result.order;
  },

  requestExport: (type, rangeDays) => {
    get().syncBillingStorage();
    if (!Number.isInteger(rangeDays) || rangeDays < 1 || rangeDays > 31) {
      throw new RangeError("Exports require 1–31 inclusive calendar days");
    }
    const now = new Date();
    const nowIso = now.toISOString();
    const expiresAt = new Date(now.getTime() + 7 * 86400000).toISOString();
    const id = `exp_${(get().exports.length + 1).toString().padStart(2, "0")}`;
    // Every range includes today. Ledger uses Dubai dates; calls and usage use explicit UTC dates.
    const startDate =
      type === "ledger"
        ? billingDate(
            new Date(now.getTime() - (rangeDays - 1) * 86400000).toISOString(),
          )
        : new Date(now.getTime() - (rangeDays - 1) * 86400000)
            .toISOString()
            .slice(0, 10);
    const endDate =
      type === "ledger" ? billingDate(nowIso) : nowIso.slice(0, 10);

    const rows = filterExportRequests(get().requests, {
      startDate,
      endDate,
      createdAt: nowIso,
    });
    const ledgerRows =
      type === "ledger"
        ? filterLedger(get().ledger, { from: startDate, to: endDate })
        : [];
    const content =
      type === "requests"
        ? serializeRequestExport(rows, get().keys)
        : type === "usage"
          ? serializeUsageExport(rows)
          : serializeLedgerCsv(
              ledgerRows,
              writtenLanguage(usePreferences.getState().language),
            );
    const newRecord: ExportRecord = {
      id,
      type,
      rangeDays,
      startDate,
      endDate,
      createdAt: nowIso,
      expiresAt,
      rowCount:
        type === "requests"
          ? rows.length
          : type === "usage"
            ? aggregateRequestUsage(rows).length
            : ledgerRows.length,
      sizeKb: Number(
        (new TextEncoder().encode(content).byteLength / 1024).toFixed(2),
      ),
      status: "completed",
      filename: `${type}_${endDate}.csv`,
    };

    set((s) => ({ exports: [newRecord, ...s.exports] }));
    return newRecord;
  },

  login: (
    email,
    enterpriseName = DEMO_ACCOUNT_PROFILE.enterpriseName,
    profile,
  ) => {
    const id = profile?.id ?? accountIdForLegacy({ ...profile, email });
    set((s) => ({
      accountProfiles: {
        ...s.accountProfiles,
        [s.user.id]: profileMetadata(s.user),
      },
      revealedKeyIds: {},
      user: {
        ...s.user,
        ...(id === s.user.id
          ? profileMetadata(s.user)
          : (s.accountProfiles[id] ?? {
              createdAt: undefined,
              passwordUpdatedAt: undefined,
              securityEvents: [],
              sessions: [currentDemoSession()],
            })),
        id,
        email,
        enterpriseName,
        username: profile?.username,
        phone: profile?.phone,
        emailVerified: profile?.emailVerified,
        isLoggedIn: true,
      },
    }));
  },

  register: (email, enterpriseName, profile) => {
    set((s) => ({
      accountProfiles: {
        ...s.accountProfiles,
        [s.user.id]: profileMetadata(s.user),
      },
      user: {
        ...s.user,
        id: profile?.id ?? accountIdForLegacy({ ...profile, email }),
        email,
        enterpriseName,
        username: profile?.username,
        phone: profile?.phone,
        emailVerified: profile?.emailVerified,
        createdAt: new Date().toISOString(),
        passwordUpdatedAt: undefined,
        securityEvents: [],
        sessions: [currentDemoSession()],
        isLoggedIn: true,
        onboardingCompleted: false,
        accountStatus: "normal",
      },
      balance: "0",
      opening: "0",
      requests: [],
      keys: [],
      selectedKey: "",
      scenario: "first",
      toolChains: {},
      revealedKeyIds: {},
      exports: [],
      ledger: [],
      rechargeOrders: [],
      demoMode: { ...s.demoMode, accountState: "new", balanceState: "zero" },
    }));
  },

  applyAccountSecurity: (identity, action) => {
    get().syncBillingStorage();
    if (
      identity.id !== get().user.id ||
      !get().user.isLoggedIn ||
      get().user.accountStatus === "deactivated"
    )
      throw new Error("ACCOUNT_CHANGED");
    const time = new Date().toISOString();
    set((s) => ({
      user: {
        ...s.user,
        username: identity.username,
        email: identity.email ?? "",
        phone: identity.phone,
        emailVerified: identity.emailVerified,
        passwordUpdatedAt:
          action === "password_changed" ? time : s.user.passwordUpdatedAt,
        sessions: [currentDemoSession()],
        securityEvents: [
          { id: newBillingId("security"), time, action },
          ...s.user.securityEvents,
        ].slice(0, 30),
      },
      revealedKeyIds: {},
    }));
  },

  logout: () => {
    set((s) => ({
      revealedKeyIds: {},
      user: {
        ...s.user,
        isLoggedIn: false,
      },
    }));
  },

  completeOnboarding: () => {
    set((s) => ({
      user: { ...s.user, onboardingCompleted: true },
    }));
  },

  updateEnterpriseName: (name) => {
    set((s) => ({
      user: { ...s.user, enterpriseName: name },
    }));
  },

  terminateOtherSessions: () => {
    set((s) => ({
      user: {
        ...s.user,
        sessions: s.user.sessions.filter((sess) => sess.isCurrent),
        securityEvents: s.user.sessions.some((sess) => !sess.isCurrent)
          ? [
              {
                id: newBillingId("security"),
                time: new Date().toISOString(),
                action: "sessions_revoked" as const,
              },
              ...s.user.securityEvents,
            ].slice(0, 30)
          : s.user.securityEvents,
      },
    }));
  },

  deactivateAccount: () => {
    get().syncBillingStorage();
    const currentBalance = BigInt(get().balance);
    const nowIso = new Date().toISOString();
    let cleared_nano = "0";

    // Dust balance < $0.01 (10^7 nano) can be cleared automatically
    if (currentBalance > 0n && currentBalance < 10000000n) {
      cleared_nano = currentBalance.toString();
      const ledgerEntry: LedgerEntry = {
        id: `tx_deact_${Math.floor(1000 + Math.random() * 9000)}`,
        time: nowIso,
        // M5 §6 + M4 §4：注销清零归入「手动调整」，description 固定为「注销清零」，
        // 不再单列一个账本类型（原 deactivation_clear 已随本次改动合并）。
        type: "adjustment",
        amount_nano: (-currentBalance).toString(),
        balance_after_nano: "0",
        description: "注销清零",
      };
      set((s) => ({
        balance: "0",
        opening: (BigInt(s.opening) - currentBalance).toString(),
        ledger: [ledgerEntry, ...s.ledger],
        user: { ...s.user, accountStatus: "deactivated", isLoggedIn: false },
      }));
    } else {
      set((s) => ({
        user: { ...s.user, accountStatus: "deactivated", isLoggedIn: false },
      }));
    }

    return { cleared_nano };
  },

  // #105：单人加款，提交即入账，不再按金额分流到待复核（原 >$1,000 走 pending 的分支已删）。
  adminCreateTopUp: (params) => {
    get().syncBillingStorage();
    const nanoAmount = BigInt(Math.round(params.amount_usd * 1_000_000_000));
    const nowIso = new Date().toISOString();
    const id = `rev_${Math.floor(100 + Math.random() * 900)}`;

    const topUp: AdminTopUpRecord = {
      id,
      enterpriseName: get().user.enterpriseName,
      amount_usd: params.amount_usd.toFixed(2),
      amount_nano: nanoAmount.toString(),
      type: params.type,
      reason: params.reason,
      operator: params.operator,
      created_at: nowIso,
    };

    const newBalance = (BigInt(get().balance) + nanoAmount).toString();
    const ledgerEntry: LedgerEntry = {
      id: `tx_adj_${Math.floor(1000 + Math.random() * 9000)}`,
      time: nowIso,
      type: "adjustment",
      amount_nano: nanoAmount.toString(),
      balance_after_nano: newBalance,
      // 操作人写进流水描述，作为单人加款的追责依据（M4 §5.2）。
      description: `手动加款（${params.type}：${params.amount_usd.toFixed(2)} 美元，操作人 ${params.operator}）`,
    };
    set((s) => ({
      balance: newBalance,
      opening: (BigInt(s.opening) + nanoAmount).toString(),
      ledger: [ledgerEntry, ...s.ledger],
      adminReviews: [topUp, ...s.adminReviews],
    }));

    return topUp;
  },

  adminUpdateConcurrency: (concurrencyLimit) => {
    set((s) => ({ user: { ...s.user, concurrencyLimit } }));
  },

  adminToggleAccountStatus: () => {
    set((s) => ({
      user: {
        ...s.user,
        accountStatus: s.user.accountStatus === "normal" ? "frozen" : "normal",
      },
    }));
  },

  adminPublishIncident: (incident) => {
    set((s) => ({
      incidents: [incident, ...s.incidents.filter((i) => i.id !== incident.id)],
    }));
  },

  resetScenario: (scenario) => {
    const session = useSystemAccess.getState().session;
    const authenticated = session.accountId !== null;
    const confirmedGuest = session.epoch > 0 && !authenticated;
    try {
      for (const name of [
        "veyra-key-draft-v2",
        "veyra-request-draft-v2",
        "veyra-connect-draft-v1",
        "veyra-settings-models-draft-v2",
        "veyra-settings-groups-draft-v2",
        "veyra-metered-retry-v1",
        "veyra-metered-simulator-v1",
      ])
        sessionStorage.removeItem(name);
      Object.keys(sessionStorage)
        .filter((name) => name.startsWith("veyra-settings-models-draft-v2-"))
        .forEach((name) => sessionStorage.removeItem(name));
    } catch {
      /* Reset still works without optional draft storage. */
    }
    {
      const config = createDefaultPricing();
      useModelPricing.setState({ config });
      try {
        localStorage.setItem(PRICING_STORAGE_KEY, JSON.stringify(config));
      } catch {
        /* Current tab still supports the demo. */
      }
    }
    const isFirst = scenario === "first";
    // first：新用户首屏，调用记录为空。
    // enterprise / pricing：恢复种子数据，概览、调用明细、数据导出等页面有可演示内容。
    const resetRequests = isFirst ? [] : initialRequests;
    // 种子记录已经产生了消费，当前余额必须是「期初 − 种子消费」，否则
    // opening − Σ(requests.cost_nano) ≠ balance，账本自检对不上。
    const balance = isFirst
      ? "0"
      : (BigInt(seedOpeningNano) - seedRequestsCost).toString();
    set((s) => ({
      scenario,
      accountProfiles: {
        ...s.accountProfiles,
        [s.user.id]: profileMetadata(s.user),
      },
      requests: resetRequests,
      keys: [],
      selectedKey: "",
      toolChains: {},
      revealedKeyIds: {},
      exports: [],
      rechargeOrders: [],
      adminReviews: [],
      incidents: [],
      balance,
      opening: isFirst ? "0" : seedOpeningNano,
      ledger: isFirst
        ? []
        : [
            {
              id: newBillingId("tx_scenario"),
              time: new Date().toISOString(),
              type: "adjustment",
              amount_nano: balance,
              balance_after_nano: balance,
              description: "演示场景初始赠送余额（模拟资金）",
            },
          ],
      user: authenticated
        ? s.user
        : {
            ...s.user,
            ...DEMO_ACCOUNT_PROFILE,
            emailVerified: undefined,
            ...(s.user.id === DEMO_ACCOUNT_PROFILE.id
              ? profileMetadata(s.user)
              : (s.accountProfiles[DEMO_ACCOUNT_PROFILE.id] ?? {
                  createdAt: undefined,
                  passwordUpdatedAt: undefined,
                  securityEvents: [],
                  sessions: [currentDemoSession()],
                })),
            isLoggedIn: !confirmedGuest,
            onboardingCompleted: !isFirst,
            accountStatus: "normal",
          },
      demoMode: {
        accountState: isFirst ? "new" : "mature",
        balanceState: isFirst ? "zero" : "normal",
        upstreamOutage: false,
        uiState: "normal",
      },
    }));
    // 立刻把重置后的场景落盘：否则后面 createKeyDetailed / recordCall 里的
    // syncBillingStorage 会读回上一次场景的旧快照，把 scenario 和 requests 覆盖回去
    // （现象：重置到 first 后 store.scenario 又变回 enterprise）。
    const resetSnapshot = billingSnapshot(get());
    if (writeBillingStorage(resetSnapshot))
      appliedBillingSignature = JSON.stringify(resetSnapshot);
    // 场景内的密钥由这里现建：企业场景给两把不同额度的密钥，
    // 定价场景给一把专门的调价验证密钥并留下一次调用记录。
    if (!isFirst) {
      const production = get().createKeyDetailed({
        name: scenario === "pricing" ? "运营定价验证" : "产品研发 · 多模型",
        tier: "pro",
        model: "gpt-4.1",
        quotaLimit_usd: 50,
        quotaPeriod: "monthly",
      });
      if (scenario === "enterprise")
        get().createKeyDetailed({
          name: "内容生产 · 独立额度",
          tier: "standard",
          model: "deepseek-chat",
          quotaLimit_usd: 10,
          quotaPeriod: "daily",
          modelAccess: "custom",
          allowedModels: ["deepseek-chat", "qwen-plus"],
        });
      get().chooseKey(production.id);
      if (scenario === "pricing")
        get().recordCall(production.id, false, "gpt-4.1");
    }
  },

  demoSetAccountState: (mode) => {
    get().resetScenario(mode === "new" ? "first" : "enterprise");
  },

  demoSetBalanceState: (mode) => {
    get().syncBillingStorage();
    const state = get();
    const target =
      mode === "zero" ? 0n : mode === "low" ? 3500000000n : 128400000000n;
    const change = target - BigInt(state.balance);
    const entry: LedgerEntry | undefined =
      change === 0n
        ? undefined
        : {
            id: newBillingId("tx_demo"),
            time: new Date().toISOString(),
            type: "adjustment",
            amount_nano: change.toString(),
            balance_after_nano: target.toString(),
            description: "Demo scenario balance adjustment",
          };
    set({
      balance: target.toString(),
      opening: (BigInt(state.opening) + change).toString(),
      ledger: entry ? [entry, ...state.ledger] : state.ledger,
      demoMode: { ...state.demoMode, balanceState: mode },
    });
  },

  demoSetUpstreamOutage: (upstreamOutage) => {
    set((s) => ({
      demoMode: { ...s.demoMode, upstreamOutage },
    }));
  },

  demoSetDataState: (uiState) => {
    set((s) => ({
      demoMode: { ...s.demoMode, uiState },
    }));
  },
}));

// Persist demo permission/usage metadata with the wallet atomically. Prompt and
// response content, credential strings, passwords and session data are excluded.
const hasSavedPreview = useProductPreview.getState().syncBillingStorage();
const stopBillingPersistence = useProductPreview.subscribe(
  (state, previous) => {
    if (receivingBillingSnapshot) return;
    if (
      state.balance === previous.balance &&
      state.ledger === previous.ledger &&
      state.rechargeOrders === previous.rechargeOrders &&
      state.requests === previous.requests &&
      state.keys === previous.keys &&
      state.selectedKey === previous.selectedKey &&
      state.scenario === previous.scenario &&
      state.toolChains === previous.toolChains &&
      state.user === previous.user &&
      state.accountProfiles === previous.accountProfiles
    )
      return;
    const snapshot = billingSnapshot(state);
    if (writeBillingStorage(snapshot))
      appliedBillingSignature = JSON.stringify(snapshot);
  },
);
const stopBillingStorageSync = listenBillingStorage(() => {
  useProductPreview.getState().syncBillingStorage();
});
// 没有可用的历史存档时（首次进入，或存档损坏被 parseBillingSnapshot 原子拒绝），
// 不能把 initialState 退化成空账户——那样首屏会是一张白纸。改为把 store 里
// 已经带着种子数据的 initialState 直接落盘，让后续订阅与刷新都从这份数据接着走。
if (!hasSavedPreview) {
  const seeded = billingSnapshot(useProductPreview.getState());
  if (writeBillingStorage(seeded))
    appliedBillingSignature = JSON.stringify(seeded);
}
useProductPreview.getState().expireRechargeOrders();

// Vite disposes the previous module before replacing it during development.
const billingHot = (
  import.meta as ImportMeta & {
    hot?: { dispose: (callback: () => void) => void };
  }
).hot;
billingHot?.dispose(() => {
  stopBillingPersistence();
  stopBillingStorageSync();
});

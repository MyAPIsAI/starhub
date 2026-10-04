/** Original local fixtures for the smart-routing interaction demo. No network calls. */
export type RoutingCopy = { zh: string; en: string; ar: string };
export type CandidateStatus = "eligible" | "degraded" | "circuit";
export type RoutingOutcome =
  "smart" | "pre-fallback" | "post-fallback" | "bypassed";
export interface RoutingCandidate {
  id: string;
  name: string;
  provider: string;
  model: string;
  region: string;
  status: CandidateStatus;
  ttft: number;
  errorRate: number;
  capacity: number;
  inputPrice: number;
  outputPrice: number;
  score: number;
  reason: RoutingCopy;
}
export interface RoutingRequest {
  id: string;
  conversationId: string;
  time: string;
  model: string;
  tier: string;
  outcome: RoutingOutcome;
  latency: number;
  cost: number;
  attempts: number;
  candidateId: string;
  snapshotId: string;
}
export interface RoutingSnapshot {
  id: string;
  publishedAt: string;
  instances: number;
  totalInstances: number;
  checksum: string;
  rules: string;
  models: number;
  resources: number;
}
export interface RoutingConfig {
  enabled: boolean;
  experimentRunning: boolean;
  phase: number;
  traffic: number;
  strategy: "balanced" | "cost" | "latency";
  fallback: boolean;
  maxAttempts: number;
  timeoutMs: number;
}
export const defaultRoutingConfig: RoutingConfig = {
  enabled: true,
  experimentRunning: true,
  phase: 2,
  traffic: 5,
  strategy: "balanced",
  fallback: true,
  maxAttempts: 2,
  timeoutMs: 2500,
};
const storageKey = "veyra-routing-demo-v1";
export function loadRoutingConfig(): RoutingConfig {
  try {
    const raw: unknown = JSON.parse(localStorage.getItem(storageKey) || "null");
    if (!raw || typeof raw !== "object") return { ...defaultRoutingConfig };
    const value = raw as Record<string, unknown>;
    const valid =
      typeof value.enabled === "boolean" &&
      typeof value.experimentRunning === "boolean" &&
      Number.isInteger(value.phase) &&
      Number(value.phase) >= 0 &&
      Number(value.phase) <= 5 &&
      typeof value.traffic === "number" &&
      Number.isFinite(value.traffic) &&
      value.traffic >= 0 &&
      value.traffic <= 100 &&
      ["balanced", "cost", "latency"].includes(String(value.strategy)) &&
      typeof value.fallback === "boolean" &&
      Number.isInteger(value.maxAttempts) &&
      Number(value.maxAttempts) >= 1 &&
      Number(value.maxAttempts) <= 3 &&
      Number.isInteger(value.timeoutMs) &&
      Number(value.timeoutMs) >= 500 &&
      Number(value.timeoutMs) <= 10000;
    return valid
      ? {
          enabled: value.enabled as boolean,
          experimentRunning: value.experimentRunning as boolean,
          phase: Number(value.phase),
          traffic: Number(value.traffic),
          strategy: value.strategy as RoutingConfig["strategy"],
          fallback: value.fallback as boolean,
          maxAttempts: Number(value.maxAttempts),
          timeoutMs: Number(value.timeoutMs),
        }
      : { ...defaultRoutingConfig };
  } catch {
    return { ...defaultRoutingConfig };
  }
}
export function saveRoutingConfig(config: RoutingConfig): boolean {
  try {
    localStorage.setItem(storageKey, JSON.stringify(config));
    return true;
  } catch {
    return false;
  }
}
export const routingCandidates: RoutingCandidate[] = [
  {
    id: "res_dx_01",
    name: "Atlas · Dubai",
    provider: "OpenAI",
    model: "gpt-4.1",
    region: "Dubai",
    status: "eligible",
    ttft: 286,
    errorRate: 0.12,
    capacity: 42,
    inputPrice: 1.2,
    outputPrice: 4.8,
    score: 97,
    reason: {
      zh: "模型能力、额度与健康检查均通过；当前综合得分最高。",
      en: "Capability, budget and health checks passed. Highest overall score.",
      ar: "اجتاز فحوص القدرات والميزانية والصحة. أعلى نتيجة إجمالية.",
    },
  },
  {
    id: "res_sg_02",
    name: "Atlas · Singapore",
    provider: "OpenAI",
    model: "gpt-4.1",
    region: "Singapore",
    status: "eligible",
    ttft: 342,
    errorRate: 0.18,
    capacity: 51,
    inputPrice: 1.1,
    outputPrice: 4.4,
    score: 93,
    reason: {
      zh: "健康且价格较低；跨区域首字延迟略高，可作为备用。",
      en: "Healthy and lower priced; slightly higher cross-region latency. Suitable fallback.",
      ar: "سليم وأقل تكلفة، مع زمن استجابة أعلى قليلًا بين المناطق. بديل مناسب.",
    },
  },
  {
    id: "res_dx_03",
    name: "Cedar · Dubai",
    provider: "Anthropic",
    model: "claude-sonnet-4-20250514",
    region: "Dubai",
    status: "eligible",
    ttft: 318,
    errorRate: 0.09,
    capacity: 38,
    inputPrice: 2.1,
    outputPrice: 10.5,
    score: 96,
    reason: {
      zh: "支持流式响应与工具调用；容量充足，满足当前服务等级。",
      en: "Streaming and tool calls supported. Capacity meets the selected service tier.",
      ar: "يدعم البث واستدعاء الأدوات. السعة مناسبة لمستوى الخدمة المحدد.",
    },
  },
  {
    id: "res_fr_04",
    name: "Cedar · Frankfurt",
    provider: "Anthropic",
    model: "claude-sonnet-4-20250514",
    region: "Frankfurt",
    status: "degraded",
    ttft: 1480,
    errorRate: 1.8,
    capacity: 89,
    inputPrice: 1.95,
    outputPrice: 9.75,
    score: 61,
    reason: {
      zh: "近期限流与首字延迟升高，降低优先级；仅在健康资源不足时使用。",
      en: "Recent rate limiting and first-token latency increased. Used only if healthy capacity is insufficient.",
      ar: "ارتفع تحديد المعدل وزمن أول رمز. يُستخدم فقط عند نقص السعة السليمة.",
    },
  },
  {
    id: "res_sg_05",
    name: "Prism · Singapore",
    provider: "Google",
    model: "gemini-2.5-pro",
    region: "Singapore",
    status: "eligible",
    ttft: 264,
    errorRate: 0.14,
    capacity: 47,
    inputPrice: 0.85,
    outputPrice: 6.8,
    score: 95,
    reason: {
      zh: "候选快照有效，能力映射完整；当前延迟处于健康区间。",
      en: "Valid snapshot and complete capability mapping. Latency is within the healthy range.",
      ar: "لقطة صالحة وتطابق كامل للقدرات. زمن الاستجابة ضمن النطاق السليم.",
    },
  },
  {
    id: "res_dx_06",
    name: "Prism · Dubai",
    provider: "Google",
    model: "gemini-2.5-pro",
    region: "Dubai",
    status: "circuit",
    ttft: 3210,
    errorRate: 8.6,
    capacity: 0,
    inputPrice: 0.8,
    outputPrice: 6.4,
    score: 0,
    reason: {
      zh: "连续健康检查失败，已触发本地熔断；恢复检查通过前不参与路由。",
      en: "Repeated health-check failures opened the local circuit. Excluded until recovery checks pass.",
      ar: "أخفق فحص الصحة مرارًا وفتح قاطع الدائرة المحلي. مستبعد حتى اجتياز فحوص التعافي.",
    },
  },
  {
    id: "res_hk_07",
    name: "Vector · Hong Kong",
    provider: "DeepSeek",
    model: "deepseek-chat",
    region: "Hong Kong",
    status: "eligible",
    ttft: 198,
    errorRate: 0.08,
    capacity: 32,
    inputPrice: 0.14,
    outputPrice: 0.28,
    score: 98,
    reason: {
      zh: "输入输出成本低，容量与延迟稳定，适合成本优先策略。",
      en: "Low input and output cost, stable capacity and latency. Suitable for cost-first routing.",
      ar: "تكلفة منخفضة للإدخال والإخراج وسعة واستجابة مستقرتان. مناسب لأولوية التكلفة.",
    },
  },
  {
    id: "res_sg_08",
    name: "Vector · Singapore",
    provider: "DeepSeek",
    model: "deepseek-chat",
    region: "Singapore",
    status: "degraded",
    ttft: 894,
    errorRate: 0.7,
    capacity: 82,
    inputPrice: 0.16,
    outputPrice: 0.32,
    score: 74,
    reason: {
      zh: "容量使用率接近阈值，保留为后备候选并持续观察。",
      en: "Capacity utilization is near the threshold. Retained as a backup under observation.",
      ar: "استخدام السعة قريب من الحد. محفوظ كبديل تحت المراقبة.",
    },
  },
];
const requestSpecs: [string, RoutingOutcome, number, number, string][] = [
  ["gpt-4.1", "smart", 18, 0.00342, "res_dx_01"],
  ["claude-sonnet-4-20250514", "post-fallback", 31, 0.00612, "res_dx_03"],
  ["gemini-2.5-pro", "pre-fallback", 12, 0.00278, "res_sg_05"],
  ["deepseek-chat", "smart", 14, 0.00039, "res_hk_07"],
  ["gpt-4.1", "bypassed", 6, 0.00391, "res_dx_01"],
  ["claude-sonnet-4-20250514", "smart", 19, 0.00648, "res_dx_03"],
  ["gemini-2.5-pro", "smart", 16, 0.00253, "res_sg_05"],
  ["deepseek-chat", "smart", 13, 0.00044, "res_hk_07"],
  ["gpt-4.1", "post-fallback", 36, 0.00378, "res_sg_02"],
  ["claude-sonnet-4-20250514", "bypassed", 7, 0.00732, "res_dx_03"],
  ["gemini-2.5-pro", "smart", 17, 0.00291, "res_sg_05"],
  ["gpt-4.1", "smart", 15, 0.00326, "res_dx_01"],
];
export const routingRequests: RoutingRequest[] = requestSpecs.map(
  ([model, outcome, latency, cost, candidateId], index) => ({
    id: `req_xy_${String(240912 - index).padStart(6, "0")}`,
    conversationId: `conv_xy_${String(8106 - Math.floor(index / 2)).padStart(4, "0")}`,
    time: `14:${String(29 - index * 2).padStart(2, "0")}:${String((42 - index * 3) % 60).padStart(2, "0")}`,
    model,
    tier: index % 3 === 0 ? "Pro" : "Standard",
    snapshotId: "snap-20260924.03",
    outcome,
    latency,
    cost,
    attempts: outcome === "post-fallback" ? 3 : 1,
    candidateId,
  }),
);
export const routingSnapshots: RoutingSnapshot[] = [
  {
    id: "snap-20260924.03",
    publishedAt: "2026-09-24 14:25:00",
    instances: 8,
    totalInstances: 8,
    checksum: "b47e9d20…8f61",
    rules: "route-rules-v2.3",
    models: 4,
    resources: 8,
  },
  {
    id: "snap-20260924.02",
    publishedAt: "2026-09-24 14:10:00",
    instances: 8,
    totalInstances: 8,
    checksum: "c12608ab…730d",
    rules: "route-rules-v2.2",
    models: 4,
    resources: 8,
  },
  {
    id: "snap-20260924.01",
    publishedAt: "2026-09-24 13:55:00",
    instances: 8,
    totalInstances: 8,
    checksum: "9a62fe08…ed43",
    rules: "route-rules-v2.2",
    models: 4,
    resources: 8,
  },
];
export const routingTrend = [
  412, 436, 458, 431, 478, 504, 492, 528, 551, 534, 563, 582,
].map((requests, index) => ({
  time: `${Math.floor((13 * 60 + 30 + index * 5) / 60)}:${String((30 + index * 5) % 60).padStart(2, "0")}`,
  requests,
  success: [
    99.52, 99.61, 99.56, 99.7, 99.65, 99.74, 99.68, 99.82, 99.77, 99.86, 99.81,
    99.89,
  ][index],
  smartSuccess: [
    99.24, 99.38, 99.3, 99.45, 99.43, 99.56, 99.5, 99.62, 99.61, 99.71, 99.66,
    99.74,
  ][index],
}));

export function filterRoutingCandidates(
  query: string,
  status = "all",
  model = "all",
) {
  const needle = query.trim().toLowerCase();
  return routingCandidates.filter(
    (candidate) =>
      (status === "all" || candidate.status === status) &&
      (model === "all" || candidate.model === model) &&
      [candidate.id, candidate.name, candidate.provider, candidate.model].some(
        (value) => value.toLowerCase().includes(needle),
      ),
  );
}
export function findRoutingRequests(query: string) {
  const needle = query.trim().toLowerCase();
  if (!needle) return [];
  return routingRequests.filter(
    (request) =>
      request.id.toLowerCase() === needle ||
      request.conversationId.toLowerCase() === needle,
  );
}

export interface RoutingAttempt {
  number: number;
  candidateId: string;
  stage: "smart" | "fallback" | "default";
  status: "success" | "failed";
  durationMs: number;
  error: "rate_limited" | "timeout" | null;
}
/** Execution durations include upstream work; request.latency is the routing decision only. */
export function getRoutingAttempts(request: RoutingRequest): RoutingAttempt[] {
  const alternate = routingCandidates.find(
    (candidate) =>
      candidate.model === request.model && candidate.id !== request.candidateId,
  );
  const attempts: RoutingAttempt[] = [];
  if (request.outcome === "post-fallback") {
    attempts.push(
      {
        number: 1,
        candidateId: alternate?.id || request.candidateId,
        stage: "smart",
        status: "failed",
        durationMs: 246,
        error: "rate_limited",
      },
      {
        number: 2,
        candidateId: alternate?.id || request.candidateId,
        stage: "smart",
        status: "failed",
        durationMs: 2500,
        error: "timeout",
      },
    );
  }
  attempts.push({
    number: attempts.length + 1,
    candidateId: request.candidateId,
    stage:
      request.outcome === "smart"
        ? "smart"
        : request.outcome === "bypassed"
          ? "default"
          : "fallback",
    status: "success",
    durationMs: 860 + request.latency * 17,
    error: null,
  });
  return attempts;
}

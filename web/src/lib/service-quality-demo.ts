import { catalogModels } from "../config/models";
import { placeholders, type TierId } from "../config/placeholders";

export type QualityStatus =
  "pass" | "near" | "below" | "insufficient" | "incomplete" | "unconfigured";
export type BucketId = "g0" | "g1" | "g2";
export type QualityMetric = "success" | BucketId;
export type QualityBucket = {
  id: BucketId;
  p95: number;
  limit: number;
  samples: number;
  status: QualityStatus;
};
export type QualityUnit = {
  id: string;
  userId: number;
  group: TierId;
  tier: "Ultra" | "Pro" | "Standard" | "Lite";
  model: string;
  stream: boolean;
  version: string;
  requests: number;
  successes: number;
  successRate: number;
  successTarget: number;
  completeness: number | null;
  minSamples: number;
  unknownInputs: number;
  buckets: QualityBucket[];
  successStatus: QualityStatus;
  performanceStatus: QualityStatus | null;
  status: QualityStatus;
};
export type QualityTrend = {
  date: string;
  success: number;
  g0: number;
  g1: number;
  g2: number;
};

export function getQualityMonths() {
  const now = new Date();
  return Array.from({ length: 4 }, (_, index) => {
    const date = new Date(
      Date.UTC(now.getUTCFullYear(), now.getUTCMonth() - index, 1),
    );
    return date.toISOString().slice(0, 7);
  });
}

const seeds = [
  {
    model: "gpt-4.1",
    group: "ultra",
    tier: "Ultra",
    stream: true,
    requests: 48620,
    failures: 57,
    latencies: [410, 810, 1390],
    userId: 1001,
  },
  {
    model: "claude-sonnet-4-20250514",
    group: "ultra",
    tier: "Ultra",
    stream: true,
    requests: 32180,
    failures: 201,
    latencies: [640, 1310, 2740],
    userId: 1001,
  },
  {
    model: "gemini-2.5-pro",
    group: "pro",
    tier: "Pro",
    stream: true,
    requests: 18450,
    failures: 94,
    latencies: [580, 1220, 2220],
    userId: 1001,
  },
  {
    model: "deepseek-chat",
    group: "pro",
    tier: "Pro",
    stream: false,
    requests: 12830,
    failures: 28,
    latencies: [0, 0, 0],
    userId: 1001,
  },
  {
    model: "qwen-plus",
    group: "standard",
    tier: "Standard",
    stream: true,
    requests: 420,
    failures: 2,
    latencies: [720, 1450, 2640],
    userId: 1001,
  },
  {
    model: "gpt-4.1",
    group: "pro",
    tier: "Pro",
    stream: false,
    requests: 9120,
    failures: 33,
    latencies: [0, 0, 0],
    userId: 1001,
  },
  {
    model: "gemini-2.5-pro",
    group: "ultra",
    tier: "Ultra",
    stream: true,
    requests: 14720,
    failures: 16,
    latencies: [470, 930, 1610],
    userId: 1001,
  },
  {
    model: "deepseek-chat",
    group: "lite",
    tier: "Lite",
    stream: true,
    requests: 6720,
    failures: 16,
    latencies: [690, 1250, 2470],
    userId: 1001,
  },
  {
    model: "gpt-4.1",
    group: "ultra",
    tier: "Ultra",
    stream: true,
    requests: 27600,
    failures: 24,
    latencies: [430, 780, 1340],
    userId: 1002,
  },
  {
    model: "claude-sonnet-4-20250514",
    group: "pro",
    tier: "Pro",
    stream: false,
    requests: 11100,
    failures: 140,
    latencies: [0, 0, 0],
    userId: 1002,
  },
] as const;

function combinedStatus(statuses: QualityStatus[]): QualityStatus {
  return [
    "unconfigured",
    "incomplete",
    "below",
    "insufficient",
    "near",
    "pass",
  ].find((status) =>
    statuses.includes(status as QualityStatus),
  ) as QualityStatus;
}

/** Illustrative contracts and deterministic samples. No production telemetry is used. */
export function createQualityUnits(month: string): QualityUnit[] {
  const currentMonth = getQualityMonths()[0];
  const monthOffset = (Number(month.slice(5)) % 3) * 0.035;
  return seeds
    .filter((seed) =>
      catalogModels.some(
        (model) =>
          model.id === seed.model && model.tierAvailability[seed.group],
      ),
    )
    .map((seed, index) => {
      const tier = placeholders.tiers.find((item) => item.id === seed.group)!;
      const terms = {
        target: Number(tier.success),
        limits: [tier.p95, tier.p95 * 2, tier.p95 * 3],
        minSamples:
          seed.group === "ultra" ? 200 : seed.group === "pro" ? 150 : 100,
      };
      const requests = Math.round(
        seed.requests * (month === currentMonth ? 1 : 1.17 + monthOffset),
      );
      const failures = Math.round(seed.failures * (requests / seed.requests));
      const successRate = ((requests - failures) / requests) * 100;
      const completeness = index === 6 ? null : 100;
      const successTarget = index === 7 ? 0 : terms.target;
      const successStatus: QualityStatus = !successTarget
        ? "unconfigured"
        : completeness === null
          ? "incomplete"
          : successRate < successTarget
            ? "below"
            : successRate - successTarget < 0.08
              ? "near"
              : "pass";
      const buckets: QualityBucket[] = seed.stream
        ? (["g0", "g1", "g2"] as BucketId[]).map((id, bucketIndex) => {
            const p95 = Math.round(
              seed.latencies[bucketIndex] *
                (month === currentMonth ? 1 : 1 - monthOffset),
            );
            const samples = Math.floor(
              requests * [0.67, 0.25, 0.08][bucketIndex],
            );
            const limit = terms.limits[bucketIndex];
            const status: QualityStatus = !successTarget
              ? "unconfigured"
              : completeness === null
                ? "incomplete"
                : p95 > limit
                  ? "below"
                  : samples < terms.minSamples
                    ? "insufficient"
                    : p95 / limit > 0.94
                      ? "near"
                      : "pass";
            return { id, p95, samples, limit, status };
          })
        : [];
      const performanceStatus = seed.stream
        ? combinedStatus(buckets.map((bucket) => bucket.status))
        : null;
      return {
        ...seed,
        id: `sq-${index + 1}`,
        version: `DEMO-${seed.tier === "Ultra" ? "U" : seed.tier === "Pro" ? "P" : seed.tier === "Lite" ? "L" : "S"}-${month.replace("-", ".")}`,
        requests,
        successes: requests - failures,
        successRate,
        successTarget,
        completeness,
        minSamples: terms.minSamples,
        unknownInputs: seed.stream
          ? Math.max(
              0,
              requests -
                buckets.reduce((sum, bucket) => sum + bucket.samples, 0),
            )
          : 0,
        buckets,
        successStatus,
        performanceStatus,
        status: combinedStatus([
          successStatus,
          ...(performanceStatus ? [performanceStatus] : []),
        ]),
      };
    });
}

export function createQualityTrend(
  unit: QualityUnit,
  month: string,
): QualityTrend[] {
  const [year, monthNumber] = month.split("-").map(Number);
  const now = new Date();
  const lastDay =
    month === getQualityMonths()[0]
      ? now.getUTCDate()
      : new Date(Date.UTC(year, monthNumber, 0)).getUTCDate();
  const points = lastDay;
  return Array.from({ length: points }, (_, index) => {
    const day = index + 1;
    const wave =
      Math.sin(index * 1.41 + (unit.requests % 11)) * 0.09 +
      Math.cos(index * 0.67) * 0.045;
    return {
      date: `${month}-${String(day).padStart(2, "0")}`,
      success: Math.max(90, Math.min(100, unit.successRate + wave)),
      g0: Math.max(0, (unit.buckets[0]?.p95 ?? 0) * (1 + wave * 1.7)),
      g1: Math.max(0, (unit.buckets[1]?.p95 ?? 0) * (1 + wave * 1.4)),
      g2: Math.max(0, (unit.buckets[2]?.p95 ?? 0) * (1 + wave * 1.2)),
    };
  });
}

export type QualityFilters = {
  userId?: number;
  group?: string;
  model?: string;
  stream?: boolean;
};
export function filterQualityUnits(
  units: QualityUnit[],
  filters: QualityFilters,
) {
  return units.filter(
    (unit) =>
      (filters.userId === undefined || unit.userId === filters.userId) &&
      (!filters.group || unit.group === filters.group) &&
      (!filters.model || unit.model === filters.model) &&
      (filters.stream === undefined || unit.stream === filters.stream),
  );
}

export function serializeQualityCsv(rows: (string | number)[][]) {
  return (
    "\uFEFF" +
    rows
      .map((row) =>
        row.map((cell) => `"${String(cell).replaceAll('"', '""')}"`).join(","),
      )
      .join("\r\n")
  );
}

export function summarizeQualityUnits(units: QualityUnit[]) {
  const needsAttention = (status: QualityStatus | null) =>
    status !== null && !["pass", "unconfigured"].includes(status);
  return {
    total: units.length,
    successAttention: units.filter((unit) => needsAttention(unit.successStatus))
      .length,
    performanceAttention: units.filter((unit) =>
      needsAttention(unit.performanceStatus),
    ).length,
    logicalRequests: units.reduce((sum, unit) => sum + unit.requests, 0),
  };
}

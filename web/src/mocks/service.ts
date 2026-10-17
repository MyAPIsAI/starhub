import { placeholders, type TierId } from "../config/placeholders";
import { sumNano } from "../lib/money";
import type { RequestRecord } from "../lib/request-record";

// Re-export for existing importers; the canonical definition now lives
// in lib/request-record.ts.
export type { RequestRecord };

const models = [
  ["gpt-4.1", "OpenAI"],
  ["claude-sonnet-4-20250514", "Anthropic"],
  ["gemini-2.5-pro", "Google"],
  ["deepseek-chat", "DeepSeek"],
  ["qwen-plus", "Qwen"],
];
const tiers: TierId[] = ["pro", "ultra", "standard", "pro", "lite"];
export const sampleRequests: RequestRecord[] = Array.from(
  { length: 48 },
  (_, i) => {
    const [model, vendor] = models[i % models.length];
    const status =
      i === 5 || i === 27 ? "failed" : i === 10 ? "interrupted" : "success";
    const input_tokens = 620 + ((i * 137) % 1700);
    const output_tokens = 148 + ((i * 53) % 710);
    const input_price_nano = [
      "1600000000",
      "3000000000",
      "750000000",
      "224000000",
      "320000000",
    ][i % 5];
    const output_price_nano = [
      "6400000000",
      "15000000000",
      "6000000000",
      "336000000",
      "960000000",
    ][i % 5];
    const cost_nano =
      status === "failed"
        ? "0"
        : (
            (BigInt(input_tokens) * BigInt(input_price_nano) +
              BigInt(output_tokens) * BigInt(output_price_nano) +
              500000n) /
            1000000n
          ).toString();
    return {
      trace_id: `tr_demo_${(0x7c9e1f + i * 1783).toString(16)}${(0xb42a - i * 7).toString(16)}`,
      model,
      vendor,
      tier: tiers[i % 5],
      status,
      time: new Date(
        Date.parse(placeholders.mock.now) - i * 113000,
      ).toISOString(),
      cost_nano,
      input_tokens,
      output_tokens,
      cache_tokens: 0,
      duration_ms: status === "failed" ? 60000 : 1034 + i * 83,
      ttft_ms: status === "failed" ? 0 : 286 + i * 19,
      fallback: i === 2 || i === 8,
      source: "playground",
      error_code:
        status === "failed"
          ? "upstream_unavailable"
          : status === "interrupted"
            ? "stream_interrupted"
            : null,
      input_price_nano,
      output_price_nano,
    };
  },
);

export interface ActivityPoint {
  date: string;
  cost_nano: string;
  previous_nano: string;
  requests: number;
  previousRequests: number;
}
export const sampleActivity: ActivityPoint[] = Array.from(
  { length: 30 },
  (_, i) => {
    const date = new Date(
      Date.parse("2026-08-20T00:00:00+04:00") + i * 86400000,
    );
    const weekend = [5, 6].includes(date.getUTCDay());
    const requests = Math.round(
      (weekend ? 650 : 1320) + Math.sin(i * 1.6) * 245 + i * 22,
    );
    return {
      date: date.toISOString(),
      requests,
      previousRequests: Math.round(requests * 0.82),
      cost_nano: (BigInt(requests) * BigInt(7754000 + i * 12345)).toString(),
      previous_nano: (BigInt(requests) * 6911000n).toString(),
    };
  },
);

async function delay(signal?: AbortSignal) {
  await new Promise<void>((resolve, reject) => {
    if (signal?.aborted) {
      reject(new DOMException("Aborted", "AbortError"));
      return;
    }
    const timeout = setTimeout(
      () => {
        signal?.removeEventListener("abort", abort);
        resolve();
      },
      placeholders.mock.minimumDelay +
        Math.random() *
          (placeholders.mock.maximumDelay - placeholders.mock.minimumDelay),
    );
    function abort() {
      clearTimeout(timeout);
      reject(new DOMException("Aborted", "AbortError"));
    }
    signal?.addEventListener("abort", abort, { once: true });
  });
}
// Independent mock REST boundary. All money fields remain strings across this boundary.
export async function mockGet(
  path: "/api/demo/activity" | "/api/demo/requests",
  signal?: AbortSignal,
) {
  await delay(signal);
  if (path === "/api/demo/requests")
    return {
      data: structuredClone(sampleRequests),
      total: sampleRequests.length,
    };
  return {
    data: structuredClone(sampleActivity),
    total_nano: sumNano(sampleActivity.map((p) => p.cost_nano)),
  };
}
export async function getRequests(
  signal?: AbortSignal,
): Promise<RequestRecord[]> {
  return (await mockGet("/api/demo/requests", signal)).data as RequestRecord[];
}
export async function getActivity(
  signal?: AbortSignal,
): Promise<ActivityPoint[]> {
  return (await mockGet("/api/demo/activity", signal)).data as ActivityPoint[];
}

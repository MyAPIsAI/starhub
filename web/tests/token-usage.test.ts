import { beforeEach, describe, it, expect, vi } from "vitest";
const fake = vi.hoisted(() => ({
  identity: [1, 7, "default", 1],
  get: vi.fn(),
}));
vi.mock("../src/lib/system-access", () => ({
  useSessionQueryIdentity: () => [...fake.identity],
  sessionQueryIdentity: () => [...fake.identity],
  useSystemAccess: { getState: () => ({}) },
}));
vi.mock("@tanstack/react-query", () => ({
  useQuery: (options: unknown) => options,
}));
vi.mock("../src/lib/api/http", () => ({
  apiGet: (...args: unknown[]) => fake.get(...args),
}));
import {
  useOwnedTokenUsage,
  validateTokenUsage,
  type TokenUsageDetails,
} from "../src/lib/api/token-usage";

function usage(): TokenUsageDetails {
  const zero = {
    requests: 0,
    quota: 0,
    unfunded_quota: 0,
    prompt_tokens: 0,
    completion_tokens: 0,
    success_count: 0,
    failed_count: 0,
  };
  return {
    schema: "token_usage_v1",
    user_id: 7,
    token_id: 2,
    observed_at: 1,
    current: {
      period: "daily",
      timezone: "UTC",
      period_id: 1,
      start: 1,
      end: 2,
      used_quota: 0,
      reserved_quota: 0,
      quota_limit: 100,
      remaining_quota: 100,
      lifetime_used_quota: 0,
    },
    days: 30,
    trend: Array.from({ length: 30 }, (_, i) => ({
      ...zero,
      date: `2026-11-${String(i + 1).padStart(2, "0")}`,
    })),
    models: [],
    model_page: 1,
    model_total: 0,
    periods: [],
    period_page: 1,
    period_total: 0,
    page_size: 20,
  };
}
beforeEach(() => {
  fake.identity = [1, 7, "default", 1];
  fake.get.mockReset();
});
describe("owned key usage", () => {
  it("keeps a known empty 30-day window distinct from an error", () => {
    const value = usage();
    expect(validateTokenUsage(value, 7, 2)).toBe(value);
  });
  it("rejects foreign keys and unsafe monetary amounts", () => {
    expect(() => validateTokenUsage(usage(), 8, 2)).toThrow(/not confirmed/);
    expect(() => validateTokenUsage(usage(), 7, 3)).toThrow(/not confirmed/);
    const value = usage();
    value.trend[0].quota = Number.MAX_SAFE_INTEGER + 1;
    expect(() => validateTokenUsage(value, 7, 2)).toThrow(/amounts/);
  });
  it("rejects invalid calendar metadata before rendering details", () => {
    const value = usage();
    value.current.timezone = "Invalid/Timezone";
    expect(() => validateTokenUsage(value, 7, 2)).toThrow(/not confirmed/);
    value.current.timezone = "UTC";
    value.page_size = 0;
    expect(() => validateTokenUsage(value, 7, 2)).toThrow(/not confirmed/);
  });
  it("keys cache entries by session and rejects a delayed old-session response", async () => {
    const query = useOwnedTokenUsage(2, 1, 1) as unknown as {
      queryKey: unknown[];
      queryFn: (ctx: { signal: AbortSignal }) => Promise<unknown>;
    };
    expect(query.queryKey).toEqual([
      "owned-token-usage",
      [1, 7, "default", 1],
      2,
      1,
      1,
    ]);
    let finish!: (value: unknown) => void;
    fake.get.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const pending = query.queryFn({ signal: new AbortController().signal });
    fake.identity = [2, 8, "default", 1];
    finish(usage());
    await expect(pending).rejects.toThrow(/session/);
  });
});

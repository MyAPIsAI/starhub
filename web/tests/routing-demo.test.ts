import { afterEach, describe, expect, it, vi } from "vitest";
import {
  defaultRoutingConfig,
  filterRoutingCandidates,
  findRoutingRequests,
  getRoutingAttempts,
  loadRoutingConfig,
  routingCandidates,
  routingRequests,
  routingSnapshots,
  routingTrend,
  saveRoutingConfig,
} from "../src/lib/routing-demo";

afterEach(() => vi.unstubAllGlobals());

describe("smart-routing local demo integrity", () => {
  it("resolves every recent request to the same decision, model resource and historical snapshot", () => {
    expect(new Set(routingRequests.map((row) => row.id)).size).toBe(
      routingRequests.length,
    );
    for (const request of routingRequests) {
      expect(findRoutingRequests(request.id)).toEqual([request]);
      expect(findRoutingRequests(request.conversationId)).toContain(request);
      expect(
        routingCandidates.find(
          (candidate) => candidate.id === request.candidateId,
        )?.model,
      ).toBe(request.model);
      expect(
        routingSnapshots.some((snapshot) => snapshot.id === request.snapshotId),
      ).toBe(true);
      const attempts = getRoutingAttempts(request);
      expect(attempts).toHaveLength(request.attempts);
      expect(attempts.at(-1)?.status).toBe("success");
      expect(attempts.at(-1)?.candidateId).toBe(request.candidateId);
      expect(request.attempts).toBe(
        request.outcome === "post-fallback" ? 3 : 1,
      );
    }
    expect(findRoutingRequests("unlisted-request")).toEqual([]);
    expect(findRoutingRequests(" ")).toEqual([]);
  });

  it("combines filters without leaking excluded candidates and uses consistent five-minute windows", () => {
    const candidates = filterRoutingCandidates("OPENAI", "eligible", "gpt-4.1");
    expect(candidates.length).toBeGreaterThan(0);
    expect(
      candidates.every(
        (row) =>
          row.provider === "OpenAI" &&
          row.status === "eligible" &&
          row.model === "gpt-4.1",
      ),
    ).toBe(true);
    expect(filterRoutingCandidates("no-such-resource", "eligible")).toEqual([]);
    expect(filterRoutingCandidates("", "circuit", "gpt-4.1")).toEqual([]);
    const minutes = routingTrend.map((row) => {
      const [hour, minute] = row.time.split(":").map(Number);
      return hour * 60 + minute;
    });
    expect(
      minutes.slice(1).every((minute, index) => minute - minutes[index] === 5),
    ).toBe(true);
    expect(routingTrend.slice(-6)).toHaveLength(6);
  });

  it("persists valid local changes, rejects corrupt settings, and supports restoring defaults", () => {
    const memory = new Map<string, string>();
    vi.stubGlobal("localStorage", {
      getItem: (key: string) => memory.get(key) ?? null,
      setItem: (key: string, value: string) => memory.set(key, value),
    });
    expect(loadRoutingConfig()).toEqual(defaultRoutingConfig);
    const saved = {
      ...defaultRoutingConfig,
      enabled: false,
      phase: 1,
      traffic: 1,
      experimentRunning: false,
    };
    expect(saveRoutingConfig(saved)).toBe(true);
    expect(loadRoutingConfig()).toEqual(saved);
    memory.set(
      "veyra-routing-demo-v1",
      JSON.stringify({ ...saved, timeoutMs: -1 }),
    );
    expect(loadRoutingConfig()).toEqual(defaultRoutingConfig);
    memory.set("veyra-routing-demo-v1", "broken-json");
    expect(loadRoutingConfig()).toEqual(defaultRoutingConfig);
    expect(saveRoutingConfig(defaultRoutingConfig)).toBe(true);
    expect(loadRoutingConfig()).toEqual(defaultRoutingConfig);
  });
});

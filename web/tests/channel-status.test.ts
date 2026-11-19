import { describe, expect, it } from "vitest";
import { channelResourceStatus } from "../src/lib/channel-status";
import type { ChannelResourceState } from "../src/lib/api/admin";

describe("channel model resource availability", () => {
  it.each([
    ["enabled", 2, 2],
    ["partial", 2, 1],
    ["closed", 2, 0],
    ["closed", 0, 0],
    ["enabled", 1, 1],
  ] as const)("uses server model summary %s %i/%i", (state, total, enabled) => {
    expect(
      channelResourceStatus({
        resource_state: { state, total, enabled, resources: [] },
      }),
    ).toBe(state);
  });
  it.each([
    { state: "enabled", total: 2, enabled: 1 },
    { state: "partial", total: 2, enabled: 3 },
    { state: "closed", total: -1, enabled: 0 },
    { state: "partial", total: 2, enabled: 0.5 },
  ])("rejects inconsistent summaries %j", (state) => {
    expect(
      channelResourceStatus({
        resource_state: { ...state, resources: [] } as ChannelResourceState,
      }),
    ).toBeUndefined();
  });
  it("does not invent availability when the server summary is missing", () => {
    expect(channelResourceStatus({})).toBeUndefined();
  });
});

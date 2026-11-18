import { describe, expect, it } from "vitest";
import { channelResourceStatus } from "../src/lib/channel-status";

describe("channel resource availability", () => {
  it.each([
    [1, 3, [1, 1, 1], "enabled"],
    [1, 3, [2, 1, 1], "partial"],
    [1, 3, [2, 2, 2], "closed"],
    [2, 3, [1, 1, 1], "closed"],
    [3, 3, [2, 1, 1], "closed"],
    [1, 1, [1], "enabled"],
    [1, 1, [2], "closed"],
    [1, 0, [], "closed"],
    [1, 3, null, "enabled"],
    [1, 3, [2], "partial"],
    [1, 1, [1, 2], "enabled"],
  ] as const)(
    "status %i with %i resources and %j reports %s",
    (status, key_count, statuses, expected) => {
      expect(
        channelResourceStatus({
          status,
          key_count,
          multi_key_status: statuses ? [...statuses] : null,
        }),
      ).toBe(expected);
    },
  );
});

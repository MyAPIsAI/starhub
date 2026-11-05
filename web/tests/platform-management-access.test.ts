import { describe, expect, it } from "vitest";
import {
  canManagePlatform,
  type SessionIdentity,
} from "../src/lib/system-access";

const confirmed: SessionIdentity = {
  accountId: 12,
  accountKind: "staff",
  group: "default",
  epoch: 1,
};

describe("platform management visibility", () => {
  it.each([5, 10, 20, 100])(
    "shows management for confirmed staff role %s",
    (role) => {
      expect(canManagePlatform({ role, session: confirmed })).toBe(true);
    },
  );
  it.each([1, 99, 101])(
    "does not grant administrator entry to role %s",
    (role) => {
      expect(canManagePlatform({ role, session: confirmed })).toBe(false);
    },
  );
  it.each(["customer", undefined] as const)(
    "rejects a client role change without a staff identity: %s",
    (accountKind) => {
      expect(
        canManagePlatform({
          role: 100,
          session: { ...confirmed, accountKind },
        }),
      ).toBe(false);
    },
  );
  it.each([null, 0, -1, NaN])(
    "requires a confirmed account identifier: %s",
    (accountId) => {
      expect(
        canManagePlatform({ role: 100, session: { ...confirmed, accountId } }),
      ).toBe(false);
    },
  );
});

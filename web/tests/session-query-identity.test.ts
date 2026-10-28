import { afterEach, describe, expect, it } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import {
  sessionQueryIdentity,
  useSystemAccess,
} from "../src/lib/system-access";
import { useProductPreview } from "../src/lib/product-preview";

const original = useSystemAccess.getState();
const preview = useProductPreview.getState();
afterEach(() => {
  useSystemAccess.setState(original, true);
  useProductPreview.setState(preview, true);
});
const identity = () => sessionQueryIdentity(useSystemAccess.getState());

describe("authoritative session query isolation", () => {
  it("isolates anonymous, same-role accounts and changed groups", () => {
    useSystemAccess.getState().setSession(null);
    const guest = identity();
    useSystemAccess
      .getState()
      .setSession({ id: 10, group: "default", role: 1 });
    const first = identity();
    useSystemAccess.getState().setSession({ id: 11, group: "vip", role: 1 });
    const second = identity();
    expect(first).not.toEqual(guest);
    expect(second).not.toEqual(first);
    useSystemAccess
      .getState()
      .setSession({ id: 11, group: "default", role: 1 });
    expect(identity()).not.toEqual(second);
    expect(useSystemAccess.getState().session).toMatchObject({
      accountId: 11,
      group: "default",
    });
  });
  it("rotates even after reauthenticating the same account and clears on logout", () => {
    const user = { id: 10, group: "default", role: 100 };
    useSystemAccess.getState().setSession(user);
    const before = identity();
    useSystemAccess.getState().setSession(user);
    expect(identity()).not.toEqual(before);
    useSystemAccess.getState().setSession(null);
    expect(useSystemAccess.getState()).toMatchObject({
      role: 1,
      session: { accountId: null, group: null },
    });
  });
  it("never obtains session identity from Demo profile edits", () => {
    useSystemAccess
      .getState()
      .setSession({ id: 10, group: "default", role: 1 });
    const before = identity();
    useProductPreview.getState().login("", "demo-changed", {
      id: "not-a-server-id",
      username: "demo-changed",
    });
    expect(identity()).toEqual(before);
    expect(useSystemAccess.getState().session.accountId).toBe(10);
  });
  it("keeps a late old-session response out of the new session cache", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    });
    try {
      useSystemAccess
        .getState()
        .setSession({ id: 10, group: "default", role: 1 });
      const oldKey = ["plaza-model", identity(), "gpt-4o"];
      let finish!: (value: { group: string }) => void;
      const old = client.fetchQuery({
        queryKey: oldKey,
        queryFn: () =>
          new Promise<{ group: string }>((resolve) => {
            finish = resolve;
          }),
      });
      useSystemAccess.getState().setSession({ id: 11, group: "vip", role: 1 });
      const newKey = ["plaza-model", identity(), "gpt-4o"];
      await client.fetchQuery({
        queryKey: newKey,
        queryFn: async () => ({ group: "vip" }),
      });
      finish({ group: "default" });
      await old;
      expect(client.getQueryData(newKey)).toEqual({ group: "vip" });
      expect(client.getQueryData(oldKey)).toEqual({ group: "default" });
    } finally {
      client.clear();
    }
  });
});

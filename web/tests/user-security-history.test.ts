import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { readUserSecurityState } from "../src/lib/user-security-state";
vi.mock("@tanstack/react-query", () => ({
  useQuery: (options: unknown) => options,
}));
vi.mock("../src/lib/system-access", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../src/lib/system-access")>();
  return {
    ...actual,
    useSessionQueryIdentity: () =>
      actual.sessionQueryIdentity(actual.useSystemAccess.getState()),
  };
});
import { useUserSecurityHistory } from "../src/lib/api/user";
import { useSystemAccess } from "../src/lib/system-access";
import { onAuthLoss } from "../src/lib/api/http";
const state = {
  schema: "security_state_v1",
  history_scope: "guarded_profile_changes_v1",
  user_id: 7,
  events: [
    { id: 3, action: "email_changed", created_at: 100 },
    { id: 2, action: "password_changed", created_at: 99 },
  ],
};
const fetchMock = vi.fn(),
  loss = vi.fn();
let stop: () => void;
beforeEach(() => {
  fetchMock.mockReset();
  loss.mockReset();
  vi.stubGlobal("fetch", fetchMock);
  vi.stubGlobal("window", { location: { pathname: "/console/settings" } });
  useSystemAccess.getState().setSession({ id: 7, group: "default", role: 1 });
  stop = onAuthLoss(loss);
});
afterEach(() => {
  stop();
  vi.unstubAllGlobals();
});
describe("owned bounded security history", () => {
  it("returns detached metadata only and preserves the explicit limited scope", () => {
    const input = {
      ...state,
      events: state.events.map((event) => ({
        ...event,
        email: "private@example.test",
        password: "secret",
      })),
    };
    const result = readUserSecurityState(input, 7);
    expect(result).toEqual(state);
    expect(result.events[0]).not.toBe(input.events[0]);
    expect(JSON.stringify(result)).not.toContain("private@example.test");
  });
  it("accepts a real empty scope without inventing earlier actions", () => {
    expect(readUserSecurityState({ ...state, events: [] }, 7).events).toEqual(
      [],
    );
  });
  it("rejects unsupported scopes, foreign ownership and malformed history", () => {
    for (const value of [
      null,
      {},
      { ...state, history_scope: "all_actions" },
      { ...state, user_id: 8 },
      { ...state, events: [...state.events, ...state.events, ...state.events] },
      ...[
        [{ id: 0, action: "email_changed", created_at: 100 }],
        [{ id: 1, action: "login", created_at: 100 }],
        [{ id: 1, action: "email_changed", created_at: 0 }],
        [
          {
            id: 1,
            action: "email_changed",
            created_at: Number.MAX_SAFE_INTEGER,
          },
        ],
        [state.events[0], state.events[0]],
        [...state.events].reverse(),
      ].map((events) => ({ ...state, events })),
    ])
      expect(() => readUserSecurityState(value, 7)).toThrow();
  });
  it("uses a full scoped cache and exact owner query with cancellation and no retries", async () => {
    const query = useUserSecurityHistory() as any;
    const epoch = useSystemAccess.getState().session.epoch;
    expect(query.queryKey).toEqual([
      "user-self",
      "security",
      [epoch, 7, "default", 1],
    ]);
    expect(query.staleTime).toBe(0);
    expect(query.retry).toBe(false);
    const signal = new AbortController().signal;
    fetchMock.mockResolvedValue(Response.json({ success: true, data: state }));
    expect(query.select(await query.queryFn({ signal }))).toEqual(state.events);
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/security-state?expected_user_id=7",
      expect.objectContaining({ signal, credentials: "same-origin" }),
    );
    expect(() => query.select({ ...state, user_id: 8 })).toThrow();
  });
  it("does not dispatch a guest or superseded manual request", async () => {
    const old = useUserSecurityHistory() as any;
    useSystemAccess.getState().setSession(null);
    await expect(old.queryFn({})).rejects.toThrow(/session/);
    const guest = useUserSecurityHistory() as any;
    expect(guest.enabled).toBe(false);
    await expect(guest.queryFn({})).rejects.toThrow(/session/);
    expect(fetchMock).not.toHaveBeenCalled();
  });
  it.each(["id", "epoch", "role", "group"])(
    "rejects old history after %s changes",
    async (field) => {
      let release!: (value: Response) => void;
      fetchMock.mockImplementation(
        () => new Promise<Response>((resolve) => (release = resolve)),
      );
      const pending = (useUserSecurityHistory() as any).queryFn({});
      if (field === "role") useSystemAccess.getState().setRole(10);
      else if (field === "group")
        useSystemAccess.setState((current) => ({
          session: { ...current.session, group: "vip" },
        }));
      else
        useSystemAccess.getState().setSession({
          id: field === "id" ? 8 : 7,
          group: "default",
          role: 1,
        });
      release(Response.json({ success: true, data: state }));
      await expect(pending).rejects.toThrow(/session/);
    },
  );
  it("does not clear a new account after a delayed unauthorized read", async () => {
    let release!: (value: Response) => void;
    fetchMock.mockImplementation(
      () => new Promise<Response>((resolve) => (release = resolve)),
    );
    const pending = (useUserSecurityHistory() as any).queryFn({});
    useSystemAccess.getState().setSession({ id: 8, group: "default", role: 1 });
    release(new Response(null, { status: 401 }));
    await expect(pending).rejects.toThrow(/session/);
    expect(useSystemAccess.getState().session.accountId).toBe(8);
    expect(loss).not.toHaveBeenCalled();
  });
});

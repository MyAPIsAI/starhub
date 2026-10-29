import { beforeEach, describe, expect, it, vi } from "vitest";
const fake = vi.hoisted(() => ({
  state: {
    role: 1,
    session: { accountId: 7 as number | null, group: "default", epoch: 1 },
  },
  get: vi.fn(),
  post: vi.fn(),
  signOut: vi.fn(),
  invalidate: vi.fn(),
  cache: undefined as unknown,
  status: "success",
}));
vi.mock("../src/lib/product-preview", () => {
  throw Error("Account devices must not import Demo identity");
});
vi.mock("../src/lib/system-access", () => ({
  sessionQueryIdentity: (state: typeof fake.state) => [
    state.session.epoch,
    state.session.accountId,
    state.session.group,
    state.role,
  ],
  useSessionQueryIdentity: () => [
    fake.state.session.epoch,
    fake.state.session.accountId,
    fake.state.session.group,
    fake.state.role,
  ],
  useSystemAccess: {
    getState: () => ({
      ...fake.state,
      setSession: (value: null) => {
        fake.signOut(value);
        fake.state.session.accountId = null;
        fake.state.session.epoch++;
      },
    }),
  },
}));
vi.mock("@tanstack/react-query", () => ({
  useQuery: (options: unknown) => options,
  useMutation: (options: unknown) => options,
  useQueryClient: () => ({
    getQueryState: () => ({ status: fake.status }),
    getQueryData: () => fake.cache,
    invalidateQueries: fake.invalidate,
  }),
}));
vi.mock("../src/lib/api/http", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../src/lib/api/http")>()),
  apiGet: (...args: unknown[]) => fake.get(...args),
  apiPost: (...args: unknown[]) => fake.post(...args),
}));
import { useSessions, useTerminateOtherSessions } from "../src/lib/api/user";
import { ApiError } from "../src/lib/api/http";
const state = {
  schema: "session_state_v1",
  guarded_management: true,
  user_id: 7,
  sessions: [{ created_at: 100, expires_at: 300, is_current: true }],
};
beforeEach(() => {
  fake.state = {
    role: 1,
    session: { accountId: 7, group: "default", epoch: 1 },
  };
  fake.get.mockReset();
  fake.post.mockReset();
  fake.signOut.mockReset();
  fake.invalidate.mockReset();
  fake.cache = structuredClone(state);
  fake.status = "success";
});
describe("confirmed device management transport", () => {
  it("keys a guarded read by the full server session and verifies its contract", async () => {
    const query = useSessions() as any,
      signal = new AbortController().signal;
    expect(query.queryKey).toEqual(["user-sessions", [1, 7, "default", 1]]);
    expect(query.enabled).toBe(true);
    expect(query.staleTime).toBe(0);
    expect(query.retry).toBe(false);
    fake.get.mockResolvedValue(state);
    const result = await query.queryFn({ signal });
    expect(query.select(result)).toEqual(state.sessions);
    expect(fake.get).toHaveBeenCalledWith(
      "/api/user/session-state?expected_user_id=7",
      signal,
      { handleUnauthorizedLocally: true },
    );
    expect(() => query.select({ ...state, user_id: 8 })).toThrow();
  });
  it("blocks guest or stale manual reads before any private request", async () => {
    const old = useSessions() as any;
    fake.state.session.epoch++;
    await expect(old.queryFn({})).rejects.toThrow(/session/);
    fake.state.session.accountId = null;
    const guest = useSessions() as any;
    expect(guest.enabled).toBe(false);
    await expect(guest.queryFn({})).rejects.toThrow(/session/);
    expect(fake.get).not.toHaveBeenCalled();
  });
  it.each(["accountId", "epoch", "group", "role"])(
    "discards delayed metadata after changing %s",
    async (field) => {
      const query = useSessions() as any;
      let release!: (value: unknown) => void;
      fake.get.mockImplementation(
        () => new Promise((resolve) => (release = resolve)),
      );
      const result = query.queryFn({});
      if (field === "accountId") fake.state.session.accountId = 8;
      else if (field === "epoch") fake.state.session.epoch++;
      else if (field === "group") fake.state.session.group = "vip";
      else fake.state.role = 10;
      release(state);
      await expect(result).rejects.toThrow(/session/);
    },
  );
  it("requires a successful matching guarded read before a mutation", async () => {
    const mutation = useTerminateOtherSessions() as any;
    expect(mutation.retry).toBe(false);
    for (const value of [
      undefined,
      [],
      { ...state, user_id: 8 },
      { ...state, guarded_management: false },
    ]) {
      fake.cache = value;
      await expect(mutation.mutationFn()).rejects.toThrow();
    }
    fake.cache = state;
    fake.status = "error";
    await expect(mutation.mutationFn()).rejects.toThrow(/guarded device list/);
    fake.status = "success";
    fake.state.session.accountId = null;
    await expect(mutation.mutationFn()).rejects.toThrow(/session/);
    expect(fake.post).not.toHaveBeenCalled();
  });
  it("sends only the confirmed expected owner and refreshes lists after completion", async () => {
    const mutation = useTerminateOtherSessions() as any;
    fake.post.mockResolvedValue({ user_id: 7, terminated: 0 });
    expect(await mutation.mutationFn()).toEqual({ user_id: 7, terminated: 0 });
    expect(fake.post).toHaveBeenCalledExactlyOnceWith(
      "/api/user/sessions/terminate-others",
      { expected_user_id: 7 },
      { handleUnauthorizedLocally: true },
    );
    mutation.onSettled();
    expect(fake.invalidate).toHaveBeenCalledWith({
      queryKey: ["user-sessions"],
    });
  });
  it("rejects stale mutation dispatch and a delayed result after account replacement", async () => {
    const old = useTerminateOtherSessions() as any;
    fake.state.session.epoch++;
    await expect(old.mutationFn()).rejects.toThrow(/session/);
    expect(fake.post).not.toHaveBeenCalled();
    const current = useTerminateOtherSessions() as any;
    let release!: (value: unknown) => void;
    fake.post.mockImplementation(
      () => new Promise((resolve) => (release = resolve)),
    );
    const result = current.mutationFn();
    fake.state.session.accountId = 8;
    release({ user_id: 7, terminated: 1 });
    await expect(result).rejects.toThrow(/session/);
  });
  it("requires an explicit matching nonnegative termination receipt", async () => {
    for (const value of [
      undefined,
      {},
      { user_id: 8, terminated: 1 },
      { user_id: 7, terminated: -1 },
    ]) {
      fake.post.mockResolvedValue(value);
      await expect(
        (useTerminateOtherSessions() as any).mutationFn(),
      ).rejects.toThrow(/confirmed/);
    }
  });
  it("does not retry uncertain termination responses", async () => {
    fake.post.mockRejectedValue(Error("lost response"));
    await expect(
      (useTerminateOtherSessions() as any).mutationFn(),
    ).rejects.toThrow(/lost/);
    expect(fake.post).toHaveBeenCalledTimes(1);
  });
  it("clears a current expired session but ignores an old unauthorized response", async () => {
    fake.get.mockRejectedValue(new ApiError(401, "expired"));
    await expect((useSessions() as any).queryFn({})).rejects.toThrow(/expired/);
    expect(fake.signOut).toHaveBeenCalledExactlyOnceWith(null);
    fake.state.session.accountId = 7;
    fake.signOut.mockClear();
    const query = useSessions() as any;
    let reject!: (reason: unknown) => void;
    fake.get.mockImplementation(
      () => new Promise((_, fail) => (reject = fail)),
    );
    const result = query.queryFn({});
    fake.state.session.accountId = 8;
    reject(new ApiError(401, "old expired"));
    await expect(result).rejects.toThrow(/session/);
    expect(fake.signOut).not.toHaveBeenCalled();
  });
});

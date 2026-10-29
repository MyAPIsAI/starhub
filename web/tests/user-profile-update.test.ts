import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const cache = vi.hoisted(() => ({
  status: "success",
  data: undefined as unknown,
  invalidate: vi.fn(),
}));
vi.mock("@tanstack/react-query", () => ({
  useMutation: (options: unknown) => options,
  useQueryClient: () => ({
    getQueryState: () => ({ status: cache.status }),
    getQueryData: () => cache.data,
    invalidateQueries: cache.invalidate,
  }),
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
import { useUpdateSelf } from "../src/lib/api/user";
import { useSystemAccess } from "../src/lib/system-access";
import { onAuthLoss } from "../src/lib/api/http";

const state = {
  schema: "profile_state_v1",
  guarded_updates: true,
  user: {
    id: 7,
    username: "owner",
    email: "owner@example.test",
    role: 1,
    status: 1,
    created_at: 100,
  },
};
const fetchMock = vi.fn(),
  loss = vi.fn();
let unsubscribe: () => void;
const mutation = () => useUpdateSelf() as any;
beforeEach(() => {
  cache.status = "success";
  cache.data = structuredClone(state);
  cache.invalidate.mockReset();
  fetchMock.mockReset();
  loss.mockReset();
  vi.stubGlobal("fetch", fetchMock);
  vi.stubGlobal("window", { location: { pathname: "/console/settings" } });
  useSystemAccess.getState().setSession({ id: 7, group: "default", role: 1 });
  unsubscribe = onAuthLoss(loss);
});
afterEach(() => {
  unsubscribe();
  vi.unstubAllGlobals();
});
describe("guarded profile mutations through the actual HTTP client", () => {
  it("sends the expected owner/email snapshot and requires the exact password receipt", async () => {
    fetchMock.mockResolvedValue(
      Response.json({
        success: true,
        data: { user_id: 7, updated_field: "password" },
      }),
    );
    const action = mutation();
    expect(action.retry).toBe(false);
    await expect(
      action.mutationFn({
        current_password: "current-secret",
        new_password: "replacement-secret",
      }),
    ).resolves.toEqual({ user_id: 7, updated_field: "password" });
    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe("/api/user/profile-state");
    expect(init.credentials).toBe("same-origin");
    expect(JSON.parse(init.body)).toEqual({
      expected_user_id: 7,
      expected_email: "owner@example.test",
      current_password: "current-secret",
      new_password: "replacement-secret",
    });
    action.onSettled();
    expect(cache.invalidate).toHaveBeenCalledWith({ queryKey: ["user-self"] });
  });
  it("sends profile email without pretending to verify it or sending a password", async () => {
    fetchMock.mockResolvedValue(
      Response.json({
        success: true,
        data: { user_id: 7, updated_field: "email" },
      }),
    );
    await mutation().mutationFn({ email: "new@example.test" });
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
      expected_user_id: 7,
      expected_email: "owner@example.test",
      email: "new@example.test",
    });
  });
  it("requires a successful current guarded profile before writing", async () => {
    for (const data of [
      null,
      {},
      { ...state, guarded_updates: undefined },
      { ...state, guarded_updates: 1 },
      { ...state, user: { ...state.user, id: 8 } },
    ]) {
      cache.data = data;
      await expect(
        mutation().mutationFn({ email: "new@example.test" }),
      ).rejects.toThrow();
    }
    cache.data = state;
    cache.status = "error";
    await expect(
      mutation().mutationFn({ email: "new@example.test" }),
    ).rejects.toThrow();
    expect(fetchMock).not.toHaveBeenCalled();
  });
  it("blocks stale and guest action dispatch", async () => {
    const old = mutation();
    useSystemAccess.getState().setSession(null);
    await expect(old.mutationFn({ email: "new@example.test" })).rejects.toThrow(
      /session/,
    );
    await expect(
      mutation().mutationFn({ email: "new@example.test" }),
    ).rejects.toThrow(/session/);
    expect(fetchMock).not.toHaveBeenCalled();
  });
  it("rejects mixed or invalid inputs without sending credential data", async () => {
    for (const input of [
      {},
      { email: "new@example.test", new_password: "replacement-secret" },
      { email: "new@example.test", current_password: "secret" },
      { email: " spaced@example.test " },
      { current_password: "secret", new_password: "short" },
      { new_password: "replacement-secret" },
      { current_password: "x".repeat(73), new_password: "replacement-secret" },
      { current_password: "secret", new_password: "汉".repeat(22) },
    ])
      await expect(mutation().mutationFn(input)).rejects.toThrow();
    expect(fetchMock).not.toHaveBeenCalled();
  });
  it.each(["id", "epoch", "group", "role"])(
    "does not accept a result after confirmed %s changes",
    async (field) => {
      let release!: (value: Response) => void;
      fetchMock.mockImplementation(
        () => new Promise<Response>((resolve) => (release = resolve)),
      );
      const pending = mutation().mutationFn({ email: "new@example.test" });
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
      release(
        Response.json({
          success: true,
          data: { user_id: 7, updated_field: "email" },
        }),
      );
      await expect(pending).rejects.toThrow(/session/);
    },
  );
  it("rejects missing, foreign and wrong-field success receipts", async () => {
    for (const data of [
      undefined,
      true,
      {},
      { user_id: 8, updated_field: "email" },
      { user_id: 7, updated_field: "password" },
    ]) {
      fetchMock.mockResolvedValue(Response.json({ success: true, data }));
      await expect(
        mutation().mutationFn({ email: "new@example.test" }),
      ).rejects.toThrow(/confirmed/);
    }
  });
  it("does not retry an uncertain write and still offers readback invalidation", async () => {
    fetchMock.mockRejectedValue(Error("lost response"));
    const action = mutation();
    await expect(
      action.mutationFn({ email: "new@example.test" }),
    ).rejects.toThrow("lost response");
    expect(fetchMock).toHaveBeenCalledTimes(1);
    action.onSettled();
    expect(cache.invalidate).toHaveBeenCalledOnce();
  });
  it("handles only a still-current 401 as expiration", async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 401 }));
    await expect(
      mutation().mutationFn({ email: "new@example.test" }),
    ).rejects.toThrow("session expired");
    expect(useSystemAccess.getState().session.accountId).toBeNull();
    expect(loss).not.toHaveBeenCalled();
  });
  it("does not sign out a new owner after an old unauthorized write response", async () => {
    let release!: (value: Response) => void;
    fetchMock.mockImplementation(
      () => new Promise<Response>((resolve) => (release = resolve)),
    );
    const pending = mutation().mutationFn({ email: "new@example.test" });
    useSystemAccess.getState().setSession({ id: 8, group: "default", role: 1 });
    release(new Response(null, { status: 401 }));
    await expect(pending).rejects.toThrow(/session/);
    expect(useSystemAccess.getState().session.accountId).toBe(8);
    expect(loss).not.toHaveBeenCalled();
  });
});

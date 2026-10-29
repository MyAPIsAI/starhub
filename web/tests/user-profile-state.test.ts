import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { readUserProfileState } from "../src/lib/user-profile-state";

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
import { useUserProfile } from "../src/lib/api/user";
import { useSystemAccess } from "../src/lib/system-access";
import { onAuthLoss } from "../src/lib/api/http";

const state = {
  schema: "profile_state_v1",
  guarded_updates: false,
  user: {
    id: 7,
    username: "Server owner",
    email: "owner@example.test",
    role: 5,
    status: 1,
    created_at: 1735689600,
  },
};
const fetchMock = vi.fn();
const loss = vi.fn();
let unsubscribe: () => void;
beforeEach(() => {
  vi.stubGlobal("fetch", fetchMock);
  vi.stubGlobal("window", { location: { pathname: "/console/settings" } });
  fetchMock.mockReset();
  loss.mockReset();
  useSystemAccess
    .getState()
    .setSession({ id: 7, username: "Server owner", group: "default", role: 5 });
  unsubscribe = onAuthLoss(loss);
});
afterEach(() => {
  unsubscribe();
  vi.unstubAllGlobals();
});

describe("server profile state", () => {
  it("returns detached public metadata and does not grant write capabilities", () => {
    const input = {
      ...state,
      user: { ...state.user, password: "must-not-escape" },
      guarded_management: true,
    };
    const result = readUserProfileState(input, 7);
    expect(result).toEqual(state);
    expect(result.user).not.toBe(input.user);
    expect(JSON.stringify(result)).not.toContain("must-not-escape");
    expect(result).not.toHaveProperty("guarded_management");
  });
  it("rejects missing schema, foreign owners and malformed metadata instead of repairing them", () => {
    for (const value of [
      null,
      {},
      { ...state, schema: "profile_state_v2" },
      ...[
        { id: 8 },
        { username: " " },
        { email: null },
        { role: 1.5 },
        { status: "1" },
        { created_at: -1 },
        { created_at: NaN },
        { created_at: Number.MAX_SAFE_INTEGER },
      ].map((patch) => ({ ...state, user: { ...state.user, ...patch } })),
    ])
      expect(() => readUserProfileState(value, 7)).toThrow();
    for (const owner of [0, -1, 1.5, Number.MAX_SAFE_INTEGER + 1])
      expect(() => readUserProfileState(state, owner)).toThrow();
  });
  it("keeps a genuinely missing legacy date and empty email explicit", () => {
    const input = {
      ...state,
      user: { ...state.user, email: "", created_at: 0 },
    };
    expect(readUserProfileState(input, 7)).toEqual(input);
  });
  it("uses the full confirmed scope and owner query with cancellation and no retry", async () => {
    const query = useUserProfile() as any;
    const scope = useSystemAccess.getState();
    expect(query.queryKey).toEqual([
      "user-self",
      [scope.session.epoch, 7, "default", 5],
    ]);
    expect(query.retry).toBe(false);
    expect(query.staleTime).toBe(0);
    const signal = new AbortController().signal;
    fetchMock.mockResolvedValue(Response.json({ success: true, data: state }));
    expect(query.select(await query.queryFn({ signal }))).toEqual({
      ...state.user,
      can_update: false,
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/user/profile-state?expected_user_id=7",
      expect.objectContaining({ signal, credentials: "same-origin" }),
    );
    expect(() =>
      query.select({ ...state, user: { ...state.user, id: 8 } }),
    ).toThrow();
  });
  it("blocks guest and superseded manual reads before dispatch", async () => {
    const query = useUserProfile() as any;
    useSystemAccess.getState().setSession(null);
    await expect(query.queryFn({})).rejects.toThrow(/session/);
    const guest = useUserProfile() as any;
    expect(guest.enabled).toBe(false);
    await expect(guest.queryFn({})).rejects.toThrow(/session/);
    expect(fetchMock).not.toHaveBeenCalled();
  });
  it.each(["id", "epoch", "group", "role"])(
    "rejects a late profile after %s changes",
    async (field) => {
      const query = useUserProfile() as any;
      let release!: (value: Response) => void;
      fetchMock.mockImplementation(
        () => new Promise<Response>((resolve) => (release = resolve)),
      );
      const pending = query.queryFn({});
      if (field === "role") useSystemAccess.getState().setRole(10);
      else if (field === "group")
        useSystemAccess.setState((current) => ({
          session: { ...current.session, group: "vip" },
        }));
      else
        useSystemAccess.getState().setSession({
          id: field === "id" ? 8 : 7,
          group: "default",
          role: 5,
        });
      release(Response.json({ success: true, data: state }));
      await expect(pending).rejects.toThrow(/session/);
    },
  );
  it("does not let an old 401 invoke global auth loss or sign out the new account", async () => {
    let release!: (value: Response) => void;
    fetchMock.mockImplementation(
      () => new Promise<Response>((resolve) => (release = resolve)),
    );
    const pending = (useUserProfile() as any).queryFn({});
    useSystemAccess.getState().setSession({ id: 8, group: "default", role: 5 });
    release(new Response(null, { status: 401 }));
    await expect(pending).rejects.toThrow(/session/);
    expect(useSystemAccess.getState().session.accountId).toBe(8);
    expect(loss).not.toHaveBeenCalled();
  });
  it("clears a genuinely expired current session without a global callback", async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 401 }));
    await expect((useUserProfile() as any).queryFn({})).rejects.toThrow(
      "session expired",
    );
    expect(useSystemAccess.getState().session.accountId).toBeNull();
    expect(loss).not.toHaveBeenCalled();
  });
});

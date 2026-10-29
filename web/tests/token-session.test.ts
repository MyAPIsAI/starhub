import { beforeEach, describe, expect, it, vi } from "vitest";
const fake = vi.hoisted(() => ({
  state: {
    role: 1,
    session: { accountId: 7 as number | null, group: "default", epoch: 1 },
  },
  get: vi.fn(),
  post: vi.fn(),
  signOut: vi.fn(),
}));
vi.mock("../src/lib/product-preview", () => {
  throw Error("Token reads must not depend on Demo identity");
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
      setSession: (user: null) => {
        fake.signOut(user);
        fake.state.session.accountId = null;
        fake.state.session.epoch++;
      },
    }),
  },
}));
vi.mock("@tanstack/react-query", () => ({
  useQuery: (options: unknown) => options,
  useMutation: (options: unknown) => options,
  useQueryClient: () => ({}),
}));
vi.mock("../src/lib/api/http", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../src/lib/api/http")>()),
  apiGet: (...args: unknown[]) => fake.get(...args),
  apiPost: (...args: unknown[]) => fake.post(...args),
  apiPut: vi.fn(),
  apiDelete: vi.fn(),
}));
import { useTokens, verifyTokenPassword } from "../src/lib/api/tokens";
import { sessionQueryIdentity } from "../src/lib/system-access";
import { ApiError } from "../src/lib/api/http";
beforeEach(() => {
  fake.state = {
    role: 1,
    session: { accountId: 7, group: "default", epoch: 1 },
  };
  fake.get.mockReset();
  fake.post.mockReset();
  fake.signOut.mockReset();
});

describe("session-bound password verification transport", () => {
  const scope = () => sessionQueryIdentity(fake.state);
  it("posts only the current owner and password to the exact owned-token path", async () => {
    fake.post.mockResolvedValue({ verified: true, user_id: 7, token_id: 2 });
    await verifyTokenPassword(2, "verification-password", scope());
    expect(fake.post).toHaveBeenCalledExactlyOnceWith(
      "/api/token/2/verify-password",
      { expected_user_id: 7, password: "verification-password" },
      { handleUnauthorizedLocally: true },
    );
  });
  it("rejects unsafe token IDs and empty or oversized UTF-8 passwords before dispatch", async () => {
    for (const id of [0, -1, 1.5, Number.MAX_SAFE_INTEGER + 1])
      await expect(verifyTokenPassword(id, "secret", scope())).rejects.toThrow(
        /Invalid/,
      );
    for (const password of ["", "x".repeat(73), "汉".repeat(25)])
      await expect(verifyTokenPassword(2, password, scope())).rejects.toThrow(
        /Invalid/,
      );
    expect(fake.post).not.toHaveBeenCalled();
  });
  it("blocks guest and superseded action scopes before sending a password", async () => {
    const old = scope();
    fake.state.session.epoch++;
    await expect(verifyTokenPassword(2, "secret", old)).rejects.toThrow(
      /session/,
    );
    fake.state.session.accountId = null;
    await expect(verifyTokenPassword(2, "secret", scope())).rejects.toThrow(
      /session/,
    );
    expect(fake.post).not.toHaveBeenCalled();
  });
  it.each(["accountId", "epoch", "group", "role"])(
    "rejects a held verification after a change to %s",
    async (field) => {
      let release!: (value: unknown) => void;
      fake.post.mockImplementation(
        () => new Promise((resolve) => (release = resolve)),
      );
      const result = verifyTokenPassword(2, "secret", scope());
      if (field === "accountId") fake.state.session.accountId = 8;
      else if (field === "epoch") fake.state.session.epoch++;
      else if (field === "group") fake.state.session.group = "vip";
      else fake.state.role = 10;
      release({ verified: true, user_id: 7, token_id: 2 });
      await expect(result).rejects.toThrow(/session/);
    },
  );
  it("requires an explicit matching verification acknowledgement", async () => {
    for (const result of [
      undefined,
      {},
      true,
      { verified: false, user_id: 7, token_id: 2 },
      { verified: true, user_id: 8, token_id: 2 },
      { verified: true, user_id: 7, token_id: 3 },
    ]) {
      fake.post.mockResolvedValue(result);
      await expect(verifyTokenPassword(2, "secret", scope())).rejects.toThrow(
        /confirmed/,
      );
    }
  });
  it("does not retry an uncertain verification response", async () => {
    fake.post.mockRejectedValue(Error("network unavailable"));
    await expect(verifyTokenPassword(2, "secret", scope())).rejects.toThrow(
      /network/,
    );
    expect(fake.post).toHaveBeenCalledTimes(1);
  });
  it("clears only a still-current expired session", async () => {
    fake.post.mockRejectedValue(new ApiError(401, "session expired"));
    await expect(verifyTokenPassword(2, "secret", scope())).rejects.toThrow(
      /session expired/,
    );
    expect(fake.signOut).toHaveBeenCalledExactlyOnceWith(null);
    expect(fake.state.session.accountId).toBeNull();
  });
  it("does not sign out a newer account after a delayed old 401", async () => {
    let reject!: (reason: unknown) => void;
    fake.post.mockImplementation(
      () => new Promise((_, fail) => (reject = fail)),
    );
    const result = verifyTokenPassword(2, "secret", scope());
    fake.state.session.accountId = 8;
    reject(new ApiError(401, "session expired"));
    await expect(result).rejects.toThrow(/unchanged session/);
    expect(fake.signOut).not.toHaveBeenCalled();
    expect(fake.state.session.accountId).toBe(8);
  });
});

describe("confirmed API key list transport", () => {
  it("uses every confirmed session dimension and forwards cancellation", async () => {
    const query = useTokens() as any,
      signal = new AbortController().signal;
    expect(query.queryKey).toEqual(["tokens", [1, 7, "default", 1]]);
    expect(query.enabled).toBe(true);
    expect(query.retry).toBe(false);
    expect(query.staleTime).toBe(0);
    const tokens = [{ id: 2, user_id: 7 }];
    fake.get.mockResolvedValue(tokens);
    expect(await query.queryFn({ signal })).toBe(tokens);
    expect(fake.get).toHaveBeenCalledWith("/api/token", signal);
  });
  it("blocks guest and stale manual reads before dispatch", async () => {
    const old = useTokens() as any;
    fake.state.session.epoch++;
    await expect(old.queryFn({})).rejects.toThrow(/session/);
    fake.state.session.accountId = null;
    const guest = useTokens() as any;
    expect(guest.enabled).toBe(false);
    await expect(guest.queryFn({})).rejects.toThrow(/session/);
    expect(fake.get).not.toHaveBeenCalled();
  });
  it.each(["accountId", "epoch", "group", "role"])(
    "discards a delayed result after a change to %s",
    async (field) => {
      const query = useTokens() as any;
      let release!: (value: unknown) => void;
      fake.get.mockImplementation(
        () => new Promise((resolve) => (release = resolve)),
      );
      const pending = query.queryFn({});
      if (field === "accountId") fake.state.session.accountId = 8;
      else if (field === "epoch") fake.state.session.epoch++;
      else if (field === "group") fake.state.session.group = "vip";
      else fake.state.role = 10;
      release([{ id: 2, user_id: 7 }]);
      await expect(pending).rejects.toThrow(/session/);
    },
  );
  it("rejects missing and foreign ownership without retaining a partial list", async () => {
    for (const value of [
      null,
      {},
      [{ id: 2 }],
      [
        { id: 2, user_id: 7 },
        { id: 3, user_id: 8 },
      ],
    ]) {
      fake.get.mockResolvedValue(value);
      await expect((useTokens() as any).queryFn({})).rejects.toThrow(
        /ownership/,
      );
    }
  });
});

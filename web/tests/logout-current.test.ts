import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
vi.mock("@tanstack/react-query", () => ({
  useMutation: (options: unknown) => options,
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
import { useLogoutCurrent } from "../src/lib/api/user";
import { useSystemAccess } from "../src/lib/system-access";
import { onAuthLoss } from "../src/lib/api/http";
const fetchMock = vi.fn(),
  loss = vi.fn();
let stop: () => void;
const action = () => useLogoutCurrent() as any;
const receipt = { schema: "logout_receipt_v1", user_id: 7, signed_out: true };
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
describe("owned current-session logout transport", () => {
  it("uses a versioned expected-owner request and requires a matching receipt", async () => {
    fetchMock.mockResolvedValue(
      Response.json({ success: true, data: receipt }),
    );
    const current = action();
    expect(current.retry).toBe(false);
    await expect(current.mutationFn()).resolves.toEqual({
      user_id: 7,
      signed_out: true,
    });
    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe("/api/user/logout-current");
    expect(init.credentials).toBe("same-origin");
    expect(JSON.parse(init.body)).toEqual({
      schema: "logout_current_v1",
      expected_user_id: 7,
    });
    expect(loss).not.toHaveBeenCalled();
  });
  it("does not dispatch for a guest or superseded scope", async () => {
    const old = action();
    useSystemAccess.getState().setSession(null);
    await expect(old.mutationFn()).rejects.toThrow(/session/);
    await expect(action().mutationFn()).rejects.toThrow(/session/);
    expect(fetchMock).not.toHaveBeenCalled();
  });
  it.each(["id", "epoch", "group", "role"])(
    "rejects a held receipt after %s changes",
    async (field) => {
      let release!: (value: Response) => void;
      fetchMock.mockImplementation(
        () => new Promise<Response>((resolve) => (release = resolve)),
      );
      const pending = action().mutationFn();
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
      release(Response.json({ success: true, data: receipt }));
      await expect(pending).rejects.toThrow(/session/);
      expect(loss).not.toHaveBeenCalled();
    },
  );
  it("rejects missing, unversioned, false and foreign acknowledgements", async () => {
    for (const data of [
      null,
      true,
      {},
      { ...receipt, schema: "logout_receipt_v0" },
      { ...receipt, user_id: 8 },
      { ...receipt, signed_out: false },
    ]) {
      fetchMock.mockResolvedValue(Response.json({ success: true, data }));
      await expect(action().mutationFn()).rejects.toThrow(/confirmed/);
    }
  });
  it("does not retry an uncertain logout", async () => {
    fetchMock.mockRejectedValue(Error("lost response"));
    await expect(action().mutationFn()).rejects.toThrow("lost response");
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(useSystemAccess.getState().session.accountId).toBe(7);
  });
  it("clears a still-current expired session without global auth loss", async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 401 }));
    await expect(action().mutationFn()).rejects.toThrow("session expired");
    expect(useSystemAccess.getState().session.accountId).toBeNull();
    expect(loss).not.toHaveBeenCalled();
  });
  it("does not sign out a new owner after an old unauthorized logout response", async () => {
    let release!: (value: Response) => void;
    fetchMock.mockImplementation(
      () => new Promise<Response>((resolve) => (release = resolve)),
    );
    const pending = action().mutationFn();
    useSystemAccess.getState().setSession({ id: 8, group: "default", role: 1 });
    release(new Response(null, { status: 401 }));
    await expect(pending).rejects.toThrow(/session/);
    expect(useSystemAccess.getState().session.accountId).toBe(8);
    expect(loss).not.toHaveBeenCalled();
  });
});

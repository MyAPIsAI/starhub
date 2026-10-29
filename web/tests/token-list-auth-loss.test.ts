import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

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

import { useTokens } from "../src/lib/api/tokens";
import { onAuthLoss } from "../src/lib/api/http";
import { useSystemAccess } from "../src/lib/system-access";

const fetchMock = vi.fn();
const globalLoss = vi.fn();
let unsubscribe: () => void;
const read = () => (useTokens() as any).queryFn({});

beforeEach(() => {
  fetchMock.mockReset();
  globalLoss.mockReset();
  vi.stubGlobal("fetch", fetchMock);
  vi.stubGlobal("window", { location: { pathname: "/console/keys" } });
  useSystemAccess.getState().setSession({ id: 7, group: "default", role: 1 });
  unsubscribe = onAuthLoss(() => {
    globalLoss();
    useSystemAccess.getState().setSession(null);
  });
});

afterEach(() => {
  unsubscribe();
  vi.unstubAllGlobals();
});

describe("API key list auth loss through the actual HTTP client", () => {
  it("clears a still-current expired session without the global callback", async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 401 }));
    await expect(read()).rejects.toThrow("session expired");
    expect(useSystemAccess.getState().session.accountId).toBeNull();
    expect(globalLoss).not.toHaveBeenCalled();
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it.each(["account", "epoch", "group", "role"])(
    "ignores a delayed old 401 after a confirmed %s change",
    async (field) => {
      let release!: (value: Response) => void;
      fetchMock.mockImplementation(
        () => new Promise<Response>((resolve) => (release = resolve)),
      );
      const pending = read();
      if (field === "role") useSystemAccess.getState().setRole(10);
      else if (field === "group")
        useSystemAccess.setState((state) => ({
          session: { ...state.session, group: "vip" },
        }));
      else
        useSystemAccess.getState().setSession({
          id: field === "account" ? 8 : 7,
          group: "default",
          role: 1,
        });
      const confirmed = useSystemAccess.getState();
      release(new Response(null, { status: 401 }));
      await expect(pending).rejects.toThrow("unchanged session");
      expect(useSystemAccess.getState()).toBe(confirmed);
      expect(globalLoss).not.toHaveBeenCalled();
      expect(fetchMock).toHaveBeenCalledTimes(1);
    },
  );

  it("preserves the confirmed owner when a read fails without an auth error", async () => {
    const confirmed = useSystemAccess.getState();
    fetchMock.mockResolvedValue(
      Response.json(
        { success: false, message: "unavailable" },
        { status: 503 },
      ),
    );
    await expect(read()).rejects.toThrow("unavailable");
    expect(useSystemAccess.getState()).toBe(confirmed);
    expect(globalLoss).not.toHaveBeenCalled();
  });

  it("still returns an owned list through the native response envelope", async () => {
    const tokens = [{ id: 2, user_id: 7 }];
    fetchMock.mockResolvedValue(Response.json({ success: true, data: tokens }));
    await expect(read()).resolves.toEqual(tokens);
    expect(useSystemAccess.getState().session.accountId).toBe(7);
    expect(globalLoss).not.toHaveBeenCalled();
  });
});

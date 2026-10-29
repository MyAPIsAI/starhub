import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { apiGet, apiRequest, onAuthLoss } from "./http";

describe("confirmed request owner", () => {
  let stop: () => void;
  let owner: number | null;
  const authLoss = vi.fn();
  const response = (headers?: HeadersInit) =>
    new Response(JSON.stringify({ success: true, data: "owned" }), { headers });

  beforeEach(() => {
    owner = 7;
    authLoss.mockClear();
    stop = onAuthLoss(
      authLoss,
      () => "scope",
      () => owner,
    );
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(response()));
    vi.stubGlobal("window", { location: { pathname: "/console" } });
  });
  afterEach(() => {
    stop();
    vi.unstubAllGlobals();
  });

  it("forces the confirmed owner while preserving caller headers and signal", async () => {
    const signal = new AbortController().signal;
    const original = new Headers({
      "x-starhub-expected-user-id": "8",
      "X-Test": "kept",
    });
    await apiRequest("/api/token", {
      method: "POST",
      headers: original,
      body: "{}",
      signal,
    });
    const init = vi.mocked(fetch).mock.calls[0][1]!;
    const headers = new Headers(init.headers);
    expect(headers.get("X-StarHub-Expected-User-ID")).toBe("7");
    expect(headers.get("X-Test")).toBe("kept");
    expect(headers.get("Content-Type")).toBe("application/json");
    expect(original.get("X-StarHub-Expected-User-ID")).toBe("8");
    expect(init.signal).toBe(signal);
    expect(init.credentials).toBe("same-origin");
  });

  it.each([
    ["GET", "/api/user/self?fresh=1"],
    ["POST", "/api/user/login"],
    ["POST", "/api/user/register"],
    ["POST", "/v1/responses"],
  ])(
    "lets %s %s discover identities or use independent API keys",
    async (method, path) => {
      vi.mocked(fetch).mockResolvedValue(
        response({ "X-StarHub-Owner-Guard": "v1", "X-StarHub-User-ID": "8" }),
      );
      await expect(apiRequest(path, { method })).resolves.toBe("owned");
      expect(
        new Headers(vi.mocked(fetch).mock.calls[0][1]?.headers).has(
          "X-StarHub-Expected-User-ID",
        ),
      ).toBe(false);
    },
  );

  it("still guards writes to the legacy self endpoint", async () => {
    await apiRequest("/api/user/self", { method: "PUT" });
    expect(
      new Headers(vi.mocked(fetch).mock.calls[0][1]?.headers).get(
        "X-StarHub-Expected-User-ID",
      ),
    ).toBe("7");
  });

  it.each([0, -1, NaN, Infinity, 9007199254740992])(
    "rejects invalid confirmed owner %s before dispatch",
    async (value) => {
      owner = value;
      await expect(apiGet("/api/token")).rejects.toMatchObject({ status: 409 });
      expect(fetch).not.toHaveBeenCalled();
    },
  );

  it("keeps anonymous public requests compatible", async () => {
    owner = null;
    await apiGet("/api/status");
    expect(
      new Headers(vi.mocked(fetch).mock.calls[0][1]?.headers).has(
        "X-StarHub-Expected-User-ID",
      ),
    ).toBe(false);
  });

  it.each<Record<string, string>>([
    { "X-StarHub-Owner-Guard": "v1", "X-StarHub-User-ID": "8" },
    { "X-StarHub-Owner-Guard": "v2", "X-StarHub-User-ID": "7" },
    { "X-StarHub-Owner-Guard": "v1", "X-StarHub-User-ID": "07" },
    { "X-StarHub-Owner-Guard": "v1" },
    { "X-StarHub-User-ID": "7" },
  ])("refuses inconsistent explicit owner metadata %j", async (headers) => {
    vi.mocked(fetch).mockResolvedValue(response(headers));
    await expect(apiGet("/api/wallet")).rejects.toMatchObject({ status: 409 });
    expect(authLoss).not.toHaveBeenCalled();
  });

  it("accepts the dispatch owner after the UI switches without relabeling it", async () => {
    let release!: (res: Response) => void;
    vi.mocked(fetch).mockImplementationOnce(
      () => new Promise((resolve) => (release = resolve)),
    );
    const pending = apiGet("/api/wallet");
    owner = 8;
    release(
      response({ "X-StarHub-Owner-Guard": "v1", "X-StarHub-User-ID": "7" }),
    );
    await expect(pending).resolves.toBe("owned");
    expect(authLoss).not.toHaveBeenCalled();
  });
});

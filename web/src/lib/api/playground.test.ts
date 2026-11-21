import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defaultDemoRequest } from "../request-contract";
import { sessionQueryIdentity, useSystemAccess } from "../system-access";
import { sendPlaygroundRequest, unlockPlaygroundKey } from "./playground";

const identity = () => sessionQueryIdentity(useSystemAccess.getState());
const signedInUser = { id: 7, group: "default", role: 1 };

describe("session-authorized playground requests", () => {
  beforeEach(() => {
    useSystemAccess.getState().setSession(signedInUser);
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("{}")));
  });
  afterEach(() => {
    useSystemAccess.getState().setSession(null);
    vi.unstubAllGlobals();
  });

  it.each([
    ["OpenAI", "/chat/completions"],
    ["Anthropic", "/messages"],
    ["Gemini", "/gemini/test-model:generateContent"],
  ] as const)(
    "uses the owned-key route for %s without sending credentials",
    async (protocol, suffix) => {
      const signal = new AbortController().signal;
      await sendPlaygroundRequest(
        12,
        {
          ...defaultDemoRequest("test-model"),
          protocol,
          sessionId: "playground-affinity-fixture",
        },
        identity(),
        signal,
      );
      const [url, init] = vi.mocked(fetch).mock.calls[0];
      expect(url).toBe(`/api/playground/12${suffix}`);
      expect(init?.credentials).toBe("same-origin");
      expect(init?.signal).toBe(signal);
      const headers = new Headers(init?.headers);
      expect(headers.get("X-Session-Id")).toBe("playground-affinity-fixture");
      expect(headers.get("X-StarHub-Expected-User-ID")).toBe("7");
      for (const header of ["Authorization", "x-api-key", "x-goog-api-key"])
        expect(headers.has(header)).toBe(false);
      expect(JSON.parse(String(init?.body))).toMatchObject(
        protocol === "Gemini"
          ? { contents: expect.any(Array) }
          : { model: "test-model" },
      );
    },
  );

  it("refuses dispatch after an account change", async () => {
    const previous = identity();
    useSystemAccess.getState().setSession({ ...signedInUser, id: 8 });
    await expect(
      sendPlaygroundRequest(
        12,
        defaultDemoRequest(),
        previous,
        new AbortController().signal,
      ),
    ).rejects.toThrow("unchanged session");
    expect(fetch).not.toHaveBeenCalled();
  });

  it("rejects a late response from a superseded session", async () => {
    let release!: (response: Response) => void;
    vi.mocked(fetch).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          release = resolve;
        }),
    );
    const pending = sendPlaygroundRequest(
      12,
      defaultDemoRequest(),
      identity(),
      new AbortController().signal,
    );
    useSystemAccess.getState().setSession(signedInUser);
    release(new Response("{}"));
    await expect(pending).rejects.toThrow("unchanged session");
  });

  it("requires unlock confirmation for the exact owner and key", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          success: true,
          data: { verified: true, user_id: 7, token_id: 13 },
        }),
      ),
    );
    await expect(
      unlockPlaygroundKey(12, "FixturePassword92", identity()),
    ).rejects.toThrow("not confirmed");
    const [url, init] = vi.mocked(fetch).mock.calls[0];
    expect(url).toBe("/api/playground/12/unlock");
    expect(JSON.parse(String(init?.body))).toEqual({
      expected_user_id: 7,
      password: "FixturePassword92",
    });
  });
});

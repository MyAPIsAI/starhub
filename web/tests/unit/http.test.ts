import { afterEach, describe, expect, it, vi } from "vitest";
import { api, invalidateSessionRequests } from "@/lib/http";

afterEach(() => vi.unstubAllGlobals());
describe("API request isolation", () => {
  it("sends cookies without a stored bearer credential", async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify({ success: true, data: [] })),
      );
    vi.stubGlobal("fetch", fetcher);
    expect(await api("/token/")).toEqual([]);
    expect(fetcher.mock.calls[0]?.[1]).toMatchObject({
      credentials: "same-origin",
    });
    expect(fetcher.mock.calls[0]?.[1].headers).toBeUndefined();
  });
  it("rejects non-JSON proxy errors and unsafe integers", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValueOnce(
          new Response("<html>bad gateway</html>", { status: 502 }),
        )
        .mockResolvedValueOnce(
          new Response('{"success":true,"data":{"quota":9007199254740993}}'),
        ),
    );
    await expect(api("/user/self")).rejects.toMatchObject({ status: 502 });
    await expect(api("/user/self")).rejects.toMatchObject({ status: 422 });
  });
  it("invalidates cached identity on 401, but not network errors", async () => {
    const listener = vi.fn();
    window.addEventListener("starhub:unauthorized", listener);
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockRejectedValueOnce(new TypeError("offline"))
        .mockResolvedValueOnce(new Response("{}", { status: 401 })),
    );
    await expect(api("/user/self")).rejects.toMatchObject({ status: 0 });
    expect(listener).not.toHaveBeenCalled();
    await expect(api("/user/self")).rejects.toMatchObject({ status: 401 });
    expect(listener).toHaveBeenCalledOnce();
    window.removeEventListener("starhub:unauthorized", listener);
  });
  it("drops late responses from a previous session", async () => {
    let finish!: (response: Response) => void;
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation(
        () =>
          new Promise<Response>((resolve) => {
            finish = resolve;
          }),
      ),
    );
    const result = api("/token/");
    invalidateSessionRequests();
    finish(
      new Response('{"success":true,"data":[{"id":1,"key":"old-secret"}]}'),
    );
    await expect(result).rejects.toMatchObject({ name: "AbortError" });
  });
});

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { getSelf } from "./auth";
import { ApiError, apiGet, apiPost, onAuthLoss } from "./http";
import { sessionQueryIdentity, useSystemAccess } from "../system-access";

describe("unauthorized responses", () => {
  const authLoss = vi.fn();
  let unsubscribe: () => void;

  beforeEach(() => {
    authLoss.mockClear();
    unsubscribe = onAuthLoss(authLoss);
    vi.stubGlobal("window", { location: { pathname: "/models" } });
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ status: 401 }));
  });

  afterEach(() => {
    unsubscribe();
    vi.unstubAllGlobals();
  });

  it.each(["/", "/models", "/models/gpt-4o-mini", "/console"])(
    "lets the startup probe handle anonymous visitors at %s",
    async (pathname) => {
      window.location.pathname = pathname;
      await expect(
        getSelf({ handleUnauthorizedLocally: true }),
      ).rejects.toMatchObject({ status: 401 });
      expect(authLoss).not.toHaveBeenCalled();
    },
  );

  it.each(["/api/user/self", "/api/token"])(
    "still notifies auth loss by default for %s",
    async (path) => {
      await expect(apiGet(path)).rejects.toBeInstanceOf(ApiError);
      expect(authLoss).toHaveBeenCalledOnce();
    },
  );

  it.each(["/login", "/register", "/forgot-password"])(
    "avoids redirect loops on %s",
    async (pathname) => {
      window.location.pathname = pathname;
      await expect(apiGet("/api/token")).rejects.toMatchObject({ status: 401 });
      expect(authLoss).not.toHaveBeenCalled();
    },
  );

  it("keeps the session cookie and cancellation signal on a successful probe", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      new Response(JSON.stringify({ success: true, data: { id: 1 } })),
    );
    const controller = new AbortController();
    await expect(
      getSelf({ handleUnauthorizedLocally: true }, controller.signal),
    ).resolves.toEqual({ id: 1 });
    expect(fetch).toHaveBeenCalledWith(
      "/api/user/self",
      expect.objectContaining({
        credentials: "same-origin",
        signal: controller.signal,
      }),
    );
    expect(authLoss).not.toHaveBeenCalled();
  });

  it("lets a session-bound password POST handle an old unauthorized response locally", async () => {
    await expect(
      apiPost(
        "/api/token/1/verify-password",
        {
          expected_user_id: 1,
          password: "synthetic-verification",
        },
        { handleUnauthorizedLocally: true },
      ),
    ).rejects.toMatchObject({ status: 401 });
    expect(authLoss).not.toHaveBeenCalled();
    expect(fetch).toHaveBeenCalledWith(
      "/api/token/1/verify-password",
      expect.objectContaining({ method: "POST", credentials: "same-origin" }),
    );
  });

  it("does not let an old subscription clear a newer listener", async () => {
    const nextListener = vi.fn();
    const stopNext = onAuthLoss(nextListener);
    unsubscribe();
    await expect(apiGet("/api/token")).rejects.toMatchObject({ status: 401 });
    expect(authLoss).not.toHaveBeenCalled();
    expect(nextListener).toHaveBeenCalledOnce();
    stopNext();
  });

  it.each(["account", "epoch", "group", "role"])(
    "does not notify global auth loss for a delayed request after %s changes",
    async (field) => {
      const initial = useSystemAccess.getState();
      useSystemAccess
        .getState()
        .setSession({ id: 7, group: "default", role: 1 });
      const stop = onAuthLoss(authLoss, () =>
        JSON.stringify(sessionQueryIdentity(useSystemAccess.getState())),
      );
      try {
        let release!: (value: Response) => void;
        vi.mocked(fetch).mockImplementationOnce(
          () => new Promise<Response>((resolve) => (release = resolve)),
        );
        const pending = apiGet("/api/wallet");
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
        await expect(pending).rejects.toMatchObject({ status: 401 });
        expect(authLoss).not.toHaveBeenCalled();
        expect(useSystemAccess.getState()).toBe(confirmed);
      } finally {
        stop();
        useSystemAccess.setState(initial, true);
      }
    },
  );

  it("still notifies auth loss for a scoped request when its session is unchanged", async () => {
    const stop = onAuthLoss(authLoss, () => "unchanged-scope");
    try {
      await expect(apiGet("/api/wallet")).rejects.toMatchObject({
        status: 401,
      });
      expect(authLoss).toHaveBeenCalledOnce();
    } finally {
      stop();
    }
  });

  it("does not deliver an old request failure to a replacement subscription", async () => {
    let release!: (value: Response) => void;
    vi.mocked(fetch).mockImplementationOnce(
      () => new Promise<Response>((resolve) => (release = resolve)),
    );
    const pending = apiGet("/api/wallet");
    const next = vi.fn(),
      stop = onAuthLoss(next);
    try {
      release(new Response(null, { status: 401 }));
      await expect(pending).rejects.toMatchObject({ status: 401 });
      expect(authLoss).not.toHaveBeenCalled();
      expect(next).not.toHaveBeenCalled();
    } finally {
      stop();
    }
  });

  it("keeps a newer subscription even when it reuses the same listener function", async () => {
    const stop = onAuthLoss(authLoss);
    unsubscribe();
    try {
      await expect(apiGet("/api/wallet")).rejects.toMatchObject({
        status: 401,
      });
      expect(authLoss).toHaveBeenCalledOnce();
    } finally {
      stop();
    }
  });
});

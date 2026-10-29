import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
const cache = vi.hoisted(() => ({
  data: undefined as unknown,
  status: "success",
  invalidate: vi.fn(),
}));
vi.mock("@tanstack/react-query", () => ({
  useMutation: (options: unknown) => options,
  useQueryClient: () => ({
    getQueryData: (key: unknown[]) =>
      key[0] === "token-choices"
        ? {
            schema: "owned_token_choices_v1",
            guarded_actions: true,
            user_id: 7,
            revision: "c".repeat(64),
            choices: [
              {
                group: "default",
                model: "server-only-model",
                protocols: ["OpenAI"],
              },
            ],
          }
        : cache.data,
    getQueryState: (key: unknown[]) => ({
      status: key[0] === "token-choices" ? "success" : cache.status,
    }),
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
import {
  useCreateToken,
  useUpdateToken,
  useDeleteToken,
  useRegenerateToken,
} from "../src/lib/api/tokens";
import { onAuthLoss } from "../src/lib/api/http";
import {
  sessionQueryIdentity,
  useSystemAccess,
} from "../src/lib/system-access";
const fetchMock = vi.fn(),
  loss = vi.fn();
let stop: () => void;
const secret = {
  id: 2,
  user_id: 7,
  key: "sk-" + "s".repeat(48),
  management_schema: "owned_token_v1",
  management_revision: "a".repeat(64),
};
const changes = [
  {
    name: "create",
    hook: useCreateToken,
    input: { name: "new" },
    method: "POST",
    path: "/api/token",
    result: secret,
  },
  {
    name: "update",
    hook: useUpdateToken,
    input: { id: 2, name: "changed", expected_revision: "a".repeat(64) },
    method: "PUT",
    path: "/api/token",
    result: undefined,
  },
  {
    name: "delete",
    hook: useDeleteToken,
    input: { id: 2, expected_revision: "a".repeat(64) },
    method: "DELETE",
    path: "/api/token/2",
    result: undefined,
  },
  {
    name: "regenerate",
    hook: useRegenerateToken,
    input: { id: 2, expected_revision: "a".repeat(64) },
    method: "POST",
    path: "/api/token/2/regenerate",
    result: secret,
  },
];
const setChange = (field: string) => {
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
};
function wrap(token: unknown, action: string) {
  const call = fetchMock.mock.calls.at(-1);
  const body = call ? JSON.parse(call[1].body) : {};
  return {
    schema: "owned_token_receipt_v1",
    user_id: 7,
    operation_id: body.operation_id,
    action,
    token_id: 2,
    revision: "a".repeat(64),
    token,
  };
}
beforeEach(() => {
  fetchMock.mockReset();
  loss.mockReset();
  cache.invalidate.mockReset();
  cache.status = "success";
  cache.data = [
    {
      id: 2,
      user_id: 7,
      management_schema: "owned_token_v1",
      management_revision: "a".repeat(64),
    },
  ];
  vi.stubGlobal("fetch", fetchMock);
  vi.stubGlobal("window", { location: { pathname: "/console/keys" } });
  useSystemAccess.getState().setSession({ id: 7, group: "default", role: 1 });
  stop = onAuthLoss(
    loss,
    () => JSON.stringify(sessionQueryIdentity(useSystemAccess.getState())),
    () => useSystemAccess.getState().session.accountId,
  );
});
afterEach(() => {
  stop();
  vi.unstubAllGlobals();
});
describe.each(changes)(
  "$name originating key scope",
  ({ hook, input, method, path, result, name }) => {
    const action = () => hook() as any;
    it("uses the actual HTTP client without automatic retry and invalidates only its exact list", async () => {
      const current = action(),
        identity = sessionQueryIdentity(useSystemAccess.getState());
      expect(current.retry).toBe(false);
      fetchMock.mockImplementation(async () =>
        Response.json({ success: true, data: wrap(result, name) }),
      );
      await expect(current.mutationFn(input)).resolves.toEqual(result ?? null);
      current.onSettled();
      const [url, init] = fetchMock.mock.calls[0];
      expect(url).toBe("/api/token/owned-action");
      expect(JSON.parse(init.body).action).toBe(name);
      expect(init.method).toBe("POST");
      expect(new Headers(init.headers).get("X-StarHub-Expected-User-ID")).toBe(
        "7",
      );
      expect(cache.invalidate).toHaveBeenCalledExactlyOnceWith({
        queryKey: ["tokens", identity],
        exact: true,
      });
      expect(loss).not.toHaveBeenCalled();
    });
    it.each(["account", "epoch", "group", "role"])(
      "blocks queued actions after %s changes",
      async (field) => {
        const current = action();
        setChange(field);
        await expect(current.mutationFn(input)).rejects.toThrow(/session/);
        current.onSettled();
        expect(fetchMock).not.toHaveBeenCalled();
        expect(cache.invalidate).not.toHaveBeenCalled();
      },
    );
    it.each(["account", "epoch", "group", "role"])(
      "discards held success after %s changes",
      async (field) => {
        let release!: (value: Response) => void;
        fetchMock.mockImplementation(
          () => new Promise<Response>((resolve) => (release = resolve)),
        );
        const current = action(),
          pending = current.mutationFn(input);
        setChange(field);
        release(Response.json({ success: true, data: result }));
        await expect(pending).rejects.toThrow(/session/);
        current.onSettled();
        expect(cache.invalidate).not.toHaveBeenCalled();
        expect(loss).not.toHaveBeenCalled();
      },
    );
    it("does not let a held old401 clear a newer account", async () => {
      let release!: (value: Response) => void;
      fetchMock.mockImplementation(
        () => new Promise<Response>((resolve) => (release = resolve)),
      );
      const pending = action().mutationFn(input);
      setChange("account");
      release(new Response(null, { status: 401 }));
      await expect(pending).rejects.toThrow(/session/);
      expect(useSystemAccess.getState().session.accountId).toBe(8);
      expect(loss).not.toHaveBeenCalled();
    });
    it("clears only the current expired identity", async () => {
      fetchMock.mockResolvedValue(new Response(null, { status: 401 }));
      await expect(action().mutationFn(input)).rejects.toMatchObject({
        status: 401,
      });
      expect(useSystemAccess.getState().session.accountId).toBeNull();
      expect(loss).not.toHaveBeenCalled();
    });
    it("does not retry an uncertain result and still requests current-owner readback", async () => {
      fetchMock.mockRejectedValue(Error("connection lost"));
      const current = action();
      await expect(current.mutationFn(input)).rejects.toThrow(
        "connection lost",
      );
      current.onSettled();
      expect(fetchMock).toHaveBeenCalledTimes(1);
      expect(cache.invalidate).toHaveBeenCalledTimes(1);
    });
    it("blocks a guest before dispatch", async () => {
      useSystemAccess.getState().setSession(null);
      await expect(action().mutationFn(input)).rejects.toThrow(/session/);
      expect(fetchMock).not.toHaveBeenCalled();
    });
    if (name !== "create") {
      it.each([0, -1, 1.5, Number.MAX_SAFE_INTEGER + 1])(
        "rejects unsafe target key ID %s before dispatch",
        async (id) => {
          const invalid = name === "update" ? { id, name: "changed" } : { id };
          await expect(action().mutationFn(invalid)).rejects.toThrow(
            /Reload owned/,
          );
          expect(fetchMock).not.toHaveBeenCalled();
        },
      );
      it.each([
        { data: undefined },
        { data: [] },
        { data: [{ id: 2, user_id: 8 }] },
        {
          data: [
            { id: 2, user_id: 7 },
            { id: 3, user_id: 8 },
          ],
        },
        {
          data: [
            { id: 2, user_id: 7 },
            { id: 2, user_id: 7 },
          ],
        },
      ])(
        "refuses unavailable, mixed or ambiguous owner rows %j",
        async ({ data }) => {
          cache.data = data;
          await expect(action().mutationFn(input)).rejects.toThrow(
            /Reload owned/,
          );
          expect(fetchMock).not.toHaveBeenCalled();
        },
      );
      it("does not accept cached rows after the list failed", async () => {
        cache.status = "error";
        await expect(action().mutationFn(input)).rejects.toThrow(
          /Reload owned/,
        );
        expect(fetchMock).not.toHaveBeenCalled();
      });
    }
    if (name === "create" || name === "regenerate") {
      it.each([
        undefined,
        {},
        { ...secret, user_id: 8 },
        { ...secret, key: "sk-aaa******bbbb" },
        { ...secret, id: 0 },
        { ...secret, id: 9007199254740992 },
      ])("refuses unconfirmed or foreign one-time secrets %j", async (data) => {
        fetchMock.mockImplementation(async () =>
          Response.json({ success: true, data: wrap(data, name) }),
        );
        await expect(action().mutationFn(input)).rejects.toThrow(
          /not confirmed/,
        );
        expect(fetchMock).toHaveBeenCalledTimes(1);
      });
    }
    if (name === "regenerate")
      it("requires the regenerated receipt to name the exact requested key", async () => {
        fetchMock.mockImplementation(async () =>
          Response.json({
            success: true,
            data: wrap({ ...secret, id: 3 }, name),
          }),
        );
        await expect(action().mutationFn(input)).rejects.toThrow(
          /not confirmed/,
        );
      });
  },
);

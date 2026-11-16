import { useQuery } from "@tanstack/react-query";
import { apiGet, apiPost } from "./http";
import { assertTokenSession } from "./tokens";
import {
  useSessionQueryIdentity,
  type sessionQueryIdentity,
} from "../system-access";
import { buildRelayRequest } from "../gateway-request";
import type { DemoRequest } from "../request-contract";

type Identity = ReturnType<typeof sessionQueryIdentity>;
type PlaygroundGrant = { user_id: number; token_id: number; unlocked: boolean };

export function usePlaygroundGrant(tokenID?: number, revision?: string) {
  const identity = useSessionQueryIdentity();
  return useQuery({
    queryKey: ["playground-grant", identity, tokenID, revision],
    enabled:
      identity[1] !== null && Number.isSafeInteger(tokenID) && tokenID! > 0,
    staleTime: 0,
    retry: false,
    queryFn: async ({ signal }) => {
      assertTokenSession(identity);
      const result = await apiGet<PlaygroundGrant>(
        `/api/playground/${tokenID}/status`,
        signal,
        { handleUnauthorizedLocally: true },
      );
      assertTokenSession(identity);
      if (
        result.user_id !== identity[1] ||
        result.token_id !== tokenID ||
        typeof result.unlocked !== "boolean"
      )
        throw Error("Playground grant ownership could not be confirmed");
      return result;
    },
  });
}

export async function unlockPlaygroundKey(
  tokenID: number,
  password: string,
  identity: Identity,
) {
  assertTokenSession(identity);
  if (
    !Number.isSafeInteger(tokenID) ||
    tokenID <= 0 ||
    !password ||
    new TextEncoder().encode(password).length > 72
  )
    throw Error("Invalid playground unlock input");
  const result = await apiPost<{
    verified: boolean;
    user_id: number;
    token_id: number;
  }>(
    `/api/playground/${tokenID}/unlock`,
    { expected_user_id: identity[1], password },
    { handleUnauthorizedLocally: true },
  );
  assertTokenSession(identity);
  if (
    result.verified !== true ||
    result.user_id !== identity[1] ||
    result.token_id !== tokenID
  )
    throw Error("Playground unlock was not confirmed");
}

export async function sendPlaygroundRequest(
  tokenID: number,
  request: DemoRequest,
  identity: Identity,
  signal: AbortSignal,
) {
  assertTokenSession(identity);
  if (!Number.isSafeInteger(tokenID) || tokenID <= 0)
    throw Error("Invalid playground key");
  const call = buildRelayRequest(request);
  const suffix =
    request.protocol === "Gemini"
      ? `/gemini/${encodeURIComponent(request.model)}:generateContent`
      : call.endpoint.slice(3);
  const response = await fetch(`/api/playground/${tokenID}${suffix}`, {
    ...call.init,
    credentials: "same-origin",
    headers: {
      ...call.init.headers,
      "X-StarHub-Expected-User-ID": String(identity[1]),
    },
    signal,
  });
  assertTokenSession(identity);
  return response;
}

import type { SessionInfo } from "./api/user";

export interface UserSessionState {
  schema: "session_state_v1";
  guarded_management: true;
  user_id: number;
  sessions: SessionInfo[];
}

export function readUserSessionState(
  value: unknown,
  owner: number,
): UserSessionState {
  const data = value as Record<string, unknown> | null;
  if (
    !data ||
    typeof data !== "object" ||
    data.schema !== "session_state_v1" ||
    data.guarded_management !== true ||
    data.user_id !== owner ||
    !Number.isSafeInteger(owner) ||
    owner <= 0 ||
    !Array.isArray(data.sessions)
  )
    throw Error("Guarded account device management is unavailable");
  let current = 0;
  const sessions = data.sessions.map((raw): SessionInfo => {
    if (!raw || typeof raw !== "object")
      throw Error("Invalid account device state");
    const item = raw as Record<string, unknown>;
    if (
      !Number.isSafeInteger(item.created_at) ||
      !Number.isSafeInteger(item.expires_at) ||
      typeof item.is_current !== "boolean" ||
      (item.created_at as number) < 0 ||
      (item.expires_at as number) < (item.created_at as number) ||
      !Number.isFinite(new Date((item.expires_at as number) * 1000).getTime())
    )
      throw Error("Invalid account device state");
    if (item.is_current && ++current > 1)
      throw Error("Ambiguous current account device");
    return {
      created_at: item.created_at as number,
      expires_at: item.expires_at as number,
      is_current: item.is_current,
    };
  });
  return {
    schema: "session_state_v1",
    guarded_management: true,
    user_id: owner,
    sessions,
  };
}

export function readSessionTermination(value: unknown, owner: number) {
  const data = value as Record<string, unknown> | null;
  if (
    !data ||
    typeof data !== "object" ||
    !Number.isSafeInteger(owner) ||
    owner <= 0 ||
    data.user_id !== owner ||
    !Number.isSafeInteger(data.terminated) ||
    (data.terminated as number) < 0
  )
    throw Error("Account device termination was not confirmed");
  return { user_id: owner, terminated: data.terminated as number };
}

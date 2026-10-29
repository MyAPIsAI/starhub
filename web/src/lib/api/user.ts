// User self-service API hooks — typed wrappers around the Go backend's
// guarded account profile and device endpoints.
//
// Key behavior:
//   - Change password requires current_password; the backend verifies it.
//   - Email is a profile field and does not require verification.
//   - Both mutations invalidate the cached user profile so callers refetch.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiPut, apiPost, apiGet, ApiError } from "./http";
import {
  sessionQueryIdentity,
  useSessionQueryIdentity,
  useSystemAccess,
} from "../system-access";
import {
  readUserSessionState,
  readSessionTermination,
} from "../user-session-state";
import { readUserProfileState } from "../user-profile-state";
import { readUserSecurityState } from "../user-security-state";

const USER_KEY = ["user-self"] as const;
const SESSIONS_KEY = ["user-sessions"] as const;

export interface UpdateSelfInput {
  expected_email?: string;
  current_password?: string;
  new_password?: string;
  email?: string;
}

export function useUpdateSelf() {
  const queryClient = useQueryClient();
  const scope = useSessionQueryIdentity();
  return useMutation({
    retry: false,
    mutationFn: async (input: UpdateSelfInput) => {
      assertUserSession(scope);
      const key = [...USER_KEY, scope];
      if (queryClient.getQueryState(key)?.status !== "success")
        throw Error("Reload the account profile before updating");
      const profile = readUserProfileState(
        queryClient.getQueryData(key),
        scope[1]!,
      );
      if (!profile.guarded_updates)
        throw Error("Guarded account profile updates are unavailable");
      if (
        input.expected_email !== undefined &&
        input.expected_email !== profile.user.email
      )
        throw new ApiError(
          409,
          "account profile changed; reload before updating",
        );
      const password = input.new_password !== undefined;
      if (password === (input.email !== undefined))
        throw Error("Exactly one profile field is required");
      if (
        password &&
        (typeof input.current_password !== "string" ||
          !input.current_password ||
          new TextEncoder().encode(input.current_password).length > 72 ||
          typeof input.new_password !== "string" ||
          new TextEncoder().encode(input.new_password).length < 8 ||
          new TextEncoder().encode(input.new_password).length > 64)
      )
        throw Error("Invalid password update");
      if (
        !password &&
        (input.current_password !== undefined ||
          typeof input.email !== "string" ||
          input.email !== input.email.trim() ||
          new TextEncoder().encode(input.email).length > 64)
      )
        throw Error("Invalid email update");
      const result = await inUserSession(scope, () =>
        apiPut<unknown>(
          "/api/user/profile-state",
          {
            expected_user_id: scope[1],
            expected_email: profile.user.email,
            ...(password
              ? {
                  current_password: input.current_password,
                  new_password: input.new_password,
                }
              : { email: input.email }),
          },
          { handleUnauthorizedLocally: true },
        ),
      );
      const receipt = result as Record<string, unknown> | null;
      if (
        !receipt ||
        typeof receipt !== "object" ||
        receipt.user_id !== scope[1] ||
        receipt.updated_field !== (password ? "password" : "email")
      )
        throw Error("Account profile update was not confirmed");
      return { user_id: scope[1]!, updated_field: receipt.updated_field };
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: USER_KEY });
    },
  });
}

/**
 * Deactivate the current user's account. The backend disables the
 * user and clears the session. Wallet balance and keys are preserved.
 */
export async function deactivateSelf(): Promise<void> {
  await apiPost<null>("/api/user/deactivate");
}

export interface SessionInfo {
  created_at: number;
  expires_at: number;
  is_current: boolean;
}

type UserSessionScope = ReturnType<typeof sessionQueryIdentity>;
const sessionStateKey = (scope: UserSessionScope) =>
  [...SESSIONS_KEY, scope] as const;

function assertUserSession(scope: UserSessionScope) {
  if (
    scope[1] === null ||
    !Number.isSafeInteger(scope[1]) ||
    scope[1] <= 0 ||
    JSON.stringify(scope) !==
      JSON.stringify(sessionQueryIdentity(useSystemAccess.getState()))
  )
    throw Error("A confirmed unchanged session is required for account data");
}

async function inUserSession<T>(
  scope: UserSessionScope,
  request: () => Promise<T>,
): Promise<T> {
  assertUserSession(scope);
  try {
    const result = await request();
    assertUserSession(scope);
    return result;
  } catch (error) {
    assertUserSession(scope);
    if (error instanceof ApiError && error.status === 401)
      useSystemAccess.getState().setSession(null);
    throw error;
  }
}

export function useSessions() {
  const scope = useSessionQueryIdentity();
  return useQuery({
    queryKey: sessionStateKey(scope),
    enabled:
      scope[1] !== null && Number.isSafeInteger(scope[1]) && scope[1] > 0,
    staleTime: 0,
    retry: false,
    queryFn: async ({ signal }) =>
      readUserSessionState(
        await inUserSession(scope, () =>
          apiGet<unknown>(
            `/api/user/session-state?expected_user_id=${scope[1]}`,
            signal,
            { handleUnauthorizedLocally: true },
          ),
        ),
        scope[1]!,
      ),
    select: (data) => readUserSessionState(data, scope[1]!).sessions,
  });
}

export function useUserProfile() {
  const scope = useSessionQueryIdentity();
  return useQuery({
    queryKey: [...USER_KEY, scope],
    enabled:
      scope[1] !== null && Number.isSafeInteger(scope[1]) && scope[1] > 0,
    staleTime: 0,
    retry: false,
    queryFn: async ({ signal }) =>
      readUserProfileState(
        await inUserSession(scope, () =>
          apiGet<unknown>(
            `/api/user/profile-state?expected_user_id=${scope[1]}`,
            signal,
            { handleUnauthorizedLocally: true },
          ),
        ),
        scope[1]!,
      ),
    select: (data) => {
      const profile = readUserProfileState(data, scope[1]!);
      return { ...profile.user, can_update: profile.guarded_updates };
    },
  });
}

export function useTerminateOtherSessions() {
  const queryClient = useQueryClient();
  const scope = useSessionQueryIdentity();
  return useMutation({
    retry: false,
    mutationFn: async () => {
      assertUserSession(scope);
      if (
        queryClient.getQueryState(sessionStateKey(scope))?.status !== "success"
      )
        throw Error(
          "A current guarded device list is required before termination",
        );
      readUserSessionState(
        queryClient.getQueryData(sessionStateKey(scope)),
        scope[1]!,
      );
      return readSessionTermination(
        await inUserSession(scope, () =>
          apiPost<unknown>(
            "/api/user/sessions/terminate-others",
            { expected_user_id: scope[1] },
            { handleUnauthorizedLocally: true },
          ),
        ),
        scope[1]!,
      );
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: SESSIONS_KEY });
    },
  });
}

export function useUserSecurityHistory() {
  const scope = useSessionQueryIdentity();
  return useQuery({
    queryKey: [...USER_KEY, "security", scope],
    enabled:
      scope[1] !== null && Number.isSafeInteger(scope[1]) && scope[1] > 0,
    staleTime: 0,
    retry: false,
    queryFn: async ({ signal }) =>
      readUserSecurityState(
        await inUserSession(scope, () =>
          apiGet<unknown>(
            `/api/user/security-state?expected_user_id=${scope[1]}`,
            signal,
            { handleUnauthorizedLocally: true },
          ),
        ),
        scope[1]!,
      ),
    select: (data) => readUserSecurityState(data, scope[1]!).events,
  });
}

export function useLogoutCurrent() {
  const scope = useSessionQueryIdentity();
  return useMutation({
    retry: false,
    mutationFn: async () => {
      const result = await inUserSession(scope, () =>
        apiPost<unknown>(
          "/api/user/logout-current",
          { schema: "logout_current_v1", expected_user_id: scope[1] },
          { handleUnauthorizedLocally: true },
        ),
      );
      const receipt = result as Record<string, unknown> | null;
      if (
        !receipt ||
        typeof receipt !== "object" ||
        receipt.schema !== "logout_receipt_v1" ||
        receipt.user_id !== scope[1] ||
        receipt.signed_out !== true
      )
        throw Error("Current-session logout was not confirmed");
      return { user_id: scope[1]!, signed_out: true as const };
    },
  });
}

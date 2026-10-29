// User self-service API hooks — typed wrappers around the Go backend's
// /api/user/self endpoint.
//
// Key behavior:
//   - Change password requires current_password; the backend verifies it.
//   - Change email does not require verification (matches the demo UI
//     which treats email as a profile field).
//   - Both mutations invalidate the cached user profile so callers refetch.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiPut, apiPost, apiGet, ApiError } from "./http";
import type { AuthUser } from "./auth";
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

const USER_KEY = ["user-self"] as const;
const SESSIONS_KEY = ["user-sessions"] as const;

export interface UpdateSelfInput {
  current_password?: string;
  new_password?: string;
  email?: string;
}

export function useUpdateSelf() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: UpdateSelfInput) =>
      apiPut<AuthUser>("/api/user/self", input),
    onSuccess: () => {
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
    select: (data) => readUserProfileState(data, scope[1]!).user,
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

// User self-service API hooks — typed wrappers around the Go backend's
// /api/user/self endpoint.
//
// Key behavior:
//   - Change password requires current_password; the backend verifies it.
//   - Change email does not require verification (matches the demo UI
//     which treats email as a profile field).
//   - Both mutations invalidate the cached user profile so callers refetch.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiPut, apiPost, apiGet } from "./http";
import type { AuthUser } from "./auth";

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

export function useSessions() {
  return useQuery({
    queryKey: SESSIONS_KEY,
    queryFn: ({ signal }) =>
      apiGet<SessionInfo[]>("/api/user/sessions", signal),
  });
}

export function useTerminateOtherSessions() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () =>
      apiPost<{ terminated: number }>("/api/user/sessions/terminate-others"),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: SESSIONS_KEY });
    },
  });
}

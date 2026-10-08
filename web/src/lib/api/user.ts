// User self-service API hooks — typed wrappers around the Go backend's
// /api/user/self endpoint.
//
// Key behavior:
//   - Change password requires current_password; the backend verifies it.
//   - Change email does not require verification (matches the demo UI
//     which treats email as a profile field).
//   - Both mutations invalidate the cached user profile so callers refetch.

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiPut } from "./http";
import type { AuthUser } from "./auth";

const USER_KEY = ["user-self"] as const;

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

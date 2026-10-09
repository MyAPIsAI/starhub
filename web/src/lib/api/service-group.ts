// Service group API hooks — typed wrappers around the Go backend's
// /api/service-group endpoints (admin only).
//
// The backend stores group metadata (name, description, enabled,
// visibility, account_ids) in a dedicated table and syncs the
// multiplier to the group_ratio option so relay billing picks up
// changes immediately.

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiPut, apiDelete } from "./http";

export interface ServiceGroup {
  id: number;
  group_id: string;
  name: string;
  description: string;
  enabled: boolean;
  visibility: "common" | "custom";
  account_ids: string[];
  multiplier: string;
  created_at: number;
  updated_at: number;
}

export interface ServiceGroupInput {
  group_id: string;
  name: string;
  description?: string;
  enabled?: boolean;
  visibility?: "common" | "custom";
  account_ids?: string[];
  multiplier: string;
}

const GROUPS_KEY = ["service-groups"] as const;

export function useServiceGroups() {
  return useQuery({
    queryKey: GROUPS_KEY,
    queryFn: ({ signal }) =>
      apiGet<ServiceGroup[]>("/api/service-group", signal),
  });
}

export function useUpsertServiceGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: ServiceGroupInput) =>
      apiPut<ServiceGroup>("/api/service-group", input),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: GROUPS_KEY });
    },
  });
}

export function useDeleteServiceGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (groupId: string) =>
      apiDelete<null>(`/api/service-group/${encodeURIComponent(groupId)}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: GROUPS_KEY });
    },
  });
}

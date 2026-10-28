// Service group API hooks — typed wrappers around the Go backend's
// /api/service-group endpoints (admin only).
//
// The backend stores group metadata (name, description, enabled,
// visibility, account_ids) in a dedicated table and syncs the
// multiplier atomically with group_ratio. Serving settings reload after
// commit; configuration sync reports separately observed loaded versions.

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiPut, apiRequest } from "./http";
import type { ServiceLevel, ServiceLevelTargets } from "../service-level";
import { useSessionQueryIdentity } from "../system-access";
import {
  readManagedServiceGroup,
  readServiceGroupCatalog,
} from "../service-group-config";

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
  revision?: number;
  sla?: ServiceLevel;
}

export interface ServiceGroupInput {
  group_id: string;
  name: string;
  description?: string;
  enabled?: boolean;
  visibility?: "common" | "custom";
  account_ids?: string[];
  multiplier: string;
  expected_group_id: number;
  expected_revision: number;
  expected_option_version: number;
}

const GROUPS_KEY = ["service-groups"] as const;
const CATALOG_KEY = ["service-group-configuration"] as const;

export function useServiceGroupCatalog(enabled: boolean) {
  const identity = useSessionQueryIdentity();
  return useQuery({
    queryKey: [...CATALOG_KEY, identity],
    enabled,
    retry: false,
    queryFn: async ({ signal }) =>
      readServiceGroupCatalog(
        await apiGet<unknown>("/api/service-group/configuration", signal),
      ),
  });
}

function invalidateGroupConfiguration(
  client: ReturnType<typeof useQueryClient>,
) {
  for (const queryKey of [
    GROUPS_KEY,
    CATALOG_KEY,
    ["plaza-models"],
    ["plaza-model"],
    ["plaza-filters"],
    ["plaza-rank"],
    ["options"],
    ["option-status"],
    ["option-history"],
    ["server-pricing"],
    ["time-pricing"],
    ["resource-publication-context"],
    ["resource-publication-state"],
  ])
    void client.invalidateQueries({ queryKey });
}

export function useServiceGroups(enabled = true) {
  const identity = useSessionQueryIdentity();
  return useQuery({
    queryKey: [...GROUPS_KEY, identity],
    enabled,
    queryFn: ({ signal }) =>
      apiGet<ServiceGroup[]>("/api/service-group", signal),
  });
}

export function useUpdateServiceGroupSLA() {
  const client = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: (input: {
      group: string;
      expected_group_id: number;
      expected_revision: number;
      targets: ServiceLevelTargets | null;
    }) => {
      const { group, ...body } = input;
      return apiPut<ServiceGroup>(
        `/api/service-group/${encodeURIComponent(group)}/sla`,
        body,
      );
    },
    onSettled: () => {
      void client.invalidateQueries({ queryKey: GROUPS_KEY });
      void client.invalidateQueries({ queryKey: CATALOG_KEY });
      void client.invalidateQueries({ queryKey: ["plaza-models"] });
      void client.invalidateQueries({ queryKey: ["plaza-model"] });
    },
  });
}

export function useUpsertServiceGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: async (input: ServiceGroupInput) =>
      readManagedServiceGroup(
        await apiPut<unknown>("/api/service-group", input),
      ),
    onSettled: () => invalidateGroupConfiguration(queryClient),
  });
}

export function useDeleteServiceGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: (input: {
      group_id: string;
      expected_group_id: number;
      expected_revision: number;
      expected_option_version: number;
    }) => {
      const { group_id, ...body } = input;
      return apiRequest<null | undefined>(
        `/api/service-group/${encodeURIComponent(group_id)}`,
        { method: "DELETE", body: JSON.stringify(body) },
      );
    },
    onSettled: () => invalidateGroupConfiguration(queryClient),
  });
}

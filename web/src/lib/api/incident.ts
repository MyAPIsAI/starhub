import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiPost } from "./http";
import {
  isAdmin,
  useSessionQueryIdentity,
  useSystemAccess,
} from "../system-access";

export interface ServiceIncident {
  id: number;
  title: string;
  description: string;
  component: string;
  status: string;
  severity: string;
  created_at: number;
  updated_at: number;
  resolved_at?: number;
}

export function useAdminIncidents(enabled: boolean) {
  const identity = useSessionQueryIdentity();
  const role = useSystemAccess((s) => s.role);
  const account = useSystemAccess((s) => s.session.accountId);
  return useQuery({
    queryKey: ["incidents", "admin", identity],
    enabled: enabled && account !== null && isAdmin(role),
    retry: false,
    queryFn: async ({ signal }) =>
      (await apiGet<ServiceIncident[] | null>("/api/incident", signal)) ?? [],
  });
}

export function useCreateIncident() {
  const client = useQueryClient();
  const identity = useSessionQueryIdentity();
  return useMutation({
    retry: false,
    mutationFn: (input: {
      title: string;
      description: string;
      component: string;
    }) => apiPost<ServiceIncident>("/api/incident", input),
    onSuccess: (incident) => {
      client.setQueryData<ServiceIncident[]>(
        ["incidents", "admin", identity],
        (previous) =>
          [
            incident,
            ...(previous ?? []).filter((entry) => entry.id !== incident.id),
          ].slice(0, 20),
      );
      void client.invalidateQueries({ queryKey: ["incidents"] });
      void client.invalidateQueries({ queryKey: ["admin-audit"] });
    },
  });
}

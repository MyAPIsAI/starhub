import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiPost, apiPut } from "./http";
import {
  isAdmin,
  useSessionQueryIdentity,
  useSystemAccess,
} from "../system-access";

export const TASK_PLUGIN_CHANNEL_TYPE = 61;

export interface TaskPluginVersion {
  id: number;
  key: string;
  version: string;
  meta_json: string;
  enabled: boolean;
}

export function useTaskPlugins() {
  const identity = useSessionQueryIdentity();
  const role = useSystemAccess((s) => s.role);
  const accountId = useSystemAccess((s) => s.session.accountId);
  return useQuery({
    queryKey: ["task-plugins", ...identity],
    queryFn: () => apiGet<TaskPluginVersion[]>("/api/task-plugin"),
    enabled: accountId !== null && isAdmin(role),
  });
}

export function useUploadTaskPlugin() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (source: string) =>
      apiPost<TaskPluginVersion>("/api/task-plugin", { source }),
    onSuccess: () => client.invalidateQueries({ queryKey: ["task-plugins"] }),
  });
}

export function useToggleTaskPlugin() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ id, enabled }: { id: number; enabled: boolean }) =>
      apiPut(`/api/task-plugin/${id}`, { enabled }),
    onSuccess: () => client.invalidateQueries({ queryKey: ["task-plugins"] }),
  });
}

export function useTestTaskPlugin() {
  return useMutation({
    mutationFn: ({
      id,
      hook,
      args,
    }: {
      id: number;
      hook: string;
      args: unknown[];
    }) => apiPost<unknown>(`/api/task-plugin/${id}/test`, { hook, args }),
  });
}

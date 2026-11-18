// Channel CRUD API hooks — wrappers around the Go backend's
// /api/channel endpoints (admin only).
//
// Create, update, and delete channels. List and test are in
// lib/api/admin.ts (useAdminChannels, useTestChannel).

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiPost, apiPut, apiDelete, apiRequest } from "./http";
import type { AdminChannel } from "./admin";

export interface ChannelModelsInput {
  channel_id?: number;
  task_plugin_id?: number;
  type: number;
  base_url: string;
  key?: string;
  key_index?: number;
}

export interface ChannelModelInfo {
  id: string;
  domain?: string;
}

// Discovery credentials stay in this request, outside the query cache.
export function fetchChannelModels(
  input: ChannelModelsInput,
  signal: AbortSignal,
) {
  return apiRequest<Array<string | ChannelModelInfo>>(
    "/api/channel/fetch_models",
    {
      method: "POST",
      body: JSON.stringify({ ...input, include_metadata: true }),
      signal,
    },
  ).then((models) =>
    models.map((entry) => (typeof entry === "string" ? { id: entry } : entry)),
  );
}

export interface ChannelInput {
  name: string;
  task_plugin_id?: number;
  type?: number;
  key: string;
  key_mode?: string;
  base_url?: string;
  models: string;
  model_capabilities?: Record<string, string>;
  group?: string;
  status?: number;
  priority?: number;
  weight?: number;
}

export interface ChannelUpdateInput extends ChannelInput {
  id: number;
}

const CHANNELS_KEY = ["admin-channels"] as const;

function invalidateChannelConfiguration(
  client: ReturnType<typeof useQueryClient>,
) {
  for (const key of [
    CHANNELS_KEY,
    ["plaza-meta"],
    ["resource-publication-context"],
    ["resource-publication-state"],
    ["resource-catalog"],
    ["plaza-models"],
    ["plaza-model"],
    ["plaza-filters"],
    ["plaza-rank"],
  ])
    void client.invalidateQueries({ queryKey: key });
}

export function useCreateChannel() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: ChannelInput) => apiPost("/api/channel", input),
    onSettled: () => invalidateChannelConfiguration(queryClient),
  });
}

export function useUpdateChannel() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: ChannelUpdateInput) => apiPut("/api/channel", input),
    onSettled: () => invalidateChannelConfiguration(queryClient),
  });
}

export function useDeleteChannel() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => apiDelete(`/api/channel/${id}`),
    onSettled: () => invalidateChannelConfiguration(queryClient),
  });
}

export function useSetChannelStatus() {
  const client = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: ({ id, enabled }: { id: number; enabled: boolean }) =>
      apiPut<AdminChannel>(`/api/channel/${id}/status`, { enabled }),
    onSuccess: (channel) => {
      client.setQueryData<AdminChannel[]>(CHANNELS_KEY, (channels) =>
        channels?.map((entry) => entry.id === channel.id ? channel : entry),
      );
    },
    onSettled: () => invalidateChannelConfiguration(client),
  });
}

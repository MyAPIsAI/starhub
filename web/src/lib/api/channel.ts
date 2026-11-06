// Channel CRUD API hooks — wrappers around the Go backend's
// /api/channel endpoints (admin only).
//
// Create, update, and delete channels. List and test are in
// lib/api/admin.ts (useAdminChannels, useTestChannel).

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiPost, apiPut, apiDelete } from "./http";

export interface ChannelInput {
  name: string;
  type?: number;
  key: string;
  key_mode?: string;
  base_url?: string;
  models: string;
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

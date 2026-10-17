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

export function useCreateChannel() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: ChannelInput) => apiPost("/api/channel", input),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: CHANNELS_KEY });
    },
  });
}

export function useUpdateChannel() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: ChannelUpdateInput) => apiPut("/api/channel", input),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: CHANNELS_KEY });
    },
  });
}

export function useDeleteChannel() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => apiDelete(`/api/channel/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: CHANNELS_KEY });
    },
  });
}

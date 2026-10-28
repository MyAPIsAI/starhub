import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiPut, apiRequest } from "./http";
import { useSessionQueryIdentity } from "../system-access";
import {
  publicationRequest,
  readPublicationContext,
  readPublicationPreview,
  type PublicationBinding,
  type PublicationContext,
  type PublicationPreview,
} from "../resource-publication";
import {
  acknowledgementMatchesReview,
  publicationCommitRequest,
  readPublicationAcknowledgement,
  readPublicationReceipt,
} from "../resource-publication-commit";

export function usePublicationContext(enabled: boolean) {
  const identity = useSessionQueryIdentity();
  return useQuery({
    queryKey: ["resource-publication-context", identity],
    enabled,
    retry: false,
    queryFn: async ({ signal }) =>
      readPublicationContext(
        await apiGet<unknown>("/api/resource/publication-context", signal),
      ),
  });
}

export function usePublishResourceReview() {
  const client = useQueryClient();
  return useMutation({
    retry: false,
    mutationFn: async (review: PublicationPreview) => {
      const acknowledgement = readPublicationAcknowledgement(
        await apiPut<unknown>(
          "/api/resource/publication",
          publicationCommitRequest(review),
        ),
      );
      if (!acknowledgementMatchesReview(acknowledgement, review))
        throw Error("Publication acknowledgement does not match review");
      return acknowledgement;
    },
    onSettled: async () => {
      await Promise.allSettled(
        [
          "resource-catalog",
          "resource-publication-context",
          "resource-publication-state",
          "admin-audit",
          "plaza-models",
          "plaza-model",
          "plaza-filters",
          "plaza-rank",
        ].map((key) => client.invalidateQueries({ queryKey: [key] })),
      );
    },
  });
}

export async function getPublicationReceipt(digest: string) {
  if (!/^[0-9a-f]{64}$/.test(digest)) throw Error("Invalid review digest");
  return readPublicationReceipt(
    await apiGet<unknown>(`/api/resource/publication-receipt/${digest}`),
  );
}

export async function previewResourcePublication(
  context: PublicationContext,
  bindings: PublicationBinding[],
  signal: AbortSignal,
) {
  const bindingKey = (items: PublicationBinding[]) =>
    JSON.stringify(
      items
        .map((item) => [item.source_channel_id, item.gateway_channel_id])
        .sort((a, b) => a[0].localeCompare(b[0])),
    );
  const request = publicationRequest(context, bindings);
  const preview = readPublicationPreview(
    await apiRequest<unknown>("/api/resource/publication-preview", {
      method: "POST",
      body: JSON.stringify(request),
      signal,
    }),
  );
  if (
    preview.resource_revision !== context.resource_revision ||
    preview.channel_revision !== context.channel_revision ||
    preview.option_version !== context.option_version ||
    bindingKey(preview.bindings) !== bindingKey(bindings)
  )
    throw Error("Publication preview does not match the selected context");
  return preview;
}

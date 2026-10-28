import { useQuery } from "@tanstack/react-query";
import { apiGet, apiRequest } from "./http";
import { useSessionQueryIdentity } from "../system-access";
import {
  publicationRequest,
  readPublicationContext,
  readPublicationPreview,
  type PublicationBinding,
  type PublicationContext,
} from "../resource-publication";

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

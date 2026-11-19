import type { AdminChannel } from "./api/admin";

export function channelResourceStatus(
  channel: Pick<AdminChannel, "resource_state">,
): "enabled" | "partial" | "closed" | undefined {
  const value = channel.resource_state;
  if (
    !value ||
    !Number.isSafeInteger(value.total) ||
    !Number.isSafeInteger(value.enabled) ||
    value.total < 0 ||
    value.enabled < 0 ||
    value.enabled > value.total
  )
    return undefined;
  const state =
    value.enabled === 0
      ? "closed"
      : value.enabled === value.total
        ? "enabled"
        : "partial";
  return value.state === state ? state : undefined;
}

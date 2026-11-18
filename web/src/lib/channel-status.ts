import type { AdminChannel } from "./api/admin";

export function channelResourceStatus(
  channel: Pick<AdminChannel, "status" | "key_count" | "multi_key_status">,
): "enabled" | "partial" | "closed" {
  if (channel.status !== 1) return "closed";
  let enabled = 0;
  for (let index = 0; index < channel.key_count; index++) {
    if ((channel.multi_key_status?.[index] ?? 1) === 1) enabled++;
  }
  if (!enabled) return "closed";
  return enabled === channel.key_count ? "enabled" : "partial";
}

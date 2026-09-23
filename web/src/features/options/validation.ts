import { z } from "zod";
const bounded = (max: number) => z.number().finite().min(0).max(max);
export function serializeOption(key: string, draft: string): string {
  if (key === "alert_webhook_url") {
    const url = draft.trim();
    if (url && !/^https?:\/\//.test(url))
      throw new Error("Use an http:// or https:// URL");
    return JSON.stringify(url);
  }
  const value: unknown = JSON.parse(draft);
  let schema: z.ZodType;
  if (["model_ratio", "completion_ratio", "group_ratio"].includes(key))
    schema = z.record(z.string().min(1), bounded(1e9));
  else if (key === "group_group_ratio")
    schema = z.record(
      z.string().min(1),
      z.record(z.string().min(1), bounded(1e9)),
    );
  else if (key === "model_price")
    schema = z.record(z.string().min(1), bounded(10000));
  else if (key === "model_price_expr")
    schema = z.record(z.string().min(1), z.string());
  else if (key === "aff_register_bonus")
    schema = z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER);
  else
    schema = bounded(
      ["aff_rebate_ratio", "alert_error_rate"].includes(key) ? 1 : 1000,
    );
  return JSON.stringify(schema.parse(value));
}
export function optionDraft(key: string, value: string): string {
  try {
    const parsed: unknown = JSON.parse(value);
    return key === "alert_webhook_url"
      ? String(parsed)
      : JSON.stringify(parsed, null, 2);
  } catch {
    return value;
  }
}

import { integer } from "@/lib/format";
import type { Row } from "@/lib/types";
export type Kind =
  "tokens" | "channels" | "users" | "logs" | "audit" | "redemptions";
export interface Field {
  key: string;
  type?:
    | "number"
    | "checkbox"
    | "textarea"
    | "password"
    | "datetime-local"
    | "select";
  choices?: [string, string][];
  initial?: string | boolean;
  required?: boolean;
  advanced?: boolean;
  wide?: boolean;
  help?: string;
}
export interface Resource {
  columns: string[];
  fields: Field[];
  create: boolean;
  edit: boolean;
  remove: boolean;
  paged?: boolean;
}
export const resources: Record<Kind, Resource> = {
  tokens: {
    create: true,
    edit: true,
    remove: true,
    columns: [
      "name",
      "user_id",
      "key",
      "remain_quota",
      "used_quota",
      "status",
      "group",
      "expired_time",
      "model_limits",
      "allow_ips",
      "auto_groups",
      "created_at",
    ],
    fields: [
      { key: "name", required: true },
      { key: "remain_quota", type: "number", initial: "500000" },
      { key: "unlimited_quota", type: "checkbox", initial: false, wide: true },
      { key: "expired_time", type: "datetime-local", help: "expirationHelp" },
      { key: "group", help: "inherited" },
      { key: "model_limits", advanced: true, help: "listHelp" },
      { key: "allow_ips", advanced: true, help: "listHelp" },
      { key: "auto_groups", advanced: true, help: "listHelp" },
    ],
  },
  channels: {
    create: true,
    edit: true,
    remove: true,
    columns: [
      "name",
      "type",
      "models",
      "group",
      "status",
      "priority",
      "weight",
      "response_time",
      "used_quota",
      "key_count",
      "multi_key_status",
    ],
    fields: [
      { key: "name", required: true },
      {
        key: "type",
        type: "select",
        initial: "1",
        choices: [
          ["1", "OpenAI"],
          ["14", "Claude"],
          ["15", "AWS Bedrock"],
          ["25", "Gemini"],
        ],
      },
      { key: "key", type: "textarea", wide: true, help: "keysHelp" },
      { key: "base_url", wide: true },
      {
        key: "models",
        type: "textarea",
        required: true,
        wide: true,
        help: "listHelp",
      },
      { key: "group", initial: "default", required: true },
      {
        key: "key_mode",
        type: "select",
        initial: "random",
        choices: [
          ["random", "random"],
          ["round_robin", "round_robin"],
        ],
      },
      { key: "priority", type: "number", initial: "0" },
      { key: "weight", type: "number", initial: "1" },
      {
        key: "status",
        type: "select",
        initial: "1",
        choices: [
          ["1", "enabled"],
          ["2", "disabled"],
        ],
      },
    ],
  },
  users: {
    create: false,
    edit: true,
    remove: false,
    columns: [
      "username",
      "email",
      "role",
      "group",
      "status",
      "quota",
      "used_quota",
      "created_at",
    ],
    fields: [
      { key: "quota", type: "number" },
      { key: "group", required: true },
      {
        key: "status",
        type: "select",
        choices: [
          ["1", "enabled"],
          ["2", "disabled"],
        ],
      },
      { key: "password", type: "password" },
    ],
  },
  logs: {
    create: false,
    edit: false,
    remove: false,
    paged: true,
    columns: [
      "created_at",
      "model",
      "user_id",
      "token_name",
      "channel_id",
      "prompt_tokens",
      "completion_tokens",
      "quota",
      "unfunded_quota",
      "used_channels",
    ],
    fields: [],
  },
  audit: {
    create: false,
    edit: false,
    remove: false,
    paged: true,
    columns: ["created_at", "actor", "method", "path", "status", "ip"],
    fields: [],
  },
  redemptions: {
    create: true,
    edit: false,
    remove: true,
    columns: [
      "id",
      "code",
      "quota",
      "status",
      "used_by",
      "used_at",
      "expired_at",
      "created_at",
    ],
    fields: [
      { key: "quota", type: "number", initial: "500000", required: true },
      { key: "count", type: "number", initial: "1", required: true },
      { key: "code" },
      { key: "expired_at", type: "datetime-local", help: "expirationHelp" },
    ],
  },
};
export type Draft = Record<string, string | boolean>;
export function makeDraft(kind: Kind, row?: Row): Draft {
  return Object.fromEntries(
    resources[kind].fields.map((f) => {
      let value = row?.[f.key] ?? f.initial ?? "";
      if (f.key === "key" || f.key === "password") value = "";
      if (f.type === "datetime-local") {
        const d =
          typeof value === "number" && value > 0
            ? new Date(value * 1000)
            : null;
        value = d
          ? new Date(d.getTime() - d.getTimezoneOffset() * 60000)
              .toISOString()
              .slice(0, 16)
          : "";
      }
      return [f.key, f.type === "checkbox" ? Boolean(value) : String(value)];
    }),
  );
}
export function buildPayload(
  kind: Kind,
  draft: Draft,
  row?: Row,
): Record<string, string | boolean | number> {
  const result: Record<string, string | boolean | number> = row
    ? { id: row.id }
    : {};
  const initial = makeDraft(kind, row);
  for (const field of resources[kind].fields) {
    const raw = draft[field.key] ?? "";
    if (row && (raw === initial[field.key] || field.key === "type")) continue;
    if ((field.key === "password" || field.key === "key") && row && raw === "")
      continue;
    if (
      kind === "tokens" &&
      field.key === "remain_quota" &&
      draft.unlimited_quota
    )
      continue;
    if (
      kind === "redemptions" &&
      field.key === "code" &&
      Number(draft.count) > 1
    )
      continue;
    if (field.required && !String(raw).trim())
      throw new Error(`${field.key}: required`);
    if (
      field.type === "number" ||
      (field.type === "select" && ["type", "status"].includes(field.key))
    ) {
      const n = integer(String(raw));
      if (n < 0 && field.key !== "priority")
        throw new Error(`${field.key}: must be >= 0`);
      result[field.key] = n;
    } else if (field.type === "datetime-local") {
      const n = raw
        ? Math.floor(new Date(String(raw)).getTime() / 1000)
        : field.key === "expired_time"
          ? -1
          : 0;
      if (!Number.isFinite(n)) throw new Error(`${field.key}: invalid date`);
      result[field.key] = n;
    } else
      result[field.key] =
        typeof raw === "string" && field.key !== "password" ? raw.trim() : raw;
  }
  if (kind === "channels") {
    if (!row && !String(draft.key).trim()) throw new Error("key: required");
    if (draft.base_url && !/^https?:\/\//.test(String(draft.base_url)))
      throw new Error("base_url: use http:// or https://");
  }
  if (
    kind === "tokens" &&
    draft.group === "auto" &&
    !String(draft.auto_groups).trim()
  )
    throw new Error("auto_groups: required");
  if (
    kind === "users" &&
    draft.password &&
    new TextEncoder().encode(String(draft.password)).length < 6
  )
    throw new Error("password: must contain at least 6 bytes");
  if (
    kind === "redemptions" &&
    (Number(draft.quota) <= 0 ||
      Number(draft.count) < 1 ||
      Number(draft.count) > 100)
  )
    throw new Error("quota > 0; count: 1–100");
  if (
    row &&
    ("quota" in result ||
      "remain_quota" in result ||
      result.unlimited_quota === true)
  )
    result.expected_quota = Number(
      row[kind === "users" ? "quota" : "remain_quota"],
    );
  return result;
}

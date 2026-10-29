import { readPricingAcknowledgement } from "./server-pricing";
import {
  readServerTimePricing,
  timeScheduleError,
  validTimeModelId,
  type TimeScheduleError,
} from "./server-time-pricing";

export const tokenPriceKeys = [
  "input_per_m",
  "output_per_m",
  "cache_read_per_m",
  "cache_write_per_m",
  "audio_input_per_m",
  "audio_output_per_m",
] as const;
export type TokenPriceKey = (typeof tokenPriceKeys)[number];
export type ServerTokenPrices = Record<TokenPriceKey, string>;
export interface TokenTimeWindow {
  id: string;
  label: string;
  days: number[];
  start: string;
  end: string;
  prices: ServerTokenPrices;
}
export interface TokenTimeSchedule {
  timezone: string;
  windows: TokenTimeWindow[];
}
export interface ServerTokenTimePricing {
  version: number;
  loaded_version: number;
  revision_guards: true;
  schema: "weekly_token_prices_v1";
  billing_active: true;
  token_only: true;
  asynchronous_tasks: true;
  multiplier_behavior: "stacked";
  rules: Record<string, TokenTimeSchedule>;
}

export function validTokenWindowPrice(value: unknown): value is string {
  if (
    typeof value !== "string" ||
    value !== value.trim() ||
    !/^(0|[1-9][0-9]{0,6})(\.[0-9]{1,9})?$/.test(value)
  )
    return false;
  const [whole, fraction = ""] = value.split(".");
  return BigInt(whole + fraction.padEnd(9, "0")) <= 1_000_000_000_000_000n;
}

export function emptyTokenTimeWindow(): TokenTimeWindow {
  return {
    id: "",
    label: "",
    days: [],
    start: "00:00",
    end: "24:00",
    prices: Object.fromEntries(
      tokenPriceKeys.map((key) => [key, ""]),
    ) as ServerTokenPrices,
  };
}

export function tokenTimeScheduleError(
  schedule: TokenTimeSchedule,
): TimeScheduleError | "prices" | null {
  const issue = timeScheduleError({
    timezone: schedule.timezone,
    windows: schedule.windows.map((window) => ({ ...window, multiplier: "1" })),
  });
  if (issue) return issue;
  return schedule.windows.some((window) =>
    tokenPriceKeys.some((key) => !validTokenWindowPrice(window.prices[key])),
  )
    ? "prices"
    : null;
}

function exact(
  value: unknown,
  keys: readonly string[],
): value is Record<string, unknown> {
  return (
    !!value &&
    typeof value === "object" &&
    !Array.isArray(value) &&
    Object.keys(value).length === keys.length &&
    keys.every((key) => Object.hasOwn(value, key))
  );
}

export function readTokenTimeAcknowledgement(value: unknown) {
  if (!value || typeof value !== "object")
    throw Error("Invalid independent token-pricing capabilities");
  const state = value as Record<string, unknown>;
  if (
    state.schema !== "weekly_token_prices_v1" ||
    state.revision_guards !== true ||
    state.billing_active !== true ||
    state.token_only !== true ||
    state.asynchronous_tasks !== true ||
    state.multiplier_behavior !== "stacked"
  )
    throw Error("Unsupported independent token-pricing capabilities");
  return readPricingAcknowledgement(value);
}

export function readServerTokenTimePricing(
  value: unknown,
): ServerTokenTimePricing {
  const versions = readTokenTimeAcknowledgement(value);
  const state = value as Record<string, unknown>;
  if (
    !state.rules ||
    typeof state.rules !== "object" ||
    Array.isArray(state.rules)
  )
    throw Error("Invalid independent token schedules");
  const entries = Object.entries(state.rules);
  if (entries.length > 1024)
    throw Error("Too many independent token schedules");
  const rules = Object.fromEntries(
    entries.map(([model, raw]) => {
      if (
        !validTimeModelId(model) ||
        ["myapis-auto", "veyra-auto"].includes(model) ||
        !exact(raw, ["timezone", "windows"]) ||
        typeof raw.timezone !== "string" ||
        !Array.isArray(raw.windows)
      )
        throw Error("Invalid independent model schedule");
      const windows = raw.windows.map((window) => {
        if (
          !exact(window, ["id", "label", "days", "start", "end", "prices"]) ||
          !exact(window.prices, tokenPriceKeys) ||
          Object.values(window.prices).some(
            (value) => !validTokenWindowPrice(value),
          ) ||
          typeof window.id !== "string" ||
          window.id !== window.id.trim() ||
          typeof window.start !== "string" ||
          window.start.length !== 5 ||
          typeof window.end !== "string" ||
          window.end.length !== 5
        )
          throw Error("Invalid independent token window prices");
        return {
          ...window,
          prices: { ...window.prices },
          days: Array.isArray(window.days) ? [...window.days] : window.days,
        } as unknown as TokenTimeWindow;
      });
      return [model, { timezone: raw.timezone, windows }];
    }),
  ) as Record<string, TokenTimeSchedule>;
  // Reuse the multiplier contract for all shared structural/weekly constraints.
  readServerTimePricing({
    ...versions,
    revision_guards: true,
    schema: "weekly_multiplier_v1",
    billing_active: true,
    rules: Object.fromEntries(
      Object.entries(rules).map(([model, schedule]) => [
        model,
        {
          ...schedule,
          windows: schedule.windows.map(({ prices: _, ...window }) => ({
            ...window,
            multiplier: 1,
          })),
        },
      ]),
    ),
  });
  if (new TextEncoder().encode(JSON.stringify(rules)).length > 1 << 20)
    throw Error("Independent token schedules exceed size limit");
  return {
    ...versions,
    revision_guards: true,
    schema: "weekly_token_prices_v1",
    billing_active: true,
    token_only: true,
    asynchronous_tasks: true,
    multiplier_behavior: "stacked",
    rules,
  };
}

export function replaceTokenTimeSchedule(
  state: ServerTokenTimePricing,
  model: string,
  schedule: TokenTimeSchedule | null,
) {
  if (
    !validTimeModelId(model) ||
    ["myapis-auto", "veyra-auto"].includes(model) ||
    (schedule && tokenTimeScheduleError(schedule))
  )
    throw Error("Invalid independent token schedule edit");
  const rules = {
    ...structuredClone(state.rules),
    ...(schedule ? { [model]: structuredClone(schedule) } : {}),
  };
  if (!schedule) delete rules[model];
  return readServerTokenTimePricing({ ...state, rules }).rules;
}

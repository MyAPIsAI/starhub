import { validServerRatio } from "./server-pricing";

export interface ServerTimeWindow {
  id: string;
  label: string;
  days: number[];
  start: string;
  end: string;
  multiplier: number;
}
export interface ServerTimeSchedule {
  timezone: string;
  windows: ServerTimeWindow[];
}
export interface ServerTimePricing {
  version: number;
  loaded_version: number;
  revision_guards: true;
  schema: "weekly_multiplier_v1";
  billing_active: true;
  rules: Record<string, ServerTimeSchedule>;
}
export type TimeWindowDraft = Omit<ServerTimeWindow, "multiplier"> & {
  multiplier: string;
};
export type TimeScheduleDraft = {
  timezone: string;
  windows: TimeWindowDraft[];
};
export type TimeScheduleError =
  | "timezone"
  | "limit"
  | "id"
  | "label"
  | "days"
  | "time"
  | "multiplier"
  | "overlap";
const bytes = (value: string) => new TextEncoder().encode(value).length;
export const validTimeModelId = (id: string) =>
  !!id && id === id.trim() && bytes(id) <= 128 && !/\p{Cc}/u.test(id);
function minute(value: string, end = false): number {
  if (end && value === "24:00") return 1440;
  if (!/^\d{2}:\d{2}$/.test(value)) return -1;
  const [hour, min] = value.split(":").map(Number);
  return hour <= 23 && min <= 59 ? hour * 60 + min : -1;
}
export function timeScheduleError(
  draft: TimeScheduleDraft,
): TimeScheduleError | null {
  if (
    !draft.timezone ||
    draft.timezone !== draft.timezone.trim() ||
    bytes(draft.timezone) > 64 ||
    draft.timezone === "Local"
  )
    return "timezone";
  if (draft.windows.length > 32) return "limit";
  const ids = new Set<string>(),
    intervals: { start: number; end: number }[] = [];
  for (const window of draft.windows) {
    if (!/^[A-Za-z0-9_.-]{1,64}$/.test(window.id) || ids.has(window.id))
      return "id";
    ids.add(window.id);
    if (
      !window.label.trim() ||
      bytes(window.label) > 128 ||
      /\p{Cc}/u.test(window.label)
    )
      return "label";
    if (
      !window.days.length ||
      window.days.length > 7 ||
      new Set(window.days).size !== window.days.length ||
      window.days.some((day) => !Number.isInteger(day) || day < 0 || day > 6)
    )
      return "days";
    if (
      !validServerRatio(window.multiplier) ||
      Number(window.multiplier) <= 0 ||
      Number(window.multiplier) > 100
    )
      return "multiplier";
    const start = minute(window.start),
      end = minute(window.end, true);
    if (start < 0 || end < 0 || start === end) return "time";
    for (const day of window.days) {
      const from = day * 1440 + start;
      let to = day * 1440 + end;
      if (to < from) to += 1440;
      if (to > 10080)
        intervals.push(
          { start: from, end: 10080 },
          { start: 0, end: to - 10080 },
        );
      else intervals.push({ start: from, end: to });
    }
  }
  intervals.sort((a, b) => a.start - b.start);
  if (
    intervals.some(
      (interval, index) =>
        index > 0 && interval.start < intervals[index - 1].end,
    )
  )
    return "overlap";
  return null;
}
export function timeScheduleDraft(
  schedule?: ServerTimeSchedule,
): TimeScheduleDraft {
  return schedule
    ? {
        timezone: schedule.timezone,
        windows: schedule.windows.map((window) => ({
          ...window,
          days: [...window.days],
          multiplier: String(window.multiplier),
        })),
      }
    : {
        timezone: "UTC",
        windows: [
          {
            id: "window-1",
            label: "",
            days: [],
            start: "00:00",
            end: "24:00",
            multiplier: "1",
          },
        ],
      };
}
function exactObject(
  value: unknown,
  keys: string[],
): value is Record<string, unknown> {
  return (
    !!value &&
    typeof value === "object" &&
    !Array.isArray(value) &&
    Object.keys(value).every((key) => keys.includes(key)) &&
    keys.every((key) => Object.hasOwn(value, key))
  );
}
export function readServerTimePricing(value: unknown): ServerTimePricing {
  if (!value || typeof value !== "object")
    throw Error("Invalid time-pricing snapshot");
  const state = value as Record<string, unknown>,
    safe = (n: unknown): n is number =>
      typeof n === "number" && Number.isSafeInteger(n) && n >= 0;
  if (
    !safe(state.version) ||
    !safe(state.loaded_version) ||
    state.revision_guards !== true ||
    state.schema !== "weekly_multiplier_v1" ||
    state.billing_active !== true ||
    !state.rules ||
    typeof state.rules !== "object" ||
    Array.isArray(state.rules)
  )
    throw Error("Unsupported time-pricing snapshot");
  const entries = Object.entries(state.rules);
  if (entries.length > 1024) throw Error("Too many time schedules");
  const rules = Object.fromEntries(
    entries.map(([id, raw]) => {
      if (
        !validTimeModelId(id) ||
        !exactObject(raw, ["timezone", "windows"]) ||
        typeof raw.timezone !== "string" ||
        !Array.isArray(raw.windows)
      )
        throw Error("Invalid time schedule");
      const windows = raw.windows.map((rawWindow) => {
        if (
          !exactObject(rawWindow, [
            "id",
            "label",
            "days",
            "start",
            "end",
            "multiplier",
          ]) ||
          typeof rawWindow.id !== "string" ||
          typeof rawWindow.label !== "string" ||
          !Array.isArray(rawWindow.days) ||
          !rawWindow.days.every((day) => typeof day === "number") ||
          typeof rawWindow.start !== "string" ||
          typeof rawWindow.end !== "string" ||
          typeof rawWindow.multiplier !== "number" ||
          !Number.isFinite(rawWindow.multiplier)
        )
          throw Error("Invalid time window");
        return {
          id: rawWindow.id,
          label: rawWindow.label,
          days: [...rawWindow.days] as number[],
          start: rawWindow.start,
          end: rawWindow.end,
          multiplier: rawWindow.multiplier,
        };
      });
      const schedule = { timezone: raw.timezone, windows };
      if (timeScheduleError(timeScheduleDraft(schedule)))
        throw Error("Invalid weekly time schedule");
      return [id, schedule];
    }),
  );
  return {
    version: state.version,
    loaded_version: state.loaded_version,
    revision_guards: true,
    schema: "weekly_multiplier_v1",
    billing_active: true,
    rules,
  };
}
export function replaceServerTimeSchedule(
  state: ServerTimePricing,
  id: string,
  draft: TimeScheduleDraft | null,
): Record<string, ServerTimeSchedule> {
  if (!validTimeModelId(id) || (draft && timeScheduleError(draft)))
    throw Error("Invalid time-schedule edit");
  const rules = draft
    ? {
        ...state.rules,
        [id]: {
          timezone: draft.timezone,
          windows: draft.windows.map((window) => ({
            ...window,
            days: [...window.days],
            multiplier: Number(window.multiplier),
          })),
        },
      }
    : { ...state.rules };
  if (!draft) delete rules[id];
  if (Object.keys(rules).length > 1024) throw Error("Too many time schedules");
  return rules;
}

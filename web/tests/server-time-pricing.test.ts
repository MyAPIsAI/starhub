import { describe, expect, it } from "vitest";
import {
  readServerTimePricing,
  replaceServerTimeSchedule,
  timeScheduleDraft,
  timeScheduleError,
  type TimeScheduleDraft,
} from "../src/lib/server-time-pricing";

const schedule = {
  timezone: "UTC",
  windows: [
    {
      id: "night",
      label: "Night",
      days: [6],
      start: "23:00",
      end: "02:00",
      multiplier: 0.8,
    },
  ],
};
const fixture = {
  version: 3,
  loaded_version: 2,
  schema: "weekly_multiplier_v1",
  billing_active: true,
  revision_guards: true,
  rules: {
    actual: schedule,
    retained: { timezone: "Asia/Shanghai", windows: [] },
  },
};
describe("actual server time-schedule editing", () => {
  it("requires activated schema and version protection even for an empty catalog", () => {
    expect(readServerTimePricing({ ...fixture, rules: {} }).rules).toEqual({});
    for (const value of [
      { ...fixture, schema: undefined },
      { ...fixture, billing_active: false },
      { ...fixture, revision_guards: undefined },
      { ...fixture, version: Number.MAX_SAFE_INTEGER + 1 },
      { ...fixture, loaded_version: -1 },
      { ...fixture, rules: null },
      { ...fixture, rules: [] },
    ])
      expect(() => readServerTimePricing(value)).toThrow();
  });
  it("retains unrelated actual schedules and copies weekdays without inventing Demo prices", () => {
    const state = readServerTimePricing(fixture),
      before = structuredClone(state),
      draft = timeScheduleDraft(state.rules.actual);
    draft.windows[0].multiplier = "0.5";
    draft.windows[0].days.push(1);
    const rules = replaceServerTimeSchedule(state, "actual", draft);
    expect(rules.actual.windows[0].multiplier).toBe(0.5);
    expect(rules.retained).toEqual(state.rules.retained);
    expect(state).toEqual(before);
    expect(replaceServerTimeSchedule(state, "actual", null)).toEqual({
      retained: state.rules.retained,
    });
    const prototype = replaceServerTimeSchedule(state, "__proto__", draft);
    expect(Object.hasOwn(prototype, "__proto__")).toBe(true);
    expect(
      Object.hasOwn(JSON.parse(JSON.stringify(prototype)), "__proto__"),
    ).toBe(true);
    expect(Object.getPrototypeOf(prototype)).toBe(Object.prototype);
  });
  it("rejects weekly wrap overlap but allows adjacent exclusive boundaries", () => {
    const draft = timeScheduleDraft(schedule);
    draft.windows.push({
      id: "sunday",
      label: "Sunday",
      days: [0],
      start: "01:00",
      end: "03:00",
      multiplier: "1.2",
    });
    expect(timeScheduleError(draft)).toBe("overlap");
    draft.windows[1].start = "02:00";
    expect(timeScheduleError(draft)).toBeNull();
    draft.windows = [
      {
        id: "all",
        label: "All",
        days: [0, 1, 2, 3, 4, 5, 6],
        start: "00:00",
        end: "24:00",
        multiplier: "100",
      },
    ];
    expect(timeScheduleError(draft)).toBeNull();
  });
  it("rejects invalid labels, weekdays, times, multiplier and unsupported fields", () => {
    const mutations: ((draft: TimeScheduleDraft) => void)[] = [
      (d) => {
        d.timezone = "Local";
      },
      (d) => {
        d.timezone = "";
      },
      (d) => {
        d.windows[0].id = "bad id";
      },
      (d) => {
        d.windows[0].label = "界".repeat(43);
      },
      (d) => {
        d.windows[0].days = [];
      },
      (d) => {
        d.windows[0].days = [1, 1];
      },
      (d) => {
        d.windows[0].days = [1.5];
      },
      (d) => {
        d.windows[0].start = "24:00";
      },
      (d) => {
        d.windows[0].end = d.windows[0].start;
      },
      (d) => {
        d.windows[0].multiplier = "0";
      },
      (d) => {
        d.windows[0].multiplier = "100.01";
      },
      (d) => {
        d.windows[0].multiplier = "NaN";
      },
    ];
    for (const mutate of mutations) {
      const draft = timeScheduleDraft(schedule);
      mutate(draft);
      expect(timeScheduleError(draft)).not.toBeNull();
      expect(() =>
        replaceServerTimeSchedule(
          readServerTimePricing(fixture),
          "actual",
          draft,
        ),
      ).toThrow();
    }
    for (const rules of [
      { bad: { ...schedule, unknown: true } },
      { bad: { ...schedule, windows: null } },
      {
        bad: {
          ...schedule,
          windows: [{ ...schedule.windows[0], tokenPrices: { input: "1" } }],
        },
      },
    ])
      expect(() => readServerTimePricing({ ...fixture, rules })).toThrow();
  });
});

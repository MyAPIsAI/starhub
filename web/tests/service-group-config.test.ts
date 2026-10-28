import { describe, expect, it } from "vitest";
import {
  configuredGroupRatio,
  readManagedServiceGroup,
  readServiceGroupCatalog,
  validGroupMultiplier,
} from "../src/lib/service-group-config";

const group = {
  id: 1,
  revision: 0,
  group_id: "actual",
  name: "Actual group",
  description: "",
  enabled: true,
  visibility: "common",
  account_ids: [],
  multiplier: "0.8",
  created_at: 0,
  updated_at: 1,
};
const catalog = {
  groups: [group],
  group_ratios: { default: 1, actual: 0.5, "legacy-zero": 0 },
  option_version: 2,
  loaded_option_version: 1,
  revision_guards: true,
  option_version_guard: true,
  account_access_guard: true,
};

describe("authoritative group configuration", () => {
  it("keeps stored metadata and independently configured prices separate", () => {
    const parsed = readServiceGroupCatalog(catalog);
    expect(parsed.groups[0].multiplier).toBe("0.8");
    expect(configuredGroupRatio(parsed, "actual")).toBe(0.5);
    expect(configuredGroupRatio(parsed, "legacy-zero")).toBe(0);
    expect(configuredGroupRatio(parsed, "missing")).toBeUndefined();
    expect(parsed.loaded_option_version).toBe(1);
    expect(parsed.option_version).toBe(2);
  });
  it("requires explicit guard support for safe empty-database creation", () => {
    expect(readServiceGroupCatalog({ ...catalog, groups: [] }).groups).toEqual(
      [],
    );
    for (const data of [
      { ...catalog, groups: [], revision_guards: undefined },
      { ...catalog, option_version_guard: false },
      { ...catalog, account_access_guard: false },
      { ...catalog, groups: [], account_access_guard: undefined },
      { ...catalog, groups: undefined },
    ])
      expect(() => readServiceGroupCatalog(data)).toThrow();
  });
  it("never treats inherited object properties as configured ratio keys", () => {
    const parsed = readServiceGroupCatalog({
      ...catalog,
      group_ratios: JSON.parse('{"constructor":0.5,"__proto__":0}'),
    });
    expect(configuredGroupRatio(parsed, "constructor")).toBe(0.5);
    expect(configuredGroupRatio(parsed, "__proto__")).toBe(0);
    expect(configuredGroupRatio(parsed, "toString")).toBeUndefined();
    expect(Object.getPrototypeOf(parsed.group_ratios)).toBe(Object.prototype);
  });
  it.each([undefined, NaN, Infinity, -1, Number.MAX_SAFE_INTEGER + 1])(
    "rejects unusable configuration version %s",
    (option_version) => {
      expect(() =>
        readServiceGroupCatalog({ ...catalog, option_version }),
      ).toThrow();
    },
  );
  it.each([undefined, -1, NaN, Infinity, 1e9 + 1])(
    "rejects corrupt ratio %s without inventing defaults",
    (ratio) => {
      expect(() =>
        readServiceGroupCatalog({
          ...catalog,
          group_ratios: { actual: ratio },
        }),
      ).toThrow();
    },
  );
  it("rejects unsafe or duplicate metadata identity and unsupported old schemas", () => {
    expect(() =>
      readManagedServiceGroup({ ...group, revision: undefined }),
    ).toThrow();
    expect(() =>
      readManagedServiceGroup({ ...group, id: Number.MAX_SAFE_INTEGER + 1 }),
    ).toThrow();
    expect(() =>
      readServiceGroupCatalog({
        ...catalog,
        groups: [group, { ...group, id: 2 }],
      }),
    ).toThrow();
    expect(() =>
      readServiceGroupCatalog({
        ...catalog,
        groups: [group, { ...group, group_id: "other" }],
      }),
    ).toThrow();
    expect(
      readManagedServiceGroup({ ...group, multiplier: "NaN" }).multiplier,
    ).toBe("NaN");
  });
  it.each(["0", "-1", "NaN", "Infinity", "100.1", "0x10", "1e999", ""])(
    "rejects invalid write multiplier %s",
    (value) => expect(validGroupMultiplier(value)).toBe(false),
  );
  it.each(["1", ".5", "100", "1e-3", "+.5"])(
    "accepts supported finite multiplier %s",
    (value) => expect(validGroupMultiplier(value)).toBe(true),
  );
});

import { beforeEach, describe, expect, it, vi } from "vitest";
const fake = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn() }));
vi.mock("../src/lib/api/http", () => ({ apiGet: fake.get, apiPut: fake.put }));
import {
  bindAccountLanguage,
  matchLanguage,
  refreshLanguagePolicy,
  setLanguagePortal,
  usePreferences,
} from "../src/lib/preferences";

describe("M9 language selection", () => {
  beforeEach(async () => {
    const storage = new Map<string, string>();
    vi.stubGlobal("localStorage", {
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => storage.set(key, value),
    });
    vi.stubGlobal("navigator", { languages: ["fr-FR"] });
    fake.get.mockResolvedValue({
      default_language: "en",
      languages: ["en", "zh", "ar"],
    });
    fake.put.mockResolvedValue({});
    bindAccountLanguage(null);
    setLanguagePortal(false);
    await refreshLanguagePolicy();
  });
  it("uses regional defaults and matches browser subtags in preference order", () => {
    const policy = {
      default_language: "en" as const,
      languages: ["en", "zh", "ar"] as ("en" | "zh" | "ar")[],
    };
    expect(matchLanguage(["fr-FR"], policy)).toBe("en");
    expect(matchLanguage(["fr", "ar-AE", "zh-CN"], policy)).toBe("ar");
    expect(matchLanguage(["de"], { ...policy, default_language: "zh" })).toBe(
      "zh",
    );
  });
  it("restores account language over browser selection and persists owner-bound changes", async () => {
    bindAccountLanguage({
      id: 42,
      account_kind: "customer",
      preferred_language: "ar",
    });
    expect(usePreferences.getState().language).toBe("ar");
    await usePreferences.getState().setLanguage("zh");
    expect(fake.put).toHaveBeenCalledWith(
      "/api/user/language",
      { expected_user_id: 42, language: "zh" },
      { portal: "workspace" },
    );
    expect(usePreferences.getState().language).toBe("zh");
    fake.put.mockRejectedValueOnce(Error("offline"));
    await usePreferences.getState().setLanguage("en");
    expect(usePreferences.getState().language).toBe("zh");
    expect(usePreferences.getState().languageError).toBe(true);
  });
  it("ignores a response belonging to a previous account", async () => {
    let finish!: () => void;
    fake.put.mockReturnValueOnce(
      new Promise<void>((resolve) => {
        finish = resolve;
      }),
    );
    bindAccountLanguage({ id: 1, preferred_language: "en" });
    const saving = usePreferences.getState().setLanguage("zh");
    bindAccountLanguage({ id: 2, preferred_language: "ar" });
    finish();
    await saving;
    expect(usePreferences.getState().language).toBe("ar");
  });
  it("restricts administration without discarding the public Arabic preference", () => {
    bindAccountLanguage({ id: 1, preferred_language: "ar" });
    setLanguagePortal(true);
    expect(usePreferences.getState().languages).toEqual(["en", "zh"]);
    expect(usePreferences.getState().language).toBe("en");
    setLanguagePortal(false);
    expect(usePreferences.getState().language).toBe("ar");
  });
  it("falls back when the region disables a stored language", async () => {
    bindAccountLanguage({ id: 1, preferred_language: "ar" });
    fake.get.mockResolvedValueOnce({
      default_language: "en",
      languages: ["en", "zh"],
    });
    await refreshLanguagePolicy();
    expect(usePreferences.getState().language).toBe("en");
  });
});

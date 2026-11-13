import { describe, expect, it } from "vitest";
import { resolveClientLocale } from "./locale-policy";

describe("deployment locale precedence", () => {
  it("restores account preferences before device and browser settings", () => {
    expect(
      resolveClientLocale("ar", "zh", ["en-US"], "en", ["ar", "en", "zh"]),
    ).toBe("ar");
    expect(
      resolveClientLocale("", "zh", ["en-US"], "en", ["ar", "en", "zh"]),
    ).toBe("zh");
  });
  it("matches browser languages and falls back to the deployment default", () => {
    expect(
      resolveClientLocale("", null, ["fr-FR", "ko-KR"], "ar", ["ar", "ko"]),
    ).toBe("ko");
    expect(resolveClientLocale("", null, ["fr-FR"], "ar", ["ar", "en"])).toBe(
      "ar",
    );
  });
  it("does not let old account or device preferences escape supported languages", () => {
    expect(resolveClientLocale("ko", "zh", ["fr"], "ar", ["ar", "en"])).toBe(
      "ar",
    );
  });
});

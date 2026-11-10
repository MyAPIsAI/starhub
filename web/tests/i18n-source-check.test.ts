import { describe, expect, it } from "vitest";
// @ts-expect-error The executable CI checker is a JavaScript module.
import { findUntranslatedUI } from "./i18n-source-check.mjs";

describe("translation source gate", () => {
  it.each([
    "<button>Save changes</button>",
    '<input aria-label="条件名称" />',
    'const labels = [["arabic", "阿拉伯文", "Arabic"]];',
    'const labels = { zh: "标题", en: "Title", ar: "عنوان" };',
    'text(ready ? "可用" : "未知", ready ? "Ready" : "Unknown", "متاح")',
  ])("rejects untranslated UI: %s", (source) => {
    expect(
      findUntranslatedUI(source, "features/example.tsx").length,
    ).toBeGreaterThan(0);
  });
  it("allows catalog references, protocols and persisted-value comparisons", () => {
    expect(
      findUntranslatedUI(
        'const retired = row.reason === "注销清零"; <button>{text(...translatedMessage("save"))}</button>; <code>/v1/messages</code>; <span>USD</span>',
        "features/example.tsx",
      ),
    ).toEqual([]);
  });
  it("rejects a translation object outside a component", () => {
    expect(
      findUntranslatedUI(
        'const labels = { zh: "标题", en: "Title", ar: "عنوان" };',
        "lib/labels.ts",
      ).length,
    ).toBeGreaterThan(0);
  });
});

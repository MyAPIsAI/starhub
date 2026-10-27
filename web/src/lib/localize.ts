import type { Language } from "../config/placeholders";

/**
 * Languages with complete written translations. Korean (and any future locale
 * without full coverage) falls back to English so the UI stays usable while
 * translations are still being authored.
 */
export function writtenLanguage(language: Language): "en" | "zh" | "ar" {
  return language === "zh" || language === "ar" ? language : "en";
}

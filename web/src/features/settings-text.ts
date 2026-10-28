import { useTranslation } from "react-i18next";

export function useSettingsText() {
  const { i18n } = useTranslation();
  const lang: "zh" | "ar" | "en" = i18n.language.startsWith("zh")
    ? "zh"
    : i18n.language.startsWith("ar")
      ? "ar"
      : "en";
  return {
    lang,
    text: (zh: string, en: string, ar: string) =>
      lang === "zh" ? zh : lang === "ar" ? ar : en,
  };
}

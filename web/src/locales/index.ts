import { createI18n } from "vue-i18n";
import zh from "./zh-CN.json";
import en from "./en-US.json";
export const i18n = createI18n({
  legacy: false,
  locale: "zh-CN",
  fallbackLocale: "en-US",
  messages: { "zh-CN": zh, "en-US": en },
});

import { defineStore } from "pinia";
import { ref, watch } from "vue";
import { usePreferredDark } from "@vueuse/core";
import { i18n } from "@/locales";
function read(key: string, choices: string[], fallback: string) {
  try {
    const value = localStorage.getItem(`starhub.${key}`);
    return value && choices.includes(value) ? value : fallback;
  } catch {
    return fallback;
  }
}
export const useUI = defineStore("ui", () => {
  const theme = ref(read("theme", ["light", "dark", "system"], "system"));
  const locale = ref(read("locale", ["zh-CN", "en-US"], "zh-CN"));
  const unit = ref(read("unit", ["usd", "quota"], "usd"));
  const density = ref(read("density", ["compact", "comfortable"], "compact"));
  const collapsed = ref(false);
  const preferredDark = usePreferredDark();
  watch(
    [theme, locale, unit, density, preferredDark],
    () => {
      document.documentElement.classList.toggle(
        "dark",
        theme.value === "dark" ||
          (theme.value === "system" && preferredDark.value),
      );
      document.documentElement.lang = locale.value;
      i18n.global.locale.value = locale.value as "zh-CN" | "en-US";
      for (const [key, value] of Object.entries({
        theme: theme.value,
        locale: locale.value,
        unit: unit.value,
        density: density.value,
      })) {
        try {
          localStorage.setItem(`starhub.${key}`, value);
        } catch {
          /* Storage is optional. */
        }
      }
    },
    { immediate: true },
  );
  return { theme, locale, unit, density, collapsed };
});

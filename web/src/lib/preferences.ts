import { create } from "zustand";
import type { Language } from "../config/placeholders";
import {
  projectLanguages,
  resolveClientLocale,
  type PublicRegionPolicy,
} from "./locale-policy";
export type DataState = "ready" | "loading" | "empty" | "error";

function preference(key: string) {
  try {
    return localStorage.getItem(`xy-${key}`);
  } catch {
    return null;
  }
}
function writePreference(key: string, value: string) {
  try {
    localStorage.setItem(`xy-${key}`, value);
  } catch {
    /* Preferences are optional. */
  }
}
function browserLanguages() {
  return typeof navigator === "undefined"
    ? []
    : (navigator.languages ?? [navigator.language]);
}
const initialLanguage = resolveClientLocale(
  "",
  preference("language"),
  browserLanguages(),
  "en",
  projectLanguages,
);
export const usePreferences = create<{
  language: Language;
  theme: "light" | "dark";
  dataState: DataState;
  accountLanguage: string;
  regionDefaultLanguage: Language;
  supportedLanguages: Language[];
  applyRegionPolicy: (policy: PublicRegionPolicy) => void;
  applyAccountLanguage: (language?: string) => void;
  setLanguage: (language: Language) => void;
  setTheme: (theme: "light" | "dark") => void;
  setDataState: (state: DataState) => void;
}>((set, get) => ({
  accountLanguage: "",
  regionDefaultLanguage: "en",
  supportedLanguages: [...projectLanguages],
  applyRegionPolicy: (policy) => {
    const supported = policy.supported_languages.filter((tag) =>
      projectLanguages.includes(tag),
    );
    if (!supported.length || !supported.includes(policy.default_language))
      return;
    set({
      regionDefaultLanguage: policy.default_language,
      supportedLanguages: supported,
      language: resolveClientLocale(
        get().accountLanguage,
        preference("language"),
        browserLanguages(),
        policy.default_language,
        supported,
      ),
    });
  },
  applyAccountLanguage: (language = "") => {
    set({
      accountLanguage: language,
      language: resolveClientLocale(
        language,
        preference("language"),
        browserLanguages(),
        get().regionDefaultLanguage,
        get().supportedLanguages,
      ),
    });
  },
  language: ["en", "zh", "ar", "ko"].includes(initialLanguage)
    ? (initialLanguage as Language)
    : "en",
  theme: preference("theme") === "dark" ? "dark" : "light",
  dataState: "ready",
  setLanguage: (language) => {
    if (!get().supportedLanguages.includes(language)) return;
    writePreference("language", language);
    set({ language, accountLanguage: language });
  },
  setTheme: (theme) => {
    writePreference("theme", theme);
    set({ theme });
  },
  setDataState: (dataState) => set({ dataState }),
}));

import { create } from "zustand";
import type { Language } from "../config/placeholders";
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
const initialLanguage = preference("language") ?? "zh";
export const usePreferences = create<{
  language: Language;
  theme: "light" | "dark";
  dataState: DataState;
  setLanguage: (language: Language) => void;
  setTheme: (theme: "light" | "dark") => void;
  setDataState: (state: DataState) => void;
}>((set) => ({
  language: ["en", "zh", "ar"].includes(initialLanguage)
    ? (initialLanguage as Language)
    : "en",
  theme: preference("theme") === "dark" ? "dark" : "light",
  dataState: "ready",
  setLanguage: (language) => {
    writePreference("language", language);
    set({ language });
  },
  setTheme: (theme) => {
    writePreference("theme", theme);
    set({ theme });
  },
  setDataState: (dataState) => set({ dataState }),
}));

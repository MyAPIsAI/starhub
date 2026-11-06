import { create } from "zustand";
import type { Language } from "../config/placeholders";
import { apiGet, apiPut } from "./api/http";
export type DataState = "ready" | "loading" | "empty" | "error";
export type SupportedLanguage = "en" | "zh" | "ar";
export interface LanguagePolicy {
  default_language: SupportedLanguage;
  languages: SupportedLanguage[];
}
const supported: SupportedLanguage[] = ["en", "zh", "ar"];
let policy: LanguagePolicy = { default_language: "en", languages: supported };
let account: {
  id: number;
  account_kind?: string;
  preferred_language?: string;
} | null = null;
let generation = 0;
let staffPortal =
  typeof location !== "undefined" &&
  (/^\/(admin|staff)(\/|$)/.test(location.pathname) ||
    location.pathname === "/finance");
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
    /* Optional device storage. */
  }
}
export function matchLanguage(
  candidates: readonly string[],
  current: LanguagePolicy,
): SupportedLanguage {
  for (const candidate of candidates) {
    const base = candidate.toLowerCase().split(/[-_]/)[0] as SupportedLanguage;
    if (current.languages.includes(base)) return base;
  }
  return current.default_language;
}
function selectedLanguage(): SupportedLanguage {
  const saved = account?.preferred_language || preference("language");
  const selected =
    saved && supported.includes(saved as SupportedLanguage)
      ? policy.languages.includes(saved as SupportedLanguage)
        ? (saved as SupportedLanguage)
        : policy.default_language
      : matchLanguage(
          typeof navigator === "undefined" ? [] : navigator.languages,
          policy,
        );
  return staffPortal && selected === "ar" ? "en" : selected;
}
function publishLanguage() {
  const language = selectedLanguage();
  usePreferences.setState({
    language,
    languages: staffPortal ? ["en", "zh"] : policy.languages,
  });
  if (typeof document !== "undefined") {
    document.documentElement.lang = language;
    document.documentElement.dir = language === "ar" ? "rtl" : "ltr";
  }
}
export function bindAccountLanguage(user: typeof account) {
  generation++;
  account = user;
  usePreferences.setState({ languageSaving: false, languageError: false });
  publishLanguage();
}
export function setLanguagePortal(staff: boolean) {
  staffPortal = staff;
  publishLanguage();
}
export async function refreshLanguagePolicy() {
  const next = await apiGet<LanguagePolicy>(
    "/api/localization",
    AbortSignal.timeout(5000),
    { handleUnauthorizedLocally: true },
  );
  if (
    !supported.includes(next.default_language) ||
    !Array.isArray(next.languages) ||
    !next.languages.length ||
    next.languages.some((v) => !supported.includes(v)) ||
    !next.languages.includes(next.default_language)
  )
    throw Error("Invalid localization policy");
  policy = next;
  publishLanguage();
}
export const usePreferences = create<{
  language: Language;
  languages: SupportedLanguage[];
  languageSaving: boolean;
  languageError: boolean;
  theme: "light" | "dark";
  dataState: DataState;
  setLanguage: (language: Language) => Promise<void>;
  setTheme: (theme: "light" | "dark") => void;
  setDataState: (state: DataState) => void;
}>((set) => ({
  language: selectedLanguage(),
  languages: staffPortal ? ["en", "zh"] : policy.languages,
  languageSaving: false,
  languageError: false,
  theme: preference("theme") === "dark" ? "dark" : "light",
  dataState: "ready",
  setLanguage: async (language) => {
    if (
      !usePreferences
        .getState()
        .languages.includes(language as SupportedLanguage) ||
      usePreferences.getState().languageSaving
    )
      return;
    const owner = account,
      epoch = generation;
    set({ languageSaving: true, languageError: false });
    try {
      if (owner) {
        const staff = owner.account_kind === "staff";
        await apiPut(
          staff ? "/api/admin/auth/language" : "/api/user/language",
          { expected_user_id: owner.id, language },
          { portal: staff ? "staff" : "workspace" },
        );
        if (epoch !== generation) return;
        account = { ...owner, preferred_language: language };
      }
      writePreference("language", language);
      publishLanguage();
    } catch {
      if (epoch === generation) set({ languageError: true });
    } finally {
      if (epoch === generation) set({ languageSaving: false });
    }
  },
  setTheme: (theme) => {
    writePreference("theme", theme);
    set({ theme });
  },
  setDataState: (dataState) => set({ dataState }),
}));

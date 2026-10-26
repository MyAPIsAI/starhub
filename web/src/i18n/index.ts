import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import en from "./en.json";
import zh from "./zh.json";
import ar from "./ar.json";
import ko from "./ko.json";
import { usePreferences } from "../lib/preferences";

i18n.use(initReactI18next).init({
  resources: {
    en: { translation: en },
    zh: { translation: zh },
    ar: { translation: ar },
    ko: { translation: ko },
  },
  lng: usePreferences.getState().language,
  fallbackLng: "en",
  interpolation: { escapeValue: false },
});
export default i18n;

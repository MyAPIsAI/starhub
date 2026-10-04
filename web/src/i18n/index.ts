import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import en from "./en.json";
import zh from "./zh.json";
import ar from "./ar.json";
import { usePreferences } from "../lib/preferences";

i18n.use(initReactI18next).init({
  resources: {
    en: { translation: en },
    zh: { translation: zh },
    ar: { translation: ar },
  },
  lng: usePreferences.getState().language,
  fallbackLng: "en",
  interpolation: { escapeValue: false },
});
export default i18n;

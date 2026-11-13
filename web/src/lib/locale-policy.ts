import type { Language } from "../config/placeholders";
import { apiGet } from "./api/http";

export const projectLanguages: Language[] = ["zh", "en", "ar", "ko"];
export interface PublicRegionPolicy {
  id: string;
  revision: number;
  default_language: Language;
  supported_languages: Language[];
  legal_version: string;
  registration_enabled: boolean;
  feature_flags: Record<string, boolean>;
  country_code: string;
  timezone: string;
}

export function getPublicRegionPolicy(signal?: AbortSignal) {
  return apiGet<PublicRegionPolicy>("/api/public/region", signal);
}

export function resolveClientLocale(
  account: string,
  saved: string | null,
  browser: readonly string[],
  fallback: string,
  supported: readonly string[],
): Language {
  const base = (tag: string) => tag.trim().toLowerCase().split("-")[0];
  const allowed = supported.filter((tag): tag is Language =>
    projectLanguages.includes(tag as Language),
  );
  for (const tag of [account, saved ?? "", ...browser, fallback]) {
    const normalized = base(tag);
    if (allowed.includes(normalized as Language)) return normalized as Language;
  }
  return allowed[0] ?? "en";
}

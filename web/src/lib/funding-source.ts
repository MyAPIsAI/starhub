import { message as translatedMessage } from "../i18n/messages";
export type FundingSource =
  "simulation" | "legacy_unverified" | "api_quota" | "staff_adjustment";

export function fundingSource(value: unknown): FundingSource {
  return value === "staff_adjustment"
    ? "staff_adjustment"
    : value === "api_quota"
      ? "api_quota"
      : value === "simulation"
        ? "simulation"
        : "legacy_unverified";
}

export function fundingSourceLabel(
  value: unknown,
  text: (zh: string, en: string, ar: string) => string,
): string {
  if (value === "staff_adjustment")
    return text(...translatedMessage("staff_balance_adjustment_62e08ca719"));
  if (value === "api_quota")
    return text(...translatedMessage("api_usage_8b6a67406f"));
  return fundingSource(value) === "simulation"
    ? text(...translatedMessage("payment_simulation_834cdcde7e"))
    : text(...translatedMessage("legacy_source_unverified_b2b8b994cc"));
}

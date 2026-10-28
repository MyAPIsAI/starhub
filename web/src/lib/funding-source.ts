export type FundingSource = "simulation" | "legacy_unverified";

export function fundingSource(value: unknown): FundingSource {
  return value === "simulation" ? "simulation" : "legacy_unverified";
}

export function fundingSourceLabel(
  value: unknown,
  text: (zh: string, en: string, ar: string) => string,
): string {
  return fundingSource(value) === "simulation"
    ? text("支付模拟", "Payment simulation", "محاكاة دفع")
    : text(
        "历史来源未核验",
        "Legacy source unverified",
        "مصدر تاريخي غير متحقق منه",
      );
}

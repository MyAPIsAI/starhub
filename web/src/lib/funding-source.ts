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
    return text(
      "后台余额调整",
      "Staff balance adjustment",
      "تعديل الرصيد الإداري",
    );
  if (value === "api_quota")
    return text("API 调用", "API usage", "استخدام API");
  return fundingSource(value) === "simulation"
    ? text("支付模拟", "Payment simulation", "محاكاة دفع")
    : text(
        "历史来源未核验",
        "Legacy source unverified",
        "مصدر تاريخي غير متحقق منه",
      );
}

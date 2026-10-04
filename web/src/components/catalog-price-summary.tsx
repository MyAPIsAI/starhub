import type { DirectoryModel } from "../config/model-directory";
import type { PricingCatalogConfig } from "../config/models";
import type { PublicCatalogPrice } from "../lib/catalog-public-pricing";
import { getOfficialReference } from "../lib/official-reference";
import { usePreferences } from "../lib/preferences";
import { describeDirectoryRate } from "./directory-price";
import "./catalog-price-summary.css";

/** Browse-level prices; sources, conditions and estimates belong to model detail. */
export function CatalogPriceSummary({
  model,
  config,
  publicPrice,
}: {
  model: DirectoryModel;
  config: PricingCatalogConfig;
  publicPrice: PublicCatalogPrice;
}) {
  const lang = usePreferences((state) => state.language);
  const text = (zh: string, en: string, ar: string) =>
    lang === "zh" ? zh : lang === "ar" ? ar : en;
  const reference =
    model.kind === "smart" ? null : getOfficialReference(model.id, config);
  const rate =
    publicPrice.status === "ready" ? publicPrice.price?.rates[0] : undefined;
  const info = rate ? describeDirectoryRate(rate, lang) : null;
  const officialLabel =
    reference?.status === "legacy"
      ? text("历史参考", "Historical", "مرجع تاريخي")
      : text("官方参考", "Official", "مرجع رسمي");
  const publicLabel = text("平台最低", "Platform from", "سعر المنصة من");
  const tokenUnit = text(
    "美元 / 百万词元",
    "USD / million tokens",
    "دولار / مليون رمز",
  );
  const statusLabels = {
    unknown: text("模型不存在", "Model not found", "النموذج غير موجود"),
    offline: text("已下架", "Retired", "متوقف"),
    "no-public-group": text(
      "暂无公开报价",
      "No public offer",
      "لا يوجد عرض عام",
    ),
    "no-resource": text(
      "暂无公开可用资源",
      "No public resource",
      "لا يوجد مورد عام متاح",
    ),
    "quote-error": text(
      "暂无法确定最低价",
      "Minimum unavailable",
      "تعذر تحديد أقل سعر",
    ),
    ready: "",
  };
  const amount = (value: string | number | undefined) =>
    value === undefined ? "—" : `$${value}`;
  const unitLabel = (unit: "image" | "second" | "request") =>
    ({
      image: text("美元 / 张", "USD / image", "دولار / صورة"),
      second: text("美元 / 秒", "USD / second", "دولار / ثانية"),
      request: text("美元 / 次", "USD / request", "دولار / طلب"),
    })[unit];
  const tokenComparison =
    rate?.rule.mode === "tokens" &&
    (!reference || reference.rate.mode === "tokens");
  const officialTokens =
    reference?.rate.mode === "tokens" ? reference.rate.pricing : null;
  const specificationPrice =
    rate?.rule.mode === "unit" &&
    Boolean(
      rate.rule.conditionRates?.length ||
      Object.keys(rate.attributes ?? {}).length,
    );
  const platformValue =
    rate?.rule.mode === "expression"
      ? text("按用量计价", "Usage-based pricing", "حسب الاستخدام")
      : specificationPrice
        ? text("按规格计价", "Specification pricing", "حسب المواصفات")
        : (info?.value ??
          text("查看计价详情", "See pricing details", "عرض تفاصيل التسعير"));
  const showPlatformAmount =
    rate?.rule.mode !== "expression" && !specificationPrice;

  return (
    <div
      className="catalog-model-price catalog-price-summary"
      data-public-group={publicPrice.groupId ?? ""}
      data-public-status={publicPrice.status}
    >
      {publicPrice.status !== "ready" ? (
        <div className="catalog-price-summary-message" data-price-kind="public">
          <span className="catalog-price-summary-label">{publicLabel}</span>
          <span className="catalog-price-summary-value">
            {statusLabels[publicPrice.status]}
          </span>
        </div>
      ) : model.kind === "smart" ? (
        <div className="catalog-price-summary-message" data-price-kind="public">
          <span className="catalog-price-summary-label">
            {text("智能路由计费", "Routing pricing", "تسعير التوجيه")}
          </span>
          <span className="catalog-price-summary-value">
            {text(
              "按实际模型计费",
              "Billed by the actual model",
              "بسعر النموذج الفعلي",
            )}
          </span>
        </div>
      ) : tokenComparison ? (
        <>
          <span className="catalog-price-summary-unit">{tokenUnit}</span>
          <table className="catalog-price-summary-table">
            <caption className="sr-only">
              {text(
                `${model.name} 输入与输出价格`,
                `${model.name} input and output prices`,
                `أسعار الإدخال والإخراج لـ ${model.name}`,
              )}
            </caption>
            <thead>
              <tr>
                <th scope="col">
                  <span className="sr-only">
                    {text("用量", "Usage", "الاستخدام")}
                  </span>
                </th>
                <th scope="col">{officialLabel}</th>
                <th scope="col">{publicLabel}</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <th scope="row">{text("输入", "Input", "إدخال")}</th>
                <td data-price-kind="official">
                  <bdi dir="ltr">
                    {amount(officialTokens?.input_per_million_usd)}
                  </bdi>
                </td>
                <td data-price-kind="public">
                  <bdi dir="ltr">{amount(rate.input)}</bdi>
                </td>
              </tr>
              <tr>
                <th scope="row">{text("输出", "Output", "إخراج")}</th>
                <td data-price-kind="official">
                  <bdi dir="ltr">
                    {amount(officialTokens?.output_per_million_usd)}
                  </bdi>
                </td>
                <td data-price-kind="public">
                  <bdi dir="ltr">{amount(rate.output)}</bdi>
                </td>
              </tr>
            </tbody>
          </table>
          {!reference && (
            <span className="catalog-price-summary-note">
              {text(
                "官价尚未核实",
                "Official rate unverified",
                "السعر الرسمي غير موثق",
              )}
            </span>
          )}
        </>
      ) : (
        <div className="catalog-price-summary-pair">
          <div data-price-kind="official">
            <span className="catalog-price-summary-label">{officialLabel}</span>
            <span className="catalog-price-summary-value">
              {reference ? (
                <bdi dir="ltr">
                  {reference.rate.mode === "tokens"
                    ? `${amount(reference.rate.pricing.input_per_million_usd)} / ${amount(reference.rate.pricing.output_per_million_usd)}`
                    : amount(reference.rate.priceUsd)}
                </bdi>
              ) : (
                "—"
              )}
            </span>
            <span className="catalog-price-summary-note">
              {reference
                ? reference.rate.mode === "tokens"
                  ? text(
                      "输入 / 输出 · ",
                      "Input / output · ",
                      "إدخال / إخراج · ",
                    ) + tokenUnit
                  : unitLabel(reference.rate.unit)
                : text("尚未核实", "Unverified", "غير موثق")}
            </span>
          </div>
          <div data-price-kind="public">
            <span className="catalog-price-summary-label">
              {showPlatformAmount
                ? publicLabel
                : text("平台计费", "Platform pricing", "تسعير المنصة")}
            </span>
            <span className="catalog-price-summary-value">
              {showPlatformAmount ? (
                <bdi dir="ltr">{platformValue}</bdi>
              ) : (
                platformValue
              )}
            </span>
            {showPlatformAmount && info?.unit && (
              <span className="catalog-price-summary-note">{info.unit}</span>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

import { Link, useParams, useSearchParams } from "react-router-dom";
import { ArrowLeft, ArrowRight } from "lucide-react";
import { PublicHeader } from "../../components/product-shell";
import { Button } from "../../components/ui/button";
import { ModelIdCopy } from "../../components/model-id-copy";
import { PlazaPrice } from "../../components/plaza-price";
import { usePreferences } from "../../lib/preferences";
import { ApiError } from "../../lib/api/http";
import { usePlazaModel, plazaToDirectoryModel } from "../../lib/api/plaza";
import "../../components/model-pricing.css";
import "../../components/access-center.css";
import "../../components/plaza.css";

export default function ModelDetail() {
  const { id } = useParams<{ id: string }>();
  const [params] = useSearchParams();
  const lang = usePreferences((state) => state.language);
  const text = (zh: string, en: string, ar: string) =>
    lang === "zh" ? zh : lang === "ar" ? ar : en;
  const from = params.get("from");
  // Restrict the return destination to the catalog in this application.
  const returnTo =
    from && (from === "/models" || from.startsWith("/models?"))
      ? from
      : "/models";
  const query = usePlazaModel(id);
  const model = query.data
    ? plazaToDirectoryModel(query.data.model)
    : undefined;
  const group = query.data?.group ?? "—";
  const notFound =
    query.error instanceof ApiError && query.error.status === 404;
  const canConnect = Boolean(model?.isAvailable && model.protocols.length);
  const connectUrl =
    "/console/connect?model=" + encodeURIComponent(model?.id ?? "");
  const related = query.data?.related.same_vendor ?? [];

  return (
    <div className="brand-home public-refined model-detail-refined">
      <PublicHeader />
      <main className="access-model-detail">
        <Button asChild variant="secondary" size="small">
          <Link to={returnTo}>
            <ArrowLeft size={14} />
            {text("返回模型目录", "Back to models", "العودة إلى النماذج")}
          </Link>
        </Button>
        {query.isPending ? (
          <section className="access-empty" role="status">
            {text("正在加载模型…", "Loading model…", "جار تحميل النموذج…")}
          </section>
        ) : query.isError ? (
          <section className="access-empty" role="alert">
            <h1>
              {notFound
                ? text("模型不存在", "Model not found", "النموذج غير موجود")
                : text(
                    "模型加载失败",
                    "Model could not be loaded",
                    "تعذر تحميل النموذج",
                  )}
            </h1>
            <p>
              {notFound
                ? text(
                    "没有找到此模型标识，请返回目录选择模型。",
                    "This model ID was not found. Choose a model from the catalog.",
                    "لم يتم العثور على هذا المعرف. اختر نموذجًا من الدليل.",
                  )
                : text(
                    "暂时无法获取模型信息，请重试。",
                    "Model information is temporarily unavailable. Please retry.",
                    "معلومات النموذج غير متوفرة مؤقتًا. حاول مرة أخرى.",
                  )}
            </p>
            <code>{id}</code>
            {!notFound && (
              <Button onClick={() => void query.refetch()}>
                {text("重试", "Retry", "إعادة المحاولة")}
              </Button>
            )}
          </section>
        ) : model ? (
          <>
            <section className="access-detail-heading">
              <div>
                <span className="access-kind">{model.vendor}</span>
                <h1>{model.name}</h1>
                <ModelIdCopy id={model.id} />
                <p>{model.description[lang]}</p>
              </div>
              <div className="access-actions">
                {canConnect ? (
                  <Button asChild variant="primary">
                    <Link to={connectUrl}>
                      <ArrowRight size={15} />
                      {text("开始接入", "Connect model", "توصيل النموذج")}
                    </Link>
                  </Button>
                ) : (
                  <Button disabled variant="secondary">
                    {model.isAvailable
                      ? text(
                          "暂无支持协议",
                          "No supported protocol",
                          "لا يوجد بروتوكول مدعوم",
                        )
                      : text(
                          "当前分组不可用",
                          "Unavailable in this group",
                          "غير متاح في المجموعة الحالية",
                        )}
                  </Button>
                )}
              </div>
            </section>
            {!model.isAvailable && (
              <div className="access-notice access-error">
                {model.unavailableReason === "channel_missing"
                  ? text(
                      "暂无可用渠道。",
                      "No available channel.",
                      "لا توجد قناة متاحة.",
                    )
                  : text(
                      "当前分组没有可用渠道，可用分组见下方。",
                      "No channel is available in your current group. Coverage is listed below.",
                      "لا توجد قناة في مجموعتك الحالية. المجموعات المتاحة أدناه.",
                    )}
              </div>
            )}
            <section className="access-facts">
              <div>
                <small>{text("支持协议", "Protocols", "البروتوكولات")}</small>
                <strong>{model.protocols.join(" / ") || "—"}</strong>
              </div>
              <div>
                <small>{text("上下文", "Context", "السياق")}</small>
                <strong>
                  <bdi>{model.contextDisplay}</bdi>
                </strong>
              </div>
              <div>
                <small>
                  {text(
                    "最大输出词元",
                    "Maximum output tokens",
                    "أقصى رموز الإخراج",
                  )}
                </small>
                <strong>
                  <bdi>
                    {model.maxOutput > 0
                      ? model.maxOutput.toLocaleString()
                      : "—"}
                  </bdi>
                </strong>
              </div>
            </section>
            <section className="access-panel">
              <h2>{text("能力", "Capabilities", "القدرات")}</h2>
              <div className="plaza-group-list">
                {model.capabilities.length ? (
                  model.capabilities.map((capability) => (
                    <span key={capability}>
                      {(
                        {
                          text: text("文本", "Text", "نص"),
                          code: text("编程", "Code", "برمجة"),
                          vision: text("视觉", "Vision", "رؤية"),
                          reasoning: text("推理", "Reasoning", "استدلال"),
                          image: text("图片", "Image", "صور"),
                          video: text("视频", "Video", "فيديو"),
                          function_call: text(
                            "工具调用",
                            "Tool calling",
                            "استدعاء الأدوات",
                          ),
                        } as Record<string, string>
                      )[capability] ?? capability}
                    </span>
                  ))
                ) : (
                  <span>{text("未提供", "Not provided", "غير متوفر")}</span>
                )}
              </div>
            </section>
            <section className="access-panel">
              <h2>
                {text(
                  "当前分组报价",
                  "Current group prices",
                  "أسعار المجموعة الحالية",
                )}
              </h2>
              <p className="access-muted">
                {text("当前分组：", "Current group: ", "المجموعة الحالية: ")}
                <bdi>{group}</bdi>
                {text(
                  "。报价已包含分组倍率 ",
                  ". Quotes include the group multiplier ",
                  ". تشمل الأسعار معامل المجموعة ",
                )}
                <bdi>{model.pricing.group_ratio}×</bdi>
              </p>
              <PlazaPrice pricing={model.pricing} detailed />
            </section>
            <section className="access-panel">
              <h2>
                {text(
                  "可用服务分组",
                  "Available service groups",
                  "مجموعات الخدمة المتاحة",
                )}
              </h2>
              <div className="plaza-group-list">
                {model.availableGroups.length ? (
                  model.availableGroups.map((availableGroup) => (
                    <span
                      key={availableGroup}
                      data-current-group={
                        availableGroup === group ? "true" : undefined
                      }
                    >
                      <bdi>{availableGroup}</bdi>
                      {availableGroup === group &&
                        text("（当前）", " (current)", " (الحالية)")}
                    </span>
                  ))
                ) : (
                  <span>
                    {text(
                      "暂无可用分组",
                      "No available group",
                      "لا توجد مجموعة متاحة",
                    )}
                  </span>
                )}
              </div>
              <p className="access-muted">
                {text(
                  "以上是渠道覆盖分组，不代表密钥已获得这些分组的权限。当前报价仅对应上方当前分组。",
                  "These groups describe channel coverage, not key permissions. The quote applies only to the current group shown above.",
                  "تمثل هذه المجموعات تغطية القنوات وليس صلاحيات المفتاح. السعر للمجموعة الحالية الموضحة أعلاه فقط.",
                )}
              </p>
            </section>
            {related.length > 0 && (
              <section className="access-panel">
                <h2>
                  {text(
                    "同厂商模型",
                    "Models from this vendor",
                    "نماذج هذا المورد",
                  )}
                </h2>
                <div className="plaza-group-list">
                  {related.map((row) => (
                    <Link
                      key={row.id}
                      to={
                        "/models/" +
                        encodeURIComponent(row.id) +
                        "?from=" +
                        encodeURIComponent(returnTo)
                      }
                    >
                      {row.name}
                    </Link>
                  ))}
                </div>
              </section>
            )}
          </>
        ) : null}
      </main>
    </div>
  );
}

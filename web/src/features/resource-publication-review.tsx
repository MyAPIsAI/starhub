import { useState } from "react";
import { Button } from "../components/ui/button";
import type {
  PublicationPair,
  PublicationPreview,
} from "../lib/resource-publication";
import { useSettingsText } from "./settings-text";

export function usePublicationIssueLabel() {
  const { text } = useSettingsText();
  return (code: string) => {
    switch (code) {
      case "unknown_source_channel":
        return text(
          "来源渠道已不存在",
          "Source channel no longer exists",
          "قناة المصدر لم تعد موجودة",
        );
      case "unknown_gateway_channel":
        return text(
          "真实渠道已不存在",
          "Gateway channel no longer exists",
          "قناة البوابة لم تعد موجودة",
        );
      case "unbound_source_channel":
        return text(
          "请显式选择真实渠道",
          "Select a gateway channel explicitly",
          "اختر قناة البوابة صراحةً",
        );
      case "unavailable_resource_or_channel":
        return text(
          "来源、资源或真实渠道不可用",
          "Source, resource or gateway channel is unavailable",
          "المصدر أو المورد أو قناة البوابة غير متاحة",
        );
      case "model_alias_not_supported":
        return text(
          "暂不支持模型别名，三个模型标识须一致",
          "Aliases are unsupported; all three model IDs must match",
          "الأسماء البديلة غير مدعومة؛ يجب تطابق معرّفات النماذج الثلاثة",
        );
      case "gateway_model_id_not_supported":
        return text(
          "模型标识超出网关支持范围",
          "Model ID exceeds gateway limits",
          "معرّف النموذج يتجاوز حدود البوابة",
        );
      case "protocol_or_capability_not_supported":
        return text(
          "首期仅支持 OpenAI 类型文本/代码资源",
          "This preview supports OpenAI-type text/code resources only",
          "تدعم هذه المعاينة موارد النص والبرمجة من نوع OpenAI فقط",
        );
      case "group_unavailable_or_unpriced":
        return text(
          "分组不可用、已删除或未配置倍率",
          "Group is unavailable, deleted or has no configured ratio",
          "المجموعة غير متاحة أو محذوفة أو بلا معامل محدد",
        );
      case "no_supported_pairs":
        return text(
          "没有可支持的模型与分组组合",
          "No supported model and group pairs",
          "لا توجد أزواج مدعومة من النماذج والمجموعات",
        );
      case "incompatible_resource_constraints":
        return text(
          "同一组合的来源能力互不兼容，请统一清单或拆分真实渠道。",
          "Sources for this pair declare incompatible capabilities. Align inventory or use separate gateway channels.",
          "قدرات مصادر هذا الزوج غير متوافقة. وحّد المخزون أو استخدم قنوات بوابة منفصلة.",
        );
      case "gateway_bound_to_other_source":
        return text(
          "真实渠道仍绑定其他来源，请先核对并解除旧绑定。",
          "This gateway belongs to another source. Review and release its existing binding first.",
          "ترتبط قناة البوابة بمصدر آخر. راجع الرابط الحالي وأزله أولًا.",
        );
      case "source_bound_to_other_gateway":
        return text(
          "来源仍绑定另一个真实渠道，请先核对并解除旧绑定。",
          "This source is bound to another gateway. Review and release its existing binding first.",
          "يرتبط هذا المصدر بقناة بوابة أخرى. راجع الرابط الحالي وأزله أولًا.",
        );
      case "binding_without_supported_pairs":
        return text(
          "该绑定没有可发布组合，请启用有效投放或取消该选择。",
          "This binding has no supported pairs. Enable valid placements or remove this selection.",
          "لا يحتوي هذا الرابط على أزواج مدعومة. فعّل إعدادات نشر صالحة أو أزل هذا الاختيار.",
        );
      case "publication_revision_exhausted":
        return text(
          "发布版本已达到安全上限，请联系管理员处理。",
          "Publication revisions reached the safe limit. Contact an administrator.",
          "بلغت إصدارات النشر الحد الآمن. تواصل مع المسؤول.",
        );
      default:
        return text(
          "服务器返回了阻塞原因，请核对来源配置",
          "The server reported a blocking issue; check source configuration",
          "أبلغ الخادم عن مشكلة مانعة؛ تحقق من إعدادات المصدر",
        );
    }
  };
}

export function PairTable({
  title,
  rows,
  existing = false,
  showConstraints = false,
}: {
  title: string;
  rows: (PublicationPair & {
    enabled?: boolean;
    capabilities?: string[];
    upstream_model?: string;
    protocols?: string[];
    context_length?: number;
  })[];
  existing?: boolean;
  showConstraints?: boolean;
}) {
  const { text } = useSettingsText();
  const [page, setPage] = useState(0);
  const pages = Math.max(1, Math.ceil(rows.length / 20));
  const current = Math.min(page, pages - 1);
  return (
    <section className="rp-pairs" data-proposed={showConstraints || undefined}>
      <h3>
        {title} · {rows.length}
      </h3>
      <div className="rp-table" role="region" tabIndex={0} aria-label={title}>
        <table>
          <thead>
            <tr>
              <th>{text("真实渠道标识", "Gateway ID", "معرّف البوابة")}</th>
              <th>{text("模型", "Model", "النموذج")}</th>
              <th>{text("分组", "Group", "المجموعة")}</th>
              {showConstraints && (
                <>
                  <th>
                    {text(
                      "共同能力",
                      "Shared capabilities",
                      "القدرات المشتركة",
                    )}
                  </th>
                  <th>{text("上下文限制", "Context limit", "حد السياق")}</th>
                </>
              )}
              {existing && (
                <th>{text("能力状态", "Ability state", "حالة القدرة")}</th>
              )}
            </tr>
          </thead>
          <tbody>
            {rows.slice(current * 20, (current + 1) * 20).map((row) => (
              <tr
                key={JSON.stringify([
                  row.gateway_channel_id,
                  row.model,
                  row.group,
                ])}
              >
                <td>
                  <code>{row.gateway_channel_id}</code>
                </td>
                <td>
                  <code>{row.model}</code>
                  {row.upstream_model && (
                    <div>
                      <code dir="ltr">
                        → {row.upstream_model} · {row.protocols?.join(" / ")}
                      </code>
                    </div>
                  )}
                </td>
                <td>
                  <code>{row.group}</code>
                </td>
                {showConstraints && (
                  <>
                    <td>
                      {row.capabilities
                        ?.map((capability) =>
                          capability === "code"
                            ? text("代码", "Code", "برمجة")
                            : capability === "text"
                              ? text("文本", "Text", "نص")
                              : capability,
                        )
                        .join(" · ")}
                    </td>
                    <td>
                      {row.context_length ? (
                        <code>{row.context_length.toLocaleString()}</code>
                      ) : (
                        text("未知", "Unknown", "غير معروف")
                      )}
                    </td>
                  </>
                )}
                {existing && (
                  <td>
                    {row.enabled
                      ? text("已启用", "Enabled", "مفعلة")
                      : text("已停用", "Disabled", "معطلة")}
                  </td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {rows.length === 0 && (
        <p>{text("没有组合", "No pairs", "لا توجد أزواج")}</p>
      )}
      {pages > 1 && (
        <div className="rp-actions">
          <Button disabled={current === 0} onClick={() => setPage(current - 1)}>
            {text("上一页", "Previous", "السابق")}
          </Button>
          <span>
            {current + 1} / {pages}
          </span>
          <Button
            disabled={current + 1 >= pages}
            onClick={() => setPage(current + 1)}
          >
            {text("下一页", "Next", "التالي")}
          </Button>
        </div>
      )}
    </section>
  );
}

export function IssueList({
  issues,
  label,
}: {
  issues: PublicationPreview["issues"];
  label: (code: string) => string;
}) {
  const { text } = useSettingsText();
  const [page, setPage] = useState(0);
  const pages = Math.ceil(issues.length / 20);
  return (
    <section
      aria-label={text("发布阻塞原因", "Publication blockers", "عوائق النشر")}
    >
      <h3>
        {text("发布阻塞原因", "Publication blockers", "عوائق النشر")} ·{" "}
        {issues.length}
      </h3>
      <ul className="rp-issues">
        {issues.slice(page * 20, (page + 1) * 20).map((issue, i) => (
          <li key={i}>
            <strong>{label(issue.code)}</strong>
            {(issue.resource_id || issue.source_channel_id || issue.group) && (
              <code>
                {[issue.resource_id, issue.source_channel_id, issue.group]
                  .filter(Boolean)
                  .join(" · ")}
              </code>
            )}
          </li>
        ))}
      </ul>
      {pages > 1 && (
        <div className="rp-actions">
          <Button disabled={page === 0} onClick={() => setPage(page - 1)}>
            {text("上一页", "Previous", "السابق")}
          </Button>
          <span>
            {page + 1} / {pages}
          </span>
          <Button
            disabled={page + 1 >= pages}
            onClick={() => setPage(page + 1)}
          >
            {text("下一页", "Next", "التالي")}
          </Button>
        </div>
      )}
    </section>
  );
}

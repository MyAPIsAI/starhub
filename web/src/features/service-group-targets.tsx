import { useState } from "react";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import { ServiceLevelValue } from "../components/service-level-value";
import {
  useServiceGroups,
  useUpdateServiceGroupSLA,
  type ServiceGroup,
} from "../lib/api/service-group";
import {
  parseServiceLevelTargets,
  readServiceLevel,
  successPercent,
} from "../lib/service-level";
import { ROLE_ADMIN, ROLE_ROOT, useSystemAccess } from "../lib/system-access";
import { useSettingsText } from "./pricing-settings-shared";
import "./service-group-targets.css";

type Draft = {
  group: ServiceGroup;
  revision: number;
  percent: string;
  latency: string;
  remove: boolean;
};

export function ServiceGroupTargets() {
  const { text } = useSettingsText();
  const role = useSystemAccess((s) => s.role);
  const allowed = role === ROLE_ADMIN || role === ROLE_ROOT;
  const query = useServiceGroups(allowed);
  const mutation = useUpdateServiceGroupSLA();
  const [draft, setDraft] = useState<Draft | null>(null);
  const [review, setReview] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);
  if (!allowed) return null;
  const data = query.isError ? undefined : query.data;
  const rows =
    data?.filter((g) =>
      `${g.group_id} ${g.name}`
        .toLowerCase()
        .includes(search.trim().toLowerCase()),
    ) ?? [];
  const pages = Math.max(1, Math.ceil(rows.length / 20));
  const currentPage = Math.min(page, pages - 1);
  const targets = draft
    ? parseServiceLevelTargets(draft.percent, draft.latency)
    : undefined;
  const current =
    draft && data?.find((g) => g.group_id === draft.group.group_id);
  const conflict =
    !!draft &&
    (!current ||
      current.id !== draft.group.id ||
      readServiceLevel(current.sla)?.revision !== draft.revision);
  const begin = (group: ServiceGroup, remove = false) => {
    const sla = readServiceLevel(group.sla);
    if (!sla || !Number.isSafeInteger(group.id) || group.id <= 0) return;
    mutation.reset();
    setError("");
    setNotice("");
    setReview(remove);
    setDraft({
      group,
      revision: sla.revision,
      remove,
      percent:
        sla.targets?.success_rate_ppm == null
          ? ""
          : successPercent(sla.targets.success_rate_ppm),
      latency:
        sla.targets?.p95_latency_ms == null
          ? ""
          : String(sla.targets.p95_latency_ms),
    });
  };
  const close = () => {
    if (!mutation.isPending) {
      setDraft(null);
      setError("");
    }
  };
  const save = async () => {
    if (!draft || !review || conflict || !data || (!draft.remove && !targets))
      return;
    try {
      const result = await mutation.mutateAsync({
        group: draft.group.group_id,
        expected_group_id: draft.group.id,
        expected_revision: draft.revision,
        targets: draft.remove ? null : targets!,
      });
      const saved = readServiceLevel(result.sla);
      if (
        !saved ||
        result.id !== draft.group.id ||
        saved.revision !== draft.revision + 1
      )
        throw new Error("Invalid target update acknowledgement");
      setDraft(null);
      setNotice(
        text(
          `目标配置已保存：${result.group_id}，v${saved.revision}。`,
          `Targets saved: ${result.group_id}, v${saved.revision}.`,
          `حُفظت الأهداف: ${result.group_id}، v${saved.revision}.`,
        ),
      );
    } catch {
      setError(
        text(
          "无法确认保存结果。请刷新核对；版本变化后须重新打开编辑。",
          "Save outcome is unconfirmed. Refresh to check and reopen after a revision change.",
          "تعذر تأكيد الحفظ. حدّث للتحقق وأعد فتح المحرر عند تغير الإصدار.",
        ),
      );
    }
  };
  return (
    <section
      className="service-group-targets"
      aria-labelledby="group-target-heading"
    >
      <header>
        <div>
          <h2 id="group-target-heading">
            {text("分组 SLA 目标", "Group SLA targets", "أهداف SLA للمجموعة")}
          </h2>
          <p>
            {text(
              "仅显示数据库中的分组。目标由运营方发布，与实测统计分开；不代表合同保证，也不改变计费或路由。停用分组不在模型页展示目标。",
              "Only database groups are listed. Operator-published targets are separate from observed statistics, carry no contractual guarantee, and do not change billing or routing. Disabled groups do not expose targets on model pages.",
              "تُعرض مجموعات قاعدة البيانات فقط. الأهداف المنشورة منفصلة عن الإحصاءات وليست ضمانًا تعاقديًا ولا تغيّر الفوترة أو التوجيه. لا تعرض صفحات النماذج أهداف المجموعات المعطّلة.",
            )}
          </p>
        </div>
        <Button
          disabled={query.isFetching}
          onClick={() => {
            setNotice("");
            void query.refetch();
          }}
        >
          {text("刷新目标", "Refresh targets", "تحديث الأهداف")}
        </Button>
      </header>
      {query.isLoading && (
        <p role="status">
          {text("正在读取目标…", "Loading targets…", "جارٍ تحميل الأهداف…")}
        </p>
      )}
      {query.isError && (
        <p role="alert">
          {text(
            "无法读取分组目标，已隐藏旧数据。",
            "Cannot load group targets. Cached data is hidden.",
            "تعذر تحميل أهداف المجموعات. أُخفيت البيانات السابقة.",
          )}
        </p>
      )}
      {notice && (
        <p role="status" className="sgt-notice">
          {notice}
        </p>
      )}
      <label>
        {text(
          "搜索目标分组",
          "Search target groups",
          "البحث في مجموعات الأهداف",
        )}
        <input
          value={search}
          onChange={(e) => {
            setSearch(e.target.value);
            setPage(0);
          }}
        />
      </label>
      {data?.length === 0 && (
        <p>
          {text(
            "暂无数据库分组，请先创建并核对真实分组配置。",
            "No database groups. Create and verify a real group configuration first.",
            "لا توجد مجموعات في قاعدة البيانات. أنشئ إعداد مجموعة فعلية وتحقق منه أولًا.",
          )}
        </p>
      )}
      <div
        className="sgt-table"
        tabIndex={0}
        role="region"
        aria-label={text(
          "分组目标列表",
          "Group target list",
          "قائمة أهداف المجموعات",
        )}
      >
        <table>
          <thead>
            <tr>
              <th>{text("分组", "Group", "المجموعة")}</th>
              <th>
                {text("发布目标", "Published targets", "الأهداف المنشورة")}
              </th>
              <th>{text("版本", "Revision", "الإصدار")}</th>
              <th>{text("操作", "Actions", "الإجراءات")}</th>
            </tr>
          </thead>
          <tbody>
            {rows.slice(currentPage * 20, currentPage * 20 + 20).map((g) => {
              const sla = readServiceLevel(g.sla);
              const editable = !!sla && Number.isSafeInteger(g.id) && g.id > 0;
              return (
                <tr key={g.id} data-target-group={g.group_id}>
                  <td>
                    <strong>{g.name}</strong>
                    <div>
                      <bdi>{g.group_id}</bdi> ·{" "}
                      {g.enabled
                        ? text("启用", "Enabled", "مفعّلة")
                        : text("停用", "Disabled", "معطّلة")}
                    </div>
                  </td>
                  <td>
                    <ServiceLevelValue value={g.sla} text={text} />
                  </td>
                  <td>
                    <bdi>{sla ? `v${sla.revision}` : "—"}</bdi>
                  </td>
                  <td>
                    <div className="sgt-actions">
                      <Button disabled={!editable} onClick={() => begin(g)}>
                        {text("编辑目标", "Edit targets", "تحرير الأهداف")}
                      </Button>
                      <Button
                        disabled={!editable || !sla?.targets}
                        onClick={() => begin(g, true)}
                      >
                        {text("取消发布", "Unpublish", "إلغاء النشر")}
                      </Button>
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      {data && (
        <div className="sgt-actions">
          <Button
            disabled={currentPage === 0}
            onClick={() => setPage(currentPage - 1)}
          >
            {text("上一页", "Previous", "السابق")}
          </Button>
          <span>
            {currentPage + 1} / {pages}
          </span>
          <Button
            disabled={currentPage >= pages - 1}
            onClick={() => setPage(currentPage + 1)}
          >
            {text("下一页", "Next", "التالي")}
          </Button>
        </div>
      )}
      <Modal
        open={!!draft}
        onOpenChange={(open) => !open && close()}
        title={
          draft?.remove
            ? text("取消发布目标", "Unpublish targets", "إلغاء نشر الأهداف")
            : text("编辑分组目标", "Edit group targets", "تحرير أهداف المجموعة")
        }
        description={text(
          "保存需要匹配分组身份与目标版本。空白表示该项未发布，0% 是有效的已发布成功率目标。",
          "Saving requires the same group identity and target revision. Blank means that metric is unpublished; 0% is a valid published success target.",
          "يتطلب الحفظ تطابق هوية المجموعة وإصدار الأهداف. الحقل الفارغ يعني عدم نشر المقياس، و0% هدف نجاح صالح.",
        )}
        footer={
          <>
            <Button disabled={mutation.isPending} onClick={close}>
              {text("取消", "Cancel", "إلغاء")}
            </Button>
            {review ? (
              <Button
                variant="primary"
                disabled={
                  mutation.isPending ||
                  conflict ||
                  !data ||
                  (!draft?.remove && !targets)
                }
                onClick={() => void save()}
              >
                {text("确认保存目标", "Confirm targets", "تأكيد الأهداف")}
              </Button>
            ) : (
              <Button
                variant="primary"
                disabled={!targets || conflict || !data}
                onClick={() => setReview(true)}
              >
                {text("预览目标", "Review targets", "مراجعة الأهداف")}
              </Button>
            )}
          </>
        }
      >
        {draft && (
          <>
            <p>
              <strong>{draft.group.name}</strong> ·{" "}
              <bdi>{draft.group.group_id}</bdi> · v{draft.revision}
            </p>
            {review ? (
              <div>
                <p>
                  {draft.remove
                    ? text(
                        "确认后将清除该分组的已发布目标。",
                        "Confirming clears this group's published targets.",
                        "يؤدي التأكيد إلى مسح أهداف المجموعة المنشورة.",
                      )
                    : text(
                        "即将发布以下目标：",
                        "Targets to publish:",
                        "الأهداف المراد نشرها:",
                      )}
                </p>
                {!draft.remove && (
                  <ServiceLevelValue
                    value={{ revision: 1, updated_at: 1, targets }}
                    text={text}
                  />
                )}
                {!draft.remove && (
                  <Button
                    onClick={() => setReview(false)}
                    disabled={mutation.isPending}
                  >
                    {text("返回编辑", "Back to edit", "العودة للتحرير")}
                  </Button>
                )}
              </div>
            ) : (
              <div className="sgt-fields">
                <label>
                  {text(
                    "成功率目标（%）",
                    "Success target (%)",
                    "نسبة النجاح المستهدفة (%)",
                  )}
                  <input
                    dir="ltr"
                    inputMode="decimal"
                    value={draft.percent}
                    onChange={(e) =>
                      setDraft({ ...draft, percent: e.target.value })
                    }
                  />
                </label>
                <label>
                  {text(
                    "P95 目标（毫秒）",
                    "P95 target (ms)",
                    "هدف P95 (مللي ثانية)",
                  )}
                  <input
                    dir="ltr"
                    inputMode="numeric"
                    value={draft.latency}
                    onChange={(e) =>
                      setDraft({ ...draft, latency: e.target.value })
                    }
                  />
                </label>
                <p>
                  {text(
                    "成功率：0–100%，最多四位小数；P95：1–86400000 整数毫秒。至少填写一项。",
                    "Success: 0–100%, up to four decimal places. P95: 1–86400000 whole milliseconds. Set at least one metric.",
                    "النجاح: 0–100% حتى أربع منازل عشرية. P95: عدد صحيح من 1 إلى 86400000 مللي ثانية. أدخل مقياسًا واحدًا على الأقل.",
                  )}
                </p>
              </div>
            )}
            {conflict && (
              <p role="alert">
                {text(
                  "分组或版本已变化，或当前读取不可用。请刷新核对并重新打开编辑。",
                  "The group or revision changed, or the current read is unavailable. Refresh and reopen the editor.",
                  "تغيّرت المجموعة أو الإصدار، أو تعذر القراءة. حدّث وأعد فتح المحرر.",
                )}
              </p>
            )}
            {error && <p role="alert">{error}</p>}
          </>
        )}
      </Modal>
    </section>
  );
}

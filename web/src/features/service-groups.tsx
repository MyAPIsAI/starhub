import { useState } from "react";
import { Link } from "react-router-dom";
import { Layers3, Plus, Search } from "lucide-react";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import { ApiError } from "../lib/api/http";
import {
  useServiceGroupCatalog,
  useUpsertServiceGroup,
  useDeleteServiceGroup,
} from "../lib/api/service-group";
import {
  configuredGroupRatio,
  validGroupDefinitionID,
  validGroupMultiplier,
  type ManagedServiceGroup,
} from "../lib/service-group-config";
import { isAdmin, useSystemAccess } from "../lib/system-access";
import { useSettingsText } from "./pricing-settings-shared";
import { ServiceGroupTargets } from "./service-group-targets";
import "../components/catalog-management.css";
import "./service-groups.css";

type Draft = {
  original: ManagedServiceGroup | null;
  id: string;
  locked: boolean;
  name: string;
  description: string;
  multiplier: string;
  enabled: boolean;
  optionVersion: number;
  ratios: Readonly<Record<string, number>>;
  remove: boolean;
};

export function ServiceGroups({ role: _role }: { role: number }) {
  const { text } = useSettingsText();
  const role = useSystemAccess((s) => s.role);
  const authenticated = useSystemAccess((s) => s.session.accountId !== null);
  const allowed = authenticated && isAdmin(role);
  const query = useServiceGroupCatalog(allowed);
  const upsert = useUpsertServiceGroup();
  const remove = useDeleteServiceGroup();
  const [draft, setDraft] = useState<Draft | null>(null);
  const [review, setReview] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);
  if (!allowed)
    return (
      <p role="alert">
        {text(
          "需要实际管理员会话。",
          "An actual administrator session is required.",
          "يلزم تسجيل دخول مسؤول فعلي.",
        )}
      </p>
    );
  const data = query.isError ? undefined : query.data;
  const groups = new Map(data?.groups.map((g) => [g.group_id, g]));
  const ids = [
    ...new Set([...groups.keys(), ...Object.keys(data?.group_ratios ?? {})]),
  ]
    .sort()
    .filter((id) =>
      `${id} ${groups.get(id)?.name ?? ""}`
        .toLowerCase()
        .includes(search.trim().toLowerCase()),
    );
  const pages = Math.max(1, Math.ceil(ids.length / 20));
  const currentPage = Math.min(page, pages - 1);
  const pending = upsert.isPending || remove.isPending;
  const versionWritable =
    !!data && data.option_version < Number.MAX_SAFE_INTEGER;
  const current = draft && groups.get(draft.id.trim());
  const conflict =
    !!draft &&
    (!data ||
      data.option_version !== draft.optionVersion ||
      (draft.original
        ? current?.id !== draft.original.id ||
          current?.revision !== draft.original.revision
        : !!current));
  const valid =
    !!draft &&
    (draft.remove ||
      (validGroupDefinitionID(draft.id.trim()) &&
        draft.name.trim().length > 0 &&
        draft.name.trim().length <= 128 &&
        draft.description.length <= 512 &&
        validGroupMultiplier(draft.multiplier)));
  const begin = (id = "", deleting = false) => {
    if (!data) return;
    const original = groups.get(id) ?? null;
    const ratio = configuredGroupRatio(data, id);
    upsert.reset();
    remove.reset();
    setError("");
    setNotice("");
    setReview(deleting);
    setDraft({
      original,
      id,
      locked: !!id,
      name: original?.name ?? id,
      description: original?.description ?? "",
      multiplier:
        original?.multiplier ??
        (ratio !== undefined && ratio > 0 && ratio <= 100
          ? String(ratio)
          : id
            ? ""
            : "1"),
      enabled: original?.enabled ?? true,
      optionVersion: data.option_version,
      ratios: data.group_ratios,
      remove: deleting,
    });
  };
  const close = () => {
    if (!pending) {
      setDraft(null);
      setError("");
    }
  };
  const save = async () => {
    if (!draft || !data || !review || !valid || conflict || !versionWritable)
      return;
    const identity = {
      expected_group_id: draft.original?.id ?? 0,
      expected_revision: draft.original?.revision ?? 0,
      expected_option_version: draft.optionVersion,
    };
    try {
      if (draft.remove) {
        const result = await remove.mutateAsync({
          group_id: draft.id,
          ...identity,
        });
        if (result != null) throw new Error("Invalid deletion acknowledgement");
      } else {
        const result = await upsert.mutateAsync({
          group_id: draft.id.trim(),
          name: draft.name.trim(),
          description: draft.description.trim(),
          enabled: draft.enabled,
          multiplier: draft.multiplier.trim(),
          visibility: draft.original?.visibility ?? "common",
          account_ids: draft.original?.account_ids ?? [],
          ...identity,
        });
        if (
          result.group_id !== draft.id.trim() ||
          result.revision !== identity.expected_revision + 1 ||
          (draft.original && result.id !== draft.original.id)
        )
          throw new Error("Invalid group update acknowledgement");
      }
      setDraft(null);
      setNotice(
        draft.remove
          ? text(
              "分组定义与对应倍率已删除。",
              "Group definition and its ratio were deleted.",
              "حُذف تعريف المجموعة ومعاملها.",
            )
          : text(
              "分组配置已保存，响应进程已完成设置重载。其他进程状态请查看配置同步。",
              "Group configuration saved; the response process reloaded its settings. Check configuration sync for other processes.",
              "حُفظت المجموعة وأعادت العملية المستجيبة تحميل الإعدادات. راجع المزامنة لبقية العمليات.",
            ),
      );
    } catch (caught) {
      setError(
        caught instanceof ApiError && caught.status === 409
          ? text(
              "分组或配置版本已变化，本次操作未覆盖。请刷新并重新打开编辑。",
              "The group or configuration version changed. This operation did not overwrite it. Refresh and reopen the editor.",
              "تغيّرت المجموعة أو إصدار الإعدادات. لم تستبدل هذه العملية التغيير. حدّث وأعد فتح المحرر.",
            )
          : text(
              "无法确认保存或重载结果。请刷新核对配置与同步状态；版本变化后须重新打开编辑。",
              "The save or reload outcome is unconfirmed. Refresh and check configuration sync; reopen after a version change.",
              "تعذر تأكيد الحفظ أو إعادة التحميل. حدّث وتحقق من المزامنة وأعد فتح المحرر عند تغير الإصدار.",
            ),
      );
    }
  };
  const oldRatio =
    draft && Object.prototype.hasOwnProperty.call(draft.ratios, draft.id.trim())
      ? draft.ratios[draft.id.trim()]
      : undefined;
  const unknown = text("未配置", "Unconfigured", "غير مهيّأ");
  return (
    <div className="catalog-management service-group-management">
      <header className="cm-heading">
        <div>
          <span className="pricing-eyebrow">
            <Layers3 size={16} />
            {text("系统设置", "System settings", "إعدادات النظام")}
          </span>
          <h1>{text("服务分组", "Service groups", "مجموعات الخدمة")}</h1>
          <p>
            {text(
              "管理数据库中的分组定义与倍率。保存会修改服务器配置；表格中的选项倍率与分组定义分别展示。",
              "Manage database group definitions and ratios. Saving changes server configuration; option ratios and stored definitions are shown separately.",
              "أدر تعريفات مجموعات قاعدة البيانات ومعاملاتها. يغيّر الحفظ إعدادات الخادم؛ تُعرض معاملات الخيارات والتعريفات بشكل منفصل.",
            )}
          </p>
        </div>
        <div className="cm-actions">
          <Button
            disabled={query.isFetching}
            onClick={() => {
              setNotice("");
              void query.refetch();
            }}
          >
            {text("刷新分组", "Refresh groups", "تحديث المجموعات")}
          </Button>
          <Button
            variant="primary"
            disabled={!versionWritable}
            onClick={() => begin()}
          >
            <Plus size={16} />
            {text("新增分组定义", "Add group definition", "إضافة تعريف مجموعة")}
          </Button>
        </div>
      </header>
      <div className="cm-callout">
        <p>
          {text(
            "这里展示数据库配置。最终费用还受模型规则、用户/分组矩阵和运行中设置影响。账号名单当前仅是元数据，网关尚未以该名单校验调用。",
            "This view shows database configuration. Final charges also depend on model rules, user/group matrices and running settings. Account lists are currently metadata; the gateway does not yet enforce these lists on calls.",
            "تعرض هذه الصفحة إعدادات قاعدة البيانات. تعتمد الرسوم أيضًا على قواعد النماذج ومصفوفات المستخدم والمجموعة والإعدادات الفعلية. قوائم الحسابات حاليًا بيانات وصفية ولا تتحقق البوابة منها في الطلبات.",
          )}
        </p>
        <Link to="/admin?tab=resources">
          {text(
            "查看配置同步",
            "View configuration sync",
            "عرض مزامنة الإعدادات",
          )}
        </Link>
      </div>
      {query.isLoading && (
        <p role="status">
          {text(
            "正在读取分组配置…",
            "Loading group configuration…",
            "جارٍ تحميل إعدادات المجموعات…",
          )}
        </p>
      )}
      {query.isError && (
        <p role="alert">
          {text(
            "无法确认服务器配置或版本保护能力，已隐藏旧数据并暂停编辑。请刷新重试。",
            "Cannot verify server configuration or version-guard support. Cached data is hidden and editing is paused. Refresh to retry.",
            "تعذر التحقق من الإعدادات أو دعم حماية الإصدار. أُخفيت البيانات السابقة وأُوقف التحرير. حدّث للمحاولة.",
          )}
        </p>
      )}
      {data && (
        <p className="sgm-versions">
          <span>
            {text("配置版本", "Configuration version", "إصدار الإعدادات")}:{" "}
            <bdi>v{data.option_version}</bdi>
          </span>
          <span>
            {text(
              "响应进程加载版本",
              "Response process loaded version",
              "الإصدار المحمّل في العملية المستجيبة",
            )}
            : <bdi>v{data.loaded_option_version}</bdi>
          </span>
        </p>
      )}
      {notice && (
        <p className="pricing-notice" role="status">
          {notice}
        </p>
      )}
      <div className="cm-toolbar">
        <label className="cm-search">
          <Search size={17} />
          <input
            aria-label={text(
              "搜索分组定义",
              "Search group definitions",
              "البحث في تعريفات المجموعات",
            )}
            value={search}
            onChange={(e) => {
              setSearch(e.target.value);
              setPage(0);
            }}
            placeholder={text(
              "搜索名称或标识",
              "Search names or identifiers",
              "ابحث بالاسم أو المعرّف",
            )}
          />
        </label>
        <span>
          {ids.length} {text("条配置", "entries", "إعدادات")}
        </span>
      </div>
      {data && data.groups.length === 0 && (
        <p>
          {text(
            "暂无数据库分组定义。已有倍率键会保留，可在下方登记；不会自动创建定义。",
            "No database group definitions. Existing ratio entries remain and can be registered below; definitions are not created automatically.",
            "لا توجد تعريفات مجموعات. تبقى مفاتيح المعاملات ويمكن تسجيلها أدناه؛ لا تُنشأ التعريفات تلقائيًا.",
          )}
        </p>
      )}
      <div
        className="cm-table"
        role="region"
        tabIndex={0}
        aria-label={text(
          "分组配置表，可横向滚动",
          "Group configuration table, horizontally scrollable",
          "جدول إعدادات المجموعات قابل للتمرير أفقيًا",
        )}
      >
        <table>
          <thead>
            <tr>
              <th>{text("分组", "Group", "المجموعة")}</th>
              <th>{text("定义状态", "Definition state", "حالة التعريف")}</th>
              <th>{text("定义倍率", "Definition ratio", "معامل التعريف")}</th>
              <th>{text("选项倍率", "Option ratio", "معامل الخيار")}</th>
              <th>
                {text("元数据版本", "Metadata revision", "إصدار البيانات")}
              </th>
              <th>{text("操作", "Actions", "الإجراءات")}</th>
            </tr>
          </thead>
          <tbody>
            {ids.slice(currentPage * 20, currentPage * 20 + 20).map((id) => {
              const group = groups.get(id);
              const ratio = data ? configuredGroupRatio(data, id) : undefined;
              const editable =
                versionWritable &&
                validGroupDefinitionID(id) &&
                (!group || group.revision < Number.MAX_SAFE_INTEGER);
              return (
                <tr key={id} data-config-group={id}>
                  <td>
                    <strong>{group?.name ?? id}</strong>
                    <code dir="ltr">{id}</code>
                    {group?.description && <small>{group.description}</small>}
                  </td>
                  <td>
                    {group
                      ? group.enabled
                        ? text("启用", "Enabled", "مفعّلة")
                        : text("停用", "Disabled", "معطّلة")
                      : text(
                          "仅有倍率，未登记定义",
                          "Ratio only; no definition",
                          "معامل فقط دون تعريف",
                        )}
                    {group?.visibility === "custom" && (
                      <small>
                        {text(
                          "历史定制元数据",
                          "Legacy custom metadata",
                          "بيانات مخصصة سابقة",
                        )}
                      </small>
                    )}
                  </td>
                  <td>
                    <bdi>{group?.multiplier ?? "—"}</bdi>
                  </td>
                  <td>
                    <bdi>{ratio === undefined ? unknown : ratio}</bdi>
                  </td>
                  <td>
                    <bdi>{group ? `v${group.revision}` : "—"}</bdi>
                  </td>
                  <td>
                    <div className="cm-actions">
                      <Button disabled={!editable} onClick={() => begin(id)}>
                        {group
                          ? text("编辑定义", "Edit definition", "تحرير التعريف")
                          : text(
                              "登记分组",
                              "Register group",
                              "تسجيل المجموعة",
                            )}
                      </Button>
                      {group && (
                        <Button
                          disabled={!editable}
                          onClick={() => begin(id, true)}
                        >
                          {text("删除定义", "Delete definition", "حذف التعريف")}
                        </Button>
                      )}
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      {data && (
        <div className="cm-actions">
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
      <ServiceGroupTargets />
      <Modal
        className="service-group-dialog"
        open={!!draft}
        onOpenChange={(open) => !open && close()}
        title={
          draft?.remove
            ? text(
                "删除分组定义",
                "Delete group definition",
                "حذف تعريف المجموعة",
              )
            : text(
                "编辑分组定义",
                "Edit group definition",
                "تحرير تعريف المجموعة",
              )
        }
        description={text(
          "保存需要匹配分组身份、元数据版本和配置版本。取消会放弃当前草稿。",
          "Saving requires matching group identity, metadata revision and configuration version. Cancel discards this draft.",
          "يتطلب الحفظ تطابق هوية المجموعة وإصدار البيانات والإعدادات. يُلغي الإغلاق المسودة الحالية.",
        )}
        footer={
          <>
            <Button disabled={pending} onClick={close}>
              {text("取消", "Cancel", "إلغاء")}
            </Button>
            {review ? (
              <Button
                variant="primary"
                disabled={pending || conflict || !valid || !versionWritable}
                onClick={() => void save()}
              >
                {text(
                  "确认分组变更",
                  "Confirm group changes",
                  "تأكيد تغييرات المجموعة",
                )}
              </Button>
            ) : (
              <Button
                variant="primary"
                disabled={!valid || conflict || !versionWritable}
                onClick={() => setReview(true)}
              >
                {text(
                  "预览分组变更",
                  "Review group changes",
                  "مراجعة تغييرات المجموعة",
                )}
              </Button>
            )}
          </>
        }
      >
        {draft && (
          <>
            <p>
              <bdi>{draft.id || "—"}</bdi> ·{" "}
              {text("元数据", "Metadata", "البيانات")}: v
              {draft.original?.revision ?? 0} ·{" "}
              {text("配置", "Configuration", "الإعدادات")}: v
              {draft.optionVersion}
            </p>
            {review ? (
              <div className="sgm-review">
                <h3>
                  {draft.remove
                    ? text(
                        "删除定义及对应倍率",
                        "Delete definition and its ratio",
                        "حذف التعريف ومعامله",
                      )
                    : draft.name.trim()}
                </h3>
                <p>
                  {text(
                    "选项倍率变更",
                    "Option ratio change",
                    "تغيير معامل الخيار",
                  )}
                  :{" "}
                  <bdi>
                    {oldRatio ?? unknown} →{" "}
                    {draft.remove || !draft.enabled
                      ? unknown
                      : draft.multiplier.trim()}
                  </bdi>
                </p>
                <p>
                  {draft.remove || !draft.enabled
                    ? text(
                        "移除倍率后，该分组不再通过网关分组准入。此操作不修改账本。",
                        "After removing the ratio, this group no longer passes gateway group admission. Ledgers are unchanged.",
                        "بعد إزالة المعامل لا تجتاز المجموعة تحقق الدخول في البوابة. لا تتغير السجلات المالية.",
                      )
                    : text(
                        "保存会发布该倍率；模型可用性仍取决于渠道配置。",
                        "Saving publishes this ratio; model availability still depends on channel configuration.",
                        "ينشر الحفظ هذا المعامل؛ يظل توفر النماذج مرتبطًا بإعداد القنوات.",
                      )}
                </p>
                {!draft.remove && (
                  <Button disabled={pending} onClick={() => setReview(false)}>
                    {text("返回编辑", "Back to edit", "العودة للتحرير")}
                  </Button>
                )}
              </div>
            ) : (
              <div className="sgm-fields">
                <label>
                  {text("分组标识", "Group identifier", "معرّف المجموعة")}
                  <input
                    dir="ltr"
                    readOnly={draft.locked}
                    maxLength={64}
                    value={draft.id}
                    onChange={(e) => setDraft({ ...draft, id: e.target.value })}
                  />
                </label>
                <label>
                  {text("分组名称", "Group name", "اسم المجموعة")}
                  <input
                    maxLength={128}
                    value={draft.name}
                    onChange={(e) =>
                      setDraft({ ...draft, name: e.target.value })
                    }
                  />
                </label>
                <label>
                  {text("分组倍率", "Group ratio", "معامل المجموعة")}
                  <input
                    dir="ltr"
                    inputMode="decimal"
                    aria-label={text(
                      "分组倍率",
                      "Group ratio",
                      "معامل المجموعة",
                    )}
                    aria-describedby="group-ratio-help"
                    maxLength={32}
                    value={draft.multiplier}
                    onChange={(e) =>
                      setDraft({ ...draft, multiplier: e.target.value })
                    }
                  />
                  <small id="group-ratio-help">
                    {text(
                      "大于 0，不超过 100。",
                      "Greater than 0 and at most 100.",
                      "أكبر من 0 وحتى 100.",
                    )}
                  </small>
                </label>
                <label className="cm-check">
                  <input
                    type="checkbox"
                    checked={draft.enabled}
                    onChange={(e) =>
                      setDraft({ ...draft, enabled: e.target.checked })
                    }
                  />
                  {text(
                    "启用分组倍率",
                    "Enable group ratio",
                    "تفعيل معامل المجموعة",
                  )}
                </label>
                <label className="cm-wide">
                  {text("分组说明", "Group description", "وصف المجموعة")}
                  <textarea
                    maxLength={512}
                    value={draft.description}
                    onChange={(e) =>
                      setDraft({ ...draft, description: e.target.value })
                    }
                  />
                </label>
                {draft.original?.visibility === "custom" && (
                  <div className="cm-wide sgm-legacy">
                    <strong>
                      {text(
                        "保留的定制元数据",
                        "Retained custom metadata",
                        "البيانات المخصصة المحفوظة",
                      )}
                    </strong>
                    <p
                      dir="ltr"
                      tabIndex={0}
                      role="region"
                      aria-label={text(
                        "保留的账号标识",
                        "Retained account identifiers",
                        "معرّفات الحسابات المحفوظة",
                      )}
                    >
                      {draft.original.account_ids.join(", ")}
                    </p>
                    <small>
                      {text(
                        "本次保留原账号名单；当前不将其作为网关授权保证。",
                        "This edit retains the stored account list; it is not a gateway authorization guarantee.",
                        "يُحفظ هنا كشف الحسابات السابق؛ وهو ليس ضمانًا لصلاحيات البوابة.",
                      )}
                    </small>
                  </div>
                )}
              </div>
            )}
            {conflict && (
              <p role="alert">
                {text(
                  "分组或配置版本已变化，或读取暂不可用。请刷新后重新打开编辑。",
                  "The group or configuration version changed, or the read is unavailable. Refresh and reopen the editor.",
                  "تغيّرت المجموعة أو إصدار الإعدادات أو تعذرت القراءة. حدّث وأعد فتح المحرر.",
                )}
              </p>
            )}
            {error && <p role="alert">{error}</p>}
          </>
        )}
      </Modal>
    </div>
  );
}

import { useState } from "react";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import { ApiError } from "../lib/api/http";
import {
  useServerTimePricing,
  useSaveServerTimePricing,
} from "../lib/api/time-pricing";
import { isAdmin, useSystemAccess } from "../lib/system-access";
import {
  replaceServerTimeSchedule,
  timeScheduleDraft,
  timeScheduleError,
  validTimeModelId,
  type ServerTimePricing,
  type TimeScheduleDraft,
  type TimeWindowDraft,
  type TimeScheduleError,
} from "../lib/server-time-pricing";
import { useSettingsText } from "./settings-text";
import "./server-time-pricing.css";

type Draft = {
  base: ServerTimePricing;
  id: string;
  locked: boolean;
  remove: boolean;
  schedule: TimeScheduleDraft;
};
export function ServerTimePricingEditor() {
  const { text } = useSettingsText(),
    role = useSystemAccess((state) => state.role),
    authenticated = useSystemAccess(
      (state) => state.session.accountId !== null,
    );
  const allowed = authenticated && isAdmin(role),
    query = useServerTimePricing(allowed),
    mutation = useSaveServerTimePricing();
  const [search, setSearch] = useState(""),
    [page, setPage] = useState(0),
    [draft, setDraft] = useState<Draft | null>(null),
    [review, setReview] = useState(false),
    [error, setError] = useState(""),
    [notice, setNotice] = useState("");
  if (!allowed) return null;
  const data = query.isError ? undefined : query.data,
    writable = !!data && data.version < Number.MAX_SAFE_INTEGER;
  const ids = data
    ? Object.keys(data.rules)
        .sort()
        .filter((id) => id.toLowerCase().includes(search.trim().toLowerCase()))
    : [];
  const pages = Math.max(1, Math.ceil(ids.length / 20)),
    currentPage = Math.min(page, pages - 1);
  const conflict = !!draft && (!data || data.version !== draft.base.version);
  const issue =
    draft && !draft.remove ? timeScheduleError(draft.schedule) : null;
  const valid =
    !!draft &&
    (draft.remove ||
      (validTimeModelId(draft.id) &&
        !["myapis-auto", "veyra-auto"].includes(draft.id) &&
        !issue &&
        (draft.locked || !Object.hasOwn(draft.base.rules, draft.id))));
  const weekdays = [
    text("周日", "Sunday", "الأحد"),
    text("周一", "Monday", "الاثنين"),
    text("周二", "Tuesday", "الثلاثاء"),
    text("周三", "Wednesday", "الأربعاء"),
    text("周四", "Thursday", "الخميس"),
    text("周五", "Friday", "الجمعة"),
    text("周六", "Saturday", "السبت"),
  ];
  const issueText: Record<TimeScheduleError, string> = {
    timezone: text(
      "请输入明确的 IANA 时区或 UTC，不可使用 Local。",
      "Enter an explicit IANA timezone or UTC; Local is not supported.",
      "أدخل منطقة IANA صريحة أو UTC؛ لا تُقبل Local.",
    ),
    limit: text(
      "每个模型最多 32 个时段。",
      "Each model supports at most 32 windows.",
      "يدعم كل نموذج 32 فترة كحد أقصى.",
    ),
    id: text(
      "时段标识须唯一，使用 1–64 个英文字母、数字、点、下划线或连字符。",
      "Window IDs must be unique, using 1–64 ASCII letters, digits, dots, underscores or hyphens.",
      "يجب أن تكون المعرفات فريدة من 1–64 حرفًا لاتينيًا أو رقمًا أو نقطة أو شرطة.",
    ),
    label: text(
      "时段名称须非空，最多 128 个 UTF-8 字节且无控制字符。",
      "Window labels must be nonempty, at most 128 UTF-8 bytes, without control characters.",
      "يجب أن يكون الاسم غير فارغ ولا يتجاوز 128 بايت UTF-8 دون محارف تحكم.",
    ),
    days: text(
      "每个时段至少选择一个星期。",
      "Select at least one weekday per window.",
      "اختر يومًا واحدًا على الأقل لكل فترة.",
    ),
    time: text(
      "使用 HH:MM，结束可为 24:00；起止不能相同。整日请填 00:00–24:00。",
      "Use HH:MM, with 24:00 allowed only as an end. Start and end must differ; use 00:00–24:00 for a whole day.",
      "استخدم HH:MM مع 24:00 للنهاية فقط. يجب اختلاف البداية والنهاية؛ لليوم الكامل استخدم 00:00–24:00.",
    ),
    multiplier: text(
      "倍率须大于 0 且不超过 100。",
      "Multiplier must be greater than 0 and at most 100.",
      "يجب أن يكون المعامل أكبر من صفر وألا يتجاوز 100.",
    ),
    overlap: text(
      "时段重叠，请检查星期及跨午夜范围。相邻边界可以共用。",
      "Windows overlap. Check weekdays and overnight ranges; adjacent boundaries are allowed.",
      "الفترات متداخلة. تحقق من الأيام وامتداد منتصف الليل؛ يُسمح بتجاور الحدود.",
    ),
  };
  const begin = (id = "", remove = false) => {
    if (!data) return;
    mutation.reset();
    setError("");
    setNotice("");
    setReview(remove);
    setDraft({
      base: data,
      id,
      locked: !!id,
      remove,
      schedule: timeScheduleDraft(
        Object.hasOwn(data.rules, id) ? data.rules[id] : undefined,
      ),
    });
  };
  const updateWindow = (index: number, changes: Partial<TimeWindowDraft>) => {
    if (draft)
      setDraft({
        ...draft,
        schedule: {
          ...draft.schedule,
          windows: draft.schedule.windows.map((window, i) =>
            i === index ? { ...window, ...changes } : window,
          ),
        },
      });
  };
  const save = async () => {
    if (!draft || !review || !valid || conflict || !writable) return;
    try {
      const result = await mutation.mutateAsync({
        expected_version: draft.base.version,
        rules: replaceServerTimeSchedule(
          draft.base,
          draft.id,
          draft.remove ? null : draft.schedule,
        ),
      });
      if (
        result.version < draft.base.version ||
        result.version > draft.base.version + 1
      )
        throw Error("Unexpected time-pricing revision");
      setDraft(null);
      setNotice(
        text(
          `分时配置已保存 v${result.version}；响应进程加载 v${result.loaded_version}，其他实例请查看配置同步。`,
          `Time pricing saved at v${result.version}; response process loaded v${result.loaded_version}. Check configuration sync for other instances.`,
          `حُفظ التسعير الزمني بالإصدار ${result.version}؛ حمّلت العملية المستجيبة ${result.loaded_version}. راجع المزامنة للعمليات الأخرى.`,
        ),
      );
    } catch (caught) {
      setError(
        caught instanceof ApiError && caught.status === 409
          ? text(
              "服务器配置已变化，未覆盖新版本。请刷新并重新打开编辑。",
              "Server configuration changed; the newer version was not overwritten. Refresh and reopen the editor.",
              "تغيّرت إعدادات الخادم ولم يُستبدل الإصدار الأحدث. حدّث وأعد فتح المحرر.",
            )
          : caught instanceof ApiError && caught.status === 400
            ? text(
                "服务器拒绝了时区或时段规则，草稿已保留。请检查输入及限制。",
                "The server rejected the timezone or schedule; the draft is retained. Check values and limits.",
                "رفض الخادم المنطقة الزمنية أو الجدول واحتُفظ بالمسودة. تحقق من القيم والحدود.",
              )
            : text(
                "无法确认保存或重载结果，请刷新核对版本后再处理。",
                "The save or reload result is unconfirmed. Refresh to verify versions before proceeding.",
                "تعذر تأكيد الحفظ أو إعادة التحميل. حدّث للتحقق من الإصدارات قبل المتابعة.",
              ),
      );
    }
  };
  return (
    <section className="server-time-pricing">
      <header>
        <div>
          <h3>
            {text(
              "服务器分时倍率",
              "Server time-based multipliers",
              "معاملات الخادم حسب الوقت",
            )}
          </h3>
          <p>
            {text(
              "真实周重复时段，适用于已接入的三类计价模式。未命中时使用倍率 1；不会创建渠道、投放或密钥权限。",
              "Actual weekly windows apply to all three pricing modes. Unmatched windows use multiplier 1. Schedules do not create channels, placements or key permissions.",
              "تطبق الفترات الأسبوعية الفعلية على أنماط التسعير الثلاثة. خارج الفترات يُستخدم المعامل 1 دون إنشاء قنوات أو موارد أو صلاحيات للمفاتيح.",
            )}
          </p>
        </div>
        <div className="stp-actions">
          <Button
            disabled={query.isFetching}
            onClick={() => {
              setNotice("");
              void query.refetch();
            }}
          >
            {text("刷新时段", "Refresh schedules", "تحديث الجداول")}
          </Button>
          <Button
            disabled={
              !writable || (!!data && Object.keys(data.rules).length >= 1024)
            }
            onClick={() => begin()}
          >
            {text("登记分时时段", "Configure time schedule", "تعيين جدول زمني")}
          </Button>
        </div>
      </header>
      {query.isLoading && (
        <p role="status">
          {text(
            "正在读取服务器时段…",
            "Loading server schedules…",
            "جارٍ تحميل جداول الخادم…",
          )}
        </p>
      )}
      {query.isError && (
        <p role="alert">
          {text(
            "无法确认实际分时规则、计费启用或版本保护，已隐藏缓存并暂停编辑。",
            "Cannot verify actual schedules, billing activation or revision guards. Cached data is hidden and editing is paused.",
            "تعذر التحقق من الجداول أو تفعيل الفوترة أو حماية الإصدار. أُخفيت البيانات السابقة وأُوقف التحرير.",
          )}
        </p>
      )}
      {notice && <p role="status">{notice}</p>}
      {data && (
        <>
          <p>
            {text(
              "数据库 / 响应进程版本",
              "Database / response process version",
              "إصدار قاعدة البيانات / العملية المستجيبة",
            )}
            :{" "}
            <bdi>
              v{data.version} / v{data.loaded_version}
            </bdi>
          </p>
          <label className="stp-search">
            {text(
              "搜索分时模型",
              "Search scheduled models",
              "البحث في النماذج المجدولة",
            )}
            <input
              value={search}
              onChange={(event) => {
                setSearch(event.target.value);
                setPage(0);
              }}
            />
          </label>
          <div
            className="stp-table"
            tabIndex={0}
            role="region"
            aria-label={text(
              "分时时段表，可横向滚动",
              "Time schedule table, horizontally scrollable",
              "جدول الفترات، قابل للتمرير أفقيًا",
            )}
          >
            <table>
              <thead>
                <tr>
                  <th>{text("模型标识", "Model ID", "معرّف النموذج")}</th>
                  <th>{text("时区", "Timezone", "المنطقة الزمنية")}</th>
                  <th>{text("时段数", "Windows", "الفترات")}</th>
                  <th>{text("操作", "Action", "الإجراء")}</th>
                </tr>
              </thead>
              <tbody>
                {ids
                  .slice(currentPage * 20, (currentPage + 1) * 20)
                  .map((id) => (
                    <tr key={id} data-time-priced-model={id}>
                      <td>
                        <bdi>{id}</bdi>
                      </td>
                      <td>
                        <bdi>{data.rules[id].timezone}</bdi>
                      </td>
                      <td>{data.rules[id].windows.length}</td>
                      <td>
                        <div className="stp-actions">
                          <Button
                            size="small"
                            disabled={!writable}
                            onClick={() => begin(id)}
                          >
                            {text("编辑时段", "Edit schedule", "تحرير الجدول")}
                          </Button>
                          <Button
                            size="small"
                            disabled={!writable}
                            onClick={() => begin(id, true)}
                          >
                            {text(
                              "删除时段配置",
                              "Delete time schedule",
                              "حذف الجدول الزمني",
                            )}
                          </Button>
                        </div>
                      </td>
                    </tr>
                  ))}
              </tbody>
            </table>
          </div>
          {!ids.length && (
            <p>
              {text(
                "当前没有匹配的服务器时段，不回退演示规则。",
                "No matching server schedules; Demo rules are not used.",
                "لا توجد جداول خادم مطابقة؛ لا تُستخدم قواعد العرض التجريبي.",
              )}
            </p>
          )}
          <div className="stp-actions">
            <Button
              disabled={currentPage === 0}
              onClick={() => setPage(currentPage - 1)}
            >
              {text("上一页", "Previous", "السابق")}
            </Button>
            <span>
              {currentPage + 1} / {pages} · {ids.length}
            </span>
            <Button
              disabled={currentPage + 1 >= pages}
              onClick={() => setPage(currentPage + 1)}
            >
              {text("下一页", "Next", "التالي")}
            </Button>
          </div>
        </>
      )}
      <Modal
        className="server-pricing-dialog server-time-dialog"
        open={!!draft}
        onOpenChange={(open) => {
          if (!open && !mutation.isPending) setDraft(null);
        }}
        preventOutsideClose
        title={
          draft?.remove
            ? text(
                "删除真实分时配置",
                "Delete actual time schedule",
                "حذف الجدول الزمني الفعلي",
              )
            : text(
                "修改真实分时配置",
                "Change actual time schedule",
                "تغيير الجدول الزمني الفعلي",
              )
        }
        description={text(
          "所选模型的全部时段按一个配置版本整体保存。取消仅放弃此草稿。",
          "All windows for the selected model are saved together against one configuration version. Cancel discards this draft.",
          "تُحفظ فترات النموذج المحدد معًا وفق إصدار إعدادات واحد. يلغي الإلغاء هذه المسودة.",
        )}
        footer={
          <>
            <Button
              disabled={mutation.isPending}
              onClick={() => setDraft(null)}
            >
              {text("取消", "Cancel", "إلغاء")}
            </Button>
            <Button
              variant="primary"
              disabled={!valid || !writable || conflict || mutation.isPending}
              onClick={() => (review ? void save() : setReview(true))}
            >
              {review
                ? text(
                    "确认分时变更",
                    "Confirm time changes",
                    "تأكيد تغييرات الوقت",
                  )
                : text(
                    "预览分时变更",
                    "Review time changes",
                    "مراجعة تغييرات الوقت",
                  )}
            </Button>
          </>
        }
      >
        {draft && (
          <>
            <p>
              <bdi>{draft.id || "—"}</bdi> · v{draft.base.version}
            </p>
            {review ? (
              <div className="stp-review">
                <p>
                  {draft.remove
                    ? text(
                        "删除此模型的时段配置，后续请求恢复基础倍率 1。",
                        "Delete this model's schedule. Future requests use base multiplier 1.",
                        "حذف جدول هذا النموذج. تستخدم الطلبات القادمة المعامل الأساسي 1.",
                      )
                    : text(
                        "保存此模型的完整时段：",
                        "Save this model's complete schedule:",
                        "حفظ الجدول الكامل لهذا النموذج:",
                      )}
                </p>
                <p>
                  <bdi>{draft.schedule.timezone}</bdi> ·{" "}
                  {draft.schedule.windows.length}
                </p>
                {draft.schedule.windows.map((window) => (
                  <div className="time-window-review" key={window.id}>
                    <bdi>{window.id}</bdi>
                    <p>{window.label}</p>
                    <p>{window.days.map((day) => weekdays[day]).join(" · ")}</p>
                    <p>
                      <bdi>
                        {window.start} → {window.end} · {window.multiplier}×
                      </bdi>
                    </p>
                  </div>
                ))}
                <p>
                  {text(
                    "跨午夜时段归开始日，结束时刻不包含在内；未命中时段使用倍率 1。按当地实际时刻匹配：夏令时重复分钟均命中，跳过的分钟不会补造。新请求在准入时冻结价格，既有请求与历史账单不重写；其他模型、基础价格和权限保持原值。",
                    "Overnight windows belong to their start weekday; ends are exclusive and unmatched windows use multiplier 1. Local wall-clock matching includes both repeated DST minutes and invents no skipped minutes. New requests capture pricing at admission; existing requests and historical bills are not rewritten. Other models, base prices and permissions retain their values.",
                    "تُنسب فترات منتصف الليل إلى يوم البداية وتُستثنى النهاية؛ خارج الفترات يُستخدم المعامل 1. تطابق الدقائق المحلية المتكررة بالتوقيت الصيفي دون اختراع الدقائق المحذوفة. تلتقط الطلبات الجديدة السعر عند القبول؛ لا تتغير الطلبات الحالية أو الفواتير السابقة أو النماذج والأسعار والصلاحيات الأخرى.",
                  )}
                </p>
                {!draft.remove && (
                  <Button
                    onClick={() => {
                      setReview(false);
                      setError("");
                    }}
                  >
                    {text("返回编辑", "Back to edit", "العودة للتحرير")}
                  </Button>
                )}
              </div>
            ) : (
              <div className="stp-fields">
                <label>
                  {text(
                    "分时模型标识",
                    "Scheduled model ID",
                    "معرّف النموذج المجدول",
                  )}
                  <input
                    dir="ltr"
                    readOnly={draft.locked}
                    maxLength={128}
                    value={draft.id}
                    onChange={(event) =>
                      setDraft({ ...draft, id: event.target.value })
                    }
                  />
                </label>
                <label>
                  {text(
                    "IANA 时区或 UTC",
                    "IANA timezone or UTC",
                    "منطقة IANA أو UTC",
                  )}
                  <input
                    dir="ltr"
                    maxLength={64}
                    value={draft.schedule.timezone}
                    onChange={(event) =>
                      setDraft({
                        ...draft,
                        schedule: {
                          ...draft.schedule,
                          timezone: event.target.value,
                        },
                      })
                    }
                  />
                </label>
                {draft.schedule.windows.map((window, index) => (
                  <fieldset
                    className="time-window-fields"
                    key={index}
                    aria-label={text(
                      `时段 ${index + 1}`,
                      `Window ${index + 1}`,
                      `الفترة ${index + 1}`,
                    )}
                  >
                    <legend>
                      {text(
                        `时段 ${index + 1}`,
                        `Window ${index + 1}`,
                        `الفترة ${index + 1}`,
                      )}
                    </legend>
                    <label>
                      {text("时段标识", "Window ID", "معرّف الفترة")}
                      <input
                        dir="ltr"
                        maxLength={64}
                        value={window.id}
                        onChange={(event) =>
                          updateWindow(index, { id: event.target.value })
                        }
                      />
                    </label>
                    <label>
                      {text("时段名称", "Window label", "اسم الفترة")}
                      <input
                        maxLength={128}
                        value={window.label}
                        onChange={(event) =>
                          updateWindow(index, { label: event.target.value })
                        }
                      />
                    </label>
                    <div className="time-window-days">
                      {[1, 2, 3, 4, 5, 6, 0].map((day) => (
                        <label key={day}>
                          <input
                            type="checkbox"
                            checked={window.days.includes(day)}
                            onChange={(event) =>
                              updateWindow(index, {
                                days: event.target.checked
                                  ? [...window.days, day]
                                  : window.days.filter(
                                      (selected) => selected !== day,
                                    ),
                              })
                            }
                          />
                          {weekdays[day]}
                        </label>
                      ))}
                    </div>
                    <label>
                      {text("开始 HH:MM", "Start HH:MM", "البداية HH:MM")}
                      <input
                        dir="ltr"
                        maxLength={5}
                        value={window.start}
                        onChange={(event) =>
                          updateWindow(index, { start: event.target.value })
                        }
                      />
                    </label>
                    <label>
                      {text(
                        "结束 HH:MM（可填 24:00）",
                        "End HH:MM (24:00 allowed)",
                        "النهاية HH:MM (يُسمح بـ 24:00)",
                      )}
                      <input
                        dir="ltr"
                        maxLength={5}
                        value={window.end}
                        onChange={(event) =>
                          updateWindow(index, { end: event.target.value })
                        }
                      />
                    </label>
                    <label>
                      {text("时段倍率", "Window multiplier", "معامل الفترة")}
                      <input
                        dir="ltr"
                        inputMode="decimal"
                        maxLength={32}
                        value={window.multiplier}
                        onChange={(event) =>
                          updateWindow(index, {
                            multiplier: event.target.value,
                          })
                        }
                      />
                    </label>
                    <Button
                      onClick={() =>
                        setDraft({
                          ...draft,
                          schedule: {
                            ...draft.schedule,
                            windows: draft.schedule.windows.filter(
                              (_, i) => i !== index,
                            ),
                          },
                        })
                      }
                    >
                      {text(
                        "移除此时段",
                        "Remove this window",
                        "إزالة هذه الفترة",
                      )}
                    </Button>
                  </fieldset>
                ))}
                <Button
                  disabled={draft.schedule.windows.length >= 32}
                  onClick={() =>
                    setDraft({
                      ...draft,
                      schedule: {
                        ...draft.schedule,
                        windows: [
                          ...draft.schedule.windows,
                          {
                            id: "",
                            label: "",
                            days: [],
                            start: "00:00",
                            end: "24:00",
                            multiplier: "1",
                          },
                        ],
                      },
                    })
                  }
                >
                  {text("添加时段", "Add window", "إضافة فترة")}
                </Button>
                <small>
                  {text(
                    "每个模型最多 32 个互不重叠的周时段，倍率大于 0 且不超过 100。所有时段共享明确时区；此版本仅支持倍率，不支持独立时段词元价格。时区是否受支持由服务器最终校验。",
                    "At most 32 non-overlapping weekly windows per model, with multiplier greater than 0 and at most 100. All windows share an explicit timezone. This version supports multipliers, not per-window token-price overrides. The server validates timezone support.",
                    "حتى 32 فترة أسبوعية غير متداخلة لكل نموذج، بمعامل أكبر من صفر وحتى 100. تشترك الفترات في منطقة زمنية صريحة. يدعم هذا الإصدار المعاملات دون أسعار رموز مستقلة لكل فترة. يتحقق الخادم من دعم المنطقة الزمنية.",
                  )}
                </small>
                {issue && <p role="alert">{issueText[issue]}</p>}
              </div>
            )}
            {conflict && (
              <p role="alert">
                {text(
                  "配置版本变化或读取失败，请刷新并重新打开编辑。",
                  "Configuration changed or could not be read. Refresh and reopen the editor.",
                  "تغيّر الإعداد أو تعذرت قراءته. حدّث وأعد فتح المحرر.",
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

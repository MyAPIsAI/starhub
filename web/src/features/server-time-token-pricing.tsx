import { useState } from "react";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import { ApiError } from "../lib/api/http";
import {
  useServerTokenTimePricing,
  useSaveServerTokenTimePricing,
} from "../lib/api/time-token-pricing";
import { isAdmin, useSessionQueryIdentity } from "../lib/system-access";
import { validTimeModelId } from "../lib/server-time-pricing";
import {
  emptyTokenTimeWindow,
  replaceTokenTimeSchedule,
  tokenPriceKeys,
  tokenTimeScheduleError,
  type ServerTokenTimePricing,
  type TokenPriceKey,
  type TokenTimeSchedule,
  type TokenTimeWindow,
} from "../lib/server-time-token-pricing";
import { useSettingsText } from "./settings-text";
import "./server-time-pricing.css";
import "./server-time-token-pricing.css";

type Draft = {
  base: ServerTokenTimePricing;
  id: string;
  locked: boolean;
  remove: boolean;
  schedule: TokenTimeSchedule;
};

export function ServerTokenTimePricingEditor() {
  const scope = useSessionQueryIdentity();
  return <TokenTimeEditor key={JSON.stringify(scope)} />;
}

function TokenTimeEditor() {
  const { text, lang } = useSettingsText(),
    scope = useSessionQueryIdentity();
  const allowed = scope[1] !== null && isAdmin(scope[3]);
  const query = useServerTokenTimePricing(allowed),
    mutation = useSaveServerTokenTimePricing();
  const [draft, setDraft] = useState<Draft | null>(null),
    [review, setReview] = useState(false),
    [attempted, setAttempted] = useState(false),
    [error, setError] = useState(""),
    [notice, setNotice] = useState(""),
    [search, setSearch] = useState(""),
    [page, setPage] = useState(0);
  if (!allowed) return null;
  const data = query.isError ? undefined : query.data;
  const writable = !!data && data.version < Number.MAX_SAFE_INTEGER;
  const ids = data
    ? Object.keys(data.rules)
        .sort()
        .filter((id) => id.toLowerCase().includes(search.trim().toLowerCase()))
    : [];
  const pages = Math.max(1, Math.ceil(ids.length / 20)),
    currentPage = Math.min(page, pages - 1);
  const conflict = !!draft && (!data || draft.base.version !== data.version);
  const issue =
    draft && !draft.remove ? tokenTimeScheduleError(draft.schedule) : null;
  const valid =
    !!draft &&
    (draft.remove ||
      (validTimeModelId(draft.id) &&
        !["myapis-auto", "veyra-auto"].includes(draft.id) &&
        !issue &&
        (draft.locked || !Object.hasOwn(draft.base.rules, draft.id))));
  const prices: Record<TokenPriceKey, string> = {
    input_per_m: text("文字输入", "Text input", "إدخال نصي"),
    output_per_m: text("文字输出", "Text output", "إخراج نصي"),
    cache_read_per_m: text("缓存读取", "Cache read", "قراءة الذاكرة المؤقتة"),
    cache_write_per_m: text("缓存写入", "Cache write", "كتابة الذاكرة المؤقتة"),
    audio_input_per_m: text("音频输入", "Audio input", "إدخال صوتي"),
    audio_output_per_m: text("音频输出", "Audio output", "إخراج صوتي"),
  };
  const weekdays = Array.from({ length: 7 }, (_, day) =>
    new Intl.DateTimeFormat(lang, { weekday: "long", timeZone: "UTC" }).format(
      new Date(Date.UTC(2026, 0, 4 + day)),
    ),
  );
  const begin = (id = "", remove = false) => {
    if (!data) return;
    mutation.reset();
    setAttempted(false);
    setError("");
    setNotice("");
    setReview(remove);
    setDraft({
      base: data,
      id,
      locked: !!id,
      remove,
      schedule: Object.hasOwn(data.rules, id)
        ? structuredClone(data.rules[id])
        : { timezone: "UTC", windows: [emptyTokenTimeWindow()] },
    });
  };
  const updateWindow = (index: number, changes: Partial<TokenTimeWindow>) => {
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
    if (
      !draft ||
      !review ||
      !valid ||
      conflict ||
      !writable ||
      mutation.isPending ||
      attempted
    )
      return;
    let rules;
    try {
      rules = replaceTokenTimeSchedule(
        draft.base,
        draft.id,
        draft.remove ? null : draft.schedule,
      );
    } catch {
      setError(
        text(
          "草稿超出限制或包含无效字段，尚未提交。",
          "The draft exceeds limits or contains invalid fields; nothing was submitted.",
          "تتجاوز المسودة الحدود أو تحتوي حقولًا غير صالحة؛ لم تُرسل.",
        ),
      );
      return;
    }
    setAttempted(true);
    setError("");
    try {
      const result = await mutation.mutateAsync({
        expected_version: draft.base.version,
        rules,
      });
      setDraft(null);
      setNotice(
        text(
          `独立单价已保存 v${result.version}；响应进程加载 v${result.loaded_version}。`,
          `Independent prices saved at v${result.version}; response process loaded v${result.loaded_version}.`,
          `حُفظت الأسعار المستقلة بالإصدار ${result.version}؛ حمّلت العملية المستجيبة ${result.loaded_version}.`,
        ),
      );
    } catch (caught) {
      setError(
        caught instanceof ApiError && caught.status === 409
          ? text(
              "配置已变化，未覆盖新版本。请刷新并重新打开，草稿暂保留。",
              "Configuration changed; the newer version was not overwritten. Refresh and reopen; the draft is retained.",
              "تغيّرت الإعدادات ولم يُستبدل الإصدار الأحدث. حدّث وأعد الفتح؛ احتُفظ بالمسودة.",
            )
          : caught instanceof ApiError && caught.status === 400
            ? text(
                "服务器拒绝了规则。检查时区、完整单价及模型是否按词元计费，刷新后重新编辑。",
                "The server rejected the rules. Check timezone, complete prices and token billing mode; refresh before editing again.",
                "رفض الخادم القواعد. تحقق من المنطقة والأسعار الكاملة ونمط فوترة الرموز؛ حدّث قبل إعادة التحرير.",
              )
            : text(
                "提交结果未确认，草稿已保留。请先刷新核对服务器配置，不要直接重复提交。",
                "The submission outcome is unconfirmed; the draft is retained. Refresh and inspect server configuration before submitting again.",
                "لم تتأكد نتيجة الإرسال واحتُفظ بالمسودة. حدّث وتحقق من إعدادات الخادم قبل إعادة الإرسال.",
              ),
      );
    }
  };
  const priceReview = (window: TokenTimeWindow) => (
    <div className="token-window-prices">
      {tokenPriceKeys.map((key) => (
        <p key={key}>
          <span>{prices[key]}</span>
          <bdi>${window.prices[key]}</bdi>
        </p>
      ))}
    </div>
  );
  return (
    <section
      className="server-time-pricing server-time-token-pricing"
      aria-label={text(
        "服务器独立时段单价",
        "Server independent window prices",
        "أسعار فترات الخادم المستقلة",
      )}
    >
      <header>
        <div>
          <h3>
            {text(
              "服务器独立时段单价",
              "Server independent window prices",
              "أسعار فترات الخادم المستقلة",
            )}
          </h3>
          <p>
            {text(
              "每个时段明确填写六类词元单价，单位为美元/百万词元。仅支持按词元计费的同步请求；现有分组与分时倍率继续叠加，异步任务暂不支持。",
              "Set all six token prices per window in USD per million tokens. Supports synchronous token billing; existing group and time multipliers still apply. Asynchronous tasks are not supported.",
              "حدد أسعار فئات الرموز الست لكل فترة بالدولار لكل مليون رمز. تدعم الطلبات المتزامنة بفوترة الرموز مع معاملات المجموعة والوقت الحالية؛ لا تُدعم المهام غير المتزامنة.",
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
            {text(
              "刷新独立单价",
              "Refresh independent prices",
              "تحديث الأسعار المستقلة",
            )}
          </Button>
          <Button
            disabled={
              !writable || (!!data && Object.keys(data.rules).length >= 1024)
            }
            onClick={() => begin()}
          >
            {text(
              "登记独立单价",
              "Configure independent prices",
              "تعيين أسعار مستقلة",
            )}
          </Button>
        </div>
      </header>
      {query.isLoading && (
        <p role="status">
          {text(
            "正在读取独立单价…",
            "Loading independent prices…",
            "جارٍ تحميل الأسعار المستقلة…",
          )}
        </p>
      )}
      {query.isError && (
        <p role="alert">
          {text(
            "无法确认服务器独立单价、计费能力或版本保护，已隐藏缓存并暂停编辑。",
            "Cannot verify server independent prices, billing capabilities or revision guards. Cached data is hidden and editing is paused.",
            "تعذر التحقق من الأسعار أو إمكانات الفوترة أو حماية الإصدار. أُخفيت البيانات السابقة وأُوقف التحرير.",
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
              "搜索独立单价模型",
              "Search independent-price models",
              "البحث في نماذج الأسعار المستقلة",
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
              "独立单价时段表，可横向滚动",
              "Independent price schedule table, horizontally scrollable",
              "جدول فترات الأسعار المستقلة، قابل للتمرير أفقيًا",
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
                    <tr key={id} data-token-time-model={id}>
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
                            {text(
                              "编辑独立单价",
                              "Edit independent prices",
                              "تحرير الأسعار المستقلة",
                            )}
                          </Button>
                          <Button
                            size="small"
                            disabled={!writable}
                            onClick={() => begin(id, true)}
                          >
                            {text(
                              "删除独立单价配置",
                              "Delete independent schedule",
                              "حذف جدول الأسعار المستقلة",
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
                "当前没有匹配的服务器独立单价配置。",
                "No matching independent server price schedules.",
                "لا توجد جداول أسعار خادم مستقلة مطابقة.",
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
        className="server-pricing-dialog server-time-dialog server-token-time-dialog"
        open={!!draft}
        preventOutsideClose
        onOpenChange={(open) => {
          if (!open && !mutation.isPending) setDraft(null);
        }}
        title={
          draft?.remove
            ? text(
                "删除独立单价配置",
                "Delete independent schedule",
                "حذف جدول الأسعار المستقلة",
              )
            : text(
                "修改独立时段单价",
                "Change independent window prices",
                "تغيير أسعار الفترات المستقلة",
              )
        }
        description={text(
          "按当前配置版本整体保存所选模型的全部独立时段；取消放弃此草稿。",
          "Save all independent windows for the selected model against the captured configuration version. Cancel discards this draft.",
          "احفظ جميع فترات النموذج المستقلة وفق إصدار الإعدادات الملتقط. يلغي الإلغاء هذه المسودة.",
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
              disabled={
                !valid ||
                !writable ||
                conflict ||
                mutation.isPending ||
                attempted
              }
              onClick={() => (review ? void save() : setReview(true))}
            >
              {review
                ? text(
                    "确认独立单价变更",
                    "Confirm independent price changes",
                    "تأكيد تغييرات الأسعار المستقلة",
                  )
                : text(
                    "预览独立单价变更",
                    "Review independent price changes",
                    "مراجعة تغييرات الأسعار المستقلة",
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
                        "删除后，后续请求使用基础词元单价；既有分时倍率继续适用。",
                        "After deletion, future requests use base token prices; existing time multipliers still apply.",
                        "بعد الحذف تستخدم الطلبات القادمة أسعار الرموز الأساسية مع استمرار معاملات الوقت الحالية.",
                      )
                    : text(
                        "保存以下完整独立时段单价，美元/百万词元：",
                        "Save these complete independent window prices, USD per million tokens:",
                        "حفظ أسعار الفترات المستقلة الكاملة التالية، بالدولار لكل مليون رمز:",
                      )}
                </p>
                <p>
                  <bdi>{draft.schedule.timezone}</bdi> ·{" "}
                  {draft.schedule.windows.length}
                </p>
                {draft.schedule.windows.map((window, index) => (
                  <div className="time-window-review" key={index}>
                    <bdi>{window.id}</bdi>
                    <p>{window.label}</p>
                    <p>{window.days.map((day) => weekdays[day]).join(" · ")}</p>
                    <bdi>
                      {window.start} → {window.end}
                    </bdi>
                    {priceReview(window)}
                  </div>
                ))}
                <p>
                  {text(
                    "跨午夜归开始日，结束时刻不含在内；按当地时刻匹配夏令时。未命中使用基础单价。新请求准入时冻结价格，历史账单不回算。零单价表示该类用量免费；仍需最少 1 配额的预扣款，结算时退还。",
                    "Overnight windows belong to their start weekday; ends are exclusive and local clock time handles DST. Outside windows, base prices apply. Pricing freezes at request admission; historical bills are not recalculated. A zero price makes that usage class free; the minimum one-quota reservation is refunded at settlement.",
                    "تُنسب فترات منتصف الليل إلى يوم البداية وتُستثنى النهاية ويُطابق الوقت المحلي مع التوقيت الصيفي. خارج الفترات تُطبق الأسعار الأساسية. تُثبت الأسعار عند قبول الطلب دون إعادة حساب الفواتير السابقة. السعر الصفري يجعل الفئة مجانية؛ يُرد حجز وحدة الحصة الدنيا عند التسوية.",
                  )}
                </p>
                {!draft.remove && (
                  <Button
                    disabled={mutation.isPending || attempted}
                    onClick={() => setReview(false)}
                  >
                    {text("返回编辑", "Back to edit", "العودة للتحرير")}
                  </Button>
                )}
              </div>
            ) : (
              <div className="stp-fields">
                <label>
                  {text(
                    "独立单价模型标识",
                    "Independent-price model ID",
                    "معرّف نموذج السعر المستقل",
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
                      `独立单价时段 ${index + 1}`,
                      `Independent price window ${index + 1}`,
                      `فترة سعر مستقلة ${index + 1}`,
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
                      {weekdays.map((day, number) => (
                        <label key={number}>
                          <input
                            type="checkbox"
                            checked={window.days.includes(number)}
                            onChange={(event) =>
                              updateWindow(index, {
                                days: event.target.checked
                                  ? [...window.days, number].sort()
                                  : window.days.filter((day) => day !== number),
                              })
                            }
                          />
                          {day}
                        </label>
                      ))}
                    </div>
                    <div className="token-window-times">
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
                          "结束 HH:MM，可填 24:00",
                          "End HH:MM; 24:00 allowed",
                          "النهاية HH:MM؛ تُقبل 24:00",
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
                    </div>
                    <div className="token-window-inputs">
                      {tokenPriceKeys.map((key) => (
                        <label key={key}>
                          {prices[key]} ·{" "}
                          {text(
                            "美元/百万词元",
                            "USD / million tokens",
                            "دولار / مليون رمز",
                          )}
                          <input
                            dir="ltr"
                            inputMode="decimal"
                            maxLength={17}
                            value={window.prices[key]}
                            onChange={(event) =>
                              updateWindow(index, {
                                prices: {
                                  ...window.prices,
                                  [key]: event.target.value,
                                },
                              })
                            }
                          />
                        </label>
                      ))}
                    </div>
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
                          emptyTokenTimeWindow(),
                        ],
                      },
                    })
                  }
                >
                  {text("添加时段", "Add window", "إضافة فترة")}
                </Button>
                <small>
                  {text(
                    "每个模型最多 32 个不重叠周时段，结束可为 24:00。六类价格均须明确填写，可为 0，最多 9 位小数且不超过 1,000,000 美元/百万词元；不自动继承基础价格。服务器最终校验时区和计费模式。",
                    "At most 32 non-overlapping weekly windows per model; 24:00 is allowed as an end. All six prices are required, allow zero, and support up to 9 decimals and 1,000,000 USD per million tokens. Base prices are not inherited. The server validates timezone and billing mode.",
                    "حتى 32 فترة أسبوعية غير متداخلة لكل نموذج؛ تُقبل 24:00 للنهاية. الأسعار الست مطلوبة وتقبل الصفر وحتى 9 منازل عشرية و1,000,000 دولار لكل مليون رمز، دون وراثة الأسعار الأساسية. يتحقق الخادم من المنطقة الزمنية ونمط الفوترة.",
                  )}
                </small>
                {issue && (
                  <p role="alert">
                    {issue === "prices"
                      ? text(
                          "请填写全部六类有效单价。",
                          "Enter all six valid prices.",
                          "أدخل الأسعار الست الصالحة.",
                        )
                      : issue === "overlap"
                        ? text(
                            "时段重叠，请检查星期和跨午夜范围。",
                            "Windows overlap; check weekdays and overnight ranges.",
                            "الفترات متداخلة؛ تحقق من الأيام وامتداد منتصف الليل.",
                          )
                        : text(
                            "请检查时区、唯一标识、名称、星期及起止时间，并遵守时段数量限制。",
                            "Check timezone, unique IDs, labels, weekdays and start/end times, and stay within the window limit.",
                            "تحقق من المنطقة والمعرفات الفريدة والأسماء والأيام وأوقات البداية والنهاية وحدود عدد الفترات.",
                          )}
                  </p>
                )}
              </div>
            )}
            {conflict && (
              <p role="alert">
                {text(
                  "配置已变化或读取失败，请刷新并重新打开编辑。",
                  "Configuration changed or could not be read. Refresh and reopen the editor.",
                  "تغيّرت الإعدادات أو تعذرت قراءتها. حدّث وأعد فتح المحرر.",
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

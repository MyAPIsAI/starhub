import { useState } from "react";
import { Link } from "react-router-dom";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import { ApiError } from "../lib/api/http";
import {
  useServerPricing,
  useSaveServerPricing,
} from "../lib/api/pricing-options";
import { isAdmin, useSystemAccess } from "../lib/system-access";
import {
  ownPrice,
  serverPricingMode,
  validServerRatio,
  validServerRequestPrice,
  validServerExpression,
  replaceServerModelPricing,
  type ServerPricing,
} from "../lib/server-pricing";
import { useSettingsText } from "./settings-text";
import "./server-token-pricing.css";
import { ServerDetailPricing } from "./server-detail-pricing";

type Draft = {
  base: ServerPricing;
  id: string;
  locked: boolean;
  input: string;
  completion: string;
  kind: ReturnType<typeof serverPricingMode>;
  request: string;
  expression: string;
};
export function ServerTokenPricing() {
  const { text } = useSettingsText();
  const role = useSystemAccess((s) => s.role),
    authenticated = useSystemAccess((s) => s.session.accountId !== null);
  const allowed = authenticated && isAdmin(role),
    query = useServerPricing(allowed),
    mutation = useSaveServerPricing();
  const [search, setSearch] = useState(""),
    [page, setPage] = useState(0),
    [draft, setDraft] = useState<Draft | null>(null),
    [review, setReview] = useState(false),
    [error, setError] = useState(""),
    [notice, setNotice] = useState("");
  if (!allowed) return null;
  const data = query.isError ? undefined : query.data;
  const ids = data
    ? [
        ...new Set([
          ...Object.keys(data.input),
          ...Object.keys(data.completion),
          ...Object.keys(data.perRequest),
          ...Object.keys(data.expressions),
        ]),
      ]
        .sort()
        .filter((id) => id.toLowerCase().includes(search.trim().toLowerCase()))
    : [];
  const pages = Math.max(1, Math.ceil(ids.length / 20)),
    currentPage = Math.min(page, pages - 1);
  const writable = !!data && data.version <= Number.MAX_SAFE_INTEGER - 4;
  const conflict = !!draft && (!data || data.version !== draft.base.version);
  const valid =
    !!draft &&
    draft.id === draft.id.trim() &&
    draft.id.length > 0 &&
    draft.id.length <= 128 &&
    !/[\u0000-\u001f\u007f]/.test(draft.id) &&
    !["myapis-auto", "veyra-auto"].includes(draft.id) &&
    (draft.locked ||
      ![
        draft.base.input,
        draft.base.completion,
        draft.base.perRequest,
        draft.base.expressions,
      ].some(
        (values) => ownPrice<string | number>(values, draft.id) !== undefined,
      )) &&
    (draft.kind === "expression"
      ? validServerExpression(draft.expression)
      : draft.kind === "per_request"
        ? validServerRequestPrice(draft.request)
        : validServerRatio(draft.input) && validServerRatio(draft.completion));
  const mode = (state: ServerPricing, id: string) =>
    ({
      per_token: text("按词元", "Per token", "لكل رمز"),
      per_request: text("按次", "Per request", "لكل طلب"),
      expression: text("表达式", "Expression", "تعبير"),
    })[serverPricingMode(state, id)];
  const begin = (id = "") => {
    if (!data) return;
    mutation.reset();
    setError("");
    setNotice("");
    setReview(false);
    setDraft({
      base: data,
      id,
      locked: !!id,
      input: ownPrice(data.input, id)?.toString() ?? "",
      completion: ownPrice(data.completion, id)?.toString() ?? "",
      kind: serverPricingMode(data, id),
      request: ownPrice(data.perRequest, id)?.toString() ?? "",
      expression: ownPrice(data.expressions, id) ?? "",
    });
  };
  const save = async () => {
    if (!draft || !review || !valid || conflict || !writable) return;
    try {
      const options = replaceServerModelPricing(
        draft.base,
        draft.id,
        draft.kind === "expression"
          ? { mode: draft.kind, expression: draft.expression }
          : draft.kind === "per_request"
            ? { mode: draft.kind, price: draft.request }
            : {
                mode: draft.kind,
                input: draft.input,
                completion: draft.completion,
              },
      );
      const result = await mutation.mutateAsync({
        expected_version: draft.base.version,
        options,
      });
      if (
        result.version < draft.base.version ||
        result.version > draft.base.version + options.length
      )
        throw Error("Unexpected pricing revision");
      setDraft(null);
      setNotice(
        text(
          `已保存服务器定价 v${result.version}；响应进程加载版本 v${result.loaded_version}。其他实例请查看配置同步。`,
          `Server pricing saved at v${result.version}; response process loaded v${result.loaded_version}. Check configuration sync for other instances.`,
          `حُفظ تسعير الخادم بالإصدار ${result.version}؛ حمّلت العملية المستجيبة الإصدار ${result.loaded_version}. راجع المزامنة للعمليات الأخرى.`,
        ),
      );
    } catch (caught) {
      setError(
        caught instanceof ApiError && caught.status === 409
          ? text(
              "服务器配置已变化，未覆盖新版本。请刷新并重新打开编辑。",
              "Server configuration changed; the new version was not overwritten. Refresh and reopen the editor.",
              "تغيّرت إعدادات الخادم ولم يُستبدل الإصدار الجديد. حدّث وأعد فتح المحرر.",
            )
          : caught instanceof ApiError && caught.status === 400
            ? text(
                "服务器拒绝了此配置，草稿已保留。请检查输入及配置限制。",
                "The server rejected this configuration; the draft is retained. Check values and configuration limits.",
                "رفض الخادم هذا الإعداد واحتُفظ بالمسودة. تحقق من القيم وحدود الإعداد.",
              )
            : text(
                "无法确认保存或重载结果。请刷新核对数据库与进程版本，勿直接重复提交旧草稿。",
                "The save or reload result is unconfirmed. Refresh to check database and process versions before submitting again.",
                "تعذر تأكيد الحفظ أو إعادة التحميل. حدّث للتحقق من إصداري قاعدة البيانات والعملية قبل الإرسال مجددًا.",
              ),
      );
    }
  };
  return (
    <section className="server-token-pricing">
      <header>
        <div>
          <h2>
            {text(
              "服务器模型定价",
              "Server model pricing",
              "تسعير نماذج الخادم",
            )}
          </h2>
          <p>
            {text(
              "这里读取并修改真实服务器计费配置，与浏览器演示价格独立。输入基准单位为美元/百万词元；输出倍率相对普通输入。分组、缓存、音频及其他计费规则仍会影响费用。",
              "Read and edit actual server billing configuration, independently of browser Demo prices. Input base is USD per million tokens; output ratio is relative to ordinary input. Group, cache, audio and other billing rules still affect charges.",
              "اقرأ إعدادات فوترة الخادم الفعلية وعدّلها مستقلة عن أسعار العرض التجريبي. أساس الإدخال بالدولار لكل مليون رمز ومعامل الإخراج نسبة إلى الإدخال العادي. تظل قواعد المجموعة والذاكرة والصوت وغيرها مؤثرة في الرسوم.",
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
            {text("刷新定价", "Refresh pricing", "تحديث التسعير")}
          </Button>
          <Button disabled={!writable} onClick={() => begin()}>
            {text(
              "登记模型定价",
              "Configure model pricing",
              "تعيين تسعير نموذج",
            )}
          </Button>
        </div>
      </header>
      <p>
        {text(
          "可登记、编辑或明确切换按词元、按次及表达式计价。所选模型的模式与价格同事务保存；变更不会创建渠道、投放资源或授予密钥权限。完整分时规则管理仍待实现。",
          "Configure, edit or explicitly switch token, per-request and expression pricing. The selected model's mode and prices are saved atomically; changes do not create channels, publish resources or grant key permissions. Full time-rule management remains pending.",
          "اضبط أو عدّل أو بدّل صراحةً التسعير لكل رمز أو طلب أو بالتعبير. يُحفظ نمط النموذج وأسعاره ذريًا دون إنشاء قنوات أو نشر موارد أو منح صلاحيات للمفاتيح. إدارة قواعد الوقت الكاملة قيد التنفيذ.",
        )}
      </p>
      {query.isLoading && (
        <p role="status">
          {text(
            "正在读取服务器定价…",
            "Loading server pricing…",
            "جارٍ تحميل تسعير الخادم…",
          )}
        </p>
      )}
      {query.isError && (
        <p role="alert">
          {text(
            "无法确认服务器快照或原子版本保护，已隐藏缓存并暂停编辑。",
            "Cannot verify the server snapshot or atomic version guard. Cached data is hidden and editing is paused.",
            "تعذر التحقق من لقطة الخادم أو حماية الإصدار الذرية. أُخفيت البيانات السابقة وأُوقف التحرير.",
          )}
        </p>
      )}
      {notice && <p role="status">{notice}</p>}
      <ServerDetailPricing data={data} />
      {data && (
        <>
          <p className="stp-versions">
            {text("数据库版本", "Database version", "إصدار قاعدة البيانات")}:{" "}
            <bdi>v{data.version}</bdi> ·{" "}
            {text(
              "响应进程加载版本",
              "Response process loaded version",
              "الإصدار المحمّل في العملية المستجيبة",
            )}
            : <bdi>v{data.loaded_version}</bdi> ·{" "}
            <Link to="/admin?tab=resources">
              {text("配置同步", "Configuration sync", "مزامنة الإعدادات")}
            </Link>
          </p>
          <label className="stp-search">
            {text(
              "搜索已配置模型",
              "Search configured models",
              "البحث في النماذج المضبوطة",
            )}
            <input
              value={search}
              onChange={(e) => {
                setSearch(e.target.value);
                setPage(0);
              }}
            />
          </label>
          <div
            className="stp-table"
            tabIndex={0}
            role="region"
            aria-label={text(
              "服务器定价表，可横向滚动",
              "Server pricing table, horizontally scrollable",
              "جدول تسعير الخادم، قابل للتمرير أفقيًا",
            )}
          >
            <table>
              <thead>
                <tr>
                  <th>{text("模型标识", "Model ID", "معرّف النموذج")}</th>
                  <th>{text("计价方式", "Pricing mode", "نمط التسعير")}</th>
                  <th>
                    {text(
                      "输入基准 USD/百万",
                      "Input base USD/M",
                      "أساس الإدخال دولار/مليون",
                    )}
                  </th>
                  <th>
                    {text(
                      "输出/输入倍率",
                      "Output/input ratio",
                      "معامل الإخراج/الإدخال",
                    )}
                  </th>
                  <th>
                    {text(
                      "按次 USD/请求",
                      "Per request USD/request",
                      "لكل طلب دولار/طلب",
                    )}
                  </th>
                  <th>{text("操作", "Action", "الإجراء")}</th>
                </tr>
              </thead>
              <tbody>
                {ids
                  .slice(currentPage * 20, (currentPage + 1) * 20)
                  .map((id) => (
                    <tr key={id} data-server-priced-model={id}>
                      <td>
                        <bdi>{id}</bdi>
                      </td>
                      <td>{mode(data, id)}</td>
                      <td>
                        <bdi>
                          {serverPricingMode(data, id) === "per_token"
                            ? (ownPrice(data.input, id) ?? "—")
                            : "—"}
                        </bdi>
                      </td>
                      <td>
                        <bdi>
                          {serverPricingMode(data, id) === "per_token"
                            ? (ownPrice(data.completion, id) ?? "—")
                            : "—"}
                        </bdi>
                      </td>
                      <td>
                        <bdi>
                          {serverPricingMode(data, id) === "per_request"
                            ? ownPrice(data.perRequest, id)
                            : "—"}
                        </bdi>
                      </td>
                      <td>
                        <Button
                          size="small"
                          disabled={!writable}
                          onClick={() => begin(id)}
                        >
                          {serverPricingMode(data, id) === "expression"
                            ? text(
                                "编辑计费表达式",
                                "Edit billing expression",
                                "تحرير تعبير الفوترة",
                              )
                            : serverPricingMode(data, id) === "per_request"
                              ? text(
                                  "编辑按次价格",
                                  "Edit request price",
                                  "تحرير سعر الطلب",
                                )
                              : text(
                                  "编辑服务器倍率",
                                  "Edit server ratios",
                                  "تحرير معاملات الخادم",
                                )}
                        </Button>
                      </td>
                    </tr>
                  ))}
              </tbody>
            </table>
          </div>
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
        className="server-pricing-dialog"
        open={!!draft}
        onOpenChange={(open) => {
          if (!open && !mutation.isPending) setDraft(null);
        }}
        preventOutsideClose
        title={
          draft?.kind === "expression"
            ? text(
                "修改真实计费表达式",
                "Change actual billing expression",
                "تغيير تعبير الفوترة الفعلي",
              )
            : draft?.kind === "per_request"
              ? text(
                  "修改真实按次价格",
                  "Change actual request price",
                  "تغيير سعر الطلب الفعلي",
                )
              : text(
                  "修改真实服务器倍率",
                  "Change actual server ratios",
                  "تغيير معاملات الخادم الفعلية",
                )
        }
        description={
          draft?.kind === "expression"
            ? text(
                "表达式求值结果为基础美元金额，分组等倍率仍适用。后端校验语法后保存，取消仅放弃草稿。",
                "The expression result is a base USD amount; group and other multipliers still apply. The server validates syntax before saving. Cancel discards this draft.",
                "نتيجة التعبير مبلغ أساسي بالدولار وتظل معاملات المجموعة وغيرها مطبقة. يتحقق الخادم من الصياغة قبل الحفظ. يلغي الإلغاء هذه المسودة.",
              )
            : draft?.kind === "per_request"
              ? text(
                  "按次价格使用美元/请求，模式与价格变更整体保存。取消仅放弃此草稿。",
                  "Per-request prices use USD/request. Mode and price changes are saved together. Cancel discards this draft.",
                  "الأسعار بالدولار لكل طلب. يُحفظ تغيير النمط والسعر معًا. يلغي الإلغاء هذه المسودة.",
                )
              : text(
                  "输入和输出倍率按同一配置版本整体保存。取消仅放弃此草稿。",
                  "Input and output ratios are saved together against one configuration version. Cancel discards this draft.",
                  "يُحفظ معاملا الإدخال والإخراج معًا وفق إصدار إعدادات واحد. يلغي الإلغاء هذه المسودة.",
                )
        }
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
              disabled={!valid || conflict || !writable || mutation.isPending}
              onClick={() => (review ? void save() : setReview(true))}
            >
              {review
                ? text(
                    "确认服务器定价",
                    "Confirm server pricing",
                    "تأكيد تسعير الخادم",
                  )
                : text(
                    "预览服务器变更",
                    "Review server changes",
                    "مراجعة تغييرات الخادم",
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
                  {text("计价方式", "Pricing mode", "نمط التسعير")}:{" "}
                  {draft.locked
                    ? mode(draft.base, draft.id)
                    : text("未配置", "Unconfigured", "غير مضبوط")}{" "}
                  →{" "}
                  {
                    {
                      per_token: text("按词元", "Per token", "لكل رمز"),
                      per_request: text("按次", "Per request", "لكل طلب"),
                      expression: text("表达式", "Expression", "تعبير"),
                    }[draft.kind]
                  }
                </p>
                {draft.kind === "expression" ? (
                  <>
                    <p>
                      {text(
                        "原表达式",
                        "Previous expression",
                        "التعبير السابق",
                      )}
                    </p>
                    <code className="stp-expression">
                      {ownPrice(draft.base.expressions, draft.id)}
                    </code>
                    <p>
                      {text(
                        "新表达式（基础美元金额）",
                        "New expression (base USD)",
                        "التعبير الجديد (دولار أساسي)",
                      )}
                    </p>
                    <code className="stp-expression">{draft.expression}</code>
                  </>
                ) : draft.kind === "per_request" ? (
                  <p>
                    {text(
                      "按次 USD/请求",
                      "Per request USD/request",
                      "لكل طلب دولار/طلب",
                    )}
                    :{" "}
                    <bdi>
                      {ownPrice(draft.base.perRequest, draft.id)} →{" "}
                      {draft.request}
                    </bdi>
                  </p>
                ) : (
                  <>
                    <p>
                      {text("输入基准", "Input base", "أساس الإدخال")}:{" "}
                      <bdi>
                        {ownPrice(draft.base.input, draft.id) ?? "—"} →{" "}
                        {draft.input}
                      </bdi>
                    </p>
                    <p>
                      {text(
                        "输出/输入倍率",
                        "Output/input ratio",
                        "معامل الإخراج/الإدخال",
                      )}
                      :{" "}
                      <bdi>
                        {ownPrice(draft.base.completion, draft.id) ?? "—"} →{" "}
                        {draft.completion}
                      </bdi>
                    </p>
                  </>
                )}
                <p>
                  {text(
                    "此操作修改所选模型后续请求的真实服务器价格，不重写历史账单；其他模型和无关计费选项保持原值。分组等规则仍会影响最终费用。数据库提交后响应进程单独重载，其他实例状态需另行确认。",
                    "This changes the selected model's real server price for future requests and leaves historical bills unchanged. Other models and unrelated pricing options retain their values. Group and other rules still affect final charges. The response process reloads after the database commit; check other instances separately.",
                    "يغيّر هذا سعر النموذج المحدد للطلبات القادمة دون إعادة كتابة الفواتير السابقة. تحتفظ النماذج والخيارات الأخرى بقيمها وتظل قواعد المجموعة وغيرها مؤثرة في الرسوم النهائية. تعيد العملية المستجيبة التحميل بعد الالتزام بقاعدة البيانات؛ تحقق من بقية العمليات منفصلة.",
                  )}
                </p>
                <p>
                  {text(
                    "切换到按词元将移除所选模型的表达式及按次价格；切换到按次将移除所选模型的表达式。按次或表达式模式保留较低优先级价格作为现有回退。所有相关键按同一版本整体保存。",
                    "Switching to per-token removes this model's expression and per-request override; switching to per-request removes its expression. Lower-priority prices are retained for existing fallback in per-request or expression mode. All affected keys are saved together against one version.",
                    "الانتقال إلى التسعير لكل رمز يزيل تعبير النموذج وسعره لكل طلب؛ والانتقال إلى التسعير لكل طلب يزيل تعبيره. تُحفظ الأسعار الأقل أولوية للرجوع إليها في النمطين الآخرين. تُحفظ المفاتيح المتأثرة معًا وفق إصدار واحد.",
                  )}
                </p>
                <Button
                  disabled={mutation.isPending}
                  onClick={() => {
                    setReview(false);
                    setError("");
                  }}
                >
                  {text("返回编辑", "Back to edit", "العودة للتحرير")}
                </Button>
              </div>
            ) : (
              <div className="stp-fields">
                <label>
                  {text(
                    "服务器模型标识",
                    "Server model ID",
                    "معرّف نموذج الخادم",
                  )}
                  <input
                    dir="ltr"
                    readOnly={draft.locked}
                    maxLength={128}
                    value={draft.id}
                    onChange={(e) => setDraft({ ...draft, id: e.target.value })}
                  />
                </label>
                <label>
                  {text("计价方式", "Pricing mode", "نمط التسعير")}
                  <select
                    aria-label={text("计价方式", "Pricing mode", "نمط التسعير")}
                    value={draft.kind}
                    onChange={(e) =>
                      setDraft({
                        ...draft,
                        kind: e.target.value as Draft["kind"],
                      })
                    }
                  >
                    <option value="per_token">
                      {text("按词元", "Per token", "لكل رمز")}
                    </option>
                    <option value="per_request">
                      {text("按次", "Per request", "لكل طلب")}
                    </option>
                    <option value="expression">
                      {text("表达式", "Expression", "تعبير")}
                    </option>
                  </select>
                </label>
                {draft.kind === "expression" ? (
                  <>
                    <label>
                      {text(
                        "计费表达式（基础美元金额）",
                        "Billing expression (base USD)",
                        "تعبير الفوترة (دولار أساسي)",
                      )}
                      <textarea
                        dir="ltr"
                        rows={5}
                        maxLength={512}
                        aria-label={text(
                          "计费表达式（基础美元金额）",
                          "Billing expression (base USD)",
                          "تعبير الفوترة (دولار أساسي)",
                        )}
                        value={draft.expression}
                        onChange={(e) =>
                          setDraft({ ...draft, expression: e.target.value })
                        }
                      />
                    </label>
                    <small>
                      {text(
                        "非空且不超过 512 个 UTF-8 字节。支持十进制数、算术/比较、三元条件及 min/max/floor/ceil/round/abs。通用变量为 prompt_tokens、completion_tokens、total_tokens；图片/视频变量仅在相应计费路径提供。语法通过不保证所有用量下求值成功；未知变量或除零会回退到原有按次/词元计费。",
                        "Use a nonempty expression of at most 512 UTF-8 bytes. Supports decimal numbers, arithmetic/comparisons, ternary conditions and min/max/floor/ceil/round/abs. Common variables: prompt_tokens, completion_tokens, total_tokens; image/video variables exist only on their billing paths. Valid syntax does not guarantee evaluation for every usage; unknown variables or division by zero fall back to existing per-request/token pricing.",
                        "استخدم تعبيرًا غير فارغ لا يتجاوز 512 بايت UTF-8. يدعم الأعداد العشرية والحساب والمقارنة والشرط الثلاثي والدوال min/max/floor/ceil/round/abs. المتغيرات العامة prompt_tokens وcompletion_tokens وtotal_tokens؛ ومتغيرات الصور والفيديو خاصة بمساراتها. صحة الصياغة لا تضمن نجاح كل تقييم؛ المتغير المجهول أو القسمة على صفر يعيدان التسعير السابق لكل طلب أو رمز.",
                      )}
                    </small>
                  </>
                ) : draft.kind === "per_request" ? (
                  <>
                    <label>
                      {text(
                        "按次 USD/请求",
                        "Per request USD/request",
                        "لكل طلب دولار/طلب",
                      )}
                      <input
                        dir="ltr"
                        inputMode="decimal"
                        maxLength={32}
                        value={draft.request}
                        onChange={(e) =>
                          setDraft({ ...draft, request: e.target.value })
                        }
                      />
                    </label>
                    <small>
                      {text(
                        "价格必须大于 0 且不超过 10000 美元/请求。零值会退出按次计价；如需其他模式，请明确选择计价方式。",
                        "Price must be greater than 0 and at most 10000 USD/request. Zero would leave per-request pricing; select another mode explicitly when needed.",
                        "يجب أن يكون السعر أكبر من صفر وألا يتجاوز 10000 دولار لكل طلب. الصفر يلغي التسعير لكل طلب؛ اختر نمطًا آخر صراحةً عند الحاجة.",
                      )}
                    </small>
                  </>
                ) : (
                  <>
                    <label>
                      {text(
                        "输入基准 USD/百万词元",
                        "Input base USD/million tokens",
                        "أساس الإدخال دولار/مليون رمز",
                      )}
                      <input
                        dir="ltr"
                        inputMode="decimal"
                        maxLength={32}
                        value={draft.input}
                        onChange={(e) =>
                          setDraft({ ...draft, input: e.target.value })
                        }
                      />
                    </label>
                    <label>
                      {text(
                        "输出/输入倍率",
                        "Output/input ratio",
                        "معامل الإخراج/الإدخال",
                      )}
                      <input
                        dir="ltr"
                        inputMode="decimal"
                        maxLength={32}
                        value={draft.completion}
                        onChange={(e) =>
                          setDraft({ ...draft, completion: e.target.value })
                        }
                      />
                    </label>
                    <small>
                      {text(
                        "两项均为 0–10 亿的有限非负数。未配置值保持空白；不从演示价格或其他模型推断。",
                        "Both values must be finite numbers from 0 to one billion. Unconfigured values stay blank; no Demo or other-model fallback is used.",
                        "يجب أن تكون القيمتان محدودتين وغير سالبتين من صفر إلى مليار. تبقى القيم غير المضبوطة فارغة دون الرجوع إلى أسعار تجريبية أو نماذج أخرى.",
                      )}
                    </small>
                  </>
                )}
              </div>
            )}
            {conflict && (
              <p role="alert">
                {text(
                  "配置版本变化或读取不可用，请刷新并重新打开编辑。",
                  "Configuration changed or cannot be read. Refresh and reopen the editor.",
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

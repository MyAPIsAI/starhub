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
  replaceServerModelRatios,
  type ServerPricing,
} from "../lib/server-pricing";
import { useSettingsText } from "./pricing-settings-shared";
import "./server-token-pricing.css";

type Draft = {
  base: ServerPricing;
  id: string;
  locked: boolean;
  input: string;
  completion: string;
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
  const writable = !!data && data.version <= Number.MAX_SAFE_INTEGER - 2;
  const conflict = !!draft && (!data || data.version !== draft.base.version);
  const valid =
    !!draft &&
    draft.id === draft.id.trim() &&
    draft.id.length > 0 &&
    draft.id.length <= 128 &&
    !/[\u0000-\u001f\u007f]/.test(draft.id) &&
    !["myapis-auto", "veyra-auto"].includes(draft.id) &&
    validServerRatio(draft.input) &&
    validServerRatio(draft.completion) &&
    serverPricingMode(draft.base, draft.id) === "per_token";
  const mode = (state: ServerPricing, id: string) =>
    ({
      per_token: text("按词元", "Per token", "لكل رمز"),
      per_request: text(
        "按次；此处只读",
        "Per request; read only here",
        "لكل طلب؛ للقراءة فقط هنا",
      ),
      expression: text(
        "表达式；此处只读",
        "Expression; read only here",
        "تعبير؛ للقراءة فقط هنا",
      ),
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
    });
  };
  const save = async () => {
    if (!draft || !review || !valid || conflict || !writable) return;
    try {
      const result = await mutation.mutateAsync({
        expected_version: draft.base.version,
        options: replaceServerModelRatios(
          draft.base,
          draft.id,
          draft.input,
          draft.completion,
        ),
      });
      if (
        result.version < draft.base.version ||
        result.version > draft.base.version + 2
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
              "服务器词元定价",
              "Server token pricing",
              "تسعير الرموز في الخادم",
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
              "登记模型倍率",
              "Configure model ratios",
              "تعيين معاملات نموذج",
            )}
          </Button>
        </div>
      </header>
      <p>
        {text(
          "保存一对倍率不会创建渠道、投放资源或授予密钥权限。按次和表达式优先的模型在此只读；完整分时规则管理仍待实现。",
          "Saving a ratio pair does not create channels, publish resources or grant key permissions. Models with per-request or expression pricing are read only here; full time-rule management remains pending.",
          "لا ينشئ حفظ المعاملين قنوات أو ينشر موارد أو يمنح صلاحيات للمفاتيح. نماذج التسعير لكل طلب أو بالتعبير للقراءة فقط هنا، ولا تزال إدارة قواعد الوقت الكاملة قيد التنفيذ.",
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
                        <bdi>{ownPrice(data.input, id) ?? "—"}</bdi>
                      </td>
                      <td>
                        <bdi>{ownPrice(data.completion, id) ?? "—"}</bdi>
                      </td>
                      <td>
                        <Button
                          size="small"
                          disabled={
                            !writable ||
                            serverPricingMode(data, id) !== "per_token"
                          }
                          onClick={() => begin(id)}
                        >
                          {text(
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
        title={text(
          "修改真实服务器倍率",
          "Change actual server ratios",
          "تغيير معاملات الخادم الفعلية",
        )}
        description={text(
          "输入和输出倍率按同一配置版本整体保存。取消仅放弃此草稿。",
          "Input and output ratios are saved together against one configuration version. Cancel discards this draft.",
          "يُحفظ معاملا الإدخال والإخراج معًا وفق إصدار إعدادات واحد. يلغي الإلغاء هذه المسودة.",
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
                <p>
                  {text(
                    "此操作修改真实服务器后续计费的一对基础倍率，不重写历史账单；其他计费选项保持原值。数据库提交后响应进程单独重载，其他实例状态需另行确认。",
                    "This changes a real server billing ratio pair for future requests and leaves historical bills unchanged. Other pricing options retain their values. The response process reloads after the database commit; check other instances separately.",
                    "يغيّر هذا الإجراء معاملي فوترة الخادم للطلبات القادمة دون إعادة كتابة الفواتير السابقة. تحتفظ خيارات التسعير الأخرى بقيمها. تعيد العملية المستجيبة التحميل بعد الالتزام بقاعدة البيانات؛ تحقق من بقية العمليات منفصلة.",
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

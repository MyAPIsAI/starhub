import { useState } from "react";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import { useSaveServerPricing } from "../lib/api/pricing-options";
import { ApiError } from "../lib/api/http";
import {
  replaceServerDetailRatios,
  validServerRatio,
  type ServerPricing,
} from "../lib/server-pricing";
import { useSettingsText } from "./pricing-settings-shared";

type Draft = { base: ServerPricing; cache: string; audio: string };
export function ServerDetailPricing({
  data,
}: {
  data: ServerPricing | undefined;
}) {
  const { text } = useSettingsText(),
    mutation = useSaveServerPricing();
  const [draft, setDraft] = useState<Draft | null>(null),
    [review, setReview] = useState(false),
    [error, setError] = useState(""),
    [notice, setNotice] = useState("");
  const valid =
    !!draft &&
    [draft.cache, draft.audio].every(
      (value) => validServerRatio(value) && Number(value) <= 1000,
    );
  const conflict = !!draft && (!data || data.version !== draft.base.version);
  const writable = !!data && data.version <= Number.MAX_SAFE_INTEGER - 2;
  const cacheLabel = text(
    "缓存输入倍率",
    "Cached input ratio",
    "معامل الإدخال المخزّن",
  );
  const audioLabel = text(
    "音频词元倍率",
    "Audio token ratio",
    "معامل الرموز الصوتية",
  );
  const save = async () => {
    if (!draft || !review || !valid || conflict || !writable) return;
    try {
      const result = await mutation.mutateAsync({
        expected_version: draft.base.version,
        options: replaceServerDetailRatios(draft.cache, draft.audio),
      });
      if (
        result.version < draft.base.version ||
        result.version > draft.base.version + 2
      )
        throw Error("Unexpected pricing revision");
      setDraft(null);
      setNotice(
        text(
          `全局倍率已保存 v${result.version}；响应进程加载 v${result.loaded_version}，其他实例请查看配置同步。`,
          `Global ratios saved at v${result.version}; response process loaded v${result.loaded_version}. Check configuration sync for other instances.`,
          `حُفظت المعاملات العامة بالإصدار ${result.version}؛ حمّلت العملية المستجيبة ${result.loaded_version}. راجع المزامنة للعمليات الأخرى.`,
        ),
      );
    } catch (caught) {
      setError(
        caught instanceof ApiError && caught.status === 409
          ? text(
              "配置已变化，未覆盖新版本。请刷新并重新打开编辑。",
              "Configuration changed; the newer version was not overwritten. Refresh and reopen the editor.",
              "تغيّرت الإعدادات ولم يُستبدل الإصدار الأحدث. حدّث وأعد فتح المحرر.",
            )
          : caught instanceof ApiError && caught.status === 400
            ? text(
                "服务器拒绝了倍率，草稿已保留。请检查输入。",
                "The server rejected these ratios; the draft is retained. Check the values.",
                "رفض الخادم المعاملات واحتُفظ بالمسودة. تحقق من القيم.",
              )
            : text(
                "无法确认保存或重载结果，请刷新核对数据库与进程版本。",
                "The save or reload result is unconfirmed. Refresh to check database and process versions.",
                "تعذر تأكيد الحفظ أو إعادة التحميل. حدّث للتحقق من إصداري قاعدة البيانات والعملية.",
              ),
      );
    }
  };
  return (
    <div className="stp-detail-ratios">
      {data && (
        <>
          <div className="stp-actions">
            <span>
              {cacheLabel}: <bdi>{data.cache}×</bdi> · {audioLabel}:{" "}
              <bdi>{data.audio}×</bdi>
            </span>
            <Button
              disabled={!writable}
              onClick={() => {
                setError("");
                setNotice("");
                setReview(false);
                mutation.reset();
                setDraft({
                  base: data,
                  cache: String(data.cache),
                  audio: String(data.audio),
                });
              }}
            >
              {text(
                "编辑全局明细倍率",
                "Edit global detail ratios",
                "تحرير معاملات التفاصيل العامة",
              )}
            </Button>
          </div>
          <p>
            {text(
              "缓存命中输入与音频输入/输出词元相对普通输入的全局倍率，仅用于按词元计费。按次与表达式保留各自规则，其他倍率仍适用。",
              "Global multipliers for cached input and audio input/output tokens relative to ordinary input, used in token billing only. Per-request and expression pricing retain their own rules; other multipliers still apply.",
              "معاملات عامة للإدخال المخزّن والرموز الصوتية الداخلة والخارجة نسبةً إلى الإدخال العادي، لفوترة الرموز فقط. يحتفظ التسعير لكل طلب وبالتعبير بقواعده وتظل المعاملات الأخرى مطبقة.",
            )}
          </p>
        </>
      )}
      {notice && <p role="status">{notice}</p>}
      <Modal
        className="server-pricing-dialog"
        open={!!draft}
        onOpenChange={(open) => {
          if (!open && !mutation.isPending) setDraft(null);
        }}
        preventOutsideClose
        title={text(
          "修改全局明细倍率",
          "Change global detail ratios",
          "تغيير معاملات التفاصيل العامة",
        )}
        description={text(
          "影响所有采用词元计费的模型后续请求，不重写历史账单。取消仅放弃草稿。",
          "Affects future requests for all models using token billing. Historical bills stay unchanged. Cancel discards this draft.",
          "يؤثر في الطلبات القادمة لجميع نماذج فوترة الرموز دون تغيير الفواتير السابقة. يلغي الإلغاء هذه المسودة.",
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
                    "确认全局倍率",
                    "Confirm global ratios",
                    "تأكيد المعاملات العامة",
                  )
                : text(
                    "预览全局变更",
                    "Review global changes",
                    "مراجعة التغييرات العامة",
                  )}
            </Button>
          </>
        }
      >
        {draft && (
          <>
            <p>v{draft.base.version}</p>
            {review ? (
              <div className="stp-review">
                <p>
                  {cacheLabel}:{" "}
                  <bdi>
                    {draft.base.cache} → {draft.cache}
                  </bdi>
                </p>
                <p>
                  {audioLabel}:{" "}
                  <bdi>
                    {draft.base.audio} → {draft.audio}
                  </bdi>
                </p>
                <p>
                  {text(
                    "两项倍率同一版本整体保存，其他模型价格、模式、分组及历史账单不改写。词元计费存在最小 1 配额，即使某项倍率为零也不代表整次请求免费。响应进程在提交后重载；其他实例状态需另行确认。",
                    "Both ratios are saved atomically against one version. Model prices, modes, groups and historical bills are not rewritten. Token billing retains a one-quota minimum; a zero multiplier does not make a whole request free. The response process reloads after commit; check other instances separately.",
                    "يُحفظ المعاملان ذريًا وفق إصدار واحد دون إعادة كتابة أسعار النماذج أو أنماطها أو مجموعاتها أو الفواتير السابقة. تحتفظ فوترة الرموز بحد أدنى وحدة حصة واحدة؛ الصفر لا يجعل الطلب كله مجانيًا. تعيد العملية المستجيبة التحميل بعد الحفظ؛ تحقق من العمليات الأخرى منفصلة.",
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
                  {cacheLabel}
                  <input
                    dir="ltr"
                    inputMode="decimal"
                    maxLength={32}
                    value={draft.cache}
                    onChange={(e) =>
                      setDraft({ ...draft, cache: e.target.value })
                    }
                  />
                </label>
                <label>
                  {audioLabel}
                  <input
                    dir="ltr"
                    inputMode="decimal"
                    maxLength={32}
                    value={draft.audio}
                    onChange={(e) =>
                      setDraft({ ...draft, audio: e.target.value })
                    }
                  />
                </label>
                <small>
                  {text(
                    "两项均为 0–1000 的有限非负数，零值会按原值保存。",
                    "Both values must be finite nonnegative numbers from 0 to 1000. Explicit zero is preserved.",
                    "يجب أن تكون القيمتان محدودتين وغير سالبتين من صفر إلى 1000. يُحفظ الصفر كما هو.",
                  )}
                </small>
              </div>
            )}
            {conflict && (
              <p role="alert">
                {text(
                  "配置变化或读取失败，请刷新并重新打开。",
                  "Configuration changed or could not be read. Refresh and reopen.",
                  "تغيّر الإعداد أو تعذرت قراءته. حدّث وأعد الفتح.",
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

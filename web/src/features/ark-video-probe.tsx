import { useEffect, useRef, useState } from "react";
import { Video, Eye } from "lucide-react";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/primitives";
import { Modal } from "../components/ui/overlay";
import { apiRequest } from "../lib/api/http";
import type { AdminChannel } from "../lib/api/admin";
import { imagePreviewURL } from "./image-probe-preview";
import { useSettingsText } from "./settings-text";
import { isArkVideoModel } from "./ark-video-models";
import "./channel-model-discovery.css";

type VideoTest = {
  id: number;
  model: string;
  status: string;
  video_url?: string;
  message?: string;
};
const terminal = (task?: VideoTest) =>
  !task || ["succeeded", "failed", "cancelled"].includes(task.status);
const failures: Record<string, [string, string, string]> = {
  upstream_video_authentication_failed: [
    "视频接口认证失败（401），请更新有效的方舟 API Key。",
    "Video API authentication failed (401). Update the Ark API key.",
    "فشلت مصادقة واجهة الفيديو (401). حدّث مفتاح Ark.",
  ],
  upstream_video_permission_denied: [
    "视频接口拒绝访问（403），请检查账号权限。",
    "Video access denied (403). Check account permissions.",
    "الوصول للفيديو مرفوض (403). تحقق من صلاحيات الحساب.",
  ],
  video_probe_already_running: [
    "已有视频测试正在运行，请查看现有结果。",
    "A video test is already running. View the existing result.",
    "اختبار فيديو قيد التشغيل. اعرض النتيجة الحالية.",
  ],
  video_probe_channel_changed: [
    "渠道配置已变化或未启用，请刷新后重试。",
    "The channel changed or is disabled. Refresh and retry.",
    "تغيرت القناة أو تعطّلت. حدّث وأعد المحاولة.",
  ],
  video_probe_submission_uncertain: [
    "无法确认上游是否接受了任务。不会自动重试，请先到方舟控制台核对，避免重复计费。",
    "Submission is uncertain. No automatic retry. Check the Ark console before another paid test.",
    "إرسال المهمة غير مؤكد. لن يُعاد تلقائيًا. تحقق من لوحة Ark قبل اختبار مدفوع آخر.",
  ],
  video_probe_result_expired: [
    "上游任务已不存在或过期。",
    "The upstream task is missing or expired.",
    "مهمة المزود غير موجودة أو منتهية.",
  ],
};

export function ArkVideoProbe({
  channel,
  disabled,
}: {
  channel: AdminChannel;
  disabled: boolean;
}) {
  const { text } = useSettingsText();
  const candidates = channel.models
    .split(",")
    .map((id) => id.trim())
    .filter(
      (id) =>
        channel.model_capabilities?.[id] === "VideoGeneration" ||
        isArkVideoModel(id) ||
        (!channel.model_capabilities?.[id] && id.startsWith("ep-")),
    );
  const [open, setOpen] = useState(false),
    [view, setView] = useState(false),
    [selected, setSelected] = useState("");
  const [prompt, setPrompt] = useState(
    "A red ball rolling slowly on a plain white background, fixed camera.",
  );
  const [task, setTask] = useState<VideoTest>(),
    [pending, setPending] = useState(false),
    [error, setError] = useState("");
  const nonce = useRef("");
  const [playbackFailed, setPlaybackFailed] = useState(false);
  useEffect(() => setPlaybackFailed(false), [task?.id, task?.video_url]);
  const mounted = useRef(true);
  const endpoint = `/api/channel/video-test/${channel.id}/task`;
  const active = !terminal(task);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  useEffect(() => {
    if (channel.type !== 32 || candidates.length === 0) return;
    const abort = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    const load = async () => {
      try {
        const result = await apiRequest<VideoTest | null>(endpoint, {
          signal: abort.signal,
        });
        if (abort.signal.aborted) return;
        if (result) setTask(result);
        if (result && !terminal(result)) timer = setTimeout(load, 4000);
      } catch (e) {
        if (!abort.signal.aborted)
          setError(e instanceof Error ? e.message : "video_probe_unavailable");
      }
    };
    void load();
    return () => {
      abort.abort();
      clearTimeout(timer);
    };
  }, [endpoint, task?.id, active, channel.type]);
  if (channel.type !== 32 || candidates.length === 0) return null;
  const label = text(
    "视频生成测试（上游计费）",
    "Test video generation (upstream charges)",
    "اختبار توليد الفيديو (رسوم المزود)",
  );
  const message = (code: string) =>
    failures[code]
      ? text(...failures[code])
      : text(
          "测试或查询失败，请检查上游接口和账号权限后重试。",
          "Test or query failed. Check the upstream API and account access.",
          "فشل الاختبار أو الاستعلام. تحقق من واجهة المزود وصلاحيات الحساب.",
        );
  const submit = async () => {
    if (pending || active || !candidates.includes(selected) || !prompt.trim())
      return;
    setPending(true);
    setError("");
    setView(true);
    try {
      const result = await apiRequest<VideoTest>(endpoint, {
        method: "POST",
        body: JSON.stringify({
          model: selected,
          prompt,
          request_id: nonce.current,
          confirm_upstream_charge: true,
        }),
      });
      if (mounted.current) setTask(result);
    } catch (e) {
      if (mounted.current)
        setError(e instanceof Error ? e.message : "video_probe_unavailable");
    } finally {
      if (mounted.current) setPending(false);
    }
  };
  const url =
    task?.status === "succeeded" ? imagePreviewURL(task.video_url) : undefined;
  return (
    <>
      <Button
        size="small"
        className="channel-probe-generate"
        aria-label={label}
        disabled={disabled || pending || active}
        onClick={() => {
          setSelected(candidates.length === 1 ? candidates[0] : "");
          setView(false);
          setError("");
          nonce.current = crypto.randomUUID();
          setOpen(true);
        }}
      >
        <Video size={14} aria-hidden="true" />
        {active
          ? text("视频处理中…", "Video processing…", "معالجة الفيديو…")
          : text("生成视频", "Generate video", "توليد فيديو")}
        <span className="channel-probe-fee">
          {text("计费", "Paid", "مدفوع")}
        </span>
      </Button>
      {(task || error) && (
        <Button
          size="small"
          variant="ghost"
          onClick={() => {
            setView(true);
            setOpen(true);
          }}
        >
          <Eye size={14} aria-hidden="true" />
          {text("查看视频结果", "View video result", "عرض نتيجة الفيديو")}
        </Button>
      )}
      <Modal
        open={open}
        onOpenChange={setOpen}
        title={
          view
            ? text("视频测试结果", "Video test result", "نتيجة اختبار الفيديو")
            : label
        }
        description={text(
          "生成一个 4 秒、480p 测试视频，由上游计费，不扣本站用户额度。关闭弹窗后任务继续，可重新打开查看。",
          "Generate one 4-second 480p video. Upstream charges apply; customer quota is not charged. The task continues after closing this dialog.",
          "توليد فيديو واحد مدته 4 ثوانٍ بدقة 480p برسوم المزود دون خصم حصة العميل. تستمر المهمة بعد إغلاق النافذة.",
        )}
        footer={
          <>
            <Button onClick={() => setOpen(false)}>
              {text("关闭", "Close", "إغلاق")}
            </Button>
            {!view && (
              <Button
                variant="primary"
                disabled={
                  disabled || pending || active || !selected || !prompt.trim()
                }
                onClick={() => void submit()}
              >
                {text(
                  "生成测试视频（上游计费）",
                  "Generate test video (upstream charges)",
                  "توليد فيديو الاختبار (رسوم المزود)",
                )}
              </Button>
            )}
            {view && !pending && (
              <Button
                onClick={async () => {
                  setError("");
                  try {
                    const result = await apiRequest<VideoTest | null>(endpoint);
                    if (result && mounted.current) setTask(result);
                  } catch (e) {
                    if (mounted.current)
                      setError(
                        e instanceof Error
                          ? e.message
                          : "video_probe_unavailable",
                      );
                  }
                }}
              >
                {text("刷新结果", "Refresh result", "تحديث النتيجة")}
              </Button>
            )}
          </>
        }
      >
        {!view ? (
          <div className="channel-model-key">
            <label htmlFor={`video-model-${channel.id}`}>
              {text("视频模型", "Video model", "نموذج الفيديو")}
            </label>
            <select
              id={`video-model-${channel.id}`}
              className="input"
              value={selected}
              onChange={(e) => setSelected(e.target.value)}
            >
              <option value="">
                {text("请选择模型", "Select a model", "اختر نموذجًا")}
              </option>
              {candidates.map((id) => (
                <option key={id} value={id}>
                  {id}
                </option>
              ))}
            </select>
            <label htmlFor={`video-prompt-${channel.id}`}>
              {text("测试提示词", "Test prompt", "نص الاختبار")}
            </label>
            <Input
              id={`video-prompt-${channel.id}`}
              value={prompt}
              maxLength={300}
              onChange={(e) => setPrompt(e.target.value)}
            />
          </div>
        ) : (
          <>
            {pending && (
              <p role="status">
                {text(
                  "正在提交视频任务…",
                  "Submitting video task…",
                  "جارٍ إرسال مهمة الفيديو…",
                )}
              </p>
            )}
            {task && !pending && (
              <p role={task.status === "failed" ? "alert" : "status"}>
                {task.status === "succeeded"
                  ? text("视频生成成功", "Video generated", "تم توليد الفيديو")
                  : task.status === "failed"
                    ? text(
                        "视频生成失败",
                        "Video generation failed",
                        "فشل توليد الفيديو",
                      )
                    : task.status === "cancelled"
                      ? text(
                          "视频任务已取消",
                          "Video task cancelled",
                          "أُلغيت مهمة الفيديو",
                        )
                      : text(
                          "视频任务处理中，请稍候…",
                          "Video task processing. Please wait…",
                          "تجري معالجة الفيديو. يُرجى الانتظار…",
                        )}
                <small style={{ display: "block", overflowWrap: "anywhere" }}>
                  <bdi>{task.model}</bdi> ·{" "}
                  {text("测试记录", "Test record", "سجل الاختبار")} {task.id}
                </small>
              </p>
            )}
            {(error || (!pending && task?.message)) && (
              <p role="alert" className="channel-model-error">
                {message(error || task?.message || "")}
              </p>
            )}
            {url && (
              <div>
                <video
                  controls
                  preload="none"
                  onError={() => setPlaybackFailed(true)}
                  src={url}
                  style={{ width: "100%", maxHeight: 360 }}
                  aria-label={text(
                    "生成的测试视频",
                    "Generated test video",
                    "فيديو الاختبار المولد",
                  )}
                />
                {playbackFailed && (
                  <p role="alert">
                    {text(
                      "浏览器无法播放此视频，请打开原视频查看或下载。",
                      "This browser could not play the video. Open the original to view or download it.",
                      "تعذر تشغيل الفيديو في المتصفح. افتح الأصل للمشاهدة أو التنزيل.",
                    )}
                  </p>
                )}
                <a
                  href={url}
                  target="_blank"
                  rel="noopener noreferrer"
                  style={{
                    color: "var(--brand)",
                    display: "inline-block",
                    marginTop: 8,
                  }}
                >
                  {text(
                    "打开原视频",
                    "Open original video",
                    "فتح الفيديو الأصلي",
                  )}
                </a>
                <p className="channel-model-hint">
                  {text(
                    "上游链接通常在 24 小时后失效，请及时保存。",
                    "Upstream links usually expire after 24 hours. Save the video promptly.",
                    "تنتهي روابط المزود عادةً بعد 24 ساعة. احفظ الفيديو سريعًا.",
                  )}
                </p>
              </div>
            )}
          </>
        )}
      </Modal>
    </>
  );
}

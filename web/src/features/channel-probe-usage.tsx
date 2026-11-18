import type { ChannelProbeUsage } from "../lib/api/admin";
import { useSettingsText } from "./settings-text";

export function ChannelProbeUsageView({ usage }: { usage: ChannelProbeUsage }) {
  const { text, lang } = useSettingsText();
  const count = (value: number | string) =>
    new Intl.NumberFormat(lang).format(
      typeof value === "string" ? BigInt(value) : value,
    );
  return (
    <section
      className="usage-probes"
      aria-label={text(
        "生成测试用量",
        "Generation test usage",
        "استخدام اختبارات التوليد",
      )}
    >
      <h4>
        {text(
          "生成测试用量",
          "Generation test usage",
          "استخدام اختبارات التوليد",
        )}
      </h4>
      <dl>
        <div>
          <dt>{text("已生成图片", "Images generated", "الصور المولدة")}</dt>
          <dd>
            {usage.image_tests > 0
              ? count(usage.images)
              : text(
                  "尚无可追溯记录",
                  "No recorded tests",
                  "لا توجد اختبارات مسجلة",
                )}
          </dd>
        </div>
        <div>
          <dt>
            {text("图片测试次数", "Image test requests", "طلبات اختبار الصور")}
          </dt>
          <dd>{count(usage.image_tests)}</dd>
        </div>
        <div>
          <dt>
            {text(
              "已生成视频 / 测试次数",
              "Videos generated / test requests",
              "الفيديوهات المولدة / طلبات الاختبار",
            )}
          </dt>
          <dd>
            {count(usage.video_successes)} / {count(usage.video_tests)}
          </dd>
        </div>
        <div>
          <dt>
            {text(
              "已返回视频时长（秒）",
              "Reported video duration (seconds)",
              "مدة الفيديو المبلّغ عنها (ثوانٍ)",
            )}
          </dt>
          <dd>{count(usage.video_seconds)}</dd>
        </div>
        <div>
          <dt>
            {text(
              "已返回视频输出用量（tokens）",
              "Reported video output tokens",
              "رموز مخرجات الفيديو المبلّغ عنها",
            )}
          </dt>
          <dd dir="ltr">
            {usage.missing_video_usage > 0 && usage.video_output_tokens === "0"
              ? "—"
              : count(usage.video_output_tokens)}
          </dd>
        </div>
      </dl>
      {(usage.uncertain_images > 0 ||
        usage.pending_videos > 0 ||
        usage.missing_video_usage > 0) && (
        <p role="status">
          {text(
            `图片结果未确认 ${count(usage.uncertain_images)} 次；视频处理中 ${count(usage.pending_videos)} 个，成功但未返回 token 用量 ${count(usage.missing_video_usage)} 个。`,
            `Unconfirmed image tests: ${count(usage.uncertain_images)}. Pending videos: ${count(usage.pending_videos)}; successful videos without token usage: ${count(usage.missing_video_usage)}.`,
            `اختبارات صور غير مؤكدة: ${count(usage.uncertain_images)}. فيديوهات قيد المعالجة: ${count(usage.pending_videos)}؛ فيديوهات ناجحة بلا استخدام رموز: ${count(usage.missing_video_usage)}.`,
          )}
        </p>
      )}
      <p className="usage-note">
        {text(
          "管理页生成测试由上游计费，不计入客户业务扣费。这里记录实际返回的用量，费用待上游账单确认，不代表免费。图片测试从本功能启用后记录，早期未保存的图片测试无法回溯；已有视频测试记录已纳入。",
          "Admin generation tests are charged upstream, separately from customer billing. Reported usage is recorded here; costs await confirmation from the upstream bill. Image tests are recorded from this feature onward; earlier unrecorded image tests cannot be recovered. Existing video tests are included.",
          "تُحاسب اختبارات الإدارة لدى المزود منفصلةً عن العملاء. يُسجل الاستخدام المبلّغ عنه هنا؛ التكلفة تنتظر فاتورة المزود ولا تعني المجانية. تُسجل الصور من تفعيل هذه الميزة ولا يمكن استعادة الاختبارات السابقة غير المسجلة. تشمل البيانات اختبارات الفيديو الحالية.",
        )}
      </p>
    </section>
  );
}

import { useState } from "react";
import { Activity } from "lucide-react";
import { Button } from "../components/ui/button";
import {
  useTestChannel,
  type AdminChannel,
  type ChannelTestResult,
} from "../lib/api/admin";
import { useSessionQueryIdentity } from "../lib/system-access";
import { ArkImageProbe } from "./ark-image-probe";
import { ArkVideoProbe } from "./ark-video-probe";
import { ChannelProbeResult } from "./channel-probe-result";
import { useSettingsText } from "./settings-text";
import "./channel-probe-controls.css";

export function ChannelProbeControls({ channel }: { channel: AdminChannel }) {
  const identity = useSessionQueryIdentity();
  // Keep temporary images scoped to the tested connection and signed-in session.
  const signature = JSON.stringify([
    identity,
    channel.type,
    channel.base_url,
    channel.models,
    channel.model_capabilities,
    channel.task_plugin_id,
  ]);
  return <ProbeControls key={signature} channel={channel} />;
}

function ProbeControls({ channel }: { channel: AdminChannel }) {
  const { text } = useSettingsText();
  const test = useTestChannel();
  const [result, setResult] = useState<ChannelTestResult>();
  const [image, setImage] = useState<ChannelTestResult>();
  const imagePending = test.isPending && test.variables?.mode === "image";
  const hasVideo =
    channel.type === 32 &&
    channel.models
      .split(",")
      .some(
        (id) =>
          channel.model_capabilities?.[id.trim()] === "VideoGeneration" ||
          id.trim().startsWith("doubao-seedance-"),
      );
  const run = (model?: string, mode?: "image") => {
    if (test.isPending) return;
    setResult(undefined);
    if (mode === "image") setImage(undefined);
    const finish = (response: ChannelTestResult) => {
      setResult(response);
      if (mode === "image") setImage(response);
    };
    test.mutate(
      { id: channel.id, model, mode },
      {
        onSuccess: finish,
        onError: (error) =>
          finish({
            ok: false,
            model: model || channel.models.split(",")[0],
            scope: mode === "image" ? "image" : "",
            elapsed_ms: 0,
            status: channel.status,
            message:
              error instanceof Error
                ? error.message
                : text(
                    "测试请求失败",
                    "Test request failed",
                    "فشل طلب الاختبار",
                  ),
          }),
      },
    );
  };
  return (
    <div className="channel-probe-controls">
      <div className="channel-probe-actions">
        <Button size="small" disabled={test.isPending} onClick={() => run()}>
          <Activity size={14} aria-hidden="true" />
          {test.isPending && !imagePending
            ? text("测试中…", "Testing…", "اختبار…")
            : text("测试连通", "Test", "اختبار")}
        </Button>
        <ArkImageProbe
          channel={channel}
          disabled={test.isPending}
          pending={imagePending}
          result={image}
          onProbe={(model) => run(model, "image")}
        />
        <ArkVideoProbe channel={channel} disabled={test.isPending} />
      </div>
      {result && <ChannelProbeResult result={result} compact />}
      {hasVideo && (
        <details className="channel-probe-video">
          <summary>
            {text("视频接入", "Video integration", "ربط الفيديو")}
          </summary>
          <p>
            {text(
              "测试连通只读取视频任务接口，不生成视频。“生成视频”会提交计费测试任务，并自动查询结果。也可通过任务 API 接入。",
              "Connectivity tests read the video task API without generating video. Generate video submits a paid test task and polls its result. The task API is also available for integration.",
              "يقرأ اختبار الاتصال واجهة المهام دون توليد فيديو. يرسل توليد الفيديو اختبارًا مدفوعًا ويتابع نتيجته. تتوفر واجهة المهام للربط أيضًا.",
            )}
          </p>
          <code dir="ltr">POST /v1/video/generations</code>
          <code dir="ltr">GET /v1/video/generations/&#123;id&#125;</code>
        </details>
      )}
    </div>
  );
}

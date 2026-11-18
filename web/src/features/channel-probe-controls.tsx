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
  const firstDomain =
    channel.model_capabilities?.[channel.models.split(",")[0]?.trim()];
  const catalogOnly =
    channel.type === 32 && firstDomain && !["LLM", "VLM"].includes(firstDomain);
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
            : catalogOnly
              ? text("目录检查", "Check catalog", "فحص الدليل")
              : text("测试连通", "Test", "اختبار")}
        </Button>
        <ArkImageProbe
          channel={channel}
          disabled={test.isPending}
          pending={imagePending}
          result={image}
          onProbe={(model) => run(model, "image")}
        />
      </div>
      {result && <ChannelProbeResult result={result} compact />}
    </div>
  );
}

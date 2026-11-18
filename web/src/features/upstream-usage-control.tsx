import { useEffect, useRef, useState } from "react";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import {
  fetchUpstreamUsage,
  type AdminChannel,
  type UpstreamUsageResult,
} from "../lib/api/admin";
import { useSessionQueryIdentity } from "../lib/system-access";
import { useSettingsText } from "./settings-text";
import { UpstreamUsageResultView } from "./upstream-usage-result";

export function UpstreamUsageControl({
  channel,
  version,
}: {
  channel: AdminChannel;
  version: number;
}) {
  const identity = useSessionQueryIdentity();
  // A refreshed channel list also invalidates results after credential-only edits.
  return (
    <UsageControl
      key={JSON.stringify([identity, channel.id, version])}
      channel={channel}
    />
  );
}

function UsageControl({ channel }: { channel: AdminChannel }) {
  const { text } = useSettingsText();
  const request = useRef<AbortController | null>(null);
  const [open, setOpen] = useState(false);
  const [pending, setPending] = useState(false);
  const [result, setResult] = useState<UpstreamUsageResult>();
  const [failed, setFailed] = useState(false);
  useEffect(
    () => () => {
      request.current?.abort();
      request.current = null;
    },
    [],
  );
  const close = () => {
    request.current?.abort();
    request.current = null;
    setOpen(false);
    setPending(false);
    setResult(undefined);
    setFailed(false);
  };
  const run = async () => {
    if (request.current) return;
    const controller = new AbortController();
    request.current = controller;
    setResult(undefined);
    setFailed(false);
    setPending(true);
    setOpen(true);
    try {
      const response = await fetchUpstreamUsage(channel.id, controller.signal);
      if (request.current !== controller || controller.signal.aborted) return;
      if (response.channel_id !== channel.id)
        throw Error("Unexpected channel usage response");
      setResult(response);
    } catch {
      if (request.current === controller && !controller.signal.aborted)
        setFailed(true);
    } finally {
      if (request.current === controller) {
        request.current = null;
        setPending(false);
      }
    }
  };
  return (
    <>
      <Button
        size="small"
        variant="secondary"
        disabled={pending}
        onClick={() => void run()}
      >
        {pending
          ? text("查询中…", "Querying…", "استعلام…")
          : text("上游对账", "Reconcile", "مطابقة")}
      </Button>
      <Modal
        open={open}
        onOpenChange={(value) => {
          if (!value) close();
        }}
        title={text("上游对账", "Upstream Reconciliation", "مطابقة المنبع")}
        description={`${channel.name} (ID: ${channel.id})`}
        size="wide"
      >
        {pending && (
          <p role="status">
            {text(
              "正在查询上游用量…",
              "Querying upstream usage…",
              "جارٍ استعلام استخدام المزود…",
            )}
          </p>
        )}
        {failed && (
          <p role="alert">
            {text(
              "上游用量查询失败，请检查登录状态和权限，稍后重试。",
              "The upstream usage query failed. Check your session and permissions, then retry.",
              "فشل استعلام الاستخدام. تحقق من الجلسة والصلاحيات ثم أعد المحاولة.",
            )}
          </p>
        )}
        {result && <UpstreamUsageResultView result={result} />}
      </Modal>
    </>
  );
}

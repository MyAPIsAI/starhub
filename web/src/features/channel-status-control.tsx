import { Button } from "../components/ui/button";
import type { AdminChannel } from "../lib/api/admin";
import { useSetChannelStatus } from "../lib/api/channel";
import { channelResourceStatus } from "../lib/channel-status";
import { useSettingsText } from "./settings-text";

export function ChannelStatusControl({ channel }: { channel: AdminChannel }) {
  const { text } = useSettingsText();
  const mutation = useSetChannelStatus();
  const status = channelResourceStatus(channel);
  const enabled = status === "closed";
  const color =
    status === "enabled"
      ? "success"
      : status === "partial"
        ? "warning"
        : "danger";
  const action = enabled
    ? text(
        `启用渠道 ${channel.name} 的所有资源`,
        `Enable all resources in channel ${channel.name}`,
        `تفعيل جميع موارد القناة ${channel.name}`,
      )
    : text(
        `关闭渠道 ${channel.name} 的所有资源`,
        `Close all resources in channel ${channel.name}`,
        `إيقاف جميع موارد القناة ${channel.name}`,
      );
  return (
    <div>
      <Button
        size="small"
        variant="ghost"
        data-channel-status={status}
        style={{
          background: `var(--${color}-soft)`,
          color: "var(--text)",
          borderColor: `var(--${color})`,
          fontWeight: 600,
        }}
        aria-label={action}
        title={action}
        loading={mutation.isPending}
        onClick={() => mutation.mutate({ id: channel.id, enabled })}
      >
        {status === "enabled"
          ? text("启用", "Enabled", "مُفعّل")
          : status === "partial"
            ? text("部分启用", "Partially enabled", "مُفعّل جزئيًا")
            : text("关闭", "Closed", "متوقف")}
      </Button>
      {mutation.isSuccess && (
        <span className="sr-only" role="status">
          {text(
            "资源状态已更新",
            "Resource status updated",
            "تم تحديث حالة الموارد",
          )}
        </span>
      )}
      {mutation.isError && (
        <p role="alert" style={{ color: "var(--danger)", margin: "6px 0 0" }}>
          {text(
            "状态更新失败，请刷新核对后重试。",
            "Status update failed. Refresh to check, then retry.",
            "تعذر تحديث الحالة. حدّث للتحقق ثم أعد المحاولة.",
          )}
        </p>
      )}
    </div>
  );
}

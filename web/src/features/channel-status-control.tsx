import { useState } from "react";
import { ChevronDown } from "lucide-react";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import type { AdminChannel } from "../lib/api/admin";
import {
  useSetChannelStatus,
  useSetChannelResourceStatus,
} from "../lib/api/channel";
import { channelResourceStatus } from "../lib/channel-status";
import { useSettingsText } from "./settings-text";

export function ChannelStatusControl({ channel }: { channel: AdminChannel }) {
  const { text } = useSettingsText();
  const mutation = useSetChannelStatus();
  const resourceMutation = useSetChannelResourceStatus();
  const [open, setOpen] = useState(false);
  const pending = mutation.isPending || resourceMutation.isPending;
  const status = channelResourceStatus(channel);
  const resources = channel.resource_state;
  if (!status || !resources)
    return (
      <span role="alert">
        {text(
          "资源状态待确认，请刷新。",
          "Resource state unavailable. Please refresh.",
          "حالة الموارد غير متاحة. يرجى التحديث.",
        )}
      </span>
    );
  const color =
    status === "enabled"
      ? "success"
      : status === "partial"
        ? "warning"
        : "danger";
  const label =
    status === "enabled"
      ? text("启用", "Enabled", "مُفعّل")
      : status === "partial"
        ? text("部分启用", "Partially enabled", "مُفعّل جزئيًا")
        : text("关闭", "Closed", "متوقف");
  const summary =
    label +
    (status === "partial" ? ` (${resources.enabled}/${resources.total})` : "");
  const action = text(
    `${summary}，管理渠道 ${channel.name} 的资源`,
    `${summary}. Manage resources in channel ${channel.name}`,
    `${summary}. إدارة موارد القناة ${channel.name}`,
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
        aria-haspopup="dialog"
        aria-expanded={open}
        disabled={pending}
        onClick={() => setOpen(true)}
      >
        {summary}
        <ChevronDown size={12} aria-hidden="true" />
      </Button>
      <Modal
        open={open}
        onOpenChange={(next) => {
          if (!pending) setOpen(next);
        }}
        title={text(
          `渠道资源：${channel.name}`,
          `Channel resources: ${channel.name}`,
          `موارد القناة: ${channel.name}`,
        )}
        description={text(
          "可批量或按模型分别启停，操作后立即生效。",
          "Enable or close all resources, or toggle individual models. Changes apply immediately.",
          "فعّل جميع الموارد أو أوقفها، أو تحكّم بكل نموذج على حدة. تُطبّق التغييرات فورًا.",
        )}
        footer={
          <Button disabled={pending} onClick={() => setOpen(false)}>
            {text("完成", "Done", "تم")}
          </Button>
        }
      >
        <p role="status">
          {text(
            `已启用 ${resources.enabled}/${resources.total} 项资源`,
            `${resources.enabled}/${resources.total} resources enabled`,
            `${resources.enabled}/${resources.total} من الموارد مفعّلة`,
          )}
        </p>
        <div
          style={{ display: "flex", flexWrap: "wrap", gap: 8, marginBlock: 12 }}
        >
          <Button
            size="small"
            data-channel-bulk="enable"
            disabled={pending || !resources.total || status === "enabled"}
            onClick={() => mutation.mutate({ id: channel.id, enabled: true })}
          >
            {text("全部启用", "Enable all", "تفعيل الكل")}
          </Button>
          <Button
            size="small"
            data-channel-bulk="close"
            disabled={pending || !resources.total || channel.status !== 1}
            onClick={() => mutation.mutate({ id: channel.id, enabled: false })}
          >
            {text("全部关闭", "Close all", "إيقاف الكل")}
          </Button>
        </div>
        {channel.status !== 1 && (
          <p>
            {text(
              "渠道已关闭，请先点击“全部启用”。",
              "This channel is closed. Select Enable all first.",
              "القناة متوقفة. اختر تفعيل الكل أولًا.",
            )}
          </p>
        )}
        {resources.resources.length ? (
          resources.resources.map((resource) => (
            <div
              key={resource.model}
              data-resource-model={resource.model}
              style={{
                display: "flex",
                justifyContent: "space-between",
                alignItems: "center",
                gap: 12,
                paddingBlock: 10,
                borderBottom: "1px solid var(--border)",
              }}
            >
              <span
                dir="auto"
                style={{ minWidth: 0, overflowWrap: "anywhere" }}
              >
                {resource.model}
              </span>
              <Button
                size="small"
                role="switch"
                aria-checked={resource.enabled}
                aria-label={text(
                  `启用资源 ${resource.model}`,
                  `Enable resource ${resource.model}`,
                  `تفعيل المورد ${resource.model}`,
                )}
                disabled={pending || channel.status !== 1}
                onClick={() =>
                  resourceMutation.mutate({
                    id: channel.id,
                    model: resource.model,
                    enabled: !resource.enabled,
                  })
                }
              >
                {resource.enabled
                  ? text("启用", "Enabled", "مُفعّل")
                  : text("关闭", "Closed", "متوقف")}
              </Button>
            </div>
          ))
        ) : (
          <p>
            {text(
              "暂无可操作资源；托管渠道需先完成发布。",
              "No resources to manage. Managed channels need a published configuration.",
              "لا توجد موارد لإدارتها. تحتاج القنوات المُدارة إلى إعداد منشور.",
            )}
          </p>
        )}
        {(mutation.isError || resourceMutation.isError) && (
          <p role="alert">
            {text(
              "状态更新失败，当前状态已保留，请重试。",
              "Update failed. The current state was preserved. Please retry.",
              "فشل التحديث. تم الاحتفاظ بالحالة الحالية. أعد المحاولة.",
            )}
          </p>
        )}
      </Modal>
      {(mutation.isSuccess || resourceMutation.isSuccess) && (
        <span className="sr-only" role="status">
          {text(
            "资源状态已更新",
            "Resource status updated",
            "تم تحديث حالة الموارد",
          )}
        </span>
      )}
      {(mutation.isError || resourceMutation.isError) && !open && (
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

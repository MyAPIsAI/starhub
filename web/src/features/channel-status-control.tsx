import { useState } from "react";
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
      <div style={{ display: "flex", flexWrap: "wrap", gap: 4 }}>
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
          loading={pending}
          disabled={!resources.total}
          onClick={() => mutation.mutate({ id: channel.id, enabled })}
        >
          {status === "enabled"
            ? text("启用", "Enabled", "مُفعّل")
            : status === "partial"
              ? text("部分启用", "Partially enabled", "مُفعّل جزئيًا")
              : text("关闭", "Closed", "متوقف")}
          {status === "partial" && ` (${resources.enabled}/${resources.total})`}
        </Button>
        <Button
          size="small"
          variant="ghost"
          onClick={() => setOpen(true)}
          data-channel-resource-manage
          disabled={pending}
          aria-label={text(
            `管理渠道 ${channel.name} 的资源`,
            `Manage resources in channel ${channel.name}`,
            `إدارة موارد القناة ${channel.name}`,
          )}
        >
          {text("资源", "Resources", "الموارد")}
        </Button>
      </div>
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
          "按模型分别启停，保存后立即生效；不改变密钥状态和投放授权。",
          "Toggle each model immediately. Key status and publication grants stay unchanged.",
          "فعّل كل نموذج أو أوقفه فورًا دون تغيير حالة المفاتيح أو صلاحيات النشر.",
        )}
        footer={
          <Button disabled={pending} onClick={() => setOpen(false)}>
            {text("关闭", "Close", "إغلاق")}
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
        {channel.status !== 1 && (
          <p>
            {text(
              "渠道已关闭，请先在渠道列表启用全部资源。",
              "This channel is closed. Enable all resources from the channel list first.",
              "القناة متوقفة. فعّل جميع الموارد من قائمة القنوات أولًا.",
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

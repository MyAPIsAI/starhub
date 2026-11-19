import { Link } from "react-router-dom";
import { Button } from "../components/ui/button";
import { useAdminChannels } from "../lib/api/admin";
import { channelResourceStatus } from "../lib/channel-status";
import { isArkVideoModel } from "./ark-video-models";
import { useSettingsText } from "./settings-text";

export function ArkVideoIntegration() {
  const { text } = useSettingsText();
  const query = useAdminChannels();
  const channels =
    query.data?.filter(
      (channel) =>
        channel.type === 32 &&
        channel.models
          .split(",")
          .some(
            (id) =>
              isArkVideoModel(id.trim()) ||
              channel.model_capabilities?.[id.trim()] === "VideoGeneration",
          ),
    ) ?? [];
  return (
    <article className="task-plugin-native">
      <div>
        <strong>
          {text("豆包 Seedance", "Doubao Seedance", "Doubao Seedance")}
        </strong>
        <p>
          {text(
            "内置适配，无需上传插件。通过火山方舟渠道接入视频生成。",
            "Built-in support. Configure a Volcengine Ark channel to generate video without uploading a plugin.",
            "دعم مدمج. أعدّ قناة Volcengine Ark لتوليد الفيديو دون رفع إضافة.",
          )}
        </p>
        {query.isPending ? (
          <p role="status">
            {text("正在读取渠道…", "Loading channels…", "جارٍ تحميل القنوات…")}
          </p>
        ) : query.isError ? (
          <div role="alert">
            {text(
              "渠道状态加载失败。",
              "Channel status is unavailable.",
              "تعذر تحميل حالة القنوات.",
            )}
            <Button
              size="small"
              disabled={query.isFetching}
              onClick={() => void query.refetch()}
            >
              {text("重试", "Retry", "إعادة المحاولة")}
            </Button>
          </div>
        ) : channels.length ? (
          <dl className="task-plugin-native-channels">
            {channels.map((channel) => {
              const status = channelResourceStatus(channel);
              return (
                <div key={channel.id} data-native-channel={channel.id}>
                  <dt>{channel.name}</dt>
                  <dd>
                    {!status
                      ? text(
                          "状态待确认",
                          "Status unconfirmed",
                          "الحالة غير مؤكدة",
                        )
                      : status === "enabled"
                        ? text("启用", "Enabled", "مُفعّل")
                        : status === "partial"
                          ? text(
                              "部分启用",
                              "Partially enabled",
                              "مُفعّل جزئيًا",
                            )
                          : text("关闭", "Closed", "متوقف")}
                  </dd>
                </div>
              );
            })}
          </dl>
        ) : (
          <p>
            {text(
              "尚未配置豆包视频渠道。",
              "No Doubao video channels configured.",
              "لم تُعدّ قنوات فيديو Doubao بعد.",
            )}
          </p>
        )}
      </div>
      <div className="task-plugin-actions">
        <Button asChild>
          <Link to="/admin?tab=resources&create_channel=ark#channels">
            {text("配置渠道", "Configure channel", "إعداد القناة")}
          </Link>
        </Button>
        <Button asChild>
          <Link to="/admin?tab=resources#channels">
            {text("管理渠道", "Manage channels", "إدارة القنوات")}
          </Link>
        </Button>
      </div>
    </article>
  );
}

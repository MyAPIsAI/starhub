import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { Button } from "../components/ui/button";
import type { AdminChannel } from "../lib/api/admin";
import { apiGet } from "../lib/api/http";
import { channelResourceStatus } from "../lib/channel-status";
import { channelTypeOption } from "../lib/channel-types";
import {
  isAdmin,
  useSessionQueryIdentity,
  useSystemAccess,
} from "../lib/system-access";
import { useSettingsText } from "./settings-text";
import "./resource-publication.css";

export function PlatformChannelList() {
  const identity = useSessionQueryIdentity();
  return <PlatformChannelListView key={JSON.stringify(identity)} />;
}

function PlatformChannelListView() {
  const { text } = useSettingsText();
  const identity = useSessionQueryIdentity();
  const role = useSystemAccess((s) => s.role);
  const account = useSystemAccess((s) => s.session.accountId);
  const enabled = account !== null && isAdmin(role);
  const query = useQuery({
    queryKey: ["admin-channels", identity],
    enabled,
    retry: false,
    queryFn: ({ signal }) => apiGet<AdminChannel[]>("/api/channel", signal),
  });
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);
  if (!enabled) return null;
  const channels = query.error ? undefined : query.data;
  const rows = (channels ?? []).filter((channel) =>
    [String(channel.id), channel.name, channel.models, channel.group].some(
      (value) => value.toLowerCase().includes(search.trim().toLowerCase()),
    ),
  );
  const pages = Math.max(1, Math.ceil(rows.length / 20));
  const current = Math.min(page, pages - 1);
  const statusLabel = (channel: AdminChannel) => {
    const status = channelResourceStatus(channel);
    return status === "enabled"
      ? text("启用", "Enabled", "مُفعّل")
      : status === "partial"
        ? text(
            `部分启用 (${channel.resource_state!.enabled}/${channel.resource_state!.total})`,
            `Partially enabled (${channel.resource_state!.enabled}/${channel.resource_state!.total})`,
            `مُفعّل جزئيًا (${channel.resource_state!.enabled}/${channel.resource_state!.total})`,
          )
        : status === "closed"
          ? text("关闭", "Closed", "متوقف")
          : text("状态待确认", "State unavailable", "الحالة غير متاحة");
  };
  return (
    <section
      id="platform-channels"
      className="rp-result"
      aria-label={text("当前平台渠道", "Platform channels", "قنوات المنصة")}
    >
      <header>
        <div>
          <h3>
            {text("当前平台渠道", "Platform channels", "قنوات المنصة")}
            {channels && ` · ${channels.length}`}
          </h3>
          <p>
            {text(
              "展示当前平台已创建的真实渠道，包含未绑定来源快照的渠道。",
              "Real channels created on this platform, including channels without an inventory source binding.",
              "القنوات الفعلية المنشأة على هذه المنصة، بما فيها القنوات غير المرتبطة بمصدر مخزون.",
            )}
          </p>
        </div>
        <div className="rp-actions">
          <Button
            disabled={query.isFetching}
            onClick={() => void query.refetch()}
          >
            {text("刷新渠道", "Refresh channels", "تحديث القنوات")}
          </Button>
          <Button asChild>
            <Link to="/admin?tab=resources#channels">
              {text("管理渠道", "Manage channels", "إدارة القنوات")}
            </Link>
          </Button>
        </div>
      </header>
      {query.isLoading && (
        <p role="status">
          {text(
            "正在读取平台渠道…",
            "Loading platform channels…",
            "جارٍ تحميل قنوات المنصة…",
          )}
        </p>
      )}
      {query.error && (
        <p role="alert">
          {text(
            "无法读取平台渠道，请刷新后重试。",
            "Platform channels are unavailable. Refresh to retry.",
            "قنوات المنصة غير متاحة. حدّث لإعادة المحاولة.",
          )}
        </p>
      )}
      {channels && (
        <>
          <label className="rp-search">
            {text(
              "搜索平台渠道",
              "Search platform channels",
              "البحث في قنوات المنصة",
            )}
            <input
              value={search}
              onChange={(event) => {
                setSearch(event.target.value);
                setPage(0);
              }}
            />
          </label>
          <div
            className="rp-table"
            role="region"
            tabIndex={0}
            aria-label={text(
              "平台渠道列表",
              "Platform channel list",
              "قائمة قنوات المنصة",
            )}
          >
            <table>
              <thead>
                <tr>
                  <th>{text("名称 / 标识", "Name / ID", "الاسم / المعرّف")}</th>
                  <th>{text("类型", "Type", "النوع")}</th>
                  <th>{text("模型", "Models", "النماذج")}</th>
                  <th>{text("分组", "Groups", "المجموعات")}</th>
                  <th>{text("资源状态", "Resource state", "حالة الموارد")}</th>
                </tr>
              </thead>
              <tbody>
                {rows.slice(current * 20, (current + 1) * 20).map((channel) => {
                  const type = channelTypeOption(channel.type);
                  return (
                    <tr key={channel.id} data-platform-channel={channel.id}>
                      <td>
                        <strong>
                          {channel.name ||
                            text(
                              "未命名渠道",
                              "Unnamed channel",
                              "قناة بلا اسم",
                            )}
                        </strong>
                        <code>{channel.id}</code>
                      </td>
                      <td>{type ? text(...type.label) : channel.type}</td>
                      <td>
                        <code>{channel.models || "—"}</code>
                      </td>
                      <td>
                        <code>{channel.group || "—"}</code>
                      </td>
                      <td>{statusLabel(channel)}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
          {rows.length === 0 && (
            <p>
              {channels.length === 0
                ? text(
                    "当前平台尚未创建渠道。",
                    "No channels have been created on this platform.",
                    "لم تُنشأ قنوات على هذه المنصة بعد.",
                  )
                : text(
                    "没有匹配的平台渠道。",
                    "No matching platform channels.",
                    "لا توجد قنوات منصة مطابقة.",
                  )}
            </p>
          )}
          {pages > 1 && (
            <nav
              className="rp-actions"
              aria-label={text(
                "平台渠道分页",
                "Platform channel pagination",
                "صفحات قنوات المنصة",
              )}
            >
              <Button
                disabled={current === 0}
                onClick={() => setPage(current - 1)}
              >
                {text("上一页", "Previous", "السابق")}
              </Button>
              <span>
                {current + 1} / {pages} · {rows.length}
              </span>
              <Button
                disabled={current + 1 >= pages}
                onClick={() => setPage(current + 1)}
              >
                {text("下一页", "Next", "التالي")}
              </Button>
            </nav>
          )}
        </>
      )}
    </section>
  );
}

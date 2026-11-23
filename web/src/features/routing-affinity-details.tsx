import { useEffect, useState } from "react";
import { Modal } from "../components/ui/overlay";
import { Button } from "../components/ui/button";
import { useAffinityDetails } from "../lib/api/routing-affinity";
import type { PublicationChannel } from "../lib/resource-publication";
import { useSettingsText } from "./settings-text";

export function AffinityDetails({
  rule,
  instance,
  channels,
  close,
}: {
  rule: string;
  instance: string;
  channels?: PublicationChannel[];
  close: () => void;
}) {
  const { text } = useSettingsText();
  const [page, setPage] = useState(1);
  const query = useAffinityDetails(instance, rule, page);
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);
  const sample = query.isError ? undefined : query.data;
  const valid = sample?.instance === instance && Array.isArray(sample.entries);
  const observedNow = sample
    ? sample.observed_at + Math.max(0, now - query.dataUpdatedAt)
    : 0;
  const live = valid
    ? sample.entries.filter((row) => row.expires_at > observedNow)
    : [];
  const names = new Map(channels?.map((c) => [c.id, c.name]));
  return (
    <Modal
      open
      title={text(
        "本副本亲和明细",
        "Replica affinity details",
        "تفاصيل تقارب النسخة",
      )}
      description={`${rule} · ${instance}`}
      onOpenChange={(v) => !v && close()}
      footer={<Button onClick={close}>{text("关闭", "Close", "إغلاق")}</Button>}
    >
      {query.isLoading ? (
        <p role="status">{text("正在读取…", "Loading…", "جارٍ التحميل…")}</p>
      ) : !valid ? (
        <p role="alert">
          {text(
            "无法读取同一副本的亲和明细，请刷新。",
            "Could not read details from the same replica. Refresh to retry.",
            "تعذّرت قراءة التفاصيل من النسخة نفسها. أعد التحديث.",
          )}
        </p>
      ) : (
        <>
          {live.length ? (
            <ul>
              {live.map((row, i) => (
                <li key={i}>
                  <bdi>
                    #{row.channel_id} · {names.get(row.channel_id) || "—"}
                  </bdi>{" "}
                  · {text("剩余", "Remaining", "المتبقي")}{" "}
                  {Math.ceil((row.expires_at - observedNow) / 1000)}{" "}
                  {text("秒", "s", "ث")}
                </li>
              ))}
            </ul>
          ) : (
            <p role="status">
              {text(
                "条目已过期或已移除，请刷新。",
                "Entries expired or were removed. Refresh to check.",
                "انتهت صلاحية الروابط أو أزيلت. حدّث للتحقق.",
              )}
            </p>
          )}
          {(page > 1 || sample.total > 50) && (
            <div>
              <Button disabled={page === 1} onClick={() => setPage(page - 1)}>
                {text("上一页", "Previous", "السابق")}
              </Button>{" "}
              {page}{" "}
              <Button
                disabled={page * 50 >= sample.total}
                onClick={() => setPage(page + 1)}
              >
                {text("下一页", "Next", "التالي")}
              </Button>
            </div>
          )}
        </>
      )}
      <Button disabled={query.isFetching} onClick={() => void query.refetch()}>
        {text("刷新明细", "Refresh details", "تحديث التفاصيل")}
      </Button>
    </Modal>
  );
}

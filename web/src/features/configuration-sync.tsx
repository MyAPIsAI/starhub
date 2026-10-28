import { useTranslation } from "react-i18next";
import { RefreshCw } from "lucide-react";
import { Button } from "../components/ui/button";
import { useOptionStatus, useReloadOptions } from "../lib/api/option";
import { ROLE_ADMIN, ROLE_ROOT, useSystemAccess } from "../lib/system-access";
import "./configuration-sync.css";

export function ConfigurationSync() {
  const { i18n } = useTranslation();
  const text = (zh: string, en: string, ar: string) =>
    i18n.language === "zh" ? zh : i18n.language === "ar" ? ar : en;
  const role = useSystemAccess((s) => s.role);
  const query = useOptionStatus(5000);
  const reload = useReloadOptions();
  if (role !== ROLE_ADMIN && role !== ROLE_ROOT) return null;
  const data = query.error ? undefined : query.data;
  const date = (timestamp: number) =>
    new Intl.DateTimeFormat(i18n.language, {
      dateStyle: "short",
      timeStyle: "medium",
      timeZone: "UTC",
    }).format(new Date(timestamp * 1000));
  return (
    <section className="configuration-sync">
      <header>
        <div>
          <h2>
            {text(
              "配置同步状态",
              "Configuration synchronization",
              "مزامنة الإعدادات",
            )}
          </h2>
          <p>
            {text(
              "重载操作作用于处理本次请求的进程；表格显示近期心跳上报。",
              "Reload applies to the process serving this request. The table shows recent heartbeat reports.",
              "تطبق إعادة التحميل على العملية التي تعالج الطلب. يعرض الجدول تقارير النبض الحديثة.",
            )}
          </p>
        </div>
        <div className="configuration-sync-actions">
          <Button
            variant="secondary"
            disabled={query.isFetching}
            onClick={() => void query.refetch()}
          >
            {text("刷新状态", "Refresh status", "تحديث الحالة")}
          </Button>
          <Button disabled={reload.isPending} onClick={() => reload.mutate()}>
            <RefreshCw size={15} />
            {reload.isPending
              ? text("正在重载…", "Reloading…", "جارٍ إعادة التحميل…")
              : text(
                  "重载本进程配置",
                  "Reload serving process",
                  "إعادة تحميل العملية الحالية",
                )}
          </Button>
        </div>
      </header>
      {query.isLoading && (
        <p role="status">
          {text(
            "正在读取配置状态…",
            "Loading configuration status…",
            "جارٍ تحميل حالة الإعدادات…",
          )}
        </p>
      )}
      {query.error && (
        <p role="alert">
          {text(
            "无法读取配置状态，请稍后重试。",
            "Configuration status is unavailable. Retry later.",
            "حالة الإعدادات غير متاحة. حاول لاحقًا.",
          )}
        </p>
      )}
      {reload.error && (
        <p role="alert">
          {text(
            "无法确认重载结果，请刷新状态核对；其他实例以各自上报为准。",
            "The reload result could not be confirmed. Refresh status to check; other instances report independently.",
            "تعذر تأكيد نتيجة إعادة التحميل. حدّث الحالة للتحقق؛ وتبلغ العمليات الأخرى بشكل مستقل.",
          )}
        </p>
      )}
      {reload.data && !reload.error && (
        <p className="configuration-sync-notice" role="status">
          {text(
            `上次重载响应：进程加载版本 v${reload.data.loaded_version}，数据库版本 v${reload.data.database_version}。`,
            `Last reload response: process loaded v${reload.data.loaded_version}; database v${reload.data.database_version}.`,
            `آخر استجابة: حمّلت العملية v${reload.data.loaded_version}؛ وقاعدة البيانات v${reload.data.database_version}.`,
          )}
        </p>
      )}
      {data && (
        <>
          <dl className="configuration-sync-versions">
            <div>
              <dt>
                {text("数据库版本", "Database version", "إصدار قاعدة البيانات")}
              </dt>
              <dd>v{data.database_version}</dd>
            </div>
            <div>
              <dt>
                {text(
                  "当前响应进程的加载版本",
                  "Serving process loaded version",
                  "إصدار العملية المستجيبة",
                )}
              </dt>
              <dd>v{data.loaded_version}</dd>
              <small data-local-sync={data.in_sync ? "current" : "different"}>
                {data.in_sync
                  ? text(
                      "本响应进程与数据库一致",
                      "Serving process matches database",
                      "العملية مطابقة لقاعدة البيانات",
                    )
                  : text(
                      "本响应进程尚未加载当前数据库版本",
                      "Serving process has a different version",
                      "للعملية إصدار مختلف",
                    )}
              </small>
            </div>
          </dl>
          {data.instances.length === 0 ? (
            <p className="configuration-sync-empty">
              {text(
                "暂无活跃实例心跳，整体同步状态未知。",
                "No active instance heartbeats; overall synchronization is unknown.",
                "لا توجد نبضات نشطة؛ حالة المزامنة العامة غير معروفة.",
              )}
            </p>
          ) : (
            <div
              className="configuration-sync-table"
              tabIndex={0}
              role="region"
              aria-label={text(
                "实例配置心跳",
                "Instance configuration heartbeats",
                "نبضات إعدادات العمليات",
              )}
            >
              <table>
                <thead>
                  <tr>
                    <th>{text("实例标识", "Instance ID", "معرّف العملية")}</th>
                    <th>
                      {text(
                        "上报版本",
                        "Reported version",
                        "الإصدار المبلغ عنه",
                      )}
                    </th>
                    <th>
                      {text("版本比较", "Version comparison", "مقارنة الإصدار")}
                    </th>
                    <th>
                      {text(
                        "最后心跳（UTC）",
                        "Last heartbeat (UTC)",
                        "آخر نبضة (UTC)",
                      )}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {data.instances.map((instance) => (
                    <tr key={instance.id}>
                      <td>
                        <code>{instance.id}</code>
                      </td>
                      <td>v{instance.version}</td>
                      <td
                        data-peer-sync={
                          instance.version === data.database_version
                            ? "current"
                            : "different"
                        }
                      >
                        {instance.version === data.database_version
                          ? text(
                              "与数据库一致",
                              "Matches database",
                              "مطابق لقاعدة البيانات",
                            )
                          : text(
                              "与数据库版本不同",
                              "Different version",
                              "إصدار مختلف",
                            )}
                      </td>
                      <td>{date(instance.last_seen)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}
    </section>
  );
}

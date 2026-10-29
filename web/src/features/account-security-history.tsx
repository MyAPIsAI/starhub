import { History } from "lucide-react";
import { Button } from "../components/ui/button";
import { useUserSecurityHistory } from "../lib/api/user";
import { ApiError } from "../lib/api/http";
import { usePreferences } from "../lib/preferences";
import { useSystemAccess } from "../lib/system-access";

export function AccountSecurityHistory() {
  const query = useUserSecurityHistory();
  const events = query.isError ? [] : (query.data ?? []);
  const language = usePreferences((state) => state.language);
  const owner = useSystemAccess((state) => state.session.accountId);
  const c = (zh: string, en: string, ar: string) =>
    language === "zh" ? zh : language === "ar" ? ar : en;
  const mismatch =
    query.error instanceof ApiError && query.error.status === 409;
  const date = (value: number) =>
    new Intl.DateTimeFormat(
      language === "zh" ? "zh-CN" : language === "ar" ? "ar" : "en",
      { dateStyle: "medium", timeStyle: "short" },
    ).format(new Date(value * 1000));
  return (
    <section className="ac-card" aria-labelledby="ac-history-title">
      <div className="ac-section-heading">
        <div>
          <h2 id="ac-history-title">
            <History size={18} />
            {c(
              "最近安全操作",
              "Recent security activity",
              "نشاط الأمان الأخير",
            )}
          </h2>
          <p>
            {c(
              "最近5条；仅覆盖本功能启用后通过此页面成功的密码、邮箱修改。更早历史、其他入口和设备操作不在此范围。",
              "Latest 5 changes. Covers successful password and email changes made here after history was enabled. Earlier actions, other entry points and device operations are outside this history.",
              "آخر 5 تغييرات ناجحة لكلمة المرور والبريد من هذه الصفحة بعد تفعيل السجل. لا يشمل الإجراءات الأقدم أو المداخل الأخرى أو عمليات الأجهزة.",
            )}
          </p>
        </div>
      </div>
      {events.length > 0 && (
        <ol className="ac-events">
          {events.map((event) => (
            <li key={event.id}>
              <span className="ac-event-dot" />
              <div>
                <strong>
                  {event.action === "password_changed"
                    ? c("修改登录密码", "Password changed", "تغيير كلمة المرور")
                    : c(
                        "更新资料邮箱",
                        "Profile email changed",
                        "تغيير بريد الحساب",
                      )}
                </strong>
                <time
                  dateTime={new Date(event.created_at * 1000).toISOString()}
                >
                  {date(event.created_at)}
                </time>
              </div>
            </li>
          ))}
        </ol>
      )}
      {query.isLoading && (
        <p role="status" className="ac-history-status">
          {c(
            "正在读取安全历史…",
            "Loading security history…",
            "جارٍ تحميل سجل الأمان…",
          )}
        </p>
      )}
      {query.isError && (
        <p role="alert" className="ac-history-status">
          {mismatch
            ? c(
                "登录账号可能已变化，请重新确认当前账号。",
                "The signed-in account may have changed. Confirm the current account.",
                "ربما تغير الحساب المسجل. أكّد الحساب الحالي.",
              )
            : c(
                "暂时无法读取安全历史，旧记录已隐藏。",
                "Security history is unavailable. Previous records are hidden.",
                "سجل الأمان غير متاح. أُخفيت السجلات السابقة.",
              )}
        </p>
      )}
      {!query.isError &&
        !query.isLoading &&
        query.isSuccess &&
        events.length === 0 && (
          <p className="ac-history-status">
            {c(
              "此记录范围内暂无操作。",
              "No changes recorded in this scope.",
              "لا تغييرات مسجلة ضمن هذا النطاق.",
            )}
          </p>
        )}
      <div className="ac-device-actions">
        <Button
          size="small"
          variant="ghost"
          disabled={query.isFetching || owner === null}
          onClick={() =>
            mismatch ? window.location.reload() : void query.refetch()
          }
        >
          {mismatch
            ? c(
                "确认当前账号",
                "Confirm current account",
                "تأكيد الحساب الحالي",
              )
            : c(
                "重新读取安全历史",
                "Reload security history",
                "إعادة تحميل سجل الأمان",
              )}
        </Button>
      </div>
    </section>
  );
}

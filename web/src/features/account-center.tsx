import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { Check, Copy, Info, LogOut, Monitor, UserRound } from "lucide-react";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/primitives";
import { AccountSecurity } from "./account-security";
import { AccountSecurityHistory } from "./account-security-history";
import { useProductPreview } from "../lib/product-preview";
import {
  useSystemAccess,
  useSessionQueryIdentity,
  sessionUsername,
} from "../lib/system-access";
import { logoutUser } from "../lib/api/auth";
import {
  useSessions,
  useUserProfile,
  useTerminateOtherSessions,
} from "../lib/api/user";
import { ApiError } from "../lib/api/http";
import { usePreferences } from "../lib/preferences";
import "./account-center.css";

export default function AccountCenter(props: { onDeactivate: () => void }) {
  const identity = useSessionQueryIdentity();
  return <AccountWorkspace key={JSON.stringify(identity)} {...props} />;
}

function AccountWorkspace({ onDeactivate }: { onDeactivate: () => void }) {
  const sessionsQuery = useSessions();
  const profileQuery = useUserProfile();
  const profile = profileQuery.isError ? undefined : profileQuery.data;
  const profileOwnerChanged =
    profileQuery.error instanceof ApiError && profileQuery.error.status === 409;
  const deviceOwnerChanged =
    sessionsQuery.error instanceof ApiError &&
    sessionsQuery.error.status === 409;
  const terminateOthersMut = useTerminateOtherSessions();
  const backendSessions = sessionsQuery.isError
    ? []
    : (sessionsQuery.data ?? []);
  const logout = useProductPreview((state) => state.logout);
  const queryClient = useQueryClient();
  const session = useSystemAccess((state) => state.session);
  const language = usePreferences((state) => state.language);
  const navigate = useNavigate();
  const c = (zh: string, en: string, ar: string) =>
    language === "zh" ? zh : language === "ar" ? ar : en;
  const [notice, setNotice] = useState("");
  const [copied, setCopied] = useState(false);
  const profileName =
    sessionUsername(session) ?? c("账号中心", "Account center", "مركز الحساب");
  const accountId =
    session.accountId === null ? null : String(session.accountId);
  const otherSessions = backendSessions.some((item) => !item.is_current);
  const formatDate = (value: string) =>
    new Intl.DateTimeFormat(
      language === "zh" ? "zh-CN" : language === "ar" ? "ar" : "en",
      {
        year: "numeric",
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      },
    ).format(new Date(value));
  async function copyId() {
    if (accountId === null) return;
    try {
      await navigator.clipboard.writeText(accountId);
      setCopied(true);
      setNotice(
        c("用户 ID（标识）已复制。", "User ID copied.", "نُسخ معرّف المستخدم."),
      );
    } catch {
      setNotice(
        c(
          "未能自动复制，请在用户 ID（标识）输入框中选中并复制。",
          "Copy failed. Select and copy the user ID from its field.",
          "تعذر النسخ. حدد معرّف المستخدم وانسخه من الحقل.",
        ),
      );
    }
  }
  async function signOut() {
    try {
      await logoutUser();
    } catch {
      /* Local cleanup also runs when the server is unreachable. */
    }
    useSystemAccess.getState().setSession(null);
    logout();
    queryClient.clear();
    navigate("/login");
  }
  return (
    <div className="account-center">
      <header className="ac-heading">
        <div>
          <h1>{c("账号中心", "Account center", "مركز الحساب")}</h1>
          <p>
            {c(
              "管理账号身份、登录方式与安全信息。",
              "Manage your identity, sign-in methods and account security.",
              "إدارة هويتك وطرق الدخول وأمان الحساب.",
            )}
          </p>
        </div>
        <span className="ac-demo-tag">
          {c("服务器账号", "Server account", "حساب الخادم")}
        </span>
      </header>
      {notice && (
        <div role="status" className="ac-notice">
          <Info size={16} />
          <span>{notice}</span>
        </div>
      )}
      <section
        className="ac-card ac-profile"
        aria-labelledby="ac-profile-title"
      >
        <div className="ac-profile-heading">
          <div className="ac-avatar" aria-hidden="true">
            <UserRound size={25} />
          </div>
          <div>
            <h2 id="ac-profile-title">{profileName}</h2>
            <p>{c("账号资料", "Account profile", "بيانات الحساب")}</p>
          </div>
          <span
            className={`ac-badge ${profile?.status === 1 ? "ac-badge--success" : "ac-badge--warning"}`}
          >
            {profile?.status === 1
              ? c("正常", "Active", "نشط")
              : profile?.status === 2
                ? c("已停用", "Disabled", "معطل")
                : c("未确认", "Unconfirmed", "غير مؤكد")}
          </span>
        </div>
        <div className="ac-identity-grid">
          <div className="ac-field">
            <label htmlFor="account-username">
              {sessionUsername(session)
                ? c("用户名", "Username", "اسم المستخدم")
                : c("登录账号", "Login account", "حساب الدخول")}
            </label>
            <Input id="account-username" value={profileName} readOnly />
            <small>
              {c(
                "用于登录，当前版本不可修改。",
                "Your sign-in name; editing is unavailable in this version.",
                "اسم الدخول؛ لا يمكن تعديله في هذا الإصدار.",
              )}
            </small>
          </div>
          <div className="ac-field">
            <label htmlFor="account-id">
              {c("用户 ID（标识）", "User ID", "معرّف المستخدم")}
            </label>
            <div className="ac-id">
              <Input
                id="account-id"
                value={accountId ?? "—"}
                readOnly
                dir="ltr"
              />
              <Button
                size="icon"
                aria-label={c(
                  "复制用户 ID（标识）",
                  "Copy user ID",
                  "نسخ معرّف المستخدم",
                )}
                onClick={copyId}
                disabled={accountId === null}
              >
                {copied ? <Check size={16} /> : <Copy size={16} />}
              </Button>
            </div>
            <small>
              {c(
                "账号的固定标识，用于定位问题与核对记录。",
                "A permanent identifier for support and record matching.",
                "معرّف ثابت لتحديد الحساب ومطابقة السجلات.",
              )}
            </small>
          </div>
          <div className="ac-meta">
            <span>{c("账号身份", "Account role", "دور الحساب")}</span>
            <strong>
              {profile?.role === 100
                ? c("超级管理员", "Root", "المسؤول الأعلى")
                : profile?.role === 10
                  ? c("平台管理员", "Administrator", "مدير المنصة")
                  : profile?.role === 5
                    ? c("财务", "Finance", "المالية")
                    : profile?.role === 1
                      ? c("普通用户", "User", "مستخدم")
                      : c("未确认", "Unconfirmed", "غير مؤكد")}
            </strong>
            <small>
              {c(
                "平台权限与服务分组授权分别管理。",
                "Platform permissions and service-group access are separate.",
                "تُدار أذونات المنصة والوصول إلى مجموعات الخدمة بشكل منفصل.",
              )}
            </small>
          </div>
          <div className="ac-meta">
            <span>{c("注册时间", "Created", "تاريخ الإنشاء")}</span>
            <strong>
              {profile?.created_at
                ? formatDate(new Date(profile.created_at * 1000).toISOString())
                : profile
                  ? c(
                      "历史账号，时间未记录",
                      "Existing account; date not recorded",
                      "حساب سابق؛ التاريخ غير مسجل",
                    )
                  : c("未确认", "Unconfirmed", "غير مؤكد")}
            </strong>
            <small>
              {c(
                "时间按当前设备时区显示。",
                "Times use your device's time zone.",
                "الأوقات حسب المنطقة الزمنية لجهازك.",
              )}
            </small>
          </div>
        </div>
        {profileQuery.isLoading && (
          <p role="status" className="ac-empty-hint">
            {c(
              "正在读取账号资料…",
              "Loading account profile…",
              "جارٍ تحميل بيانات الحساب…",
            )}
          </p>
        )}
        {profileQuery.isError && (
          <p role="alert" className="ac-empty-hint">
            {profileOwnerChanged
              ? c(
                  "登录账号可能已变化，请重新确认当前账号。",
                  "The signed-in account may have changed. Confirm the current account.",
                  "ربما تغير الحساب المسجل. أكّد الحساب الحالي.",
                )
              : c(
                  "暂时无法读取账号资料，旧资料已隐藏。",
                  "The account profile is unavailable. Previous details are hidden.",
                  "بيانات الحساب غير متاحة. أُخفيت التفاصيل السابقة.",
                )}
          </p>
        )}
        <div className="ac-device-actions">
          <Button
            size="small"
            variant="ghost"
            disabled={profileQuery.isFetching || session.accountId === null}
            onClick={() =>
              profileOwnerChanged
                ? window.location.reload()
                : void profileQuery.refetch()
            }
          >
            {profileOwnerChanged
              ? c(
                  "确认当前账号",
                  "Confirm current account",
                  "تأكيد الحساب الحالي",
                )
              : c("重新读取资料", "Reload profile", "إعادة تحميل البيانات")}
          </Button>
        </div>
      </section>
      <AccountSecurity profile={profile} />
      <div className="ac-lower-grid">
        <section className="ac-card" aria-labelledby="ac-devices-title">
          <div className="ac-section-heading">
            <div>
              <h2 id="ac-devices-title">
                <Monitor size={18} />
                {c("登录设备", "Signed-in devices", "أجهزة الدخول")}
              </h2>
              <p>
                {c(
                  "活跃会话列表。",
                  "Active sessions for your account.",
                  "الجلسات النشطة لحسابك.",
                )}
              </p>
            </div>
          </div>
          <div className="ac-devices">
            {backendSessions.map((session, i) => (
              <div className="ac-device" key={i}>
                <Monitor size={18} />
                <div>
                  <strong>
                    {session.is_current
                      ? c("当前会话", "Current session", "الجلسة الحالية")
                      : c("其他会话", "Other session", "جلسة أخرى")}
                  </strong>
                  <p dir="ltr">
                    {c("创建于", "Created", "أُنشئت")}{" "}
                    {new Date(session.created_at * 1000).toLocaleString(
                      language === "zh" ? "zh-CN" : "en-GB",
                    )}
                  </p>
                  <small>
                    {c("过期于", "Expires", "تنتهي")}{" "}
                    {new Date(session.expires_at * 1000).toLocaleString(
                      language === "zh" ? "zh-CN" : "en-GB",
                    )}
                  </small>
                </div>
                {session.is_current && (
                  <span className="ac-badge ac-badge--success">
                    {c("当前", "Current", "الحالي")}
                  </span>
                )}
              </div>
            ))}
            {sessionsQuery.isLoading && (
              <p role="status" className="ac-empty-hint">
                {c(
                  "正在读取登录设备…",
                  "Loading account devices…",
                  "جارٍ تحميل أجهزة الحساب…",
                )}
              </p>
            )}
            {sessionsQuery.isError && (
              <div role="alert">
                <p className="ac-empty-hint">
                  {deviceOwnerChanged
                    ? c(
                        "登录账号可能已变化，请重新确认当前账号。",
                        "The signed-in account may have changed. Confirm the current account.",
                        "ربما تغير الحساب المسجل. أكّد الحساب الحالي.",
                      )
                    : c(
                        "暂时无法读取设备列表，旧记录已隐藏。",
                        "The device list is unavailable. Previous records are hidden.",
                        "قائمة الأجهزة غير متاحة. أُخفيت السجلات السابقة.",
                      )}
                </p>
              </div>
            )}
            {!backendSessions.length &&
              !sessionsQuery.isLoading &&
              !sessionsQuery.isError &&
              session.accountId !== null && (
                <p className="ac-empty-hint">
                  {c(
                    "暂无活跃会话。",
                    "No active sessions.",
                    "لا توجد جلسات نشطة.",
                  )}
                </p>
              )}
          </div>
          <div className="ac-device-actions">
            <Button
              size="small"
              variant="ghost"
              onClick={() =>
                deviceOwnerChanged
                  ? window.location.reload()
                  : void sessionsQuery.refetch()
              }
              disabled={
                sessionsQuery.isFetching ||
                terminateOthersMut.isPending ||
                session.accountId === null
              }
            >
              {deviceOwnerChanged
                ? c(
                    "确认当前账号",
                    "Confirm current account",
                    "تأكيد الحساب الحالي",
                  )
                : c("重新读取设备", "Reload devices", "إعادة تحميل الأجهزة")}
            </Button>
            <Button
              size="small"
              disabled={!otherSessions || terminateOthersMut.isPending}
              onClick={() => {
                terminateOthersMut.mutate(undefined, {
                  onSuccess: () => {
                    setNotice(
                      c(
                        "其他设备已下线。",
                        "Other devices were signed out.",
                        "سُجل خروج الأجهزة الأخرى.",
                      ),
                    );
                  },
                  onError: () =>
                    setNotice(
                      c(
                        "未能确认设备操作结果，请重新读取列表后核对。",
                        "The device operation could not be confirmed. Reload the list to check.",
                        "تعذر تأكيد عملية الأجهزة. أعد تحميل القائمة للتحقق.",
                      ),
                    ),
                });
              }}
            >
              {c(
                "下线其他设备",
                "Sign out other devices",
                "خروج الأجهزة الأخرى",
              )}
            </Button>
            <Button size="small" variant="ghost" onClick={() => signOut()}>
              <LogOut size={14} />
              {c("退出当前账号", "Sign out", "تسجيل الخروج")}
            </Button>
          </div>
        </section>
        <AccountSecurityHistory />
      </div>
      <section className="ac-close">
        <div>
          <h2>{c("注销账号", "Close account", "إغلاق الحساب")}</h2>
          <p>
            {c(
              "注销前需处理余额和未完成订单，确认后将无法继续调用。",
              "Resolve your balance and pending orders before closing. Model access stops after confirmation.",
              "سوِّ الرصيد والطلبات المعلقة قبل الإغلاق. سيتوقف استخدام النماذج بعد التأكيد.",
            )}
          </p>
        </div>
        <Button size="small" variant="ghost" onClick={onDeactivate}>
          {c("申请注销", "Close account", "طلب الإغلاق")}
        </Button>
      </section>
    </div>
  );
}

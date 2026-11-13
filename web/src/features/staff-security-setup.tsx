import { useEffect, useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/primitives";
import { getSelf, logoutUser, type AuthUser } from "../lib/api/auth";
import { apiGet, apiPost, apiPut } from "../lib/api/http";
import { usePreferences } from "../lib/preferences";
import { useSystemAccess } from "../lib/system-access";
import { useProductPreview } from "../lib/product-preview";
import "./staff-security-setup.css";

export default function StaffSecuritySetup() {
  const language = usePreferences((s) => s.language);
  const identity = useSystemAccess((s) => s.session);
  const navigate = useNavigate();
  const c = (zh: string, en: string, ar: string) =>
    language === "zh" ? zh : language === "ar" ? ar : en;
  const [user, setUser] = useState<AuthUser | null>(null);
  const [enabled, setEnabled] = useState(false);
  const [password, setPassword] = useState("");
  const [nextPassword, setNextPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [secret, setSecret] = useState("");
  const [code, setCode] = useState("");
  const [backupCodes, setBackupCodes] = useState<string[]>([]);
  const [savedCodes, setSavedCodes] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    const abort = new AbortController();
    setUser(null);
    setPassword("");
    setNextPassword("");
    setConfirmation("");
    setSecret("");
    setCode("");
    setBackupCodes([]);
    setSavedCodes(false);
    setError("");
    Promise.all([
      getSelf({}, abort.signal),
      apiGet<{ enabled: boolean }>("/api/user/2fa/status", abort.signal),
    ])
      .then(([profile, factor]) => {
        if (!abort.signal.aborted && profile.id === identity.accountId) {
          setUser(profile);
          setEnabled(factor.enabled);
        }
      })
      .catch((e: unknown) => {
        if (!abort.signal.aborted)
          setError(
            e instanceof Error ? e.message : "Unable to load security settings",
          );
      });
    return () => abort.abort();
  }, [identity.accountId, identity.epoch]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!user || busy) return;
    const ownerEpoch = identity.epoch;
    const current = () =>
      useSystemAccess.getState().session.epoch === ownerEpoch;
    setBusy(true);
    setError("");
    try {
      if (user.admin_password_change_required) {
        if (nextPassword !== confirmation)
          throw new Error(
            c(
              "两次密码不一致",
              "Passwords do not match",
              "كلمتا المرور غير متطابقتين",
            ),
          );
        const updated = await apiPut<AuthUser>("/api/user/self", {
          current_password: password,
          new_password: nextPassword,
        });
        if (current()) {
          setUser(updated);
          setPassword("");
          setNextPassword("");
          setConfirmation("");
        }
      } else if (!secret) {
        const result = await apiPost<{ secret: string }>(
          "/api/user/2fa/setup",
          { password },
        );
        if (current()) {
          setSecret(result.secret);
          setPassword("");
        }
      } else {
        const result = await apiPost<{ backup_codes: string[] }>(
          "/api/user/2fa/enable",
          { code },
        );
        if (current()) {
          setBackupCodes(result.backup_codes);
          setSecret("");
          setCode("");
          setEnabled(true);
        }
      }
    } catch (e) {
      if (current())
        setError(
          e instanceof Error ? e.message : "Unable to update security settings",
        );
    } finally {
      if (current()) setBusy(false);
    }
  }

  async function signOut() {
    if (busy) return;
    const ownerEpoch = identity.epoch;
    setBusy(true);
    setError("");
    try {
      await logoutUser();
      if (useSystemAccess.getState().session.epoch !== ownerEpoch) return;
      useSystemAccess.getState().setSession(null);
      useProductPreview.getState().logout();
      navigate("/login", { replace: true });
    } catch (e) {
      if (useSystemAccess.getState().session.epoch === ownerEpoch) {
        setError(e instanceof Error ? e.message : "Unable to sign out");
        setBusy(false);
      }
    }
  }

  return (
    <main className="staff-security" dir={language === "ar" ? "rtl" : "ltr"}>
      <h1>
        {c(
          "完成后台安全设置",
          "Complete staff security setup",
          "إكمال إعدادات أمان الموظفين",
        )}
      </h1>
      <p>
        {c(
          "首次使用后台需更换密码、绑定身份验证器，然后重新登录。",
          "Change your initial password, enable an authenticator, and sign in again to access staff tools.",
          "غيّر كلمة المرور الأولية وفعّل تطبيق المصادقة ثم سجّل الدخول مجددًا للوصول إلى أدوات الموظفين.",
        )}
      </p>
      {error && <p role="alert">{error}</p>}
      {!user ? (
        <p role="status">{c("正在加载…", "Loading…", "جارٍ التحميل…")}</p>
      ) : user.admin_password_change_required || !enabled ? (
        <form onSubmit={submit}>
          {user.admin_password_change_required || !secret ? (
            <label>
              {c("当前密码", "Current password", "كلمة المرور الحالية")}
              <Input
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                maxLength={64}
              />
            </label>
          ) : null}
          {user.admin_password_change_required ? (
            <>
              <label>
                {c("新密码", "New password", "كلمة المرور الجديدة")}
                <Input
                  type="password"
                  autoComplete="new-password"
                  value={nextPassword}
                  onChange={(e) => setNextPassword(e.target.value)}
                  required
                  minLength={8}
                  maxLength={64}
                />
              </label>
              <label>
                {c(
                  "确认新密码",
                  "Confirm new password",
                  "تأكيد كلمة المرور الجديدة",
                )}
                <Input
                  type="password"
                  autoComplete="new-password"
                  value={confirmation}
                  onChange={(e) => setConfirmation(e.target.value)}
                  required
                  minLength={8}
                  maxLength={64}
                />
              </label>
            </>
          ) : secret ? (
            <>
              <label>
                {c(
                  "在身份验证器中手动添加此密钥",
                  "Add this setup key to your authenticator",
                  "أضف مفتاح الإعداد هذا إلى تطبيق المصادقة",
                )}
                <textarea readOnly dir="ltr" value={secret} data-private />
              </label>
              <label>
                {c(
                  "六位动态码",
                  "Six-digit authenticator code",
                  "رمز المصادقة المكوّن من ستة أرقام",
                )}
                <Input
                  dir="ltr"
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  pattern="[0-9]{6}"
                  maxLength={6}
                  value={code}
                  onChange={(e) => setCode(e.target.value)}
                  required
                />
              </label>
            </>
          ) : null}
          <Button type="submit" disabled={busy}>
            {user.admin_password_change_required
              ? c("更换密码", "Change password", "تغيير كلمة المرور")
              : secret
                ? c(
                    "启用双重验证",
                    "Enable two-factor authentication",
                    "تفعيل المصادقة الثنائية",
                  )
                : c(
                    "设置身份验证器",
                    "Set up authenticator",
                    "إعداد تطبيق المصادقة",
                  )}
          </Button>
        </form>
      ) : (
        <>
          <p>
            {c(
              "安全设置已就绪，请重新登录完成双重验证。",
              "Security settings are ready. Sign in again to verify both factors.",
              "إعدادات الأمان جاهزة. سجّل الدخول مجددًا للتحقق من العاملين.",
            )}
          </p>
          {backupCodes.length > 0 && (
            <>
              <p>
                {c(
                  "请安全保存恢复码，此处仅显示一次。后台双重验证重置需联系运维。",
                  "Save these recovery codes securely; they are shown once. Staff factor resets require your operator.",
                  "احفظ رموز الاسترداد بأمان؛ تظهر مرة واحدة. إعادة تعيين مصادقة الموظف تتطلب مشغّل النظام.",
                )}
              </p>
              <pre dir="ltr" data-private>
                {backupCodes.join("\n")}
              </pre>
              <label>
                <input
                  type="checkbox"
                  checked={savedCodes}
                  onChange={(e) => setSavedCodes(e.target.checked)}
                />{" "}
                {c(
                  "我已安全保存恢复码",
                  "I have saved the recovery codes",
                  "لقد حفظت رموز الاسترداد",
                )}
              </label>
            </>
          )}
          <Button
            disabled={busy || (backupCodes.length > 0 && !savedCodes)}
            onClick={signOut}
          >
            {c("重新登录", "Sign in again", "تسجيل الدخول مجددًا")}
          </Button>
        </>
      )}
      <Button variant="ghost" disabled={busy} onClick={signOut}>
        {c("退出登录", "Sign out", "تسجيل الخروج")}
      </Button>
    </main>
  );
}

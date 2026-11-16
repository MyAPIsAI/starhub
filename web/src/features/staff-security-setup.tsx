import { useEffect, useRef, useState, type FormEvent } from "react";
import { CircleAlert } from "lucide-react";
import { useNavigate } from "react-router-dom";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/primitives";
import { getSelf, logoutUser, type AuthUser } from "../lib/api/auth";
import { ApiError, apiGet, apiPost, apiPut } from "../lib/api/http";
import { usePreferences } from "../lib/preferences";
import { useSystemAccess } from "../lib/system-access";
import { useProductPreview } from "../lib/product-preview";
import "./staff-security-setup.css";

type FactorStatus = { enabled: boolean; can_skip?: boolean };

export default function StaffSecuritySetup() {
  const language = usePreferences((s) => s.language);
  const identity = useSystemAccess((s) => s.session);
  const navigate = useNavigate();
  const c = (zh: string, en: string, ar: string) =>
    language === "zh" ? zh : language === "ar" ? ar : en;
  const [user, setUser] = useState<AuthUser | null>(null);
  const [enabled, setEnabled] = useState(false);
  const [canSkip, setCanSkip] = useState(false);
  const [password, setPassword] = useState("");
  const [nextPassword, setNextPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [secret, setSecret] = useState("");
  const [code, setCode] = useState("");
  const [backupCodes, setBackupCodes] = useState<string[]>([]);
  const [savedCodes, setSavedCodes] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [errorField, setErrorField] = useState<
    "current" | "new" | "confirmation" | null
  >(null);
  const currentInput = useRef<HTMLInputElement>(null);
  const newInput = useRef<HTMLInputElement>(null);
  const confirmationInput = useRef<HTMLInputElement>(null);
  const passwordHelp = c(
    "新密码须为 8–64 字节，至少包含一个英文字母和一个数字；不能使用常见弱密码或与当前密码相同。非英文字符会占用多个字节。",
    "Use 8–64 bytes with at least one English letter and one digit. Avoid common weak passwords and your current password. Non-ASCII characters use multiple bytes.",
    "استخدم من 8 إلى 64 بايت مع حرف إنجليزي واحد ورقم واحد على الأقل. تجنب كلمات المرور الشائعة الضعيفة وكلمة المرور الحالية. الأحرف غير اللاتينية تستهلك عدة بايتات.",
  );

  function passwordError(
    message: string,
    field: "current" | "new" | "confirmation",
  ) {
    setError(message);
    setErrorField(field);
    const input =
      field === "current"
        ? currentInput
        : field === "new"
          ? newInput
          : confirmationInput;
    input.current?.focus();
  }

  useEffect(() => {
    const abort = new AbortController();
    setUser(null);
    setEnabled(false);
    setCanSkip(false);
    setPassword("");
    setNextPassword("");
    setConfirmation("");
    setSecret("");
    setCode("");
    setBackupCodes([]);
    setSavedCodes(false);
    setError("");
    setErrorField(null);
    Promise.all([
      getSelf({}, abort.signal),
      apiGet<FactorStatus>("/api/user/2fa/status", abort.signal),
    ])
      .then(([profile, factor]) => {
        if (!abort.signal.aborted && profile.id === identity.accountId) {
          setUser(profile);
          setEnabled(factor.enabled);
          setCanSkip(factor.can_skip === true);
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
    setError("");
    setErrorField(null);
    if (user.admin_password_change_required) {
      if (!password) {
        passwordError(
          c(
            "请输入当前密码。",
            "Enter your current password.",
            "أدخل كلمة المرور الحالية.",
          ),
          "current",
        );
        return;
      }
      const normalized = nextPassword.trim();
      const length = new TextEncoder().encode(normalized).length;
      if (
        length < 8 ||
        length > 64 ||
        !/[A-Za-z]/.test(normalized) ||
        !/[0-9]/.test(normalized)
      ) {
        passwordError(passwordHelp, "new");
        return;
      }
      if (normalized === password) {
        passwordError(
          c(
            "新密码不能与当前密码相同。",
            "Choose a new password that differs from your current password.",
            "اختر كلمة مرور جديدة تختلف عن كلمة المرور الحالية.",
          ),
          "new",
        );
        return;
      }
      if (nextPassword !== confirmation) {
        passwordError(
          c(
            "两次密码不一致。",
            "Passwords do not match.",
            "كلمتا المرور غير متطابقتين.",
          ),
          "confirmation",
        );
        return;
      }
    }
    setBusy(true);
    try {
      if (user.admin_password_change_required) {
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
        if (!current()) return;
        const factor = await apiGet<FactorStatus>("/api/user/2fa/status");
        if (current()) {
          setEnabled(factor.enabled);
          setCanSkip(factor.can_skip === true);
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
          setCanSkip(false);
        }
      }
    } catch (e) {
      if (!current()) return;
      if (
        e instanceof ApiError &&
        e.status === 400 &&
        e.message ===
          "password must be 8-64 characters with at least one letter and one digit"
      )
        passwordError(
          c(
            "新密码不符合安全要求。" + passwordHelp,
            "The new password does not meet the security requirements. " +
              passwordHelp,
            "كلمة المرور الجديدة لا تستوفي متطلبات الأمان. " + passwordHelp,
          ),
          "new",
        );
      else if (
        e instanceof ApiError &&
        e.message === "current password is incorrect"
      )
        passwordError(
          c(
            "当前密码不正确，请重新输入。",
            "Current password is incorrect. Try again.",
            "كلمة المرور الحالية غير صحيحة. حاول مجددًا.",
          ),
          "current",
        );
      else if (
        e instanceof ApiError &&
        e.message === "new password must differ from the current password"
      )
        passwordError(
          c(
            "新密码不能与当前密码相同。",
            "Choose a new password that differs from your current password.",
            "اختر كلمة مرور جديدة تختلف عن كلمة المرور الحالية.",
          ),
          "new",
        );
      else
        setError(
          e instanceof Error ? e.message : "Unable to update security settings",
        );
    } finally {
      if (current()) setBusy(false);
    }
  }

  async function skip() {
    if (
      busy ||
      !user ||
      user.admin_password_change_required ||
      enabled ||
      !canSkip
    )
      return;
    const ownerEpoch = identity.epoch;
    setBusy(true);
    setError("");
    setErrorField(null);
    try {
      const profile = await apiPost<AuthUser>("/api/user/2fa/skip");
      if (useSystemAccess.getState().session.epoch !== ownerEpoch) return;
      if (
        profile.id !== identity.accountId ||
        profile.admin_security_required !== false ||
        profile.admin_password_change_required !== false
      )
        throw new Error("Security settings could not be confirmed.");
      setSecret("");
      setCode("");
      setPassword("");
      useSystemAccess.getState().setSession(profile);
      navigate("/console", { replace: true });
    } catch {
      if (useSystemAccess.getState().session.epoch === ownerEpoch)
        setError(
          c(
            "暂时无法跳过双重验证设置，请刷新页面后重试或继续绑定身份验证器。",
            "Unable to skip two-factor setup. Reload and try again, or continue setting up your authenticator.",
            "تعذر تخطي إعداد المصادقة الثنائية. أعد تحميل الصفحة وحاول مجددًا أو تابع إعداد تطبيق المصادقة.",
          ),
        );
    } finally {
      if (useSystemAccess.getState().session.epoch === ownerEpoch)
        setBusy(false);
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
        {canSkip
          ? c(
              "可现在绑定身份验证器，也可暂时跳过并进入后台。跳过仅对本次登录有效，下次登录时会再次提示。",
              "Set up an authenticator now, or skip for now to enter the console. Skipping applies only to this session; you will be prompted again next time you sign in.",
              "أعدّ تطبيق المصادقة الآن أو تخطّه مؤقتًا للدخول إلى لوحة التحكم. ينطبق التخطي على هذه الجلسة فقط؛ سيُطلب منك الإعداد عند تسجيل الدخول مجددًا.",
            )
          : c(
              "首次使用后台需更换密码、绑定身份验证器，然后重新登录。",
              "Change your initial password, enable an authenticator, and sign in again to access staff tools.",
              "غيّر كلمة المرور الأولية وفعّل تطبيق المصادقة ثم سجّل الدخول مجددًا للوصول إلى أدوات الموظفين.",
            )}
      </p>
      {error && (
        <div
          role="alert"
          id="staff-security-error"
          className="staff-security-error"
        >
          <CircleAlert size={18} aria-hidden="true" />
          <span>{error}</span>
        </div>
      )}
      {!user ? (
        <p role="status">{c("正在加载…", "Loading…", "جارٍ التحميل…")}</p>
      ) : user.admin_password_change_required || !enabled ? (
        <form
          onSubmit={submit}
          noValidate={user.admin_password_change_required}
        >
          {user.admin_password_change_required || !secret ? (
            <label>
              {c("当前密码", "Current password", "كلمة المرور الحالية")}
              <Input
                ref={currentInput}
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                maxLength={64}
                aria-invalid={errorField === "current" || undefined}
                aria-describedby={
                  errorField === "current" ? "staff-security-error" : undefined
                }
              />
            </label>
          ) : null}
          {user.admin_password_change_required ? (
            <>
              <div className="staff-security-password">
                <label>
                  {c("新密码", "New password", "كلمة المرور الجديدة")}
                  <Input
                    ref={newInput}
                    type="password"
                    autoComplete="new-password"
                    value={nextPassword}
                    onChange={(e) => setNextPassword(e.target.value)}
                    required
                    minLength={8}
                    maxLength={64}
                    aria-invalid={errorField === "new" || undefined}
                    aria-describedby={
                      errorField === "new"
                        ? "staff-security-password-help staff-security-error"
                        : "staff-security-password-help"
                    }
                  />
                </label>
                <small
                  id="staff-security-password-help"
                  className="staff-security-help"
                >
                  {passwordHelp}
                </small>
              </div>
              <label>
                {c(
                  "确认新密码",
                  "Confirm new password",
                  "تأكيد كلمة المرور الجديدة",
                )}
                <Input
                  ref={confirmationInput}
                  type="password"
                  autoComplete="new-password"
                  value={confirmation}
                  onChange={(e) => setConfirmation(e.target.value)}
                  required
                  minLength={8}
                  maxLength={64}
                  aria-invalid={errorField === "confirmation" || undefined}
                  aria-describedby={
                    errorField === "confirmation"
                      ? "staff-security-error"
                      : undefined
                  }
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
      {user && !user.admin_password_change_required && !enabled && canSkip && (
        <Button className="staff-security-skip" disabled={busy} onClick={skip}>
          {c("暂时跳过", "Skip for now", "تخطي الآن")}
        </Button>
      )}
      <Button variant="ghost" disabled={busy} onClick={signOut}>
        {c("退出登录", "Sign out", "تسجيل الخروج")}
      </Button>
    </main>
  );
}

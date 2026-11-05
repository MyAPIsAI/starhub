import { useEffect, useId, useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { Eye, EyeOff } from "lucide-react";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/primitives";
import { staffAuth, useStaffText, type StaffAuthState } from "../lib/api/staff";
import { authUserProfile, getSelf } from "../lib/api/auth";
import { apiPost, ApiError } from "../lib/api/http";
import { useProductPreview } from "../lib/product-preview";
import { useSystemAccess } from "../lib/system-access";
import { useSessionRestore } from "../lib/session-restore";
import "./auth/staff-auth.css";

export function PasswordField({
  label,
  value,
  onChange,
  autoComplete,
  minLength,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  autoComplete: string;
  minLength?: number;
}) {
  const id = useId(),
    text = useStaffText();
  const [visible, setVisible] = useState(false);
  return (
    <div className="auth-field">
      <label htmlFor={id}>{label}</label>
      <div className="auth-password-input">
        <Input
          id={id}
          type={visible ? "text" : "password"}
          autoComplete={autoComplete}
          value={value}
          onChange={(event) => onChange(event.target.value)}
          required
          minLength={minLength}
          maxLength={72}
        />
        <button
          type="button"
          className="auth-password-toggle"
          aria-pressed={visible}
          aria-label={
            visible
              ? text("隐藏密码", "Hide password", "إخفاء كلمة المرور")
              : text("显示密码", "Show password", "إظهار كلمة المرور")
          }
          onClick={() => setVisible((current) => !current)}
        >
          {visible ? <EyeOff size={16} /> : <Eye size={16} />}
        </button>
      </div>
    </div>
  );
}

export function verificationError(
  error: unknown,
  text: ReturnType<typeof useStaffText>,
) {
  return error instanceof ApiError && error.status === 429
    ? text(
        "尝试次数过多，请稍后再试。",
        "Too many attempts. Please try later.",
        "محاولات كثيرة. حاول لاحقًا.",
      )
    : text(
        "验证未通过。请检查凭据、验证码及公司网络或 VPN。",
        "Verification failed. Check your credentials, code and company network or VPN.",
        "فشل التحقق. تحقّق من بياناتك والرمز وشبكة الشركة أو VPN.",
      );
}

export default function StaffVerification({
  state,
  password,
  onPasswordChange,
  onStateChange,
  onCancel,
}: {
  state: StaffAuthState;
  password: string;
  onPasswordChange: (password: string) => void;
  onStateChange: (state: StaffAuthState) => void;
  onCancel: () => void;
}) {
  const text = useStaffText(),
    navigate = useNavigate(),
    { complete } = useSessionRestore();
  const [newPassword, setNewPassword] = useState("");
  const [code, setCode] = useState("");
  const [secret, setSecret] = useState("");
  const [showSecret, setShowSecret] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const id = useId();
  useEffect(() => {
    if (state.stage !== "ready") return;
    let live = true;
    getSelf({ handleUnauthorizedLocally: true }, undefined, "staff")
      .then(async (staffUser) => {
        if (!live) return;
        const requestedAdmin =
          new URLSearchParams(window.location.search).get("continue") ===
          "admin";
        const openWorkspace = staffUser.platform_admin && !requestedAdmin;
        const user = openWorkspace
          ? await getSelf(
              { handleUnauthorizedLocally: true },
              undefined,
              "customer",
            )
          : staffUser;
        if (!live) return;
        if (user.id !== staffUser.id)
          throw Error("Workspace owner changed during sign-in");
        useSystemAccess.getState().setSession(user);
        useProductPreview
          .getState()
          .login(user.email ?? "", user.username, authUserProfile(user));
        complete();
        navigate(openWorkspace ? "/console" : "/admin", { replace: true });
      })
      .catch(() => {
        if (live)
          setError(
            text(
              "无法确认登录状态，请重试。",
              "Unable to confirm your session. Please retry.",
              "تعذر تأكيد الجلسة. حاول مجددًا.",
            ),
          );
      });
    return () => {
      live = false;
    };
  }, [state.stage, attempt]);
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      if (state.stage === "change_password") {
        onStateChange(await staffAuth.password(password, newPassword));
        onPasswordChange("");
        setNewPassword("");
      } else {
        onStateChange(await staffAuth.verify(code));
        setSecret("");
        setShowSecret(false);
        setCode("");
      }
    } catch (caught) {
      setError(verificationError(caught, text));
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="auth-staff-verification">
      <p className="auth-staff-status" role="status">
        {state.username} ·{" "}
        {state.stage === "change_password"
          ? text(
              "首次登录：修改临时密码",
              "First sign in: change temporary password",
              "أول دخول: تغيير كلمة المرور المؤقتة",
            )
          : state.stage === "enroll"
            ? text(
                "首次登录：绑定双重验证（2FA）",
                "First sign in: set up two-factor authentication (2FA)",
                "أول دخول: إعداد المصادقة الثنائية (2FA)",
              )
            : text(
                "双重验证（2FA）",
                "Two-factor authentication (2FA)",
                "المصادقة الثنائية (2FA)",
              )}
      </p>
      {error && (
        <p role="alert" className="auth-error">
          {error}
        </p>
      )}
      {state.stage === "ready" ? (
        <>
          <p role="status">
            {text(
              "正在确认登录状态…",
              "Checking session…",
              "جارٍ التحقق من الجلسة…",
            )}
          </p>
          {error && (
            <Button
              variant="primary"
              className="auth-submit"
              onClick={() => {
                setError("");
                setAttempt((value) => value + 1);
              }}
            >
              {text("重试", "Retry", "إعادة المحاولة")}
            </Button>
          )}
        </>
      ) : (
        <form onSubmit={submit} className="auth-form">
          {state.stage === "change_password" && (
            <>
              <PasswordField
                label={text(
                  "当前临时密码",
                  "Current temporary password",
                  "كلمة المرور المؤقتة",
                )}
                value={password}
                onChange={onPasswordChange}
                autoComplete="current-password"
              />
              <PasswordField
                label={text(
                  "新密码（12–72 字节）",
                  "New password (12–72 bytes)",
                  "كلمة مرور جديدة (12–72 بايت)",
                )}
                value={newPassword}
                onChange={setNewPassword}
                autoComplete="new-password"
                minLength={12}
              />
            </>
          )}
          {state.stage === "enroll" && (
            <>
              <p className="auth-field-hint">
                {text(
                  "在验证器应用中新增账号，选择基于时间的验证码并输入设置密钥。完成验证前无法进入后台。",
                  "Add an account in your authenticator, select time-based codes and enter the setup key. Back-office access stays locked until verification succeeds.",
                  "أضف حسابًا في تطبيق المصادقة واختر رموزًا زمنية وأدخل مفتاح الإعداد. لن يتاح الدخول قبل نجاح التحقق.",
                )}
              </p>
              {!secret ? (
                <Button
                  type="button"
                  variant="secondary"
                  disabled={busy}
                  onClick={async () => {
                    setBusy(true);
                    setError("");
                    try {
                      setSecret((await staffAuth.enroll()).secret);
                    } catch (caught) {
                      setError(verificationError(caught, text));
                    } finally {
                      setBusy(false);
                    }
                  }}
                >
                  {text(
                    "获取设置密钥",
                    "Get setup key",
                    "الحصول على مفتاح الإعداد",
                  )}
                </Button>
              ) : (
                <>
                  <Button
                    type="button"
                    variant="secondary"
                    onClick={() => setShowSecret((value) => !value)}
                  >
                    {showSecret
                      ? text(
                          "隐藏设置密钥",
                          "Hide setup key",
                          "إخفاء مفتاح الإعداد",
                        )
                      : text(
                          "显示设置密钥",
                          "Show setup key",
                          "إظهار مفتاح الإعداد",
                        )}
                  </Button>
                  {showSecret && (
                    <code dir="ltr" className="staff-secret">
                      {secret}
                    </code>
                  )}
                </>
              )}
            </>
          )}
          {(state.stage === "enroll" || state.stage === "verify") && (
            <div className="auth-field">
              <label htmlFor={id}>
                {text(
                  "6 位验证码",
                  "6-digit verification code",
                  "رمز التحقق من 6 أرقام",
                )}
              </label>
              <Input
                id={id}
                autoComplete="one-time-code"
                inputMode="numeric"
                pattern="[0-9]{6}"
                maxLength={6}
                value={code}
                onChange={(event) => setCode(event.target.value)}
                required
              />
            </div>
          )}
          <Button
            type="submit"
            variant="primary"
            className="auth-submit"
            disabled={busy || (state.stage === "enroll" && !secret)}
          >
            {busy
              ? text("处理中…", "Working…", "جارٍ التنفيذ…")
              : state.stage === "change_password"
                ? text(
                    "修改密码并继续",
                    "Change password and continue",
                    "تغيير كلمة المرور والمتابعة",
                  )
                : text("验证并进入后台", "Verify and enter", "التحقق والدخول")}
          </Button>
        </form>
      )}
      <div className="auth-helper-row">
        <button
          type="button"
          disabled={busy}
          onClick={async () => {
            setBusy(true);
            setError("");
            try {
              await staffAuth.logout();
              onPasswordChange("");
              onCancel();
            } catch (caught) {
              setError(verificationError(caught, text));
            } finally {
              setBusy(false);
            }
          }}
        >
          {text("返回登录", "Back to sign in", "العودة للدخول")}
        </button>
      </div>
      <p className="auth-field-hint">
        {text(
          "验证器遗失时，请联系另一位 Root；唯一 Root 通过部署终端恢复。",
          "Lost authenticator? Contact another Root. A sole Root recovers through the deployment terminal.",
          "فقدت تطبيق المصادقة؟ اتصل بمستخدم Root آخر. يستعيد Root الوحيد الوصول عبر طرفية النشر.",
        )}
      </p>
    </div>
  );
}

export function StaffInstallation({
  onComplete,
  onCancel,
}: {
  onComplete: (state: StaffAuthState, password: string) => void;
  onCancel: () => void;
}) {
  const text = useStaffText(),
    id = useId();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  return (
    <form
      className="auth-form"
      onSubmit={async (event) => {
        event.preventDefault();
        if (busy) return;
        setBusy(true);
        setError("");
        try {
          await apiPost(
            "/api/admin/install",
            { username, password, installation_token: token },
            { handleUnauthorizedLocally: true },
          );
          setToken("");
          onComplete(await staffAuth.login(username, password), password);
        } catch (caught) {
          setError(verificationError(caught, text));
        } finally {
          setBusy(false);
        }
      }}
    >
      {error && (
        <p role="alert" className="auth-error">
          {error}
        </p>
      )}
      <div className="auth-field">
        <label htmlFor={id + "-username"}>
          {text("账号", "Username", "اسم المستخدم")}
        </label>
        <Input
          id={id + "-username"}
          autoComplete="username"
          value={username}
          onChange={(event) => setUsername(event.target.value)}
          required
          minLength={3}
          maxLength={64}
        />
      </div>
      <PasswordField
        label={text(
          "密码（12–72 字节）",
          "Password (12–72 bytes)",
          "كلمة المرور (12–72 بايت)",
        )}
        value={password}
        onChange={setPassword}
        autoComplete="new-password"
        minLength={12}
      />
      <div className="auth-field">
        <label htmlFor={id + "-token"}>
          {text(
            "部署终端的一次性安装令牌",
            "One-time token from deployment terminal",
            "رمز التثبيت لمرة واحدة من طرفية النشر",
          )}
        </label>
        <Input
          id={id + "-token"}
          type="password"
          autoComplete="off"
          value={token}
          onChange={(event) => setToken(event.target.value)}
          required
        />
      </div>
      <Button
        type="submit"
        variant="primary"
        className="auth-submit"
        disabled={busy}
      >
        {text("初始化 Root", "Initialize Root", "تهيئة Root")}
      </Button>
      <div className="auth-helper-row">
        <button type="button" disabled={busy} onClick={onCancel}>
          {text("返回登录", "Back to sign in", "العودة للدخول")}
        </button>
      </div>
    </form>
  );
}

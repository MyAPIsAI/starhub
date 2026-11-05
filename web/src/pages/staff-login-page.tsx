import { useEffect, useRef, useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { BrandMark } from "../components/brand";
import { Preferences } from "../components/product-shell";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/primitives";
import { staffAuth, useStaffText, type StaffAuthState } from "../lib/api/staff";
import { apiGet } from "../lib/api/http";
import StaffVerification, {
  PasswordField,
  StaffInstallation,
  verificationError,
} from "./staff-login";
import "./auth/auth-dialog.css";

export default function StaffLoginPage() {
  const text = useStaffText();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [state, setState] = useState<StaffAuthState | null>(null);
  const [installation, setInstallation] = useState(false);
  const [canInstall, setCanInstall] = useState(false);
  const [checking, setChecking] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const accountInput = useRef<HTMLInputElement>(null);

  useEffect(() => {
    let live = true;
    staffAuth
      .state()
      .then((value) => {
        if (live) setState(value);
      })
      .catch(() => {})
      .finally(() => {
        if (live) setChecking(false);
      });
    apiGet<{ available: boolean }>("/api/admin/install", undefined, {
      handleUnauthorizedLocally: true,
    })
      .then((value) => {
        if (live) setCanInstall(value.available);
      })
      .catch(() => {});
    return () => {
      live = false;
    };
  }, []);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (busy || checking) return;
    setBusy(true);
    setError("");
    try {
      const next = await staffAuth.login(username, password);
      setState(next);
      if (next.stage !== "change_password") setPassword("");
    } catch (caught) {
      setError(verificationError(caught, text));
    } finally {
      setBusy(false);
    }
  };

  return (
    <main className="staff-login-page">
      <section
        className="auth-dialog staff-login-card"
        aria-labelledby="staff-login-title"
      >
        <header className="auth-dialog-header">
          <div className="staff-login-toolbar">
            <Link to="/" className="auth-wordmark">
              <BrandMark />
              <span>MyAPIs</span>
            </Link>
            <Preferences />
          </div>
          <h1 id="staff-login-title">
            {installation
              ? text(
                  "初始化管理员",
                  "Initialize administrator",
                  "تهيئة المسؤول",
                )
              : text("员工登录", "Staff sign in", "تسجيل دخول الموظفين")}
          </h1>
          <p>
            {text(
              "仅限内部账号。客户请使用客户登录入口。",
              "Internal accounts only. Customers use the customer sign-in page.",
              "للحسابات الداخلية فقط. يستخدم العملاء صفحة دخول العملاء.",
            )}
          </p>
        </header>
        <div className="auth-dialog-body">
          {checking ? (
            <p role="status">
              {text(
                "正在检查会话…",
                "Checking session…",
                "جارٍ التحقق من الجلسة…",
              )}
            </p>
          ) : state ? (
            <StaffVerification
              state={state}
              password={password}
              onPasswordChange={setPassword}
              onStateChange={setState}
              onCancel={() => {
                setState(null);
                setPassword("");
                setError("");
                requestAnimationFrame(() => accountInput.current?.focus());
              }}
            />
          ) : installation ? (
            <StaffInstallation
              onComplete={(next, currentPassword) => {
                setState(next);
                setPassword(currentPassword);
                setInstallation(false);
                setCanInstall(false);
              }}
              onCancel={() => setInstallation(false)}
            />
          ) : (
            <form className="auth-form" onSubmit={submit}>
              {error && (
                <p className="auth-error" role="alert">
                  {error}
                </p>
              )}
              <div className="auth-field">
                <label htmlFor="staff-account">
                  {text("员工账号", "Staff username", "اسم مستخدم الموظف")}
                </label>
                <Input
                  ref={accountInput}
                  id="staff-account"
                  autoComplete="username"
                  required
                  maxLength={64}
                  value={username}
                  onChange={(event) => setUsername(event.target.value)}
                />
              </div>
              <PasswordField
                label={text("密码", "Password", "كلمة المرور")}
                value={password}
                onChange={setPassword}
                autoComplete="current-password"
              />
              <Button
                className="auth-submit"
                type="submit"
                variant="primary"
                disabled={busy}
              >
                {busy
                  ? text("正在登录…", "Signing in…", "جارٍ تسجيل الدخول…")
                  : text(
                      "登录并验证",
                      "Sign in and verify",
                      "تسجيل الدخول والتحقق",
                    )}
              </Button>
              {canInstall && (
                <div className="auth-switch">
                  <button type="button" onClick={() => setInstallation(true)}>
                    {text(
                      "首次安装：创建 Root",
                      "First installation: create Root",
                      "التثبيت الأول: إنشاء Root",
                    )}
                  </button>
                </div>
              )}
            </form>
          )}
        </div>
        <footer className="auth-dialog-footer">
          <span>
            {text(
              "每次重新登录需要双重验证（2FA）。验证后的会话有效期为 8 小时，期间刷新或切换工作空间无需再次输入验证码。",
              "Each new sign-in requires 2FA. A verified session lasts 8 hours; refreshing or switching workspaces does not require another code.",
              "يتطلب كل دخول جديد المصادقة الثنائية. تستمر الجلسة المتحققة 8 ساعات؛ لا يتطلب تحديث الصفحة أو تبديل مساحات العمل رمزًا آخر.",
            )}
          </span>
        </footer>
      </section>
    </main>
  );
}

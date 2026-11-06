import { message as translatedMessage } from "../i18n/messages";
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
                  ...translatedMessage("initialize_administrator_f80c2e119d"),
                )
              : text(...translatedMessage("staff_sign_in_96dcc13567"))}
          </h1>
          <p>
            {text(
              ...translatedMessage(
                "internal_accounts_only_customers_use_the_cust_00f5b12605",
              ),
            )}
          </p>
        </header>
        <div className="auth-dialog-body">
          {checking ? (
            <p role="status">
              {text(...translatedMessage("checking_session_f7e3c816ae"))}
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
                  {text(...translatedMessage("staff_username_e1eeb34be2"))}
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
                label={text(...translatedMessage("password_02b393215e"))}
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
                  ? text(...translatedMessage("signing_in_76477573bc"))
                  : text(...translatedMessage("sign_in_and_verify_1105ca78f6"))}
              </Button>
              {canInstall && (
                <div className="auth-switch">
                  <button type="button" onClick={() => setInstallation(true)}>
                    {text(
                      ...translatedMessage(
                        "first_installation_create_root_0c218d5b72",
                      ),
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
              ...translatedMessage(
                "each_new_sign_in_requires_2fa_a_verified_sess_cf1924de26",
              ),
            )}
          </span>
        </footer>
      </section>
    </main>
  );
}

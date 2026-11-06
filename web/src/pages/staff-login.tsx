import { message as translatedMessage } from "../i18n/messages";
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
              ? text(...translatedMessage("hide_password_fde939f99a"))
              : text(...translatedMessage("show_password_4090d99064"))
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
        ...translatedMessage("too_many_attempts_please_try_later_81d78c1669"),
      )
    : text(
        ...translatedMessage(
          "verification_failed_check_your_credentials_co_2228619432",
        ),
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
              ...translatedMessage(
                "unable_to_confirm_your_session_please_retry_668def7179",
              ),
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
              ...translatedMessage(
                "first_sign_in_change_temporary_password_e2345d8e88",
              ),
            )
          : state.stage === "enroll"
            ? text(
                ...translatedMessage(
                  "first_sign_in_set_up_two_factor_authenticatio_698127f7cb",
                ),
              )
            : text(
                ...translatedMessage(
                  "two_factor_authentication_2fa_07a1d27b59",
                ),
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
            {text(...translatedMessage("checking_session_c44b3558d1"))}
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
              {text(...translatedMessage("retry_c16455021c"))}
            </Button>
          )}
        </>
      ) : (
        <form onSubmit={submit} className="auth-form">
          {state.stage === "change_password" && (
            <>
              <PasswordField
                label={text(
                  ...translatedMessage("current_temporary_password_e9accdab2f"),
                )}
                value={password}
                onChange={onPasswordChange}
                autoComplete="current-password"
              />
              <PasswordField
                label={text(
                  ...translatedMessage("new_password_12_72_bytes_310adf3e59"),
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
                  ...translatedMessage(
                    "add_an_account_in_your_authenticator_select_t_e7a749ed32",
                  ),
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
                  {text(...translatedMessage("get_setup_key_fe0aa4e334"))}
                </Button>
              ) : (
                <>
                  <Button
                    type="button"
                    variant="secondary"
                    onClick={() => setShowSecret((value) => !value)}
                  >
                    {showSecret
                      ? text(...translatedMessage("hide_setup_key_57e979d5fe"))
                      : text(...translatedMessage("show_setup_key_88ebbc5c1f"))}
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
                  ...translatedMessage("6_digit_verification_code_9a7d40dbfc"),
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
              ? text(...translatedMessage("working_acf0cfceef"))
              : state.stage === "change_password"
                ? text(
                    ...translatedMessage(
                      "change_password_and_continue_7d89a4f635",
                    ),
                  )
                : text(...translatedMessage("verify_and_enter_fddab11b16"))}
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
          {text(...translatedMessage("back_to_sign_in_b4bfef6bb7"))}
        </button>
      </div>
      <p className="auth-field-hint">
        {text(
          ...translatedMessage(
            "lost_authenticator_contact_another_root_a_sol_31cce945bf",
          ),
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
          {text(...translatedMessage("username_cd595a0c3d"))}
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
        label={text(...translatedMessage("password_12_72_bytes_295fee76fd"))}
        value={password}
        onChange={setPassword}
        autoComplete="new-password"
        minLength={12}
      />
      <div className="auth-field">
        <label htmlFor={id + "-token"}>
          {text(
            ...translatedMessage(
              "one_time_token_from_deployment_terminal_f641d364e5",
            ),
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
        {text(...translatedMessage("initialize_root_b71c9b1d4c"))}
      </Button>
      <div className="auth-helper-row">
        <button type="button" disabled={busy} onClick={onCancel}>
          {text(...translatedMessage("back_to_sign_in_b4bfef6bb7"))}
        </button>
      </div>
    </form>
  );
}

import { LegalDocumentContent } from "../legal/document";
import { message as translatedMessage } from "../../i18n/messages";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import * as Dialog from "@radix-ui/react-dialog";
import { ArrowLeft, ArrowRight, Eye, EyeOff, Info, X } from "lucide-react";
import { BrandMark } from "../../components/brand";
import { Button } from "../../components/ui/button";
import { Input } from "../../components/ui/primitives";
import { usePreferences } from "../../lib/preferences";
import { useProductPreview } from "../../lib/product-preview";
import { useSystemAccess } from "../../lib/system-access";
import { authUserProfile, loginUser, registerUser } from "../../lib/api/auth";
import { ApiError } from "../../lib/api/http";
import { useSessionRestore } from "../../lib/session-restore";
import "./auth-dialog.css";

type AuthMode = "login" | "register" | "reset";
type Policy = "terms" | "privacy";
const modeFor = (path: string): AuthMode | null =>
  path === "/login"
    ? "login"
    : path === "/register"
      ? "register"
      : path === "/forgot-password"
        ? "reset"
        : null;

export default function AuthDialog() {
  const location = useLocation();
  const navigate = useNavigate();
  const { complete: completeSessionRestore } = useSessionRestore();
  const language = usePreferences((s) => s.language);
  const c = (zh: string, en: string, ar: string) =>
    language === "zh" ? zh : language === "ar" ? ar : en;
  const mode = modeFor(location.pathname);
  const documentParam = new URLSearchParams(location.search).get("document");
  const policy: Policy | null =
    documentParam === "terms" || documentParam === "privacy"
      ? documentParam
      : null;
  const method =
    new URLSearchParams(location.search).get("method") === "phone"
      ? "phone"
      : "password";
  const [account, setAccount] = useState("");
  const [password, setPassword] = useState("");
  const [username, setUsername] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [confirmError, setConfirmError] = useState(false);
  const [agreed, setAgreed] = useState(false);
  const [visiblePasswords, setVisiblePasswords] = useState<
    Record<string, boolean>
  >({});
  const [error, setError] = useState("");
  const [registered, setRegistered] = useState(false);
  const [busy, setBusy] = useState(false);
  const heading = useRef<HTMLHeadingElement>(null);
  const errorRef = useRef<HTMLDivElement>(null);
  const bodyRef = useRef<HTMLDivElement>(null);
  const previousView = useRef("");
  const policyTrigger = useRef<Policy | null>(null);
  const formScroll = useRef(0);
  const operation = useRef(0);

  useEffect(() => {
    operation.current += 1;
    setBusy(false);
    setError("");
    setVisiblePasswords({});
  }, [mode, method]);
  useEffect(() => {
    const view = `${mode}/${policy}`;
    if (previousView.current !== view && mode) {
      const returning =
        previousView.current.endsWith("/terms") ||
        previousView.current.endsWith("/privacy");
      const frame = requestAnimationFrame(() => {
        if (!policy && returning) {
          if (bodyRef.current) bodyRef.current.scrollTop = formScroll.current;
          const returnTarget = policyTrigger.current
            ? document.querySelector<HTMLButtonElement>(
                `[data-auth-policy="${policyTrigger.current}"]`,
              )
            : heading.current;
          (returnTarget ?? heading.current)?.focus({ preventScroll: true });
        } else {
          bodyRef.current?.scrollTo(0, 0);
          heading.current?.focus({ preventScroll: true });
        }
      });
      previousView.current = view;
      return () => cancelAnimationFrame(frame);
    }
    previousView.current = view;
  }, [mode, policy]);
  useEffect(() => {
    if (error) errorRef.current?.focus({ preventScroll: false });
  }, [error]);
  useEffect(
    () => () => {
      operation.current += 1;
    },
    [],
  );

  function setMethod(next: "password" | "phone") {
    const params = new URLSearchParams(location.search);
    if (next === "phone") params.set("method", "phone");
    else params.delete("method");
    navigate(
      { pathname: "/login", search: params.toString() },
      { replace: true },
    );
  }
  function changeMode(next: AuthMode) {
    setRegistered(false);
    navigate(
      next === "register"
        ? "/register"
        : next === "reset"
          ? "/forgot-password"
          : "/login",
      { replace: true },
    );
  }
  function showPolicy(next: Policy) {
    policyTrigger.current = next;
    formScroll.current = bodyRef.current?.scrollTop ?? 0;
    const params = new URLSearchParams(location.search);
    params.set("document", next);
    navigate(
      { pathname: location.pathname, search: params.toString() },
      { replace: true },
    );
  }
  function backToForm() {
    const params = new URLSearchParams(location.search);
    params.delete("document");
    navigate(
      { pathname: location.pathname, search: params.toString() },
      { replace: true },
    );
  }
  function close() {
    operation.current += 1;
    navigate("/", { replace: true });
  }
  function explain(caught: unknown): string {
    if (caught instanceof ApiError) {
      if (caught.status === 401)
        return c(
          ...translatedMessage(
            "the_account_or_password_is_incorrect_please_t_f7b96c4cf8",
          ),
        );
      if (caught.status === 403 && caught.message === "user is disabled")
        return c(
          ...translatedMessage(
            "this_account_is_disabled_contact_the_administ_17dabe2605",
          ),
        );
      if (
        caught.status === 403 &&
        caught.message === "staff access requires the company network or VPN"
      )
        return c(
          ...translatedMessage(
            "connect_through_the_company_network_or_vpn_to_a19511ca91",
          ),
        );
      if (caught.status === 429)
        return c(
          ...translatedMessage("too_many_attempts_please_try_later_81d78c1669"),
        );
      if (caught.status === 403)
        return c(
          ...translatedMessage(
            "sign_in_was_denied_contact_the_administrator_a218135410",
          ),
        );
      if (caught.status === 409)
        return c(
          ...translatedMessage(
            "this_username_is_already_taken_choose_another_8a9b4220fc",
          ),
        );
      return caught.message;
    }
    return c(
      ...translatedMessage(
        "unable_to_continue_your_inputs_are_preserved__865306b2d9",
      ),
    );
  }
  async function submit(event: FormEvent) {
    event.preventDefault();
    if (
      busy ||
      (mode !== "register" && (mode !== "login" || method !== "password"))
    )
      return;
    setError("");
    if (!agreed) {
      setError(
        c(
          ...translatedMessage(
            "read_and_accept_the_terms_and_privacy_notice__73d6050598",
          ),
        ),
      );
      return;
    }
    if (mode === "register" && newPassword !== confirmPassword) {
      setConfirmError(true);
      setError(
        c(
          ...translatedMessage(
            "passwords_do_not_match_please_confirm_your_pa_745e3fae3a",
          ),
        ),
      );
      return;
    }
    setConfirmError(false);
    const ticket = ++operation.current;
    setBusy(true);
    try {
      if (mode === "register") {
        const user = await registerUser(username, newPassword, "");
        if (ticket !== operation.current) return;
        setAccount(user.username);
        setPassword("");
        setNewPassword("");
        setConfirmPassword("");
        setRegistered(true);
        // Registration creates an account but does not establish a server session.
        navigate("/login", { replace: true });
        return;
      } else {
        // Login: password method only (phone not supported by backend).
        const { user } = await loginUser(account, password);
        if (ticket !== operation.current) return;
        useProductPreview
          .getState()
          .login(user.email ?? "", user.username, authUserProfile(user));
        useSystemAccess.getState().setSession(user);
        completeSessionRestore();
      }
      setPassword("");
      setNewPassword("");
      setConfirmPassword("");
      navigate("/console", {
        replace: true,
      });
    } catch (caught) {
      if (ticket === operation.current) {
        setError(explain(caught));
      }
    } finally {
      if (ticket === operation.current) {
        setBusy(false);
      }
    }
  }
  const title =
    policy === "terms"
      ? c(...translatedMessage("terms_of_service_d38598ca08"))
      : policy === "privacy"
        ? c(...translatedMessage("privacy_notice_a66b0d4095"))
        : mode === "register"
          ? c(...translatedMessage("create_your_account_061afa15ea"))
          : mode === "reset"
            ? c(...translatedMessage("reset_your_password_928072b65e"))
            : c(...translatedMessage("welcome_back_1d9c0d500a"));
  const description = policy
    ? c(...translatedMessage("published_legal_help"))
    : mode === "register"
      ? c(
          ...translatedMessage(
            "one_key_to_start_working_with_multiple_models_123d687c0c",
          ),
        )
      : mode === "reset"
        ? c(
            ...translatedMessage(
              "password_recovery_is_unavailable_contact_the__f5a8455096",
            ),
          )
        : c(
            ...translatedMessage(
              "sign_in_and_switch_models_in_the_tools_you_kn_d9de79fafc",
            ),
          );

  function passwordField(
    id: string,
    label: string,
    value: string,
    update: (v: string) => void,
    isNew = false,
  ) {
    return (
      <div className="auth-field">
        <label htmlFor={id}>{label}</label>
        <div className="auth-password-input">
          <Input
            id={id}
            type={visiblePasswords[id] ? "text" : "password"}
            autoComplete={isNew ? "new-password" : "current-password"}
            value={value}
            onChange={(e) => {
              update(e.target.value);
              setConfirmError(false);
              setError("");
            }}
            aria-invalid={
              (id === "auth-confirm-password" && confirmError) || undefined
            }
            required
            minLength={isNew ? 6 : undefined}
            maxLength={isNew ? 64 : 72}
            placeholder={
              id === "auth-confirm-password"
                ? c(
                    ...translatedMessage(
                      "enter_your_password_again_1251a73d19",
                    ),
                  )
                : isNew
                  ? c(...translatedMessage("at_least_6_characters_f73ef16def"))
                  : c(...translatedMessage("enter_your_password_aca4207a72"))
            }
          />
          <button
            type="button"
            className="auth-password-toggle"
            aria-label={
              visiblePasswords[id]
                ? c(...translatedMessage("hide_password_fde939f99a"))
                : c(...translatedMessage("show_password_4090d99064"))
            }
            aria-pressed={Boolean(visiblePasswords[id])}
            onClick={() =>
              setVisiblePasswords((current) => ({
                ...current,
                [id]: !current[id],
              }))
            }
          >
            {visiblePasswords[id] ? <EyeOff size={17} /> : <Eye size={17} />}
          </button>
        </div>
      </div>
    );
  }
  const consent = (
    <div className="auth-consent">
      <input
        id="auth-consent"
        aria-label={c(
          ...translatedMessage(
            "i_have_read_and_agree_to_the_terms_of_service_1125a13a5a",
          ),
        )}
        type="checkbox"
        checked={agreed}
        onChange={(e) => {
          setAgreed(e.target.checked);
          setError("");
        }}
      />
      <div className="auth-consent-text">
        <label htmlFor="auth-consent">
          {c(...translatedMessage("i_have_read_and_agree_to_3f74b32eb6"))}
        </label>
        <button
          type="button"
          data-auth-policy="terms"
          onClick={() => showPolicy("terms")}
        >
          {c(...translatedMessage("terms_of_service_e7dc68d3c2"))}
        </button>
        <label htmlFor="auth-consent">
          {c(...translatedMessage("and_acknowledge_c2cfb94798"))}
        </label>
        <button
          type="button"
          data-auth-policy="privacy"
          onClick={() => showPolicy("privacy")}
        >
          {c(...translatedMessage("privacy_notice_b9265574bf"))}
        </button>
      </div>
    </div>
  );

  return (
    <Dialog.Root
      open={Boolean(mode)}
      onOpenChange={(open) => {
        if (!open) {
          if (policy) backToForm();
          else close();
        }
      }}
    >
      <Dialog.Portal>
        <Dialog.Overlay className="auth-backdrop" />
        <Dialog.Content
          className={`auth-dialog ${policy ? "auth-dialog--policy" : mode === "register" ? "auth-dialog--register" : ""}`}
          onOpenAutoFocus={(event) => {
            event.preventDefault();
            heading.current?.focus();
          }}
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            requestAnimationFrame(() => {
              const target = Array.from(
                document.querySelectorAll<HTMLElement>(
                  ".veyra-public-login, .veyra-public-menu",
                ),
              ).find((element) => element.getClientRects().length > 0);
              target?.focus({ preventScroll: true });
            });
          }}
          onPointerDownOutside={(event) => event.preventDefault()}
          onEscapeKeyDown={(event) => {
            if (policy) {
              event.preventDefault();
              backToForm();
            }
          }}
        >
          <div className="auth-dialog-header">
            <div className="auth-wordmark">
              <BrandMark />
              <span>MyAPIs</span>
            </div>
            <button
              type="button"
              className="auth-close"
              onClick={close}
              aria-label={c(
                ...translatedMessage("close_and_return_home_cc6606707d"),
              )}
            >
              <X size={20} />
            </button>
            {policy && (
              <button type="button" className="auth-back" onClick={backToForm}>
                <ArrowLeft size={14} className="directional" />
                {c(...translatedMessage("back_to_form_76ccd28ef6"))}
              </button>
            )}
            <Dialog.Title ref={heading} tabIndex={-1}>
              {title}
            </Dialog.Title>
            <Dialog.Description>{description}</Dialog.Description>
          </div>
          <div className="auth-dialog-body" ref={bodyRef}>
            {policy ? (
              <div className="auth-policy">
                <LegalDocumentContent kind={policy} />
              </div>
            ) : (
              <>
                {mode === "login" && registered && (
                  <p className="auth-field-hint" role="status">
                    {c(
                      ...translatedMessage(
                        "account_created_sign_in_with_your_username_an_8b3bc540d1",
                      ),
                    )}
                  </p>
                )}
                {mode === "login" && (
                  <div
                    className="auth-methods"
                    role="group"
                    aria-label={c(
                      ...translatedMessage("login_method_ab6c5314b3"),
                    )}
                  >
                    <button
                      type="button"
                      aria-pressed={method === "password"}
                      data-state={method === "password" ? "active" : "inactive"}
                      onClick={() => setMethod("password")}
                    >
                      {c(...translatedMessage("password_15c90631ef"))}
                    </button>
                    <button
                      type="button"
                      aria-pressed={method === "phone"}
                      data-state={method === "phone" ? "active" : "inactive"}
                      onClick={() => setMethod("phone")}
                    >
                      {c(...translatedMessage("phone_code_958ce3021c"))}
                    </button>
                  </div>
                )}
                {mode === "reset" ||
                (mode === "login" && method === "phone") ? (
                  <div className="auth-unavailable" role="status">
                    <span>
                      <Info size={24} />
                    </span>
                    <p>
                      {mode === "reset"
                        ? c(
                            ...translatedMessage(
                              "password_recovery_is_unavailable_contact_the__020bb77c9b",
                            ),
                          )
                        : c(
                            ...translatedMessage(
                              "phone_code_login_is_unavailable_use_your_user_fbb542e006",
                            ),
                          )}
                    </p>
                    <Button
                      variant="primary"
                      onClick={() => changeMode("login")}
                    >
                      {c(...translatedMessage("use_password_login_ce48f27728"))}
                    </Button>
                  </div>
                ) : (
                  <form onSubmit={submit} className="auth-form">
                    {error && (
                      <div
                        className="auth-error"
                        role="alert"
                        tabIndex={-1}
                        ref={errorRef}
                      >
                        {error}
                      </div>
                    )}
                    {mode === "login" && method === "password" && (
                      <>
                        <div className="auth-field">
                          <label htmlFor="auth-account">
                            {c(...translatedMessage("account_b00965aefc"))}
                          </label>
                          <Input
                            id="auth-account"
                            type="text"
                            autoComplete="username"
                            value={account}
                            onChange={(e) => {
                              setAccount(e.target.value);
                              setError("");
                            }}
                            required
                            maxLength={254}
                            placeholder={c(
                              ...translatedMessage("username_52bf46ce51"),
                            )}
                            dir="ltr"
                          />
                        </div>
                        {passwordField(
                          "auth-password",
                          c(...translatedMessage("password_02b393215e")),
                          password,
                          setPassword,
                        )}
                        <div className="auth-helper-row">
                          <button
                            type="button"
                            onClick={() => {
                              changeMode("reset");
                            }}
                          >
                            {c(
                              ...translatedMessage(
                                "forgot_password_dabee5bde1",
                              ),
                            )}
                          </button>
                        </div>
                      </>
                    )}
                    {mode === "register" && (
                      <>
                        <div className="auth-field">
                          <label htmlFor="auth-username">
                            {c(...translatedMessage("username_52bf46ce51"))}
                          </label>
                          <Input
                            id="auth-username"
                            dir="ltr"
                            value={username}
                            onChange={(e) => {
                              setUsername(e.target.value);
                              setError("");
                            }}
                            required
                            minLength={3}
                            maxLength={64}
                            pattern="(?:[a-zA-Z0-9_.]|-){3,64}"
                            autoComplete="username"
                            placeholder={c(
                              ...translatedMessage(
                                "choose_a_username_028c5b33e5",
                              ),
                            )}
                            aria-describedby="auth-username-hint"
                          />
                          <p
                            id="auth-username-hint"
                            className="auth-field-hint"
                          >
                            {c(
                              ...translatedMessage(
                                "3_64_ascii_letters_digits_underscores_hyphens_e165c8cd66",
                              ),
                            )}
                          </p>
                        </div>
                        {passwordField(
                          "auth-new-password",
                          c(...translatedMessage("create_password_5636ea0f66")),
                          newPassword,
                          setNewPassword,
                          true,
                        )}
                        {passwordField(
                          "auth-confirm-password",
                          c(
                            ...translatedMessage("confirm_password_db65e83f31"),
                          ),
                          confirmPassword,
                          setConfirmPassword,
                          true,
                        )}
                        <p className="auth-field-hint">
                          {c(
                            ...translatedMessage(
                              "registration_currently_uses_a_username_and_pa_e5acd7e3b6",
                            ),
                          )}
                        </p>
                      </>
                    )}
                    {consent}
                    {mode === "register" && (
                      <p className="auth-registration-note">
                        {c(
                          ...translatedMessage(
                            "creating_an_account_does_not_clear_current_de_6c698381a5",
                          ),
                        )}
                      </p>
                    )}
                    <Button
                      variant="primary"
                      type="submit"
                      className="auth-submit"
                      loading={busy}
                    >
                      {mode === "register"
                        ? c(...translatedMessage("create_account_7a5f6b2b53"))
                        : c(...translatedMessage("log_in_432aa7fcba"))}
                      <ArrowRight size={16} className="directional" />
                    </Button>
                  </form>
                )}
                {mode !== "reset" && (
                  <div className="auth-switch">
                    {mode === "login" ? (
                      <>
                        {c(...translatedMessage("new_to_myapis_0be912aed4"))}
                        <button
                          type="button"
                          onClick={() => changeMode("register")}
                        >
                          {c(...translatedMessage("sign_up_2648aa5036"))}
                        </button>
                      </>
                    ) : (
                      <>
                        {mode === "register" &&
                          c(
                            ...translatedMessage(
                              "already_have_an_account_f18c9abb50",
                            ),
                          )}
                        <button
                          type="button"
                          onClick={() => changeMode("login")}
                        >
                          {c(...translatedMessage("back_to_login_5ad34b7a60"))}
                        </button>
                      </>
                    )}
                  </div>
                )}
              </>
            )}
          </div>
          <div className="auth-dialog-footer">
            <Info size={14} />
            <span>
              {c(
                ...translatedMessage(
                  "accounts_are_saved_on_the_server_login_sessio_cfa27a3775",
                ),
              )}
            </span>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

import { message as translatedMessage } from "../i18n/messages";
import { useRef, useState, type FormEvent } from "react";
import {
  Eye,
  EyeOff,
  Info,
  KeyRound,
  Mail,
  ShieldCheck,
  Smartphone,
} from "lucide-react";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/primitives";
import { Modal } from "../components/ui/overlay";
import { useUpdateSelf, type useUserProfile } from "../lib/api/user";
import { ApiError } from "../lib/api/http";
import { usePreferences } from "../lib/preferences";

type Profile = ReturnType<typeof useUserProfile>["data"];

export function AccountSecurity({ profile }: { profile: Profile }) {
  const language = usePreferences((state) => state.language);
  const c = (zh: string, en: string, ar: string) =>
    language === "zh" ? zh : language === "ar" ? ar : en;
  const update = useUpdateSelf();
  const [mode, setMode] = useState<"password" | "email" | null>(null);
  const [current, setCurrent] = useState("");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [email, setEmail] = useState("");
  const [visible, setVisible] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [pending, setPending] = useState(false);
  const [uncertain, setUncertain] = useState(false);
  const active = useRef(false);
  const draftEmail = useRef("");
  const first = useRef<HTMLInputElement>(null);
  const allowed = profile?.can_update === true;

  function close() {
    if (active.current) return;
    setMode(null);
    setCurrent("");
    setPassword("");
    setConfirmation("");
    setEmail("");
    setVisible(false);
    setError("");
    setUncertain(false);
  }
  function open(next: "password" | "email") {
    if (!allowed || active.current) return;
    close();
    draftEmail.current = profile?.email ?? "";
    setNotice("");
    setMode(next);
    setEmail(profile?.email ?? "");
  }
  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!mode || !allowed || active.current || uncertain) return;
    if (
      mode === "password" &&
      (password.length < 8 ||
        new TextEncoder().encode(password).length > 64 ||
        !current ||
        password !== confirmation)
    ) {
      setError(
        c(
          ...translatedMessage(
            "enter_your_current_password_and_matching_new__6a5f5640f4",
          ),
        ),
      );
      return;
    }
    active.current = true;
    setPending(true);
    setError("");
    try {
      await update.mutateAsync(
        mode === "password"
          ? {
              expected_email: draftEmail.current,
              current_password: current,
              new_password: password,
            }
          : { expected_email: draftEmail.current, email: email.trim() },
      );
      setNotice(
        mode === "password"
          ? c(
              ...translatedMessage(
                "password_updated_use_the_new_password_at_your_d78036fe51",
              ),
            )
          : c(
              ...translatedMessage(
                "profile_email_saved_email_verification_and_em_c2dc646cfc",
              ),
            ),
      );
      active.current = false;
      close();
    } catch (caught) {
      if (
        caught instanceof ApiError &&
        caught.status === 403 &&
        caught.message.includes("current password")
      )
        setError(
          c(...translatedMessage("current_password_is_incorrect_81e1cb0eb2")),
        );
      else if (caught instanceof ApiError && caught.status === 409)
        setError(
          c(
            ...translatedMessage(
              "the_account_or_profile_changed_close_this_dia_f1ff763da6",
            ),
          ),
        );
      else if (caught instanceof ApiError && caught.status === 429)
        setError(
          c(
            ...translatedMessage(
              "too_many_attempts_try_again_later_a64840b74a",
            ),
          ),
        );
      else if (caught instanceof ApiError && caught.status === 400)
        setError(
          c(
            ...translatedMessage(
              "check_the_password_length_or_email_format_39ba6c929d",
            ),
          ),
        );
      else {
        setUncertain(true);
        setError(
          c(
            ...translatedMessage(
              "the_save_could_not_be_confirmed_close_this_di_675d714e22",
            ),
          ),
        );
      }
    } finally {
      active.current = false;
      setPending(false);
    }
  }
  const masked = profile?.email.replace(/^(.{1,2})[^@]*(@.*)$/, "$1***$2");
  return (
    <>
      <section className="ac-card" aria-labelledby="ac-security-title">
        <div className="ac-section-heading">
          <div>
            <h2 id="ac-security-title">
              <ShieldCheck size={18} />
              {c(...translatedMessage("sign_in_security_1b06bafdc6"))}
            </h2>
            <p>
              {c(
                ...translatedMessage(
                  "sign_in_with_your_server_username_and_passwor_9efbb1f0ce",
                ),
              )}
            </p>
          </div>
        </div>
        {notice && (
          <div role="status" className="ac-notice">
            <Info size={16} />
            <span>{notice}</span>
          </div>
        )}
        {!allowed && (
          <p className="ac-empty-hint">
            {c(
              ...translatedMessage(
                "reload_your_account_profile_changes_require_e_6fbbfd9d8f",
              ),
            )}
          </p>
        )}
        <div className="ac-security-row">
          <span className="ac-row-icon">
            <KeyRound size={19} />
          </span>
          <div className="ac-row-content">
            <h3>{c(...translatedMessage("password_7f7a868751"))}</h3>
            <p>
              {c(
                ...translatedMessage(
                  "changes_require_your_current_password_phone_p_b7387c8fbe",
                ),
              )}
            </p>
          </div>
          <Button
            data-account-action="password"
            size="small"
            disabled={!allowed || pending}
            onClick={() => open("password")}
          >
            {c(...translatedMessage("change_password_54c4e196bc"))}
          </Button>
        </div>
        <div className="ac-security-row">
          <span className="ac-row-icon">
            <Smartphone size={19} />
          </span>
          <div className="ac-row-content">
            <h3>{c(...translatedMessage("phone_abba197f77"))}</h3>
            <Input
              className="ac-contact-value"
              id="account-phone"
              aria-label={c(...translatedMessage("linked_phone_d4892b59b0"))}
              value={c(...translatedMessage("unavailable_cddbd3a531"))}
              readOnly
            />
            <p>
              {c(
                ...translatedMessage(
                  "phone_linking_sms_sign_in_and_password_recove_e6529ef560",
                ),
              )}
            </p>
          </div>
          <Button data-account-action="phone" size="small" disabled>
            {c(...translatedMessage("link_c9a152d363"))}
          </Button>
        </div>
        <div className="ac-security-row">
          <span className="ac-row-icon">
            <Mail size={19} />
          </span>
          <div className="ac-row-content">
            <h3>{c(...translatedMessage("email_218a18e187"))}</h3>
            <Input
              className="ac-contact-value"
              id="account-email"
              aria-label={c(...translatedMessage("profile_email_61b39133e9"))}
              value={
                profile
                  ? masked || c(...translatedMessage("not_provided_c3f269cf5b"))
                  : c(...translatedMessage("unconfirmed_6d40d4c4c5"))
              }
              readOnly
              dir={masked ? "ltr" : undefined}
            />
            <p>
              {c(
                ...translatedMessage(
                  "email_is_a_profile_field_sign_in_with_your_us_f29c126ea5",
                ),
              )}
            </p>
          </div>
          <Button
            data-account-action="email"
            size="small"
            disabled={!allowed || pending}
            onClick={() => open("email")}
          >
            {c(...translatedMessage("edit_6224671192"))}
          </Button>
        </div>
      </section>
      <Modal
        open={mode !== null}
        onOpenChange={(next) => {
          if (!next) close();
        }}
        title={
          mode === "password"
            ? c(...translatedMessage("change_password_0470176a4a"))
            : c(...translatedMessage("edit_profile_email_a53bd401b0"))
        }
        description={
          mode === "password"
            ? c(
                ...translatedMessage(
                  "verify_your_current_password_to_save_a_new_on_ab51d9453a",
                ),
              )
            : c(
                ...translatedMessage(
                  "email_is_profile_data_no_verification_code_is_27a37c369c",
                ),
              )
        }
        initialFocus={first}
        preventOutsideClose
        className="ac-security-dialog"
        footer={
          <>
            <Button onClick={close} disabled={pending}>
              {c(...translatedMessage("cancel_0828e4ce08"))}
            </Button>
            <Button
              variant="primary"
              form="account-security-form"
              type="submit"
              loading={pending}
              disabled={!allowed || uncertain}
            >
              {c(...translatedMessage("save_6774e725d1"))}
            </Button>
          </>
        }
      >
        <form
          id="account-security-form"
          className="ac-security-form"
          onSubmit={submit}
        >
          {error && (
            <div
              role="alert"
              className="ac-form-error"
              id="account-security-error"
            >
              {error}
            </div>
          )}
          {mode === "password" ? (
            <>
              <div className="ac-field">
                <label htmlFor="security-current-password">
                  {c(...translatedMessage("current_password_8b8cddc381"))}
                </label>
                <Input
                  ref={first}
                  id="security-current-password"
                  type={visible ? "text" : "password"}
                  autoComplete="current-password"
                  value={current}
                  onChange={(event) => setCurrent(event.target.value)}
                  disabled={pending}
                  required
                  aria-describedby={
                    error ? "account-security-error" : undefined
                  }
                />
              </div>
              <div className="ac-field">
                <label htmlFor="security-new-password">
                  {c(...translatedMessage("new_password_29bd27acfe"))}
                </label>
                <Input
                  id="security-new-password"
                  type={visible ? "text" : "password"}
                  autoComplete="new-password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  disabled={pending}
                  minLength={8}
                  required
                  aria-describedby="security-password-help"
                />
                <small id="security-password-help">
                  {c(
                    ...translatedMessage(
                      "at_least_8_characters_at_most_64_bytes_non_as_65deb3d0d6",
                    ),
                  )}
                </small>
              </div>
              <div className="ac-field">
                <label htmlFor="security-confirm-password">
                  {c(...translatedMessage("confirm_new_password_f33eca8ac7"))}
                </label>
                <Input
                  id="security-confirm-password"
                  type={visible ? "text" : "password"}
                  autoComplete="new-password"
                  value={confirmation}
                  onChange={(event) => setConfirmation(event.target.value)}
                  disabled={pending}
                  required
                />
              </div>
              <Button
                variant="ghost"
                onClick={() => setVisible((value) => !value)}
                disabled={pending}
                aria-pressed={visible}
              >
                {visible ? <EyeOff size={16} /> : <Eye size={16} />}{" "}
                {visible
                  ? c(...translatedMessage("hide_password_fde939f99a"))
                  : c(...translatedMessage("show_password_4090d99064"))}
              </Button>
            </>
          ) : (
            <div className="ac-field">
              <label htmlFor="security-email">
                {c(...translatedMessage("profile_email_optional_d45d1983d1"))}
              </label>
              <Input
                ref={first}
                id="security-email"
                type="email"
                autoComplete="email"
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                disabled={pending}
                maxLength={64}
              />
            </div>
          )}
        </form>
      </Modal>
    </>
  );
}

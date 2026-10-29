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
          "请输入当前密码，并确认两次新密码一致；新密码至少8个字符且不超过64字节。",
          "Enter your current password and matching new passwords: at least 8 characters, at most 64 bytes.",
          "أدخل كلمة المرور الحالية وجديدة متطابقة: 8 أحرف على الأقل و64 بايتًا كحد أقصى.",
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
              "密码已更新，下次登录请使用新密码。登录设备可在下方单独管理。",
              "Password updated. Use the new password at your next sign-in. Manage signed-in devices separately below.",
              "حُدثت كلمة المرور. استخدم الجديدة للدخول التالي، وأدر الأجهزة المسجلة بشكل منفصل أدناه.",
            )
          : c(
              "账号资料邮箱已保存。邮箱验证和邮箱登录尚未开放。",
              "Profile email saved. Email verification and email sign-in are unavailable.",
              "حُفظ بريد الحساب. التحقق بالبريد والدخول به غير متاحين.",
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
          c(
            "当前密码不正确。",
            "Current password is incorrect.",
            "كلمة المرور الحالية غير صحيحة.",
          ),
        );
      else if (caught instanceof ApiError && caught.status === 409)
        setError(
          c(
            "账号或资料已变化。请关闭窗口并重新读取账号资料后再试。",
            "The account or profile changed. Close this dialog and reload the profile before trying again.",
            "تغير الحساب أو بياناته. أغلق النافذة وأعد تحميل البيانات قبل المحاولة.",
          ),
        );
      else if (caught instanceof ApiError && caught.status === 429)
        setError(
          c(
            "尝试过于频繁，请稍后再试。",
            "Too many attempts. Try again later.",
            "محاولات كثيرة. حاول لاحقًا.",
          ),
        );
      else if (caught instanceof ApiError && caught.status === 400)
        setError(
          c(
            "输入不符合要求，请检查密码长度或邮箱格式。",
            "Check the password length or email format.",
            "تحقق من طول كلمة المرور أو صيغة البريد.",
          ),
        );
      else {
        setUncertain(true);
        setError(
          c(
            "未能确认保存结果。请关闭窗口，核对已保存的邮箱或通过登录核对密码后再操作。",
            "The save could not be confirmed. Close this dialog and check the saved email or verify the password by signing in before trying again.",
            "تعذر تأكيد الحفظ. أغلق النافذة وتحقق من البريد المحفوظ أو من كلمة المرور بتسجيل الدخول قبل المحاولة مجددًا.",
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
              {c("登录与安全", "Sign-in & security", "الدخول والأمان")}
            </h2>
            <p>
              {c(
                "使用服务器用户名和密码登录。",
                "Sign in with your server username and password.",
                "ادخل باسم المستخدم وكلمة المرور على الخادم.",
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
              "请先重新读取账号资料；仅在服务器明确支持时开放修改。",
              "Reload your account profile. Changes require explicit server support.",
              "أعد تحميل بيانات الحساب. تتطلب التغييرات دعمًا صريحًا من الخادم.",
            )}
          </p>
        )}
        <div className="ac-security-row">
          <span className="ac-row-icon">
            <KeyRound size={19} />
          </span>
          <div className="ac-row-content">
            <h3>{c("登录密码", "Password", "كلمة المرور")}</h3>
            <p>
              {c(
                "修改时验证当前密码；尚无手机号找回密码。",
                "Changes require your current password. Phone password recovery is unavailable.",
                "يتطلب التغيير كلمة المرور الحالية. الاستعادة عبر الهاتف غير متاحة.",
              )}
            </p>
          </div>
          <Button
            data-account-action="password"
            size="small"
            disabled={!allowed || pending}
            onClick={() => open("password")}
          >
            {c("修改密码", "Change password", "تغيير كلمة المرور")}
          </Button>
        </div>
        <div className="ac-security-row">
          <span className="ac-row-icon">
            <Smartphone size={19} />
          </span>
          <div className="ac-row-content">
            <h3>{c("手机号", "Phone", "الهاتف")}</h3>
            <Input
              className="ac-contact-value"
              id="account-phone"
              aria-label={c("绑定手机号", "Linked phone", "الهاتف المرتبط")}
              value={c("尚未接入", "Unavailable", "غير متاح")}
              readOnly
            />
            <p>
              {c(
                "手机绑定、验证码登录和找回密码尚未接入。",
                "Phone linking, SMS sign-in and password recovery are unavailable.",
                "ربط الهاتف والدخول بالرسائل واستعادة كلمة المرور غير متاحة.",
              )}
            </p>
          </div>
          <Button data-account-action="phone" size="small" disabled>
            {c("绑定", "Link", "ربط")}
          </Button>
        </div>
        <div className="ac-security-row">
          <span className="ac-row-icon">
            <Mail size={19} />
          </span>
          <div className="ac-row-content">
            <h3>{c("邮箱", "Email", "البريد الإلكتروني")}</h3>
            <Input
              className="ac-contact-value"
              id="account-email"
              aria-label={c("资料邮箱", "Profile email", "بريد الحساب")}
              value={
                profile
                  ? masked || c("未填写", "Not provided", "غير محدد")
                  : c("未确认", "Unconfirmed", "غير مؤكد")
              }
              readOnly
              dir={masked ? "ltr" : undefined}
            />
            <p>
              {c(
                "邮箱仅作为账号资料保存；当前使用用户名和密码登录，尚无邮箱验证。",
                "Email is a profile field. Sign in with your username and password; email verification is unavailable.",
                "البريد ضمن بيانات الحساب. ادخل باسم المستخدم وكلمة المرور؛ التحقق بالبريد غير متاح.",
              )}
            </p>
          </div>
          <Button
            data-account-action="email"
            size="small"
            disabled={!allowed || pending}
            onClick={() => open("email")}
          >
            {c("编辑", "Edit", "تعديل")}
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
            ? c("修改登录密码", "Change password", "تغيير كلمة المرور")
            : c("修改资料邮箱", "Edit profile email", "تعديل بريد الحساب")
        }
        description={
          mode === "password"
            ? c(
                "验证当前密码后保存新密码。登录设备保持不变，可单独管理。",
                "Verify your current password to save a new one. Signed-in devices stay active and can be managed separately.",
                "تحقق بكلمة المرور الحالية لحفظ جديدة. تبقى الأجهزة المسجلة نشطة ويمكن إدارتها منفصلة.",
              )
            : c(
                "邮箱仅作为资料保存，不发送验证码，也不用于登录。",
                "Email is profile data. No verification code is sent and it is not a sign-in method.",
                "البريد من بيانات الحساب. لا يُرسل رمز تحقق ولا يُستخدم للدخول.",
              )
        }
        initialFocus={first}
        preventOutsideClose
        className="ac-security-dialog"
        footer={
          <>
            <Button onClick={close} disabled={pending}>
              {c("取消", "Cancel", "إلغاء")}
            </Button>
            <Button
              variant="primary"
              form="account-security-form"
              type="submit"
              loading={pending}
              disabled={!allowed || uncertain}
            >
              {c("保存", "Save", "حفظ")}
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
                  {c("当前密码", "Current password", "كلمة المرور الحالية")}
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
                  {c("新密码", "New password", "كلمة المرور الجديدة")}
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
                    "至少8个字符，最多64字节；非英文字符可能更早达到上限。",
                    "At least 8 characters, at most 64 bytes. Non-ASCII characters may reach the limit sooner.",
                    "8 أحرف على الأقل و64 بايتًا كحد أقصى. قد تبلغ الأحرف غير اللاتينية الحد أسرع.",
                  )}
                </small>
              </div>
              <div className="ac-field">
                <label htmlFor="security-confirm-password">
                  {c(
                    "确认新密码",
                    "Confirm new password",
                    "تأكيد كلمة المرور الجديدة",
                  )}
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
                  ? c("隐藏密码", "Hide password", "إخفاء كلمة المرور")
                  : c("显示密码", "Show password", "إظهار كلمة المرور")}
              </Button>
            </>
          ) : (
            <div className="ac-field">
              <label htmlFor="security-email">
                {c(
                  "资料邮箱（可留空）",
                  "Profile email (optional)",
                  "بريد الحساب (اختياري)",
                )}
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

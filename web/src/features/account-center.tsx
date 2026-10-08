import { useEffect, useRef, useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import {
  Check,
  Copy,
  Eye,
  EyeOff,
  History,
  Info,
  KeyRound,
  LogOut,
  Mail,
  Monitor,
  ShieldCheck,
  Smartphone,
  UserRound,
} from "lucide-react";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/primitives";
import { Modal } from "../components/ui/overlay";
import { useProductPreview } from "../lib/product-preview";
import { useSystemAccess, isAdmin, ROLE_USER } from "../lib/system-access";
import { logoutUser } from "../lib/api/auth";
import { useUpdateSelf } from "../lib/api/user";
import { ApiError } from "../lib/api/http";
import { usePreferences } from "../lib/preferences";
import {
  accountDisplayName,
  DEMO_ACCOUNT_PROFILE,
} from "../lib/account-identity";
import {
  demoAuth,
  DEMO_ACCOUNT,
  DemoAuthError,
  normalizeDemoEmail,
  normalizeDemoPhone,
} from "../lib/demo-auth";
import "./account-center.css";

type SecurityMode = "password" | "phone" | "email";
type ContactDraft = { mode: "phone" | "email"; target: string };
const draftKey = (id: string) => `veyra.account-security-draft.${id}`;
function readDraft(id: string): ContactDraft | null {
  try {
    const value = JSON.parse(sessionStorage.getItem(draftKey(id)) ?? "null");
    return value &&
      (value.mode === "phone" || value.mode === "email") &&
      typeof value.target === "string"
      ? value
      : null;
  } catch {
    return null;
  }
}

function hasContactConflict(
  profile: { phone?: string; email?: string },
  identity: { phone?: string; email?: string } | null,
) {
  if (!identity) return false;
  try {
    return Boolean(
      (profile.phone && normalizeDemoPhone(profile.phone) !== identity.phone) ||
      (profile.email && normalizeDemoEmail(profile.email) !== identity.email),
    );
  } catch {
    return true;
  }
}

export default function AccountCenter({
  onDeactivate,
}: {
  onDeactivate: () => void;
}) {
  const user = useProductPreview((state) => state.user);
  const applySecurity = useProductPreview(
    (state) => state.applyAccountSecurity,
  );
  const terminateOthers = useProductPreview(
    (state) => state.terminateOtherSessions,
  );
  const logout = useProductPreview((state) => state.logout);
  const queryClient = useQueryClient();
  const updateSelfMut = useUpdateSelf();
  const role = useSystemAccess((state) => state.role);
  const language = usePreferences((state) => state.language);
  const navigate = useNavigate();
  const c = (zh: string, en: string, ar: string) =>
    language === "zh" ? zh : language === "ar" ? ar : en;
  const [mode, setMode] = useState<SecurityMode | null>(
    () => readDraft(user.id)?.mode ?? null,
  );
  const [target, setTarget] = useState(() => readDraft(user.id)?.target ?? "");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [code, setCode] = useState("");
  const [visible, setVisible] = useState<Record<string, boolean>>({});
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [copied, setCopied] = useState(false);
  const [now, setNow] = useState(Date.now());
  const firstInput = useRef<HTMLInputElement>(null);
  const errorRef = useRef<HTMLDivElement>(null);
  const operation = useRef(0);
  const previousAccount = useRef(user.id);
  const purpose = mode === "email" ? "bind-email" : "change-phone";
  const challenge =
    mode && mode !== "password"
      ? demoAuth.challenge(purpose, target, user.id)
      : undefined;
  const cooldown = Math.max(
    0,
    Math.ceil((demoAuth.cooldownUntil() - now) / 1000),
  );
  const credentialAvailable =
    user.isLoggedIn &&
    user.accountStatus !== "deactivated" &&
    Boolean(demoAuth.identity(user.id)) &&
    !hasContactConflict(user, demoAuth.identity(user.id));
  const profileName = accountDisplayName(
    user,
    c("演示账号", "Demo account", "حساب تجريبي"),
  );
  const phone = user.phone?.replace(/(\d{3})\d{4}(\d{4})$/, "$1 **** $2");
  const email = user.email
    ? user.email.replace(/^(.{1,2})[^@]*(@.*)$/, "$1***$2")
    : "";
  const otherSessions = user.sessions.filter(
    (session) => !session.isCurrent,
  ).length;
  const events = user.securityEvents ?? [];
  const formatDate = (value?: string) =>
    value && Number.isFinite(Date.parse(value))
      ? new Intl.DateTimeFormat(
          language === "zh" ? "zh-CN" : language === "ar" ? "ar" : "en",
          {
            year: "numeric",
            month: "short",
            day: "numeric",
            hour: "2-digit",
            minute: "2-digit",
          },
        ).format(new Date(value))
      : c("尚无记录", "Not recorded", "غير مسجل");

  useEffect(() => {
    if (!mode) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [mode]);
  useEffect(() => {
    if (error) errorRef.current?.focus();
  }, [error]);
  useEffect(() => {
    if (previousAccount.current !== user.id) {
      operation.current += 1;
      previousAccount.current = user.id;
      setMode(null);
      setTarget("");
      setCurrentPassword("");
      setNewPassword("");
      setConfirmation("");
      setCode("");
      setError("");
      setNotice("");
      setCopied(false);
      setBusy(false);
      return;
    }
    try {
      if (mode && mode !== "password")
        sessionStorage.setItem(
          draftKey(user.id),
          JSON.stringify({ mode, target }),
        );
      else sessionStorage.removeItem(draftKey(user.id));
    } catch {
      /* A draft is optional; credential persistence reports its own errors. */
    }
  }, [mode, target, user.id]);
  useEffect(
    () => () => {
      operation.current += 1;
    },
    [],
  );

  function close() {
    if (busy) return;
    operation.current += 1;
    setMode(null);
    setTarget("");
    setCurrentPassword("");
    setNewPassword("");
    setConfirmation("");
    setCode("");
    setVisible({});
    setError("");
  }
  function open(next: SecurityMode) {
    setMode(next);
    setTarget("");
    setCurrentPassword("");
    setNewPassword("");
    setConfirmation("");
    setCode("");
    setVisible({});
    setError("");
    setNotice("");
    setNow(Date.now());
  }
  function explain(caught: unknown) {
    if (!(caught instanceof DemoAuthError))
      return c(
        "未能完成操作，输入已保留，请重试。",
        "Unable to complete this action. Your inputs are preserved; try again.",
        "تعذر إكمال الإجراء. احتُفظ بالمدخلات؛ حاول مجددًا.",
      );
    const messages: Record<string, string> = {
      INVALID_CREDENTIALS: c(
        "当前密码不正确，请重新输入。",
        "The current password is incorrect. Please try again.",
        "كلمة المرور الحالية غير صحيحة. حاول مجددًا.",
      ),
      ACCOUNT_NOT_FOUND: c(
        "当前标签页没有该账号的登录凭证，请重新登录。",
        "This tab has no credentials for this account. Sign in again.",
        "لا توجد بيانات دخول لهذا الحساب في علامة التبويب. سجّل الدخول مجددًا.",
      ),
      AUTH_STATE_CHANGED: c(
        "登录状态发生变化，请关闭此窗口并重新登录。",
        "Your sign-in state changed. Close this dialog and sign in again.",
        "تغيرت حالة الدخول. أغلق النافذة وسجّل الدخول مجددًا.",
      ),
      INVALID_EMAIL: c(
        "请输入有效邮箱地址。",
        "Enter a valid email address.",
        "أدخل عنوان بريد إلكتروني صالحًا.",
      ),
      INVALID_PHONE: c(
        "请输入有效手机号，例如 13800138000。",
        "Enter a valid phone number, such as 13800138000.",
        "أدخل رقم هاتف صالحًا، مثل 13800138000.",
      ),
      EMAIL_EXISTS: c(
        "该邮箱已被其他账号使用，请更换邮箱。",
        "This email is used by another account. Use a different address.",
        "هذا البريد مستخدم في حساب آخر. استخدم عنوانًا آخر.",
      ),
      PHONE_EXISTS: c(
        "该手机号已被其他账号使用，请更换号码。",
        "This phone belongs to another account. Use a different number.",
        "هذا الهاتف مستخدم في حساب آخر. استخدم رقمًا آخر.",
      ),
      CONTACT_UNCHANGED: c(
        "与当前绑定信息相同，无需再次绑定。",
        "This is already linked to your account.",
        "هذه البيانات مرتبطة بحسابك بالفعل.",
      ),
      PASSWORD_UNCHANGED: c(
        "新密码不能与当前密码相同。",
        "Choose a password different from your current password.",
        "اختر كلمة مرور مختلفة عن الحالية.",
      ),
      WEAK_PASSWORD: c(
        "密码至少 8 位，需同时包含字母和数字。",
        "Use at least 8 characters with both letters and numbers.",
        "استخدم 8 محارف على الأقل تشمل حروفًا وأرقامًا.",
      ),
      CODE_REQUIRED: c(
        "请输入验证码。",
        "Enter the verification code.",
        "أدخل رمز التحقق.",
      ),
      CODE_NOT_ISSUED: c(
        "请先获取当前号码或邮箱的演示验证码。",
        "First get a demo code for this phone or email.",
        "اطلب أولًا رمزًا تجريبيًا لهذا الهاتف أو البريد.",
      ),
      CODE_INVALID: c(
        "验证码不正确，请检查后重试。",
        "The code is incorrect. Please try again.",
        "رمز التحقق غير صحيح. حاول مجددًا.",
      ),
      CODE_EXPIRED: c(
        "验证码已过期，请重新获取。",
        "The code expired. Request another code.",
        "انتهت صلاحية الرمز. اطلب رمزًا جديدًا.",
      ),
      CODE_EXHAUSTED: c(
        "验证码错误次数过多，请重新获取。",
        "Too many incorrect codes. Request another code.",
        "تكررت أخطاء الرمز. اطلب رمزًا جديدًا.",
      ),
      CODE_COOLDOWN: c(
        "请等待倒计时结束后重新获取验证码。",
        "Wait for the countdown before requesting another code.",
        "انتظر انتهاء العد التنازلي قبل طلب رمز آخر.",
      ),
      TEMPORARILY_LOCKED: c(
        "密码尝试次数过多，请稍后重试，或通过手机号找回密码。",
        "Too many password attempts. Try later or reset via your phone.",
        "محاولات كلمة المرور كثيرة. حاول لاحقًا أو استعدها عبر الهاتف.",
      ),
      STORAGE_UNAVAILABLE: c(
        "浏览器无法保存修改，账号信息未更新。请检查存储权限后重试。",
        "Your browser could not save this change. Check storage access and try again.",
        "تعذر حفظ التغيير في المتصفح. تحقق من صلاحية التخزين وحاول مجددًا.",
      ),
      CRYPTO_UNAVAILABLE: c(
        "当前浏览器无法处理密码，请使用本机预览或安全连接。",
        "Password processing is unavailable. Use a local preview or secure connection.",
        "معالجة كلمة المرور غير متاحة. استخدم معاينة محلية أو اتصالًا آمنًا.",
      ),
    };
    return (
      messages[caught.code] ??
      c(
        "未能完成操作，输入已保留，请重试。",
        "Unable to complete this action. Your inputs are preserved; try again.",
        "تعذر إكمال الإجراء. احتُفظ بالمدخلات؛ حاول مجددًا.",
      )
    );
  }
  async function issueCode() {
    if (!credentialAvailable) return;
    const token = ++operation.current;
    setError("");
    setBusy(true);
    try {
      await demoAuth.issueSecurityCode({
        accountId: user.id,
        currentPassword,
        purpose,
        target,
      });
      if (token === operation.current) {
        setCode("");
        setNow(Date.now());
      }
    } catch (caught) {
      if (token === operation.current) setError(explain(caught));
    } finally {
      if (token === operation.current) setBusy(false);
    }
  }
  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!mode || busy || !credentialAvailable) return;
    if (mode === "password" && newPassword !== confirmation) {
      setError(
        c(
          "两次输入的新密码不一致，请检查确认密码。",
          "The new passwords do not match. Check the confirmation.",
          "كلمتا المرور الجديدتان غير متطابقتين. تحقق من التأكيد.",
        ),
      );
      return;
    }
    const token = ++operation.current;
    setError("");
    setBusy(true);
    try {
      // Password and email changes go to the backend; phone stays on
      // the demo path because the backend has no phone support.
      if (mode === "password") {
        await updateSelfMut.mutateAsync({
          current_password: currentPassword,
          new_password: newPassword,
        });
        if (token !== operation.current) return;
        applySecurity(
          {
            id: user.id,
            username: user.username ?? user.email ?? "",
            email: user.email,
            phone: user.phone,
          },
          "password_changed",
        );
      } else if (mode === "email") {
        await updateSelfMut.mutateAsync({
          email: target,
        });
        if (token !== operation.current) return;
        applySecurity(
          {
            id: user.id,
            username: user.username ?? user.email ?? "",
            email: target,
            phone: user.phone,
          },
          "email_changed",
        );
      } else {
        // Phone change: demo-only (no backend support).
        const identity = await demoAuth.changeContact({
          accountId: user.id,
          currentPassword,
          purpose,
          target,
          code,
        });
        if (token !== operation.current) return;
        applySecurity(identity, "phone_changed");
      }
      setNotice(
        mode === "password"
          ? c(
              "密码已更新，下次登录请使用新密码。其他模拟设备已下线，接口密钥保持不变。",
              "Password updated. Use it on your next sign-in. Other demo devices were signed out; API keys are unchanged.",
              "حُدّثت كلمة المرور. استخدمها للدخول التالي. سُجل خروج الأجهزة التجريبية الأخرى دون تغيير مفاتيح الواجهة.",
            )
          : c(
              "绑定信息已更新，可用于下次登录。其他模拟设备已下线，余额和密钥权限保持不变。",
              "Contact updated and available for your next sign-in. Other demo devices were signed out; balance and key permissions are unchanged.",
              "حُدّثت بيانات الاتصال ويمكن استخدامها للدخول التالي. سُجل خروج الأجهزة الأخرى دون تغيير الرصيد أو أذونات المفاتيح.",
            ),
      );
      setMode(null);
      setTarget("");
      setCurrentPassword("");
      setNewPassword("");
      setConfirmation("");
      setCode("");
      setVisible({});
    } catch (caught) {
      if (token !== operation.current) return;
      if (caught instanceof ApiError) {
        if (caught.status === 403 && caught.message.includes("current password")) {
          setError(
            c(
              "当前密码不正确。",
              "Current password is incorrect.",
              "كلمة المرور الحالية غير صحيحة.",
            ),
          );
        } else {
          setError(caught.message);
        }
      } else {
        setError(explain(caught));
      }
    } finally {
      if (token === operation.current) setBusy(false);
    }
  }
  async function copyId() {
    try {
      await navigator.clipboard.writeText(user.id);
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
  async function signOut(path = "/login") {
    try {
      await logoutUser();
    } catch {
      // Best-effort: clear local state even if the server is unreachable.
    }
    useSystemAccess.getState().setRole(ROLE_USER);
    logout();
    queryClient.clear();
    navigate(path);
  }
  function passwordField(
    id: string,
    label: string,
    value: string,
    setter: (next: string) => void,
    autoComplete: "current-password" | "new-password",
    hint?: string,
  ) {
    return (
      <div className="ac-field">
        <label htmlFor={id}>{label}</label>
        <div className="ac-password">
          <Input
            ref={id === "security-current-password" ? firstInput : undefined}
            id={id}
            type={visible[id] ? "text" : "password"}
            autoComplete={autoComplete}
            disabled={busy}
            value={value}
            onChange={(event) => {
              setter(event.target.value);
              setError("");
            }}
            required
            minLength={autoComplete === "new-password" ? 8 : undefined}
            maxLength={128}
            aria-describedby={hint ? `${id}-hint` : undefined}
          />
          <button
            type="button"
            aria-label={
              visible[id]
                ? c("隐藏密码", "Hide password", "إخفاء كلمة المرور")
                : c("显示密码", "Show password", "إظهار كلمة المرور")
            }
            aria-pressed={Boolean(visible[id])}
            onClick={() =>
              setVisible((state) => ({ ...state, [id]: !state[id] }))
            }
          >
            {visible[id] ? <EyeOff size={17} /> : <Eye size={17} />}
          </button>
        </div>
        {hint && <small id={`${id}-hint`}>{hint}</small>}
      </div>
    );
  }
  const modalTitle =
    mode === "password"
      ? c("修改登录密码", "Change password", "تغيير كلمة المرور")
      : mode === "phone"
        ? c(
            user.phone ? "更换手机号" : "绑定手机号",
            user.phone ? "Change phone" : "Link phone",
            user.phone ? "تغيير الهاتف" : "ربط الهاتف",
          )
        : c(
            user.email ? "更换邮箱" : "绑定邮箱",
            user.email ? "Change email" : "Link email",
            user.email ? "تغيير البريد" : "ربط البريد",
          );
  const actionLabel = (action: string) =>
    ({
      password_changed: c(
        "修改登录密码",
        "Password changed",
        "تغيير كلمة المرور",
      ),
      phone_changed: c("更新绑定手机号", "Phone updated", "تحديث الهاتف"),
      email_changed: c("更新绑定邮箱", "Email updated", "تحديث البريد"),
      sessions_revoked: c(
        "下线其他模拟设备",
        "Other demo devices signed out",
        "خروج الأجهزة التجريبية الأخرى",
      ),
    })[action] ??
    c("账号安全操作", "Account security action", "إجراء أمان الحساب");

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
          {c("本地演示", "Local demo", "عرض محلي")}
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
            className={`ac-badge ${user.accountStatus === "normal" ? "ac-badge--success" : "ac-badge--warning"}`}
          >
            {user.accountStatus === "normal"
              ? c("正常", "Active", "نشط")
              : user.accountStatus === "frozen"
                ? c("已冻结", "Frozen", "مجمّد")
                : c("已注销", "Closed", "مغلق")}
          </span>
        </div>
        <div className="ac-identity-grid">
          <div className="ac-field">
            <label htmlFor="account-username">
              {user.username
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
              <Input id="account-id" value={user.id} readOnly dir="ltr" />
              <Button
                size="icon"
                aria-label={c(
                  "复制用户 ID（标识）",
                  "Copy user ID",
                  "نسخ معرّف المستخدم",
                )}
                onClick={copyId}
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
              {isAdmin(role)
                ? c(
                    "平台管理员 · 演示",
                    "Platform administrator · Demo",
                    "مدير المنصة · تجريبي",
                  )
                : c("普通用户", "User", "مستخدم")}
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
              {user.createdAt
                ? formatDate(user.createdAt)
                : c(
                    "历史账号，时间未记录",
                    "Existing account; date not recorded",
                    "حساب سابق؛ التاريخ غير مسجل",
                  )}
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
      </section>
      <section className="ac-card" aria-labelledby="ac-security-title">
        <div className="ac-section-heading">
          <div>
            <h2 id="ac-security-title">
              <ShieldCheck size={18} />
              {c("登录与安全", "Sign-in & security", "الدخول والأمان")}
            </h2>
            <p>
              {c(
                "手机号和邮箱是登录方式；更换后用户标识保持不变。",
                "Phone and email are sign-in methods. Changing them keeps your user ID.",
                "الهاتف والبريد طريقتان للدخول؛ تغييرهما لا يغير معرّف المستخدم.",
              )}
            </p>
          </div>
        </div>
        {!credentialAvailable && (
          <div className="ac-recovery">
            <Info size={17} />
            <p>
              {c(
                user.isLoggedIn
                  ? "当前标签页未保留该账号的有效登录凭证，请重新登录后修改安全信息。现有余额与记录会保留。"
                  : "当前未登录，请重新登录后修改安全信息。现有余额与记录会保留。",
                user.isLoggedIn
                  ? "This tab has no valid credentials for this account. Sign in again to change security settings; your balance and records are kept."
                  : "You are signed out. Sign in again to change security settings; your balance and records are kept.",
                user.isLoggedIn
                  ? "لا توجد بيانات دخول صالحة لهذا الحساب في علامة التبويب. سجّل الدخول مجددًا لتغيير الأمان؛ سيبقى الرصيد والسجلات."
                  : "أنت خارج الحساب. سجّل الدخول مجددًا لتغيير الأمان؛ سيبقى الرصيد والسجلات.",
              )}
            </p>
            <Button size="small" onClick={() => signOut()}>
              {c("重新登录", "Sign in again", "الدخول مجددًا")}
            </Button>
          </div>
        )}
        <div className="ac-security-row">
          <span className="ac-row-icon">
            <KeyRound size={19} />
          </span>
          <div className="ac-row-content">
            <h3>{c("登录密码", "Password", "كلمة المرور")}</h3>
            <p>
              {user.passwordUpdatedAt
                ? `${c("最近修改：", "Last changed: ", "آخر تغيير: ")}${formatDate(user.passwordUpdatedAt)}`
                : c(
                    "使用当前密码验证身份后修改。忘记密码可通过手机号找回。",
                    "Verify your current password to change it, or reset via your phone.",
                    "تحقق بكلمة المرور الحالية لتغييرها، أو استعدها عبر الهاتف.",
                  )}
            </p>
          </div>
          <Button
            data-account-action="password"
            size="small"
            disabled={!credentialAvailable}
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
              value={phone ?? c("未绑定", "Not linked", "غير مرتبط")}
              readOnly
              dir={phone ? "ltr" : undefined}
            />
            <p>
              {c(
                "用于手机号密码登录、验证码登录与找回密码。",
                "Used for password sign-in, code sign-in and password reset.",
                "للدخول بكلمة المرور أو الرمز واستعادة كلمة المرور.",
              )}
            </p>
          </div>
          <Button
            data-account-action="phone"
            size="small"
            disabled={!credentialAvailable}
            onClick={() => open("phone")}
          >
            {user.phone
              ? c("更换", "Change", "تغيير")
              : c("绑定", "Link", "ربط")}
          </Button>
        </div>
        <div className="ac-security-row">
          <span className="ac-row-icon">
            <Mail size={19} />
          </span>
          <div className="ac-row-content">
            <h3>
              {c("邮箱", "Email", "البريد الإلكتروني")}
              <span className="ac-optional">
                {c("选填", "Optional", "اختياري")}
              </span>
            </h3>
            <Input
              className="ac-contact-value"
              id="account-email"
              aria-label={c("绑定邮箱", "Linked email", "البريد المرتبط")}
              value={email || c("未绑定", "Not linked", "غير مرتبط")}
              readOnly
              dir={email ? "ltr" : undefined}
            />
            <p>
              {user.email && !user.emailVerified
                ? c(
                    "历史登录邮箱；未补记验证状态，可重新验证并更换。",
                    "Existing login email; verification history is unavailable. Verify a new email to replace it.",
                    "بريد دخول سابق؛ سجل التحقق غير متاح. تحقق من بريد جديد لاستبداله.",
                  )
                : c(
                    "验证后可用于邮箱密码登录；不影响正常接入与调用。",
                    "After verification, use it for email/password sign-in. It is optional for model access.",
                    "بعد التحقق يمكن الدخول بالبريد وكلمة المرور؛ وهو اختياري للوصول إلى النماذج.",
                  )}
            </p>
          </div>
          <Button
            data-account-action="email"
            size="small"
            disabled={!credentialAvailable}
            onClick={() => open("email")}
          >
            {user.email
              ? c("更换", "Change", "تغيير")
              : c("绑定", "Link", "ربط")}
          </Button>
        </div>
        <div className="ac-section-note">
          <Info size={15} />
          <span>
            {c(
              "密码和邮箱修改已联通后端；手机绑定仍为本地演示，不发送真实短信。修改安全信息不改变余额、历史账单或接口密钥权限。",
              "Password and email changes are backed by the server; phone binding remains a local demo (no real SMS). Security changes preserve your balance, billing history and API-key permissions.",
              "تعديل كلمة المرور والبريد مدعوم من الخادم؛ ربط الهاتف تجريبي محلي بلا رسائل حقيقية. لا تغير إجراءات الأمان الرصيد أو الفواتير أو أذونات المفاتيح.",
            )}
          </span>
        </div>
      </section>
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
                  "演示设备列表；下线操作仅改变本地状态。",
                  "Demo devices; signing out changes local state only.",
                  "أجهزة تجريبية؛ الخروج يغير الحالة المحلية فقط.",
                )}
              </p>
            </div>
          </div>
          <div className="ac-devices">
            {user.sessions.map((session) => (
              <div className="ac-device" key={session.id}>
                <Monitor size={18} />
                <div>
                  <strong>
                    {session.device.replace(/\s*\(Current\)/, "")}
                  </strong>
                  <p dir="ltr">{session.ip}</p>
                  <small>
                    {session.isCurrent
                      ? c("正在使用", "In use now", "قيد الاستخدام")
                      : c(
                          "模拟历史会话",
                          "Simulated previous session",
                          "جلسة سابقة تجريبية",
                        )}
                  </small>
                </div>
                {session.isCurrent && (
                  <span className="ac-badge ac-badge--success">
                    {c("当前", "Current", "الحالي")}
                  </span>
                )}
              </div>
            ))}
          </div>
          <div className="ac-device-actions">
            <Button
              size="small"
              disabled={!otherSessions}
              onClick={() => {
                terminateOthers();
                setNotice(
                  c(
                    "其他模拟设备已下线。",
                    "Other demo devices were signed out.",
                    "سُجل خروج الأجهزة التجريبية الأخرى.",
                  ),
                );
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
        <section className="ac-card" aria-labelledby="ac-history-title">
          <div className="ac-section-heading">
            <div>
              <h2 id="ac-history-title">
                <History size={18} />
                {c(
                  "最近安全操作",
                  "Recent security activity",
                  "نشاط الأمان الأخير",
                )}
              </h2>
              <p>
                {c(
                  "记录本账号中心的成功操作，不含密码或验证码。",
                  "Successful actions in this account center; no passwords or codes are recorded.",
                  "الإجراءات الناجحة في مركز الحساب هذا دون حفظ كلمات المرور أو الرموز.",
                )}
              </p>
            </div>
          </div>
          {events.length ? (
            <ol className="ac-events">
              {events.slice(0, 5).map((entry) => (
                <li key={entry.id}>
                  <span className="ac-event-dot" />
                  <div>
                    <strong>{actionLabel(entry.action)}</strong>
                    <time dateTime={entry.time}>{formatDate(entry.time)}</time>
                  </div>
                </li>
              ))}
            </ol>
          ) : (
            <div className="ac-empty">
              <History size={25} />
              <strong>
                {c(
                  "暂无安全操作",
                  "No security activity yet",
                  "لا يوجد نشاط أمان بعد",
                )}
              </strong>
              <p>
                {c(
                  "修改密码、绑定信息或下线设备后，记录会出现在这里。",
                  "Password, contact and device changes will appear here.",
                  "ستظهر هنا تغييرات كلمة المرور وبيانات الاتصال والأجهزة.",
                )}
              </p>
            </div>
          )}
        </section>
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

      <Modal
        open={Boolean(mode)}
        onOpenChange={(next) => {
          if (!next) close();
        }}
        title={modalTitle}
        description={
          mode === "password"
            ? c(
                "验证当前密码后设置新密码。保存后其他模拟设备会下线，当前会话保留。",
                "Verify your current password, then set a new one. Other demo devices will be signed out; this session stays active.",
                "تحقق بكلمة المرور الحالية ثم عيّن جديدة. ستخرج الأجهزة التجريبية الأخرى وتبقى هذه الجلسة.",
              )
            : c(
                "先验证当前密码，再验证新的联系方式。确认前不会替换当前绑定。",
                "Verify your password, then the new contact. Your existing contact stays until you confirm.",
                "تحقق بكلمة المرور ثم ببيانات الاتصال الجديدة. تبقى البيانات الحالية حتى التأكيد.",
              )
        }
        initialFocus={firstInput}
        preventOutsideClose
        className="ac-security-dialog"
        footer={
          <>
            <Button onClick={close} disabled={busy}>
              {c("取消", "Cancel", "إلغاء")}
            </Button>
            <Button
              variant="primary"
              form="account-security-form"
              type="submit"
              loading={busy}
              disabled={!credentialAvailable}
            >
              {mode === "password"
                ? c("保存新密码", "Save password", "حفظ كلمة المرور")
                : c("确认绑定", "Confirm contact", "تأكيد الربط")}
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
              tabIndex={-1}
              ref={errorRef}
              className="ac-form-error"
            >
              {error}
            </div>
          )}
          {passwordField(
            "security-current-password",
            c("当前密码", "Current password", "كلمة المرور الحالية"),
            currentPassword,
            setCurrentPassword,
            "current-password",
            user.id === DEMO_ACCOUNT_PROFILE.id
              ? c(
                  `默认演示账号的初始口令为 ${DEMO_ACCOUNT.password}（演示口令）；若已修改，请使用新密码。`,
                  `The default demo account's initial password is ${DEMO_ACCOUNT.password} (demo password). If you changed it, use your new password.`,
                  `كلمة المرور الأولية للحساب التجريبي الافتراضي هي ${DEMO_ACCOUNT.password} (كلمة مرور تجريبية). إذا غيّرتها، فاستخدم كلمة المرور الجديدة.`,
                )
              : undefined,
          )}
          {mode === "password" ? (
            <>
              {passwordField(
                "security-new-password",
                c("新密码", "New password", "كلمة المرور الجديدة"),
                newPassword,
                setNewPassword,
                "new-password",
                c(
                  "至少 8 位，同时包含字母和数字。",
                  "At least 8 characters, including letters and numbers.",
                  "8 محارف على الأقل تشمل حروفًا وأرقامًا.",
                ),
              )}
              {passwordField(
                "security-confirm-password",
                c(
                  "确认新密码",
                  "Confirm new password",
                  "تأكيد كلمة المرور الجديدة",
                ),
                confirmation,
                setConfirmation,
                "new-password",
              )}
            </>
          ) : (
            <>
              <div className="ac-field">
                <label htmlFor="security-target">
                  {mode === "phone"
                    ? c("新手机号", "New phone number", "رقم الهاتف الجديد")
                    : c("新邮箱", "New email address", "البريد الجديد")}
                </label>
                <Input
                  id="security-target"
                  disabled={busy}
                  type={mode === "phone" ? "tel" : "email"}
                  autoComplete={mode === "phone" ? "tel" : "email"}
                  value={target}
                  onChange={(event) => {
                    setTarget(event.target.value);
                    setCode("");
                    setError("");
                  }}
                  required
                  dir="ltr"
                  placeholder={
                    mode === "phone" ? "13800138000" : "name@example.com"
                  }
                />
                <small>
                  {mode === "phone"
                    ? c(
                        "支持中国大陆 11 位手机号，可带 +86 前缀。",
                        "Use an 11-digit mainland China mobile number, optionally prefixed with +86.",
                        "استخدم رقم هاتف محمول صينيًا من 11 رقمًا، ويمكن إضافة البادئة +86.",
                      )
                    : c(
                        "邮箱绑定为选填，验证后可用于密码登录。",
                        "Email is optional. Once verified, it can be used with your password.",
                        "البريد اختياري. بعد التحقق يمكن استخدامه مع كلمة المرور.",
                      )}
                </small>
              </div>
              <div className="ac-field">
                <label htmlFor="security-code">
                  {c("验证码", "Verification code", "رمز التحقق")}
                </label>
                <div className="ac-code">
                  <Input
                    id="security-code"
                    disabled={busy}
                    value={code}
                    onChange={(event) => {
                      setCode(
                        event.target.value.replace(/\D/g, "").slice(0, 6),
                      );
                      setError("");
                    }}
                    inputMode="numeric"
                    autoComplete="one-time-code"
                    required
                    pattern="[0-9]{6}"
                    maxLength={6}
                    placeholder={c(
                      "6 位验证码",
                      "6-digit code",
                      "رمز من 6 أرقام",
                    )}
                    dir="ltr"
                  />
                  <Button
                    data-account-send-code
                    size="small"
                    onClick={issueCode}
                    disabled={
                      busy ||
                      !credentialAvailable ||
                      !currentPassword ||
                      !target ||
                      cooldown > 0
                    }
                  >
                    {cooldown > 0
                      ? c(
                          `${cooldown} 秒后重试`,
                          `Retry in ${cooldown}s`,
                          `بعد ${cooldown} ث`,
                        )
                      : c("获取演示验证码", "Get demo code", "طلب رمز تجريبي")}
                  </Button>
                </div>
              </div>
              {challenge && (
                <div className="ac-code-preview" role="status">
                  <Info size={17} />
                  <div>
                    <strong>
                      {challenge.expiresAt > now
                        ? c(
                            `演示验证码：${challenge.code}`,
                            `Demo code: ${challenge.code}`,
                            `الرمز التجريبي: ${challenge.code}`,
                          )
                        : c(
                            "验证码已过期，请重新获取。",
                            "Code expired. Request another.",
                            "انتهت صلاحية الرمز. اطلب آخر.",
                          )}
                    </strong>
                    <p>
                      {c(
                        "未发送真实短信或邮件。验证码 10 分钟有效；刷新后需重新输入当前密码。",
                        "No real SMS or email was sent. Valid for 10 minutes; re-enter your current password after refreshing.",
                        "لم تُرسل رسالة حقيقية. الرمز صالح 10 دقائق؛ أعد إدخال كلمة المرور بعد التحديث.",
                      )}
                    </p>
                  </div>
                </div>
              )}
            </>
          )}
          <div className="ac-form-bottom">
            <span>
              {c(
                "密码和验证码不会保存为表单草稿。",
                "Passwords and entered codes are never saved as form drafts.",
                "لا تُحفظ كلمات المرور والرموز المدخلة كمسودات.",
              )}
            </span>
            <button type="button" onClick={() => signOut("/forgot-password")}>
              {c("忘记密码？", "Forgot password?", "نسيت كلمة المرور؟")}
            </button>
          </div>
        </form>
      </Modal>
    </div>
  );
}

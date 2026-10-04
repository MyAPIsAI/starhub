import { useEffect, useRef, useState, type FormEvent } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import * as Dialog from "@radix-ui/react-dialog";
import {
  ArrowLeft,
  ArrowRight,
  Check,
  Eye,
  EyeOff,
  Info,
  X,
} from "lucide-react";
import { BrandMark } from "../../components/brand";
import { Button } from "../../components/ui/button";
import { Input } from "../../components/ui/primitives";
import { usePreferences } from "../../lib/preferences";
import { useProductPreview } from "../../lib/product-preview";
import { useSystemAccess } from "../../lib/system-access";
import { demoAuth, DEMO_ACCOUNT, DemoAuthError } from "../../lib/demo-auth";
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
  const [phone, setPhone] = useState("");
  const [phoneCode, setPhoneCode] = useState("");
  const [registerPhone, setRegisterPhone] = useState("");
  const [username, setUsername] = useState("");
  const [registerCode, setRegisterCode] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [confirmError, setConfirmError] = useState(false);
  const [resetPhone, setResetPhone] = useState("");
  const [resetCode, setResetCode] = useState("");
  const [resetPassword, setResetPassword] = useState("");
  const [agreed, setAgreed] = useState(false);
  const [human, setHuman] = useState(false);
  const [visiblePasswords, setVisiblePasswords] = useState<
    Record<string, boolean>
  >({});
  const [error, setError] = useState("");
  const [resetDone, setResetDone] = useState(false);
  const [busy, setBusy] = useState(false);
  const [now, setNow] = useState(Date.now());
  const heading = useRef<HTMLHeadingElement>(null);
  const errorRef = useRef<HTMLDivElement>(null);
  const bodyRef = useRef<HTMLDivElement>(null);
  const previousView = useRef("");
  const policyTrigger = useRef<Policy | null>(null);
  const formScroll = useRef(0);
  const operation = useRef(0);
  const purpose =
    mode === "register"
      ? "register"
      : mode === "reset"
        ? "reset"
        : "phone-login";
  const target =
    mode === "register" ? registerPhone : mode === "reset" ? resetPhone : phone;
  const challenge = demoAuth.challenge(purpose, target);
  const resendSeconds = Math.max(
    0,
    Math.ceil((demoAuth.cooldownUntil() - now) / 1000),
  );
  const lockSeconds = Math.max(
    0,
    Math.ceil((demoAuth.lockedUntil() - now) / 1000),
  );

  useEffect(() => {
    if (!mode) return;
    setNow(Date.now());
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [mode]);
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
    setResetDone(false);
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
  function explain(caught: unknown) {
    if (!(caught instanceof DemoAuthError))
      return c(
        "暂时无法完成，请重试。输入已保留。",
        "Unable to continue. Your inputs are preserved; please retry.",
        "تعذر المتابعة. تم الاحتفاظ بالمدخلات؛ حاول مجددًا.",
      );
    const messages: Record<string, string> = {
      INVALID_USERNAME: c(
        "用户名需为 2–24 位，以文字开头，可含数字、下划线或短横线。",
        "Use 2–24 characters, starting with a letter. Letters, numbers, underscores and hyphens are allowed.",
        "استخدم 2–24 محرفًا تبدأ بحرف، ويمكن أن تتضمن أرقامًا وشرطات سفلية أو قصيرة.",
      ),
      USERNAME_EXISTS: c(
        "该用户名已被使用，请换一个。",
        "This username is already taken. Choose another.",
        "اسم المستخدم مستخدم بالفعل. اختر اسمًا آخر.",
      ),
      PHONE_EXISTS: c(
        "该手机号已注册，请直接登录或找回密码。",
        "This phone is already registered. Log in or reset your password.",
        "رقم الهاتف مسجل بالفعل. سجّل الدخول أو أعد تعيين كلمة المرور.",
      ),
      INVALID_PHONE: c(
        "请输入有效手机号，例如 13800138000。",
        "Enter a valid phone number, such as 13800138000.",
        "أدخل رقم هاتف صالحًا، مثل 13800138000.",
      ),
      INVALID_CREDENTIALS: c(
        "账号或密码不正确，请检查后重试。",
        "The account or password is incorrect. Please try again.",
        "الحساب أو كلمة المرور غير صحيحة. حاول مجددًا.",
      ),
      ACCOUNT_NOT_FOUND: c(
        "未找到该手机号对应的演示账号，请先注册或使用演示手机号。",
        "No demo account is linked to this phone. Register first or use the demo phone.",
        "لا يوجد حساب تجريبي مرتبط بهذا الهاتف. سجّل أولًا أو استخدم هاتف العرض.",
      ),
      WEAK_PASSWORD: c(
        "密码至少 8 位，且包含字母和数字。",
        "Use at least 8 characters, including letters and numbers.",
        "استخدم 8 أحرف على الأقل تتضمن حروفًا وأرقامًا.",
      ),
      CODE_REQUIRED: c(
        "请输入 6 位验证码。",
        "Enter the 6-digit code.",
        "أدخل الرمز المكون من 6 أرقام.",
      ),
      CODE_NOT_ISSUED: c(
        "请先为当前手机号获取验证码。",
        "Request a code for the current phone number first.",
        "اطلب رمزًا لرقم الهاتف الحالي أولًا.",
      ),
      CODE_INVALID: c(
        "验证码不正确，请重新输入。连续错 5 次后需重新获取。",
        "Incorrect code. Request a new one after 5 failed attempts.",
        "الرمز غير صحيح. اطلب رمزًا جديدًا بعد 5 محاولات خاطئة.",
      ),
      CODE_EXPIRED: c(
        "验证码已过期或失效，请重新获取。",
        "This code expired or was invalidated. Request a new one.",
        "انتهت صلاحية الرمز أو أُلغي. اطلب رمزًا جديدًا.",
      ),
      CODE_EXHAUSTED: c(
        "验证码已连续输错 5 次，请重新获取。",
        "Five incorrect attempts. Request a new code.",
        "خمس محاولات خاطئة. اطلب رمزًا جديدًا.",
      ),
      CODE_COOLDOWN: c(
        "获取过于频繁，请等待倒计时结束。",
        "Please wait until the resend countdown ends.",
        "انتظر انتهاء العد التنازلي قبل إعادة الطلب.",
      ),
      TEMPORARILY_LOCKED: c(
        "尝试次数过多，登录已暂时锁定，请等待倒计时结束。",
        "Too many attempts. Login is temporarily locked until the countdown ends.",
        "محاولات كثيرة. الدخول مقفل مؤقتًا حتى انتهاء العد التنازلي.",
      ),
      STORAGE_UNAVAILABLE: c(
        "浏览器会话存储不可用，无法保存演示账号。请允许存储后重试。",
        "Browser session storage is unavailable. Enable storage and retry.",
        "تخزين الجلسة غير متاح. فعّل التخزين وحاول مجددًا.",
      ),
      CRYPTO_UNAVAILABLE: c(
        "当前浏览器无法保存口令摘要，请使用本地预览地址或安全连接。",
        "Password hashing is unavailable. Use localhost or a secure connection.",
        "تجزئة كلمة المرور غير متاحة. استخدم اتصالًا آمنًا أو مضيفًا محليًا.",
      ),
    };
    return (
      messages[caught.code] ??
      c(
        "操作未完成，请检查输入后重试。",
        "Unable to complete. Check your inputs and retry.",
        "تعذر الإكمال. تحقق من المدخلات وحاول مجددًا.",
      )
    );
  }
  function sendCode() {
    try {
      demoAuth.issueCode(purpose, target);
      setError("");
    } catch (caught) {
      setError(explain(caught));
    }
    setNow(Date.now());
  }
  async function submit(event: FormEvent) {
    event.preventDefault();
    if (busy) return;
    setError("");
    if (mode !== "reset" && !agreed) {
      setError(
        c(
          "请阅读并勾选服务协议与隐私政策后继续。",
          "Read and accept the terms and privacy notice to continue.",
          "اقرأ اتفاقية الخدمة وسياسة الخصوصية ووافق عليهما للمتابعة.",
        ),
      );
      return;
    }
    if (mode === "register" && !human) {
      setError(
        c(
          "请完成人机验证演示勾选。",
          "Complete the demo verification checkbox.",
          "أكمل خانة التحقق التجريبية.",
        ),
      );
      return;
    }
    if (mode === "register" && newPassword !== confirmPassword) {
      setConfirmError(true);
      setError(
        c(
          "两次输入的密码不一致，请重新确认。",
          "Passwords do not match. Please confirm your password again.",
          "كلمتا المرور غير متطابقتين. أكّد كلمة المرور مجددًا.",
        ),
      );
      return;
    }
    setConfirmError(false);
    const ticket = ++operation.current;
    setBusy(true);
    try {
      if (mode === "reset") {
        await demoAuth.resetPassword(resetPhone, resetCode, resetPassword);
        if (ticket !== operation.current) return;
        setAccount(resetPhone);
        setPassword("");
        setResetPassword("");
        setResetCode("");
        setResetDone(true);
        return;
      }
      const identity =
        mode === "register"
          ? await demoAuth.register({
              username,
              phone: registerPhone,
              password: newPassword,
              code: registerCode,
            })
          : method === "phone"
            ? await demoAuth.loginPhone(phone, phoneCode)
            : await demoAuth.loginPassword(account, password);
      if (ticket !== operation.current) return;
      if (mode === "register") {
        useProductPreview
          .getState()
          .register(
            identity.email ?? "",
            identity.enterpriseName ?? identity.username,
            {
              id: identity.id,
              username: identity.username,
              phone: identity.phone,
              emailVerified: identity.emailVerified,
            },
          );
        useSystemAccess.getState().setRole("customer");
      } else
        useProductPreview
          .getState()
          .login(
            identity.email ?? "",
            identity.enterpriseName ?? identity.username,
            {
              id: identity.id,
              username: identity.username,
              phone: identity.phone,
              emailVerified: identity.emailVerified,
            },
          );
      setPassword("");
      setNewPassword("");
      setConfirmPassword("");
      setPhoneCode("");
      setRegisterCode("");
      navigate(mode === "register" ? "/console/connect" : "/console", {
        replace: true,
      });
    } catch (caught) {
      if (ticket === operation.current) setError(explain(caught));
    } finally {
      if (ticket === operation.current) {
        setBusy(false);
        setNow(Date.now());
      }
    }
  }
  const title =
    policy === "terms"
      ? c("服务协议", "Terms of service", "اتفاقية الخدمة")
      : policy === "privacy"
        ? c("隐私政策", "Privacy notice", "سياسة الخصوصية")
        : mode === "register"
          ? c("创建你的账户", "Create your account", "أنشئ حسابك")
          : mode === "reset"
            ? c("重置密码", "Reset your password", "إعادة تعيين كلمة المرور")
            : c("欢迎回来", "Welcome back", "مرحبًا بعودتك");
  const description = policy
    ? c(
        "演示草案 · 仅展示阅读与勾选流程",
        "Demo draft · for the reading and consent flow only",
        "مسودة تجريبية · لعرض القراءة والموافقة فقط",
      )
    : mode === "register"
      ? c(
          "一把密钥，开始你的多模型之旅。",
          "One key to start working with multiple models.",
          "مفتاح واحد لبدء العمل مع نماذج متعددة.",
        )
      : mode === "reset"
        ? c(
            "验证注册手机号后，设置新的演示密码。",
            "Verify your registered phone to set a new demo password.",
            "تحقق من هاتفك المسجل لتعيين كلمة مرور تجريبية جديدة.",
          )
        : c(
            "登录后，在熟悉的工具中自由切换模型。",
            "Sign in and switch models in the tools you know.",
            "سجّل الدخول وبدّل النماذج في أدواتك المفضلة.",
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
            maxLength={128}
            placeholder={
              id === "auth-confirm-password"
                ? c(
                    "请再次输入密码",
                    "Enter your password again",
                    "أدخل كلمة المرور مجددًا",
                  )
                : isNew
                  ? c(
                      "至少 8 位，含字母和数字",
                      "8+ characters, letters and numbers",
                      "8 أحرف أو أكثر، حروف وأرقام",
                    )
                  : c("请输入密码", "Enter your password", "أدخل كلمة المرور")
            }
          />
          <button
            type="button"
            className="auth-password-toggle"
            aria-label={
              visiblePasswords[id]
                ? c("隐藏密码", "Hide password", "إخفاء كلمة المرور")
                : c("显示密码", "Show password", "إظهار كلمة المرور")
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
  function codeField(value: string, update: (v: string) => void) {
    return (
      <div className="auth-field">
        <label htmlFor="auth-code">
          {c("验证码", "Verification code", "رمز التحقق")}
        </label>
        <div className="auth-code-input">
          <Input
            id="auth-code"
            inputMode="numeric"
            autoComplete="one-time-code"
            value={value}
            onChange={(e) => {
              update(e.target.value.replace(/\D/g, "").slice(0, 6));
              setError("");
            }}
            required
            pattern="[0-9]{6}"
            maxLength={6}
            placeholder={c("6 位验证码", "6-digit code", "رمز من 6 أرقام")}
            dir="ltr"
          />
          <Button disabled={busy || resendSeconds > 0} onClick={sendCode}>
            {resendSeconds > 0
              ? c(
                  `${resendSeconds} 秒后重试`,
                  `Retry in ${resendSeconds}s`,
                  `إعادة بعد ${resendSeconds}ث`,
                )
              : c("获取验证码", "Get code", "الحصول على رمز")}
          </Button>
        </div>
        {challenge && (
          <p className="auth-code-hint" role="status">
            {challenge.expiresAt > now && challenge.attemptsRemaining > 0 ? (
              <>
                {c("模拟验证码：", "Demo code: ", "رمز تجريبي: ")}
                <strong dir="ltr">{challenge.code}</strong>
                {c(
                  " · 10 分钟内有效，未发送真实消息",
                  " · valid for 10 minutes; no message sent",
                  " · صالح 10 دقائق؛ لم تُرسل رسالة",
                )}
              </>
            ) : (
              c(
                "验证码已过期或失效，请重新获取。",
                "Code expired or invalidated. Request a new one.",
                "انتهت صلاحية الرمز. اطلب رمزًا جديدًا.",
              )
            )}
          </p>
        )}
      </div>
    );
  }
  const consent = (
    <div className="auth-consent">
      <input
        id="auth-consent"
        aria-label={c(
          "我已阅读并同意服务协议，并知悉隐私政策",
          "I have read and agree to the terms of service and acknowledge the privacy notice",
          "قرأت وأوافق على اتفاقية الخدمة وأقر بالاطلاع على سياسة الخصوصية",
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
          {c("我已阅读并同意", "I have read and agree to ", "قرأت وأوافق على ")}
        </label>
        <button
          type="button"
          data-auth-policy="terms"
          onClick={() => showPolicy("terms")}
        >
          {c("《服务协议》", "Terms of service", "اتفاقية الخدمة")}
        </button>
        <label htmlFor="auth-consent">
          {c("，并知悉", ", and acknowledge ", "، وأقر بالاطلاع على ")}
        </label>
        <button
          type="button"
          data-auth-policy="privacy"
          onClick={() => showPolicy("privacy")}
        >
          {c("《隐私政策》", "Privacy notice", "سياسة الخصوصية")}
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
              <span>Veyra</span>
              <span className="auth-demo-badge">
                {c("演示", "Demo", "عرض")}
              </span>
            </div>
            <button
              type="button"
              className="auth-close"
              onClick={close}
              aria-label={c(
                "关闭并返回主页",
                "Close and return home",
                "إغلاق والعودة للرئيسية",
              )}
            >
              <X size={20} />
            </button>
            {policy && (
              <button type="button" className="auth-back" onClick={backToForm}>
                <ArrowLeft size={14} className="directional" />
                {c("返回表单", "Back to form", "العودة للنموذج")}
              </button>
            )}
            <Dialog.Title ref={heading} tabIndex={-1}>
              {title}
            </Dialog.Title>
            <Dialog.Description>{description}</Dialog.Description>
          </div>
          <div className="auth-dialog-body" ref={bodyRef}>
            {policy ? (
              <article className="auth-policy">
                <div className="auth-notice">
                  <Info size={16} />
                  <span>
                    {c(
                      "这不是正式法律文本，不代表已完成上线合规审核。",
                      "This is not a legal document or a production compliance review.",
                      "هذا ليس نصًا قانونيًا أو مراجعة امتثال للإنتاج.",
                    )}
                  </span>
                </div>
                {policy === "terms" ? (
                  <>
                    <h3>
                      {c("1. 演示范围", "1. Demo scope", "1. نطاق العرض")}
                    </h3>
                    <p>
                      {c(
                        "本项目用于体验模型接入、权限和费用流程。登录、验证码、模型调用及支付均为本地模拟，不提供真实模型服务，不发送短信或邮件。",
                        "This project demonstrates model access, permissions and costs. Authentication, codes, model calls and payments are simulated locally. No real model service, SMS or email is provided.",
                        "يعرض المشروع الوصول للنماذج والصلاحيات والتكاليف. المصادقة والرموز والطلبات والمدفوعات محلية تجريبية؛ لا خدمات نماذج أو رسائل فعلية.",
                      )}
                    </p>
                    <h3>
                      {c(
                        "2. 账户与预付费",
                        "2. Accounts and prepaid balance",
                        "2. الحساب والرصيد المسبق",
                      )}
                    </h3>
                    <p>
                      {c(
                        "注册会进入零余额的新账户演示，无免费额度。创建充值订单不代表到账。调用仍受分组权限、密钥额度及账户余额约束。",
                        "Registration starts a zero-balance demo with no free credits. Creating an order does not add funds. Calls remain subject to group permissions, key limits and balance.",
                        "يبدأ التسجيل برصيد تجريبي صفري دون ائتمان مجاني. إنشاء طلب شحن لا يضيف رصيدًا. تظل الطلبات خاضعة للصلاحيات وحدود المفتاح والرصيد.",
                      )}
                    </p>
                    <h3>
                      {c("3. 演示数据", "3. Demo data", "3. بيانات العرض")}
                    </h3>
                    <p>
                      {c(
                        "此设备共用一份演示工作台。注册新账户或重置场景会替换当前演示业务数据；请仅使用虚构资料。正式服务条款须在上线前另行确定。",
                        "This device shares one demo console. Registering or resetting a scenario replaces its business data. Use fictional details only. Production terms must be defined separately before launch.",
                        "يستخدم الجهاز لوحة تحكم تجريبية مشتركة. التسجيل أو إعادة السيناريو يستبدل بيانات الأعمال. استخدم بيانات خيالية فقط. يجب إعداد شروط الإنتاج قبل الإطلاق.",
                      )}
                    </p>
                  </>
                ) : (
                  <>
                    <h3>
                      {c(
                        "1. 当前保存的内容",
                        "1. What is stored",
                        "1. ما يُحفظ",
                      )}
                    </h3>
                    <p>
                      {c(
                        "演示账户和加盐口令摘要保存在当前标签页的会话存储中；明文密码不会持久保存。账号资料（包括用户标识、用户名、手机号及绑定邮箱）、业务演示数据和偏好保存在此浏览器的本地存储中。关闭标签页会结束本轮注册账号的演示会话。",
                        "Demo accounts and salted password hashes use this tab’s session storage; plain-text passwords are not persisted. Account profile details (including user ID, username, phone and linked email), business demo data and preferences use browser local storage. Closing the tab ends the registered-account demo session.",
                        "تُحفظ الحسابات التجريبية وتجزئات كلمات المرور المملحة في جلسة علامة التبويب دون حفظ كلمة المرور النصية. تُحفظ بيانات ملف الحساب، ومنها اسم المستخدم والهاتف، وبيانات العرض والتفضيلات محليًا. إغلاق التبويب ينهي جلسة الحساب المسجل.",
                      )}
                    </p>
                    <h3>
                      {c(
                        "2. 验证码与资料",
                        "2. Codes and details",
                        "2. الرموز والبيانات",
                      )}
                    </h3>
                    <p>
                      {c(
                        "验证码只在浏览器中生成和比对，不向手机号或邮箱发送消息。请勿输入真实密码、个人敏感信息或支付资料。这里只演示阅读与同意状态，不生成正式的合规同意记录。",
                        "Codes are generated and checked in the browser. No messages are sent to phones or email addresses. Do not enter real passwords, sensitive personal data or payment details. Consent here is a UI demonstration, not a production consent record.",
                        "تُنشأ الرموز وتُفحص في المتصفح دون إرسال رسائل. لا تدخل كلمات مرور حقيقية أو بيانات حساسة أو دفع. الموافقة هنا عرض للواجهة وليست سجل موافقة للإنتاج.",
                      )}
                    </p>
                    <h3>
                      {c("3. 清理方式", "3. Clearing data", "3. مسح البيانات")}
                    </h3>
                    <p>
                      {c(
                        "你可以关闭当前标签页结束认证演示会话，或在浏览器的网站数据设置中清除本地演示数据。正式隐私政策须在实际服务与数据处理方式确定后另行提供。",
                        "Close this tab to end the auth demo session, or clear this site’s browser data to remove local demo data. A production privacy notice must be provided when the real service and data processing are defined.",
                        "أغلق التبويب لإنهاء جلسة المصادقة التجريبية، أو امسح بيانات الموقع لإزالة العرض المحلي. تُعد سياسة الإنتاج عند تحديد الخدمة ومعالجة البيانات الفعلية.",
                      )}
                    </p>
                  </>
                )}
                <Button className="auth-submit" onClick={backToForm}>
                  {c(
                    "返回继续填写",
                    "Return to the form",
                    "العودة لإكمال النموذج",
                  )}
                </Button>
              </article>
            ) : mode === "reset" && resetDone ? (
              <div className="auth-reset-success" role="status">
                <span>
                  <Check size={24} />
                </span>
                <h3>
                  {c(
                    "演示密码已更新",
                    "Demo password updated",
                    "تم تحديث كلمة المرور التجريبية",
                  )}
                </h3>
                <p>
                  {c(
                    "可以使用用户名、手机号或已绑定邮箱配合新密码登录。没有发送真实短信。",
                    "Use your username, phone or linked email with the new password. No SMS was sent.",
                    "ادخل باسم المستخدم أو الهاتف وكلمة المرور الجديدة. لم تُرسل رسالة فعلية.",
                  )}
                </p>
                <Button
                  variant="primary"
                  className="auth-submit"
                  onClick={() => {
                    setMethod("password");
                    changeMode("login");
                  }}
                >
                  {c("返回登录", "Back to login", "العودة للدخول")}
                </Button>
              </div>
            ) : (
              <>
                {mode === "login" && (
                  <div
                    className="auth-methods"
                    role="group"
                    aria-label={c("登录方式", "Login method", "طريقة الدخول")}
                  >
                    <button
                      type="button"
                      aria-pressed={method === "password"}
                      data-state={method === "password" ? "active" : "inactive"}
                      onClick={() => setMethod("password")}
                    >
                      {c("账号密码", "Password", "كلمة المرور")}
                    </button>
                    <button
                      type="button"
                      aria-pressed={method === "phone"}
                      data-state={method === "phone" ? "active" : "inactive"}
                      onClick={() => setMethod("phone")}
                    >
                      {c("手机号验证码", "Phone code", "رمز الهاتف")}
                    </button>
                  </div>
                )}
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
                  {mode === "login" &&
                    method === "password" &&
                    lockSeconds > 0 && (
                      <p className="auth-lock" role="status">
                        {c(
                          `密码登录锁定还剩 ${Math.floor(lockSeconds / 60)} 分 ${lockSeconds % 60} 秒；可切换手机号验证码，或重置密码。`,
                          `Password login locked for ${Math.floor(lockSeconds / 60)}m ${lockSeconds % 60}s. Use a phone code or reset your password.`,
                          `أعد المحاولة بعد ${Math.floor(lockSeconds / 60)}د ${lockSeconds % 60}ث`,
                        )}
                      </p>
                    )}
                  {mode === "login" && method === "password" && (
                    <>
                      <div className="auth-field">
                        <label htmlFor="auth-account">
                          {c("账号", "Account", "الحساب")}
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
                            "用户名、手机号或已绑定邮箱",
                            "Username, phone or linked email",
                            "اسم المستخدم أو الهاتف أو البريد المرتبط",
                          )}
                          dir="ltr"
                        />
                      </div>
                      {passwordField(
                        "auth-password",
                        c("密码", "Password", "كلمة المرور"),
                        password,
                        setPassword,
                      )}
                      <div className="auth-helper-row">
                        <button
                          type="button"
                          onClick={() => {
                            setAccount(DEMO_ACCOUNT.username);
                            setPassword(DEMO_ACCOUNT.password);
                            setError("");
                          }}
                        >
                          {c(
                            "填入演示账号",
                            "Use demo account",
                            "استخدم الحساب التجريبي",
                          )}
                        </button>
                        <button
                          type="button"
                          onClick={() => {
                            setResetPhone(
                              /^1[3-9]\d{9}$/.test(account.trim())
                                ? account.trim()
                                : "",
                            );
                            changeMode("reset");
                          }}
                        >
                          {c(
                            "忘记密码？",
                            "Forgot password?",
                            "نسيت كلمة المرور؟",
                          )}
                        </button>
                      </div>
                    </>
                  )}
                  {mode === "login" && method === "phone" && (
                    <>
                      <div className="auth-field">
                        <label htmlFor="auth-phone">
                          {c("手机号", "Phone number", "رقم الهاتف")}
                        </label>
                        <div className="auth-phone-input">
                          <span dir="ltr">+86</span>
                          <Input
                            id="auth-phone"
                            type="tel"
                            inputMode="tel"
                            autoComplete="tel-national"
                            value={phone}
                            onChange={(e) => {
                              setPhone(e.target.value);
                              setPhoneCode("");
                              setError("");
                            }}
                            required
                            maxLength={11}
                            placeholder="13800138000"
                            dir="ltr"
                          />
                        </div>
                      </div>
                      {codeField(phoneCode, setPhoneCode)}
                      <div className="auth-helper-row">
                        <button
                          type="button"
                          onClick={() => {
                            setPhone("13800138000");
                            setPhoneCode("");
                            setError("");
                          }}
                        >
                          {c(
                            "填入演示手机号",
                            "Use demo phone",
                            "استخدم هاتف العرض",
                          )}
                        </button>
                        <span>
                          {c(
                            "支持本地已注册手机号",
                            "Registered demo phones supported",
                            "يدعم الهواتف التجريبية المسجلة",
                          )}
                        </span>
                      </div>
                    </>
                  )}
                  {mode === "register" && (
                    <>
                      <div className="auth-field">
                        <label htmlFor="auth-username">
                          {c("用户名", "Username", "اسم المستخدم")}
                        </label>
                        <Input
                          id="auth-username"
                          value={username}
                          onChange={(e) => {
                            setUsername(e.target.value);
                            setError("");
                          }}
                          required
                          minLength={2}
                          maxLength={24}
                          autoComplete="username"
                          placeholder={c(
                            "请输入用户名",
                            "Choose a username",
                            "اختر اسم مستخدم",
                          )}
                          aria-describedby="auth-username-hint"
                        />
                        <p id="auth-username-hint" className="auth-field-hint">
                          {c(
                            "2–24 位，以文字开头，可含数字、下划线或短横线",
                            "2–24 characters, starting with a letter; letters, numbers, _ and -",
                            "2–24 محرفًا تبدأ بحرف، مع أرقام أو شرطات سفلية أو قصيرة",
                          )}
                        </p>
                      </div>
                      <div className="auth-field">
                        <label htmlFor="auth-register-phone">
                          {c("手机号", "Phone number", "رقم الهاتف")}
                        </label>
                        <div className="auth-phone-input">
                          <span dir="ltr">+86</span>
                          <Input
                            id="auth-register-phone"
                            type="tel"
                            inputMode="tel"
                            value={registerPhone}
                            onChange={(e) => {
                              setRegisterPhone(e.target.value);
                              setRegisterCode("");
                              setError("");
                            }}
                            required
                            maxLength={11}
                            autoComplete="tel-national"
                            placeholder={c(
                              "请输入手机号",
                              "Your phone number",
                              "رقم هاتفك",
                            )}
                            dir="ltr"
                          />
                        </div>
                      </div>
                      {codeField(registerCode, setRegisterCode)}
                      {passwordField(
                        "auth-new-password",
                        c("设置密码", "Create password", "إنشاء كلمة مرور"),
                        newPassword,
                        setNewPassword,
                        true,
                      )}
                      {passwordField(
                        "auth-confirm-password",
                        c("确认密码", "Confirm password", "تأكيد كلمة المرور"),
                        confirmPassword,
                        setConfirmPassword,
                        true,
                      )}
                      <label className="auth-human">
                        <input
                          type="checkbox"
                          checked={human}
                          onChange={(e) => {
                            setHuman(e.target.checked);
                            setError("");
                          }}
                        />
                        <span>
                          {c(
                            "完成人机验证（模拟）",
                            "Complete verification (simulated)",
                            "إكمال التحقق (محاكاة)",
                          )}
                        </span>
                        <Check
                          size={16}
                          aria-hidden="true"
                          style={{ opacity: human ? 1 : 0.2 }}
                        />
                      </label>
                    </>
                  )}
                  {mode === "reset" && (
                    <>
                      <div className="auth-field">
                        <label htmlFor="auth-reset-phone">
                          {c("注册手机号", "Registered phone", "الهاتف المسجل")}
                        </label>
                        <div className="auth-phone-input">
                          <span dir="ltr">+86</span>
                          <Input
                            id="auth-reset-phone"
                            type="tel"
                            inputMode="tel"
                            value={resetPhone}
                            onChange={(e) => {
                              setResetPhone(e.target.value);
                              setResetCode("");
                              setError("");
                            }}
                            required
                            maxLength={11}
                            autoComplete="tel-national"
                            placeholder={c(
                              "请输入注册手机号",
                              "Your registered phone",
                              "رقم هاتفك المسجل",
                            )}
                            dir="ltr"
                          />
                        </div>
                      </div>
                      {codeField(resetCode, setResetCode)}
                      {passwordField(
                        "auth-reset-password",
                        c("新密码", "New password", "كلمة المرور الجديدة"),
                        resetPassword,
                        setResetPassword,
                        true,
                      )}
                    </>
                  )}
                  {mode !== "reset" && consent}
                  {mode === "register" && (
                    <p className="auth-registration-note">
                      {c(
                        "注册将替换当前演示业务数据，进入零余额的新账户；不会赠送额度。",
                        "Registration replaces this console’s demo data with a new, zero-balance account. No free credits.",
                        "يستبدل التسجيل بيانات لوحة التحكم التجريبية بحساب جديد برصيد صفري، دون ائتمان مجاني.",
                      )}
                    </p>
                  )}
                  <Button
                    variant="primary"
                    type="submit"
                    className="auth-submit"
                    loading={busy}
                    disabled={
                      mode === "login" &&
                      method === "password" &&
                      lockSeconds > 0
                    }
                  >
                    {mode === "register"
                      ? c(
                          "创建演示账户",
                          "Create demo account",
                          "إنشاء حساب تجريبي",
                        )
                      : mode === "reset"
                        ? c(
                            "更新演示密码",
                            "Update demo password",
                            "تحديث كلمة المرور التجريبية",
                          )
                        : c("登录", "Log in", "تسجيل الدخول")}
                    <ArrowRight size={16} className="directional" />
                  </Button>
                </form>
                <div className="auth-switch">
                  {mode === "login" ? (
                    <>
                      {c("还没有账户？", "New to Veyra?", "ليس لديك حساب؟")}
                      <button
                        type="button"
                        onClick={() => changeMode("register")}
                      >
                        {c("立即注册", "Sign up", "إنشاء حساب")}
                      </button>
                    </>
                  ) : (
                    <>
                      {mode === "register" &&
                        c(
                          "已有账户？",
                          "Already have an account?",
                          "لديك حساب؟",
                        )}
                      <button type="button" onClick={() => changeMode("login")}>
                        {c("返回登录", "Back to login", "العودة للدخول")}
                      </button>
                    </>
                  )}
                </div>
              </>
            )}
          </div>
          <div className="auth-dialog-footer">
            <Info size={14} />
            <span>
              {c(
                "本地演示，不发送短信或邮件。请使用虚构资料。",
                "Local demo. No SMS or email is sent. Use fictional details.",
                "عرض محلي بلا رسائل فعلية. استخدم بيانات خيالية.",
              )}
            </span>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

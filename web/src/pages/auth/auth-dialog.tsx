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
          "账号或密码不正确，请检查后重试。",
          "The account or password is incorrect. Please try again.",
          "الحساب أو كلمة المرور غير صحيحة. حاول مجددًا.",
        );
      if (caught.status === 403)
        return c(
          "该账户已被禁用，请联系管理员。",
          "This account is disabled. Contact the administrator.",
          "هذا الحساب معطل. اتصل بالمسؤول.",
        );
      if (caught.status === 409)
        return c(
          "该用户名已被使用，请换一个。",
          "This username is already taken. Choose another.",
          "اسم المستخدم مستخدم بالفعل. اختر اسمًا آخر.",
        );
      return caught.message;
    }
    return c(
      "暂时无法完成，请重试。输入已保留。",
      "Unable to continue. Your inputs are preserved; please retry.",
      "تعذر المتابعة. تم الاحتفاظ بالمدخلات؛ حاول مجددًا.",
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
          "请阅读并勾选服务协议与隐私政策后继续。",
          "Read and accept the terms and privacy notice to continue.",
          "اقرأ اتفاقية الخدمة وسياسة الخصوصية ووافق عليهما للمتابعة.",
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
      if (ticket === operation.current) setError(explain(caught));
    } finally {
      if (ticket === operation.current) {
        setBusy(false);
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
            "密码找回暂不可用，请联系管理员。",
            "Password recovery is unavailable. Contact the administrator.",
            "استعادة كلمة المرور غير متاحة. تواصل مع المسؤول.",
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
            minLength={isNew ? 6 : undefined}
            maxLength={64}
            placeholder={
              id === "auth-confirm-password"
                ? c(
                    "请再次输入密码",
                    "Enter your password again",
                    "أدخل كلمة المرور مجددًا",
                  )
                : isNew
                  ? c("至少 6 位", "At least 6 characters", "6 أحرف على الأقل")
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
              <span>MyAPIs</span>
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
                      {c("1. 当前范围", "1. Current scope", "1. النطاق الحالي")}
                    </h3>
                    <p>
                      {c(
                        "用户名注册与密码登录使用服务器接口。模型调用取决于部署中的上游渠道、权限和配额；当前充值收银台仍为模拟流程，短信及验证邮件尚未接入。",
                        "Username registration and password login use server APIs. Model calls depend on configured upstream channels, permissions and quota. The current recharge checkout is simulated; SMS and verification email are unavailable.",
                        "يستخدم التسجيل والدخول واجهات الخادم. تعتمد طلبات النماذج على القنوات والصلاحيات والحصة. الدفع الحالي محاكاة؛ الرسائل النصية ورسائل التحقق غير متاحة.",
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
                        "注册成功后仍需登录。当前新账号 API 配额默认是零；支付钱包与 API 配额分别记账。创建充值订单不代表付款确认，模拟充值不能兑换真实 API 配额。",
                        "Registration must be followed by login. New accounts currently default to zero API quota; payment wallet and API quota are separate. Creating an order is not payment confirmation, and simulated recharge cannot be exchanged for live API quota.",
                        "يلزم تسجيل الدخول بعد إنشاء الحساب. تبدأ الحصة الافتراضية للحسابات الجديدة بصفر؛ المحفظة والحصة منفصلتان. إنشاء طلب ليس تأكيد دفع ولا يتحول الشحن التجريبي إلى حصة فعلية.",
                      )}
                    </p>
                    <h3>
                      {c("3. 数据范围", "3. Data scope", "3. نطاق البيانات")}
                    </h3>
                    <p>
                      {c(
                        "服务器账户数据与浏览器中的演示场景数据分开。注册不会替换现有账本、密钥或演示场景；登录后的实际权限由服务器校验。此说明仍为界面草案。",
                        "Server account data is separate from browser Demo scenarios. Registration does not replace existing ledgers, keys or scenarios. The server checks actual permissions after login. This notice remains a UI draft.",
                        "بيانات حساب الخادم منفصلة عن سيناريوهات المتصفح. التسجيل لا يستبدل السجلات أو المفاتيح أو السيناريوهات. يتحقق الخادم من الصلاحيات الفعلية. يظل هذا الإشعار مسودة واجهة.",
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
                        "服务器保存用户名与口令哈希；登录使用 HttpOnly Cookie 和服务器会话，不将会话密钥写入浏览器本地存储。浏览器仍保存账号显示资料、偏好和其余演示数据。当前注册表单不收集手机号。",
                        "The server stores usernames and password hashes. Login uses an HttpOnly cookie and a server session; session keys are not saved to browser local storage. Display profiles, preferences and remaining Demo data are stored in the browser. This registration form collects no phone number.",
                        "يحفظ الخادم أسماء المستخدمين وتجزئات كلمات المرور. يستخدم الدخول ملف ارتباط HttpOnly وجلسة خادم، دون حفظ مفتاح الجلسة في التخزين المحلي. تُحفظ ملفات العرض والتفضيلات وبيانات العرض المتبقية في المتصفح. لا يجمع النموذج رقم هاتف.",
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
                        "短信和验证邮件尚未接入，当前不提供手机号认证或验证码登录。阅读与勾选状态仍只在界面处理，服务器没有保存版本化的同意记录。",
                        "SMS and verification email are unavailable; phone verification and code login are not supported. Reading and consent state are handled in the UI; the server does not retain a versioned consent record.",
                        "الرسائل النصية ورسائل التحقق غير متاحة؛ لا يُدعم توثيق الهاتف أو الدخول برمز. تُعالج الموافقة في الواجهة ولا يحفظ الخادم سجل موافقة بإصدار محدد.",
                      )}
                    </p>
                    <h3>
                      {c("3. 清理方式", "3. Clearing data", "3. مسح البيانات")}
                    </h3>
                    <p>
                      {c(
                        "请使用退出登录来结束当前服务器会话；仅关闭标签页不会注销。清除浏览器网站数据不会删除服务器账户或账本。正式隐私政策仍需由部署方另行提供。",
                        "Use Sign out to end the current server session; closing a tab does not log out. Clearing browser data does not delete server accounts or ledgers. The deployment operator must provide the final privacy policy separately.",
                        "استخدم تسجيل الخروج لإنهاء جلسة الخادم؛ إغلاق التبويب لا ينهيها. مسح بيانات المتصفح لا يحذف حسابات الخادم أو السجلات. يقدّم مشغّل النشر سياسة الخصوصية النهائية بشكل منفصل.",
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
            ) : (
              <>
                {mode === "login" && registered && (
                  <p className="auth-field-hint" role="status">
                    {c(
                      "账户已创建。请使用用户名和密码登录；尚未建立新会话。",
                      "Account created. Sign in with your username and password; no new session has been established.",
                      "أُنشئ الحساب. سجّل الدخول باسم المستخدم وكلمة المرور؛ لم تُنشأ جلسة جديدة بعد.",
                    )}
                  </p>
                )}
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
                {mode === "reset" ||
                (mode === "login" && method === "phone") ? (
                  <div className="auth-unavailable" role="status">
                    <span>
                      <Info size={24} />
                    </span>
                    <p>
                      {mode === "reset"
                        ? c(
                            "密码找回尚未接入，请联系管理员协助恢复。当前页面不会收集手机号、发送验证码或修改密码。",
                            "Password recovery is unavailable. Contact the administrator for help. This page does not collect phone numbers, send codes or change passwords.",
                            "استعادة كلمة المرور غير متاحة. تواصل مع المسؤول للمساعدة. لا تجمع هذه الصفحة أرقام هواتف ولا ترسل رموزًا ولا تغيّر كلمات المرور.",
                          )
                        : c(
                            "手机号验证码登录尚未接入。请使用用户名和密码登录；当前页面不会发送或校验验证码。",
                            "Phone-code login is unavailable. Use your username and password; this page does not send or verify codes.",
                            "الدخول برمز الهاتف غير متاح. استخدم اسم المستخدم وكلمة المرور؛ لا ترسل هذه الصفحة رموزًا ولا تتحقق منها.",
                          )}
                    </p>
                    <Button
                      variant="primary"
                      onClick={() => changeMode("login")}
                    >
                      {c(
                        "使用账号密码登录",
                        "Use password login",
                        "الدخول بكلمة المرور",
                      )}
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
                              "用户名",
                              "Username",
                              "اسم المستخدم",
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
                              setAccount("root");
                              setPassword("123456");
                              setError("");
                            }}
                          >
                            {c(
                              "填入管理员账号",
                              "Use admin account",
                              "استخدم حساب المسؤول",
                            )}
                          </button>
                          <button
                            type="button"
                            onClick={() => {
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
                    {mode === "register" && (
                      <>
                        <div className="auth-field">
                          <label htmlFor="auth-username">
                            {c("用户名", "Username", "اسم المستخدم")}
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
                              "请输入用户名",
                              "Choose a username",
                              "اختر اسم مستخدم",
                            )}
                            aria-describedby="auth-username-hint"
                          />
                          <p
                            id="auth-username-hint"
                            className="auth-field-hint"
                          >
                            {c(
                              "3–64 位英文字母、数字、下划线、短横线或点号",
                              "3–64 ASCII letters, digits, underscores, hyphens or dots",
                              "3–64 حرفًا لاتينيًا أو رقمًا أو شرطة سفلية أو شرطة أو نقطة",
                            )}
                          </p>
                        </div>
                        {passwordField(
                          "auth-new-password",
                          c("设置密码", "Create password", "إنشاء كلمة مرور"),
                          newPassword,
                          setNewPassword,
                          true,
                        )}
                        {passwordField(
                          "auth-confirm-password",
                          c(
                            "确认密码",
                            "Confirm password",
                            "تأكيد كلمة المرور",
                          ),
                          confirmPassword,
                          setConfirmPassword,
                          true,
                        )}
                        <p className="auth-field-hint">
                          {c(
                            "当前仅支持用户名与密码注册，不收集或验证手机号。短信验证尚未接入。",
                            "Registration currently uses a username and password. Phone numbers are neither collected nor verified; SMS verification is unavailable.",
                            "التسجيل حاليًا باسم مستخدم وكلمة مرور. لا تُجمع أرقام الهواتف ولا تُتحقق؛ التحقق بالرسائل غير متاح.",
                          )}
                        </p>
                      </>
                    )}
                    {consent}
                    {mode === "register" && (
                      <p className="auth-registration-note">
                        {c(
                          "创建账号不会清空当前演示数据；注册成功后需要登录。新账号 API 配额默认是零。",
                          "Creating an account does not clear current Demo data. Sign in after registration; new accounts default to zero API quota.",
                          "إنشاء الحساب لا يمسح بيانات العرض الحالية. سجّل الدخول بعد التسجيل؛ تبدأ حصة الحساب الجديد بصفر.",
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
                        ? c("创建账户", "Create account", "إنشاء حساب")
                        : c("登录", "Log in", "تسجيل الدخول")}
                      <ArrowRight size={16} className="directional" />
                    </Button>
                  </form>
                )}
                {mode !== "reset" && (
                  <div className="auth-switch">
                    {mode === "login" ? (
                      <>
                        {c("还没有账户？", "New to MyAPIs?", "ليس لديك حساب؟")}
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
                        <button
                          type="button"
                          onClick={() => changeMode("login")}
                        >
                          {c("返回登录", "Back to login", "العودة للدخول")}
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
                "账户由服务器保存，登录会话使用 Cookie。当前不发送短信或验证邮件。",
                "Accounts are saved on the server; login sessions use cookies. SMS and verification email are unavailable.",
                "تُحفظ الحسابات على الخادم وتستخدم الجلسات ملفات الارتباط. الرسائل النصية ورسائل التحقق غير متاحة.",
              )}
            </span>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

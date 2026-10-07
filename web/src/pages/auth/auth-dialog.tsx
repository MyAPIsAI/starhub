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
  const heading = useRef<HTMLHeadingElement>(null);
  const errorRef = useRef<HTMLDivElement>(null);
  const bodyRef = useRef<HTMLDivElement>(null);
  const previousView = useRef("");
  const policyTrigger = useRef<Policy | null>(null);
  const formScroll = useRef(0);
  const operation = useRef(0);
  const resendSeconds = 0;
  const lockSeconds = 0;

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
  function sendCode() {
    setError(
      c(
        "验证码功能暂未接入后端，请使用密码登录。",
        "Verification codes are not available yet. Use password login.",
        "رموز التحقق غير متاحة بعد. استخدم تسجيل الدخول بكلمة المرور.",
      ),
    );
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
        setError(
          c(
            "密码重置暂未接入后端，请联系管理员。",
            "Password reset is not available yet. Contact the administrator.",
            "إعادة تعيين كلمة المرور غير متاحة بعد. اتصل بالمسؤول.",
          ),
        );
        return;
      }
      if (mode === "register") {
        const user = await registerUser(username, newPassword, "");
        if (ticket !== operation.current) return;
        useProductPreview
          .getState()
          .register(user.email ?? "", user.username, authUserProfile(user));
        useSystemAccess.getState().setRole(user.role);
      } else {
        // Login: password method only (phone not supported by backend).
        const { user } = await loginUser(account, password);
        if (ticket !== operation.current) return;
        useProductPreview
          .getState()
          .login(user.email ?? "", user.username, authUserProfile(user));
        useSystemAccess.getState().setRole(user.role);
        completeSessionRestore();
      }
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

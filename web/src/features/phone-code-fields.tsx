import { useEffect, useRef, useState } from "react";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/primitives";
import { ApiError } from "../lib/api/http";
import { usePreferences } from "../lib/preferences";
import { phoneCooldownKey, phoneRetrySeconds } from "../lib/phone-cooldown";
import "./phone-code-fields.css";

export function usePhoneCode(country = "CN") {
  const [phone, setPhone] = useState("");
  const [code, setCode] = useState("");
  const [sending, setSending] = useState(false);
  const [deadlines, setDeadlines] = useState<Record<string, number>>({});
  const [now, setNow] = useState(Date.now());
  const [requested, setRequested] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const revision = useRef(0);
  const active = useRef(false);
  const key = phoneCooldownKey(phone, country);
  const deadline = deadlines[key] ?? 0;
  const remaining = Math.max(0, Math.ceil((deadline - now) / 1000));
  useEffect(() => {
    if (deadline <= Date.now()) return;
    setNow(Date.now());
    const timer = window.setInterval(() => {
      const time = Date.now();
      setNow(time);
      if (time >= deadline) window.clearInterval(timer);
    }, 500);
    return () => window.clearInterval(timer);
  }, [deadline]);
  useEffect(
    () => () => {
      revision.current++;
    },
    [],
  );
  function reset() {
    revision.current++;
    active.current = false;
    setPhone("");
    setCode("");
    setError(null);
    setRequested(false);
    setSending(false);
  }
  async function send(
    request: (phone: string) => Promise<{ retry_after: number }>,
  ) {
    if (active.current || Date.now() < deadline) return;
    const ticket = ++revision.current;
    active.current = true;
    setSending(true);
    setError(null);
    setRequested(false);
    try {
      const result = await request(phone);
      if (ticket !== revision.current) return;
      setNow(Date.now());
      setDeadlines((values) => ({
        ...values,
        [key]: Date.now() + phoneRetrySeconds(result.retry_after) * 1000,
      }));
      setRequested(true);
    } catch (caught) {
      if (ticket !== revision.current) return;
      setError(caught);
      if (caught instanceof ApiError && caught.status === 429) {
        setNow(Date.now());
        setDeadlines((values) => ({
          ...values,
          [key]: Date.now() + phoneRetrySeconds(caught.retryAfter) * 1000,
        }));
      }
    } finally {
      if (ticket === revision.current) {
        active.current = false;
        setSending(false);
      }
    }
  }
  return {
    phone,
    code,
    setCode,
    sending,
    remaining,
    requested,
    error,
    reset,
    send,
    changePhone(value: string) {
      revision.current++;
      active.current = false;
      setSending(false);
      setNow(Date.now());
      setPhone(value);
      setCode("");
      setError(null);
      setRequested(false);
    },
  };
}

export function PhoneCodeFields({
  state,
  onSend,
  disabled = false,
  id = "phone",
  country = "",
  purpose = "verification",
}: {
  state: ReturnType<typeof usePhoneCode>;
  onSend: () => void;
  disabled?: boolean;
  id?: string;
  country?: string;
  purpose?: "login" | "verification";
}) {
  const language = usePreferences((s) => s.language);
  const c = (zh: string, en: string, ar: string) =>
    language === "zh" ? zh : language === "ar" ? ar : en;
  const error = state.error;
  const errorRef = useRef<HTMLParagraphElement>(null);
  let message = c(
    "无法发送验证码，请核对号码、短信配置后重试。",
    "Could not request a code. Check the number and SMS configuration, then retry.",
    "تعذر طلب الرمز. تحقق من الرقم وإعداد الرسائل وحاول مجددًا.",
  );
  if (error instanceof ApiError) {
    if (
      purpose === "login" &&
      error.status === 409 &&
      error.message === "phone_not_linked"
    ) {
      message = c(
        "该手机号尚未关联账号，请先注册，或使用账号密码登录后在账号中心绑定手机号。本次未发送验证码。",
        "This phone number is not linked to an account. Register first, or sign in with your password and link it in Account center. No code was sent.",
        "رقم الهاتف غير مرتبط بحساب. سجّل أولًا أو ادخل بكلمة المرور واربطه في مركز الحساب. لم يُرسل رمز.",
      );
    } else if (error.status === 429) {
      message = c(
        "发送过于频繁，请稍后再试。",
        "Too many attempts. Try again later.",
        "محاولات كثيرة. حاول لاحقًا.",
      );
    } else if (purpose === "login" && error.status === 403) {
      message = c(
        "该手机号暂不能用于短信登录，请使用账号密码登录或联系管理员。",
        "SMS sign-in is unavailable for this phone. Use password login or contact the administrator.",
        "الدخول بالرسائل غير متاح لهذا الرقم. استخدم كلمة المرور أو تواصل مع المسؤول.",
      );
    } else if (error.status === 403) {
      message = c(
        "请核对当前密码及短信功能权限。",
        "Check your current password and SMS availability.",
        "تحقق من كلمة المرور وتوفر الرسائل.",
      );
    } else if (purpose === "login" && error.status === 503) {
      message = c(
        "验证码发送服务暂不可用，请稍后重试或使用账号密码登录。",
        "The SMS service is temporarily unavailable. Retry later or use password login.",
        "خدمة الرسائل غير متاحة مؤقتًا. حاول لاحقًا أو ادخل بكلمة المرور.",
      );
    }
  }
  useEffect(() => {
    if (purpose === "login" && error != null) errorRef.current?.focus();
  }, [error, purpose]);
  return (
    <div className="phone-code-fields">
      {purpose === "login" && (
        <p className="phone-code-guidance" id={`${id}-guidance`}>
          {c(
            "短信登录仅支持已绑定的手机号。新用户请先注册；已有账号请使用密码登录后，在账号中心绑定手机号。",
            "SMS sign-in requires a linked phone number. New users should register first. Existing users can sign in with a password and link a phone in Account center.",
            "يتطلب الدخول بالرسائل رقم هاتف مرتبطًا. على المستخدم الجديد التسجيل أولًا. يمكن للمستخدم الحالي الدخول بكلمة المرور وربط هاتف في مركز الحساب.",
          )}
        </p>
      )}
      <div className="auth-field">
        <label htmlFor={`${id}-number`}>
          {c("手机号", "Phone number", "رقم الهاتف")}
        </label>
        <Input
          id={`${id}-number`}
          type="tel"
          dir="ltr"
          autoComplete="tel"
          aria-describedby={purpose === "login" ? `${id}-guidance` : undefined}
          required
          maxLength={32}
          disabled={disabled || state.sending}
          value={state.phone}
          onChange={(event) => state.changePhone(event.target.value)}
          placeholder={
            country === "CN" || !country
              ? c(
                  "中国大陆手机号（+86）",
                  "Mainland China number (+86)",
                  "رقم صيني (+86)",
                )
              : c(
                  "含国家区号的手机号",
                  "Phone with country calling code",
                  "رقم مع رمز الدولة",
                )
          }
        />
      </div>
      <div className="auth-field">
        <label htmlFor={`${id}-code`}>
          {c("短信验证码", "SMS code", "رمز الرسالة")}
        </label>
        <div className="phone-code-input">
          <Input
            id={`${id}-code`}
            dir="ltr"
            inputMode="numeric"
            autoComplete="one-time-code"
            pattern="[0-9]{6}"
            maxLength={6}
            required
            disabled={disabled || state.sending}
            value={state.code}
            onChange={(event) =>
              state.setCode(event.target.value.replace(/\D/g, ""))
            }
          />
          <Button
            type="button"
            variant="secondary"
            disabled={
              disabled ||
              state.sending ||
              state.remaining > 0 ||
              !state.phone.trim()
            }
            onClick={onSend}
          >
            {state.sending
              ? c("发送中…", "Requesting…", "جارٍ الطلب…")
              : state.remaining > 0
                ? c(
                    `${state.remaining}秒后重发`,
                    `Retry in ${state.remaining}s`,
                    `أعد بعد ${state.remaining}ث`,
                  )
                : c("获取验证码", "Get code", "طلب الرمز")}
          </Button>
        </div>
      </div>
      {state.requested && (
        <p className="phone-code-notice" role="status">
          {purpose === "login"
            ? c(
                "请求已受理，不代表短信已发送。仅向已绑定且账号可用的手机号发送验证码，10分钟内有效。未收到时，请核对绑定情况或使用密码登录。",
                "Request accepted; this does not confirm SMS was sent. Codes are sent only to numbers linked to active accounts and expire in 10 minutes. If none arrives, check the phone link or use password login.",
                "قُبل الطلب، ولا يعني ذلك إرسال رسالة. تُرسل الرموز إلى الأرقام المرتبطة بحسابات نشطة وتصلح لعشر دقائق. إذا لم يصل رمز، تحقق من ربط الهاتف أو ادخل بكلمة المرور.",
              )
            : c(
                "请求已受理；若号码可用于当前操作，将收到验证码，10分钟内有效。",
                "Request accepted. If the number is eligible, a code will arrive and expire in 10 minutes.",
                "قُبل الطلب. سيصل رمز صالح لعشر دقائق إذا كان الرقم مؤهلًا.",
              )}
        </p>
      )}
      {error != null && (
        <p
          className="phone-code-error"
          role="alert"
          ref={errorRef}
          tabIndex={-1}
        >
          {message}
        </p>
      )}
    </div>
  );
}

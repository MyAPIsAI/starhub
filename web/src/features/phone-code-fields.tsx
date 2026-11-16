import { useEffect, useRef, useState } from "react";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/primitives";
import { ApiError } from "../lib/api/http";
import { usePreferences } from "../lib/preferences";
import "./phone-code-fields.css";

export function usePhoneCode() {
  const [phone, setPhone] = useState("");
  const [code, setCode] = useState("");
  const [sending, setSending] = useState(false);
  const [deadline, setDeadline] = useState(0);
  const [now, setNow] = useState(Date.now());
  const [requested, setRequested] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const revision = useRef(0);
  const active = useRef(false);
  const remaining = Math.max(0, Math.ceil((deadline - now) / 1000));
  useEffect(() => {
    if (!deadline) return;
    const timer = window.setInterval(() => setNow(Date.now()), 500);
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
      setDeadline(Date.now() + Math.max(60, result.retry_after || 60) * 1000);
      setRequested(true);
    } catch (caught) {
      if (ticket !== revision.current) return;
      setError(caught);
      if (caught instanceof ApiError && caught.status === 429) {
        setNow(Date.now());
        setDeadline(Date.now() + 60000);
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
}: {
  state: ReturnType<typeof usePhoneCode>;
  onSend: () => void;
  disabled?: boolean;
  id?: string;
  country?: string;
}) {
  const language = usePreferences((s) => s.language);
  const c = (zh: string, en: string, ar: string) =>
    language === "zh" ? zh : language === "ar" ? ar : en;
  const error = state.error;
  const message =
    error instanceof ApiError && error.status === 429
      ? c(
          "发送过于频繁，请稍后再试。",
          "Too many attempts. Try again later.",
          "محاولات كثيرة. حاول لاحقًا.",
        )
      : error instanceof ApiError && error.status === 403
        ? c(
            "请核对当前密码及短信功能权限。",
            "Check your current password and SMS availability.",
            "تحقق من كلمة المرور وتوفر الرسائل.",
          )
        : c(
            "无法发送验证码，请核对号码、短信配置后重试。",
            "Could not request a code. Check the number and SMS configuration, then retry.",
            "تعذر طلب الرمز. تحقق من الرقم وإعداد الرسائل وحاول مجددًا.",
          );
  return (
    <div className="phone-code-fields">
      <div className="auth-field">
        <label htmlFor={`${id}-number`}>
          {c("手机号", "Phone number", "رقم الهاتف")}
        </label>
        <Input
          id={`${id}-number`}
          type="tel"
          dir="ltr"
          autoComplete="tel"
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
          {c(
            "请求已受理；若号码可用于当前操作，将收到验证码，10分钟内有效。",
            "Request accepted. If the number is eligible, a code will arrive and expire in 10 minutes.",
            "قُبل الطلب. سيصل رمز صالح لعشر دقائق إذا كان الرقم مؤهلًا.",
          )}
        </p>
      )}
      {error != null && (
        <p className="phone-code-error" role="alert">
          {message}
        </p>
      )}
    </div>
  );
}

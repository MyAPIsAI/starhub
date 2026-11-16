import { useEffect, useRef, useState, type FormEvent } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Smartphone } from "lucide-react";
import { toast } from "sonner";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/primitives";
import { Modal } from "../components/ui/overlay";
import {
  bindPhone,
  getPhoneBinding,
  requestPhoneBindingCode,
} from "../lib/api/phone";
import { ApiError } from "../lib/api/http";
import { getPublicRegionPolicy } from "../lib/locale-policy";
import { usePreferences } from "../lib/preferences";
import {
  sessionQueryIdentity,
  useSessionQueryIdentity,
  useSystemAccess,
} from "../lib/system-access";
import { PhoneCodeFields, usePhoneCode } from "./phone-code-fields";
import { SMSConfigurationButton } from "./sms-configuration";

export function AccountPhone() {
  const scope = useSessionQueryIdentity();
  return <AccountPhoneState key={JSON.stringify(scope)} scope={scope} />;
}

function AccountPhoneState({
  scope,
}: {
  scope: ReturnType<typeof useSessionQueryIdentity>;
}) {
  const language = usePreferences((s) => s.language);
  const c = (zh: string, en: string, ar: string) =>
    language === "zh" ? zh : language === "ar" ? ar : en;
  const client = useQueryClient();
  const [open, setOpen] = useState(false);
  const owner = scope[1];
  const status = useQuery({
    queryKey: ["phone-binding", ...scope],
    enabled: owner != null,
    queryFn: ({ signal }) => getPhoneBinding(owner!, signal),
    retry: false,
  });
  const capability = useQuery({
    queryKey: ["public-region-phone", ...scope],
    enabled: owner != null,
    queryFn: ({ signal }) => getPublicRegionPolicy(signal),
    retry: false,
  });
  const available = capability.data?.phone_login_enabled === true;
  const bound = status.data?.user_id === owner && status.data.bound;
  const current = () =>
    JSON.stringify(scope) ===
    JSON.stringify(sessionQueryIdentity(useSystemAccess.getState()));
  const reload = () => {
    void status.refetch();
    void capability.refetch();
  };
  return (
    <>
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
            value={
              bound
                ? status.data!.masked_phone
                : status.isPending
                  ? c("读取中…", "Loading…", "جارٍ التحميل…")
                  : status.isError
                    ? c("无法确认", "Unconfirmed", "غير مؤكد")
                    : c("未绑定", "Not linked", "غير مرتبط")
            }
            readOnly
            dir={bound ? "ltr" : undefined}
          />
          <p>
            {bound
              ? available
                ? c(
                    "已验证，可使用短信验证码登录。换绑与短信找回密码暂未开放。",
                    "Verified for SMS sign-in. Phone replacement and SMS password recovery are unavailable.",
                    "موثق للدخول بالرسائل. تغيير الرقم واستعادة كلمة المرور غير متاحين.",
                  )
                : c(
                    "手机号已绑定；短信登录当前不可用，可刷新状态或使用密码登录。",
                    "Phone linked. SMS sign-in is currently unavailable; refresh or use password login.",
                    "الهاتف مرتبط. الدخول بالرسائل غير متاح حاليًا؛ حدّث أو استخدم كلمة المرور.",
                  )
              : available
                ? c(
                    "验证并绑定手机号后，可使用短信验证码登录。",
                    "Verify and link your phone to sign in with an SMS code.",
                    "وثّق واربط هاتفك للدخول برمز رسالة.",
                  )
                : capability.isError
                  ? c(
                      "无法读取短信状态，请刷新重试。",
                      "SMS availability could not be loaded. Refresh to retry.",
                      "تعذر تحميل حالة الرسائل. حدّث للمحاولة.",
                    )
                  : c(
                      "管理员启用短信服务后，可验证并绑定手机号。",
                      "Phone linking is available after an administrator enables SMS.",
                      "يتاح ربط الهاتف بعد تفعيل الرسائل بواسطة المسؤول.",
                    )}
          </p>
        </div>
        <div className="ac-phone-actions">
          <SMSConfigurationButton />
          {!bound && (
            <Button
              data-account-action="phone"
              size="small"
              disabled={
                !available ||
                !status.data ||
                status.isError ||
                status.isFetching
              }
              onClick={() => setOpen(true)}
            >
              {c("绑定", "Link", "ربط")}
            </Button>
          )}
          <Button
            size="small"
            variant="ghost"
            disabled={status.isFetching || capability.isFetching}
            onClick={reload}
          >
            {c("刷新", "Refresh", "تحديث")}
          </Button>
        </div>
      </div>
      {open && owner != null && (
        <PhoneBindingDialog
          owner={owner}
          country={capability.data?.country_code ?? ""}
          current={current}
          onClose={() => setOpen(false)}
          onBound={() => {
            if (!current()) return;
            setOpen(false);
            void client.invalidateQueries({
              queryKey: ["phone-binding", ...scope],
            });
            void client.invalidateQueries({
              queryKey: ["user-self", "security"],
            });
          }}
        />
      )}
    </>
  );
}

function PhoneBindingDialog({
  owner,
  country,
  current,
  onClose,
  onBound,
}: {
  owner: number;
  country: string;
  current: () => boolean;
  onClose: () => void;
  onBound: () => void;
}) {
  const language = usePreferences((s) => s.language);
  const c = (zh: string, en: string, ar: string) =>
    language === "zh" ? zh : language === "ar" ? ar : en;
  const phone = usePhoneCode();
  const [password, setPassword] = useState("");
  const [pending, setPending] = useState(false);
  const [uncertain, setUncertain] = useState(false);
  const [error, setError] = useState("");
  const active = useRef(false);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  const first = useRef<HTMLInputElement>(null);
  const input = () => ({
    expected_user_id: owner,
    current_password: password,
    phone: phone.phone,
    code: phone.code,
  });
  async function submit(event: FormEvent) {
    event.preventDefault();
    if (active.current || phone.sending || uncertain || !current()) return;
    active.current = true;
    setPending(true);
    setError("");
    try {
      const result = await bindPhone(input());
      if (!mounted.current || !current()) return;
      if (!result.bound || result.user_id !== owner)
        throw new Error("unconfirmed binding");
      toast.success(
        c("手机号绑定成功", "Phone linked successfully", "تم ربط الهاتف بنجاح"),
      );
      onBound();
    } catch (caught) {
      if (!mounted.current || !current()) return;
      if (caught instanceof ApiError && caught.status === 400) {
        setError(
          c(
            "验证码错误、已过期或用途不符，请核对后重试。",
            "The code is invalid or expired. Check it and retry.",
            "الرمز غير صالح أو منتهي. تحقق وحاول مجددًا.",
          ),
        );
      } else if (caught instanceof ApiError && caught.status === 403) {
        setError(
          c(
            "请核对当前密码及短信功能权限。",
            "Check your current password and SMS availability.",
            "تحقق من كلمة المرور وتوفر الرسائل.",
          ),
        );
      } else if (caught instanceof ApiError && caught.status === 429) {
        setError(
          c(
            "尝试过于频繁，请稍后再试。",
            "Too many attempts. Try again later.",
            "محاولات كثيرة. حاول لاحقًا.",
          ),
        );
      } else {
        setUncertain(true);
        setError(
          c(
            "号码已占用、账号发生变化或结果未确认，请关闭后刷新状态。",
            "The phone is occupied, the account changed, or the result is unconfirmed. Close and refresh the status.",
            "الرقم مستخدم أو تغير الحساب أو لم تتأكد النتيجة. أغلق وحدّث الحالة.",
          ),
        );
      }
    } finally {
      active.current = false;
      if (mounted.current && current()) {
        setPending(false);
        setPassword("");
      }
    }
  }
  return (
    <Modal
      open
      onOpenChange={(value) => {
        if (!value && !active.current && !phone.sending) {
          mounted.current = false;
          onClose();
        }
      }}
      className="phone-binding-dialog"
      initialFocus={first}
      title={c("绑定手机号", "Link phone", "ربط الهاتف")}
      description={c(
        "验证当前密码和手机号。绑定成功后可使用短信验证码登录。",
        "Verify your current password and phone to enable SMS sign-in.",
        "تحقق من كلمة المرور والهاتف لتفعيل الدخول بالرسائل.",
      )}
      footer={
        <Button
          type="submit"
          form="phone-binding-form"
          variant="primary"
          disabled={pending || phone.sending || uncertain}
          loading={pending}
        >
          {c("确认绑定", "Confirm link", "تأكيد الربط")}
        </Button>
      }
    >
      <form
        id="phone-binding-form"
        className="phone-binding-form"
        onSubmit={submit}
      >
        {error && (
          <p className="phone-code-error" role="alert">
            {error}
          </p>
        )}
        <label htmlFor="phone-binding-password">
          {c("当前密码", "Current password", "كلمة المرور الحالية")}
          <Input
            ref={first}
            id="phone-binding-password"
            type="password"
            autoComplete="current-password"
            maxLength={72}
            required
            disabled={pending || phone.sending || uncertain}
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
        </label>
        <PhoneCodeFields
          id="bind-phone"
          country={country}
          state={phone}
          disabled={pending || uncertain}
          onSend={() => {
            if (!password || !current()) {
              first.current?.reportValidity();
              return;
            }
            setError("");
            void phone.send((number) =>
              requestPhoneBindingCode({
                expected_user_id: owner,
                current_password: password,
                phone: number,
              }),
            );
          }}
        />
      </form>
    </Modal>
  );
}

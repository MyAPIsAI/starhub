import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { Settings2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "../components/ui/button";
import { Input, Switch } from "../components/ui/primitives";
import { Modal } from "../components/ui/overlay";
import { ApiError } from "../lib/api/http";
import {
  getSMSConfiguration,
  saveSMSConfiguration,
  type SMSConfiguration,
} from "../lib/api/sms-configuration";
import {
  isAdmin,
  sessionQueryIdentity,
  useSessionQueryIdentity,
  useSystemAccess,
} from "../lib/system-access";
import { usePreferences } from "../lib/preferences";
import "./sms-configuration.css";

export function SMSConfigurationButton() {
  const scope = useSessionQueryIdentity();
  const [open, setOpen] = useState(false);
  const language = usePreferences((state) => state.language);
  if (!isAdmin(scope[3]) || scope[1] === null) return null;
  const label =
    language === "zh"
      ? "短信配置"
      : language === "ar"
        ? "إعداد الرسائل"
        : "SMS configuration";
  return (
    <>
      <Button
        size="small"
        variant="secondary"
        data-account-action="sms-config"
        onClick={() => setOpen(true)}
      >
        <Settings2 size={14} />
        {label}
      </Button>
      {open && (
        <SMSConfigurationDialog
          key={JSON.stringify(scope)}
          scope={scope}
          onClose={() => setOpen(false)}
        />
      )}
    </>
  );
}

const emptyForm = () => ({
  enabled: false,
  access_key_id: "",
  access_secret: "",
  clear_secret: false,
  sign_name: "",
  template_code: "",
  password: "",
});

function SMSConfigurationDialog({
  scope,
  onClose,
}: {
  scope: ReturnType<typeof sessionQueryIdentity>;
  onClose: () => void;
}) {
  const language = usePreferences((state) => state.language);
  const c = (zh: string, en: string, ar: string) =>
    language === "zh" ? zh : language === "ar" ? ar : en;
  const queryClient = useQueryClient();
  const [data, setData] = useState<SMSConfiguration>();
  const [form, setForm] = useState(emptyForm);
  const [attempt, setAttempt] = useState(0);
  const [loading, setLoading] = useState(true);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [uncertain, setUncertain] = useState(false);
  const active = useRef(false);
  const first = useRef<HTMLInputElement>(null);
  const isCurrent = () =>
    JSON.stringify(scope) ===
    JSON.stringify(sessionQueryIdentity(useSystemAccess.getState()));
  const readOnly = data?.source === "environment_bridge";

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setData(undefined);
    setForm(emptyForm());
    setError("");
    setUncertain(false);
    void getSMSConfiguration(scope[1]!, controller.signal)
      .then((value) => {
        if (controller.signal.aborted || !isCurrent()) return;
        setData(value);
        setForm({
          ...emptyForm(),
          enabled: value.enabled,
          access_key_id: value.access_key_id,
          sign_name: value.sign_name,
          template_code: value.template_code,
        });
      })
      .catch(() => {
        if (!controller.signal.aborted && isCurrent())
          setError(
            c(
              "无法读取短信配置，请重试。",
              "SMS configuration could not be loaded. Retry.",
              "تعذر تحميل إعداد الرسائل. حاول مجددًا.",
            ),
          );
      })
      .finally(() => {
        if (!controller.signal.aborted && isCurrent()) setLoading(false);
      });
    return () => controller.abort();
  }, [attempt]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!data || active.current || readOnly || uncertain || !isCurrent())
      return;
    active.current = true;
    setPending(true);
    setError("");
    setNotice("");
    toast.dismiss("sms-configuration-saved");
    try {
      await saveSMSConfiguration({
        expected_user_id: scope[1]!,
        expected_version: data.version,
        current_password: form.password,
        enabled: form.enabled,
        access_key_id: form.access_key_id.trim(),
        access_secret: form.access_secret,
        clear_secret: form.clear_secret,
        sign_name: form.sign_name.trim(),
        template_code: form.template_code.trim(),
      });
      if (!isCurrent()) return;
      void queryClient.invalidateQueries({ queryKey: ["public-region-phone"] });
      const message = c(
        "短信配置已保存并应用；本次未发送短信。",
        "SMS configuration saved and applied. No SMS was sent.",
        "حُفظ إعداد الرسائل وطُبق. لم تُرسل رسالة.",
      );
      setNotice(message);
      toast.success(c("保存成功", "Saved successfully", "تم الحفظ بنجاح"), {
        id: "sms-configuration-saved",
        description: message,
        duration: 5000,
      });
      setAttempt((value) => value + 1);
    } catch (caught) {
      if (!isCurrent()) return;
      if (caught instanceof ApiError && caught.status === 403) {
        setError(
          c(
            "请核对当前密码及管理员权限。",
            "Check your current password and administrator access.",
            "تحقق من كلمة المرور الحالية وصلاحية المسؤول.",
          ),
        );
      } else if (caught instanceof ApiError && caught.status === 400) {
        setError(
          c(
            "请检查访问凭证、短信签名和模板代码；启用前需要填写完整。",
            "Check the credentials, sign name and template code. All are required before enabling.",
            "تحقق من بيانات الاعتماد والتوقيع ورمز القالب. يلزم استكمالها قبل التفعيل.",
          ),
        );
      } else if (caught instanceof ApiError && caught.status === 429) {
        setError(
          c(
            "尝试过于频繁，请稍后重试。",
            "Too many attempts. Try again later.",
            "محاولات كثيرة. حاول لاحقًا.",
          ),
        );
      } else {
        setUncertain(true);
        setError(
          c(
            "配置已变化或保存结果未确认，请重新读取后核对。",
            "Configuration changed or the save is unconfirmed. Reload and check before retrying.",
            "تغير الإعداد أو لم يتأكد الحفظ. أعد التحميل والتحقق قبل المحاولة.",
          ),
        );
      }
    } finally {
      active.current = false;
      if (isCurrent()) {
        setPending(false);
        setForm((value) => ({ ...value, password: "", access_secret: "" }));
      }
    }
  }

  return (
    <Modal
      open
      onOpenChange={(value) => {
        if (!value && !active.current) onClose();
      }}
      title={c(
        "阿里云短信配置",
        "Alibaba Cloud SMS configuration",
        "إعداد رسائل Alibaba Cloud",
      )}
      description={c(
        "管理员配置全站短信发送。此处不会绑定个人手机号。",
        "Administrators configure SMS for the service. This does not link your personal phone.",
        "يُعد المسؤول الرسائل للخدمة. لا يؤدي ذلك لربط هاتفك الشخصي.",
      )}
      initialFocus={first}
      preventOutsideClose
      size="wide"
      className="sms-config-dialog"
      footer={
        <>
          <Button onClick={onClose} disabled={pending}>
            {c("关闭", "Close", "إغلاق")}
          </Button>
          <Button
            onClick={() => {
              setNotice("");
              setAttempt((value) => value + 1);
            }}
            disabled={pending || loading}
          >
            {c("重新读取", "Reload", "إعادة التحميل")}
          </Button>
          {!readOnly && (
            <Button
              variant="primary"
              type="submit"
              form="sms-config-form"
              loading={pending}
              disabled={loading || !data || uncertain}
            >
              {c("保存配置", "Save configuration", "حفظ الإعداد")}
            </Button>
          )}
        </>
      }
    >
      {loading && (
        <p role="status">
          {c(
            "正在读取短信配置…",
            "Loading SMS configuration…",
            "جارٍ تحميل إعداد الرسائل…",
          )}
        </p>
      )}
      {error && (
        <p role="alert" className="ac-form-error">
          {error}
        </p>
      )}
      {notice && (
        <p role="status" className="ac-notice">
          {notice}
        </p>
      )}
      {readOnly && (
        <p role="status" className="ac-notice">
          {c(
            "短信由部署配置管理，请联系运维修改。",
            "SMS is managed by deployment configuration. Contact your operator to make changes.",
            "تُدار الرسائل من إعداد النشر. تواصل مع المشغل لتعديلها.",
          )}
        </p>
      )}
      {data && (
        <form
          id="sms-config-form"
          className="sms-config-form"
          onSubmit={submit}
        >
          <fieldset disabled={pending || readOnly || uncertain}>
            <div className="sms-config-toggle">
              <label htmlFor="sms-enabled">
                {c("启用短信服务", "Enable SMS service", "تفعيل خدمة الرسائل")}
              </label>
              <Switch
                id="sms-enabled"
                label={c(
                  "启用短信服务",
                  "Enable SMS service",
                  "تفعيل خدمة الرسائل",
                )}
                checked={readOnly ? data.effective_enabled : form.enabled}
                onChange={(enabled) => {
                  if (!readOnly && !pending && !uncertain)
                    setForm((value) => ({ ...value, enabled }));
                }}
              />
            </div>
            <div className="sms-config-grid">
              <div className="ac-field">
                <label htmlFor="sms-access-id">AccessKey ID</label>
                <Input
                  ref={first}
                  id="sms-access-id"
                  autoComplete="off"
                  maxLength={128}
                  value={form.access_key_id}
                  onChange={(event) =>
                    setForm({ ...form, access_key_id: event.target.value })
                  }
                  dir="ltr"
                />
              </div>
              <div className="ac-field">
                <label htmlFor="sms-access-secret">AccessKey Secret</label>
                <Input
                  id="sms-access-secret"
                  type="password"
                  autoComplete="new-password"
                  maxLength={256}
                  value={form.access_secret}
                  disabled={form.clear_secret}
                  onChange={(event) =>
                    setForm({ ...form, access_secret: event.target.value })
                  }
                  dir="ltr"
                  aria-describedby="sms-secret-help"
                />
                <small id="sms-secret-help">
                  {data.secret_configured
                    ? c(
                        "已设置；留空保留当前秘密。",
                        "Configured. Leave blank to keep the current secret.",
                        "مُعد. اتركه فارغًا للاحتفاظ بالسر الحالي.",
                      )
                    : c(
                        "尚未设置访问秘密。",
                        "No access secret configured.",
                        "لم يُعد سر الوصول.",
                      )}
                </small>
              </div>
              <div className="ac-field">
                <label htmlFor="sms-sign-name">
                  {c("短信签名", "SMS sign name", "توقيع الرسائل")}
                </label>
                <Input
                  id="sms-sign-name"
                  maxLength={256}
                  value={form.sign_name}
                  onChange={(event) =>
                    setForm({ ...form, sign_name: event.target.value })
                  }
                />
              </div>
              <div className="ac-field">
                <label htmlFor="sms-template-code">
                  {c("短信模板代码", "SMS template code", "رمز قالب الرسائل")}
                </label>
                <Input
                  id="sms-template-code"
                  placeholder="SMS_123456789"
                  maxLength={104}
                  value={form.template_code}
                  onChange={(event) =>
                    setForm({ ...form, template_code: event.target.value })
                  }
                  dir="ltr"
                />
              </div>
            </div>
            <p className="sms-config-hint">
              {c(
                "使用已审核通过的中国大陆短信签名与验证码模板，模板变量为 code。保存配置不会发送短信。",
                "Use an approved mainland China SMS sign and verification template with a code variable. Saving sends no SMS.",
                "استخدم توقيعًا معتمدًا وقالب تحقق للصين القارية بمتغير code. الحفظ لا يرسل رسالة.",
              )}
            </p>
            {data.secret_configured && (
              <label className="sms-config-clear">
                <input
                  type="checkbox"
                  checked={form.clear_secret}
                  onChange={(event) =>
                    setForm({
                      ...form,
                      clear_secret: event.target.checked,
                      access_secret: "",
                    })
                  }
                />
                {c(
                  "清除已保存秘密（需先关闭短信服务）",
                  "Clear the saved secret (disable SMS first)",
                  "حذف السر المحفوظ (عطل الرسائل أولًا)",
                )}
              </label>
            )}
            {!readOnly && (
              <div className="ac-field">
                <label htmlFor="sms-current-password">
                  {c(
                    "当前登录密码",
                    "Current sign-in password",
                    "كلمة مرور الدخول الحالية",
                  )}
                </label>
                <Input
                  id="sms-current-password"
                  type="password"
                  autoComplete="current-password"
                  required
                  maxLength={72}
                  value={form.password}
                  onChange={(event) =>
                    setForm({ ...form, password: event.target.value })
                  }
                />
              </div>
            )}
          </fieldset>
        </form>
      )}
    </Modal>
  );
}

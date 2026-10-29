import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import { useSetFinanceRole } from "../lib/api/finance";
import { ApiError } from "../lib/api/http";
import { type AdminUser } from "../lib/api/admin";
import {
  ROLE_FINANCE,
  ROLE_ROOT,
  ROLE_USER,
  useSessionQueryIdentity,
} from "../lib/system-access";

export function FinanceRoleControl({
  user,
  onUpdated,
}: {
  user: AdminUser;
  onUpdated: (role: number) => void;
}) {
  const scope = useSessionQueryIdentity();
  return (
    <FinanceRoleControlView
      key={JSON.stringify([...scope, user.id, user.role])}
      user={user}
      onUpdated={onUpdated}
    />
  );
}

function FinanceRoleControlView({
  user,
  onUpdated,
}: {
  user: AdminUser;
  onUpdated: (role: number) => void;
}) {
  const [open, setOpen] = useState(false);
  const { i18n } = useTranslation();
  const text = (zh: string, en: string, ar: string) =>
    i18n.language === "zh" ? zh : i18n.language === "ar" ? ar : en;
  const scope = useSessionQueryIdentity();
  const mutation = useSetFinanceRole();
  if (
    scope[1] === null ||
    scope[3] !== ROLE_ROOT ||
    (user.role !== ROLE_USER && user.role !== ROLE_FINANCE)
  )
    return null;
  const enabled = user.role !== ROLE_FINANCE;
  const label = enabled
    ? text(
        "授予财务只读权限",
        "Grant read-only finance access",
        "منح صلاحية مالية للقراءة فقط",
      )
    : text("收回财务权限", "Revoke finance access", "سحب الصلاحية المالية");
  return (
    <div>
      <Button
        variant="secondary"
        onClick={() => {
          mutation.reset();
          setOpen(true);
        }}
      >
        {label}
      </Button>
      <Modal
        open={open}
        onOpenChange={(value) => {
          if (!mutation.isPending) setOpen(value);
        }}
        title={label}
        description={`${user.username} · #${user.id}`}
        footer={
          <>
            <Button
              variant="secondary"
              disabled={mutation.isPending}
              onClick={() => setOpen(false)}
            >
              {text("取消", "Cancel", "إلغاء")}
            </Button>
            <Button
              disabled={mutation.isPending}
              onClick={() =>
                mutation.mutate(
                  { id: user.id, enabled, expected_role: user.role },
                  {
                    onSuccess: () => {
                      onUpdated(enabled ? ROLE_FINANCE : ROLE_USER);
                      setOpen(false);
                    },
                  },
                )
              }
            >
              {mutation.isPending
                ? text("正在更新…", "Updating…", "جارٍ التحديث…")
                : text("确认变更", "Confirm change", "تأكيد التغيير")}
            </Button>
          </>
        }
      >
        <p>
          {enabled
            ? text(
                "该账户将可读取全平台财务汇总，包括钱包、充值、流水与 API 配额消费。",
                "This account will be able to read platform-wide wallet, top-up, ledger and API quota summaries.",
                "سيتمكن هذا الحساب من قراءة ملخصات المحافظ والشحن والسجل وحصة الواجهة على مستوى المنصة.",
              )
            : text(
                "该账户将恢复为普通用户，下次财务请求即不再获准。",
                "This account will return to a regular user and its next finance request will be denied.",
                "سيعود الحساب مستخدمًا عاديًا وسيُرفض طلبه المالي التالي.",
              )}
        </p>
        {mutation.error && (
          <p role="alert">
            {mutation.error instanceof ApiError
              ? mutation.error.message
              : text(
                  "无法确认变更结果，请刷新账户角色后再操作。",
                  "The result could not be confirmed. Reload the account role before trying again.",
                  "تعذر تأكيد النتيجة. أعد تحميل دور الحساب قبل المحاولة مجددًا.",
                )}
          </p>
        )}
      </Modal>
    </div>
  );
}

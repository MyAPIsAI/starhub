import { message as translatedMessage } from "../i18n/messages";
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
    ? text(...translatedMessage("grant_read_only_finance_access_29ab002618"))
    : text(...translatedMessage("revoke_finance_access_65e929edac"));
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
              {text(...translatedMessage("cancel_0828e4ce08"))}
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
                ? text(...translatedMessage("updating_06852373b6"))
                : text(...translatedMessage("confirm_change_d9a625c488"))}
            </Button>
          </>
        }
      >
        <p>
          {enabled
            ? text(
                ...translatedMessage(
                  "this_account_will_be_able_to_read_platform_wi_df2d08bf92",
                ),
              )
            : text(
                ...translatedMessage(
                  "this_account_will_return_to_a_regular_user_an_9c2507fab1",
                ),
              )}
        </p>
        {mutation.error && (
          <p role="alert">
            {mutation.error instanceof ApiError
              ? mutation.error.message
              : text(
                  ...translatedMessage(
                    "the_result_could_not_be_confirmed_reload_the__7273b16c8b",
                  ),
                )}
          </p>
        )}
      </Modal>
    </div>
  );
}

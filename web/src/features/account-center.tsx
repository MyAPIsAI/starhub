import { regionalLocale } from "../lib/locale-format";
import { message as translatedMessage } from "../i18n/messages";
import { useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { Check, Copy, Info, LogOut, Monitor, UserRound } from "lucide-react";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/primitives";
import { AccountSecurity } from "./account-security";
import { AccountSecurityHistory } from "./account-security-history";
import { useProductPreview } from "../lib/product-preview";
import {
  useSystemAccess,
  useSessionQueryIdentity,
  sessionUsername,
  sessionQueryIdentity,
} from "../lib/system-access";
import {
  useSessions,
  useUserProfile,
  useTerminateOtherSessions,
  useLogoutCurrent,
} from "../lib/api/user";
import { ApiError } from "../lib/api/http";
import { usePreferences } from "../lib/preferences";
import "./account-center.css";

export default function AccountCenter(props: { onDeactivate: () => void }) {
  const identity = useSessionQueryIdentity();
  return <AccountWorkspace key={JSON.stringify(identity)} {...props} />;
}

function AccountWorkspace({ onDeactivate }: { onDeactivate: () => void }) {
  const sessionsQuery = useSessions();
  const logoutCurrent = useLogoutCurrent();
  const scope = useSessionQueryIdentity();
  const signingOut = useRef(false);
  const profileQuery = useUserProfile();
  const profile = profileQuery.isError ? undefined : profileQuery.data;
  const profileOwnerChanged =
    profileQuery.error instanceof ApiError && profileQuery.error.status === 409;
  const deviceOwnerChanged =
    sessionsQuery.error instanceof ApiError &&
    sessionsQuery.error.status === 409;
  const terminateOthersMut = useTerminateOtherSessions();
  const backendSessions = sessionsQuery.isError
    ? []
    : (sessionsQuery.data ?? []);
  const logout = useProductPreview((state) => state.logout);
  const queryClient = useQueryClient();
  const session = useSystemAccess((state) => state.session);
  const language = usePreferences((state) => state.language);
  const navigate = useNavigate();
  const c = (zh: string, en: string, ar: string) =>
    language === "zh" ? zh : language === "ar" ? ar : en;
  const [notice, setNotice] = useState("");
  const [logoutError, setLogoutError] = useState("");
  const [copied, setCopied] = useState(false);
  const profileName =
    sessionUsername(session) ??
    c(...translatedMessage("account_center_60e51179ff"));
  const accountId =
    session.accountId === null ? null : String(session.accountId);
  const otherSessions = backendSessions.some((item) => !item.is_current);
  const formatDate = (value: string) =>
    new Intl.DateTimeFormat(regionalLocale(language), {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    }).format(new Date(value));
  async function copyId() {
    if (accountId === null) return;
    try {
      await navigator.clipboard.writeText(accountId);
      setCopied(true);
      setNotice(c(...translatedMessage("user_id_copied_a7ab87cd8f")));
    } catch {
      setNotice(
        c(
          ...translatedMessage(
            "copy_failed_select_and_copy_the_user_id_from__877044d541",
          ),
        ),
      );
    }
  }
  async function signOut() {
    if (signingOut.current) return;
    signingOut.current = true;
    setNotice("");
    setLogoutError("");
    const isCurrent = () =>
      JSON.stringify(scope) ===
      JSON.stringify(sessionQueryIdentity(useSystemAccess.getState()));
    try {
      await logoutCurrent.mutateAsync();
      if (!isCurrent()) return;
      useSystemAccess.getState().setSession(null);
      logout();
      queryClient.clear();
      navigate("/login");
    } catch {
      if (isCurrent())
        setLogoutError(
          c(
            ...translatedMessage(
              "sign_out_could_not_be_confirmed_reload_device_dd76a96c33",
            ),
          ),
        );
    } finally {
      signingOut.current = false;
    }
  }
  return (
    <div className="account-center">
      <header className="ac-heading">
        <div>
          <h1>{c(...translatedMessage("account_center_60e51179ff"))}</h1>
          <p>
            {c(
              ...translatedMessage(
                "manage_your_identity_sign_in_methods_and_acco_21278dee79",
              ),
            )}
          </p>
        </div>
        <span className="ac-demo-tag">
          {c(...translatedMessage("server_account_46645f86b3"))}
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
            <p>{c(...translatedMessage("account_profile_cdbd818f5d"))}</p>
          </div>
          <span
            className={`ac-badge ${profile?.status === 1 ? "ac-badge--success" : "ac-badge--warning"}`}
          >
            {profile?.status === 1
              ? c(...translatedMessage("active_1dcd1c3d8f"))
              : profile?.status === 2
                ? c(...translatedMessage("disabled_2556c8592f"))
                : c(...translatedMessage("unconfirmed_6d40d4c4c5"))}
          </span>
        </div>
        <div className="ac-identity-grid">
          <div className="ac-field">
            <label htmlFor="account-username">
              {sessionUsername(session)
                ? c(...translatedMessage("username_52bf46ce51"))
                : c(...translatedMessage("login_account_a78d09d30b"))}
            </label>
            <Input id="account-username" value={profileName} readOnly />
            <small>
              {c(
                ...translatedMessage(
                  "your_sign_in_name_editing_is_unavailable_in_t_5f4a59f856",
                ),
              )}
            </small>
          </div>
          <div className="ac-field">
            <label htmlFor="account-id">
              {c(...translatedMessage("user_id_e23e6a991e"))}
            </label>
            <div className="ac-id">
              <Input
                id="account-id"
                value={accountId ?? "—"}
                readOnly
                dir="ltr"
              />
              <Button
                size="icon"
                aria-label={c(...translatedMessage("copy_user_id_e6772ffc1b"))}
                onClick={copyId}
                disabled={accountId === null}
              >
                {copied ? <Check size={16} /> : <Copy size={16} />}
              </Button>
            </div>
            <small>
              {c(
                ...translatedMessage(
                  "a_permanent_identifier_for_support_and_record_e904b47c07",
                ),
              )}
            </small>
          </div>
          <div className="ac-meta">
            <span>{c(...translatedMessage("account_role_bb4f156761"))}</span>
            <strong>
              {session.platformAdmin
                ? c(...translatedMessage("platform_administrator_489c68ad11"))
                : profile?.role === 100
                  ? c(...translatedMessage("root_93c072d1ee"))
                  : profile?.role === 10
                    ? c(...translatedMessage("administrator_7d64c1eea3"))
                    : profile?.role === 5
                      ? c(...translatedMessage("finance_dfaa8d1e15"))
                      : profile?.role === 1
                        ? c(...translatedMessage("user_f0f3c6b09f"))
                        : c(...translatedMessage("unconfirmed_6d40d4c4c5"))}
            </strong>
            <small>
              {c(
                ...translatedMessage(
                  "platform_permissions_and_service_group_access_1fc081723e",
                ),
              )}
            </small>
          </div>
          <div className="ac-meta">
            <span>{c(...translatedMessage("created_e7096b514b"))}</span>
            <strong>
              {profile?.created_at
                ? formatDate(new Date(profile.created_at * 1000).toISOString())
                : profile
                  ? c(
                      ...translatedMessage(
                        "existing_account_date_not_recorded_fc9fa43f0d",
                      ),
                    )
                  : c(...translatedMessage("unconfirmed_6d40d4c4c5"))}
            </strong>
            <small>
              {c(
                ...translatedMessage(
                  "times_use_your_device_s_time_zone_a9dba7c06c",
                ),
              )}
            </small>
          </div>
        </div>
        {profileQuery.isLoading && (
          <p role="status" className="ac-empty-hint">
            {c(...translatedMessage("loading_account_profile_29056796f7"))}
          </p>
        )}
        {profileQuery.isError && (
          <p role="alert" className="ac-empty-hint">
            {profileOwnerChanged
              ? c(
                  ...translatedMessage(
                    "the_signed_in_account_may_have_changed_confir_2764a99c4c",
                  ),
                )
              : c(
                  ...translatedMessage(
                    "the_account_profile_is_unavailable_previous_d_660a9c847b",
                  ),
                )}
          </p>
        )}
        <div className="ac-device-actions">
          <Button
            size="small"
            variant="ghost"
            disabled={profileQuery.isFetching || session.accountId === null}
            onClick={() =>
              profileOwnerChanged
                ? window.location.reload()
                : void profileQuery.refetch()
            }
          >
            {profileOwnerChanged
              ? c(...translatedMessage("confirm_current_account_9bdf8613b1"))
              : c(...translatedMessage("reload_profile_a2022bbf8f"))}
          </Button>
        </div>
      </section>
      {session.accountKind !== "staff" && <AccountSecurity profile={profile} />}
      <div className="ac-lower-grid">
        <section className="ac-card" aria-labelledby="ac-devices-title">
          <div className="ac-section-heading">
            <div>
              <h2 id="ac-devices-title">
                <Monitor size={18} />
                {c(...translatedMessage("signed_in_devices_568e5614ae"))}
              </h2>
              <p>
                {c(
                  ...translatedMessage(
                    "active_sessions_for_your_account_9abddb32c7",
                  ),
                )}
              </p>
            </div>
          </div>
          <div className="ac-devices">
            {backendSessions.map((session, i) => (
              <div className="ac-device" key={i}>
                <Monitor size={18} />
                <div>
                  <strong>
                    {session.is_current
                      ? c(...translatedMessage("current_session_25470fb346"))
                      : c(...translatedMessage("other_session_7b212c4b24"))}
                  </strong>
                  <p dir="ltr">
                    {c(...translatedMessage("created_1402d40fad"))}{" "}
                    {new Date(session.created_at * 1000).toLocaleString(
                      language === "zh" ? "zh-CN" : "en-GB",
                    )}
                  </p>
                  <small>
                    {c(...translatedMessage("expires_1faaae491b"))}{" "}
                    {new Date(session.expires_at * 1000).toLocaleString(
                      language === "zh" ? "zh-CN" : "en-GB",
                    )}
                  </small>
                </div>
                {session.is_current && (
                  <span className="ac-badge ac-badge--success">
                    {c(...translatedMessage("current_1214f62533"))}
                  </span>
                )}
              </div>
            ))}
            {sessionsQuery.isLoading && (
              <p role="status" className="ac-empty-hint">
                {c(...translatedMessage("loading_account_devices_142e1b67a8"))}
              </p>
            )}
            {sessionsQuery.isError && (
              <div role="alert">
                <p className="ac-empty-hint">
                  {deviceOwnerChanged
                    ? c(
                        ...translatedMessage(
                          "the_signed_in_account_may_have_changed_confir_2764a99c4c",
                        ),
                      )
                    : c(
                        ...translatedMessage(
                          "the_device_list_is_unavailable_previous_recor_a934660fa1",
                        ),
                      )}
                </p>
              </div>
            )}
            {!backendSessions.length &&
              !sessionsQuery.isLoading &&
              !sessionsQuery.isError &&
              session.accountId !== null && (
                <p className="ac-empty-hint">
                  {c(...translatedMessage("no_active_sessions_b451a8bb07"))}
                </p>
              )}
          </div>
          {logoutError && (
            <div className="ac-devices">
              <p role="alert" className="ac-empty-hint">
                {logoutError}
              </p>
            </div>
          )}
          <div className="ac-device-actions">
            <Button
              size="small"
              variant="ghost"
              onClick={() =>
                deviceOwnerChanged
                  ? window.location.reload()
                  : void sessionsQuery.refetch()
              }
              disabled={
                sessionsQuery.isFetching ||
                terminateOthersMut.isPending ||
                session.accountId === null
              }
            >
              {deviceOwnerChanged
                ? c(...translatedMessage("confirm_current_account_9bdf8613b1"))
                : c(...translatedMessage("reload_devices_cf18e295e8"))}
            </Button>
            <Button
              size="small"
              disabled={!otherSessions || terminateOthersMut.isPending}
              onClick={() => {
                terminateOthersMut.mutate(undefined, {
                  onSuccess: () => {
                    setNotice(
                      c(
                        ...translatedMessage(
                          "other_devices_were_signed_out_378f2c3fa1",
                        ),
                      ),
                    );
                  },
                  onError: () =>
                    setNotice(
                      c(
                        ...translatedMessage(
                          "the_device_operation_could_not_be_confirmed_r_94d580c98f",
                        ),
                      ),
                    ),
                });
              }}
            >
              {c(...translatedMessage("sign_out_other_devices_f0a6f5cd66"))}
            </Button>
            <Button
              size="small"
              variant="ghost"
              disabled={logoutCurrent.isPending || session.accountId === null}
              loading={logoutCurrent.isPending}
              onClick={() => signOut()}
            >
              <LogOut size={14} />
              {c(...translatedMessage("sign_out_4fb3312a6b"))}
            </Button>
          </div>
        </section>
        <AccountSecurityHistory />
      </div>
      {session.accountKind !== "staff" && (
        <section className="ac-close">
          <div>
            <h2>{c(...translatedMessage("close_account_e9a0b4116f"))}</h2>
            <p>
              {c(
                ...translatedMessage(
                  "resolve_your_balance_and_pending_orders_befor_f23e84d733",
                ),
              )}
            </p>
          </div>
          <Button size="small" variant="ghost" onClick={onDeactivate}>
            {c(...translatedMessage("close_account_04aaf76b86"))}
          </Button>
        </section>
      )}
    </div>
  );
}

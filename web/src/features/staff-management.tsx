import { message as translatedMessage } from "../i18n/messages";
import { AdminCallDrawer } from "./admin-customers";
import type { AdminCall } from "./admin-shared";
import { useState, type FormEvent } from "react";
import { Search } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useSearchParams } from "react-router-dom";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/primitives";
import { Modal } from "../components/ui/overlay";
import { apiGet, apiPost, apiPut, ApiError } from "../lib/api/http";
import {
  useSessionQueryIdentity,
  useSystemAccess,
  canReadFinance,
  isAdmin,
} from "../lib/system-access";
import {
  staffAuth,
  staffRoleName,
  useStaffText,
  type StaffAccount,
} from "../lib/api/staff";
import { type AdminUser } from "../lib/api/admin";
import { formatMoney } from "../lib/money";
import "../components/staff.css";

export function StaffLogout() {
  const text = useStaffText(),
    navigate = useNavigate(),
    cache = useQueryClient();
  const [busy, setBusy] = useState(false),
    [error, setError] = useState(false);
  return (
    <>
      <Button
        variant="secondary"
        size="small"
        disabled={busy}
        onClick={async () => {
          setBusy(true);
          try {
            await staffAuth.logout();
            useSystemAccess.getState().setSession(null);
            cache.clear();
            navigate("/admin/login", { replace: true });
          } catch {
            setError(true);
          } finally {
            setBusy(false);
          }
        }}
      >
        {text(...translatedMessage("sign_out_85a15f8738"))}
      </Button>
      {error && (
        <span role="alert">
          {text(...translatedMessage("sign_out_failed_retry_5fd5b113ec"))}
        </span>
      )}
    </>
  );
}

function staffSaveMessage(
  error: unknown,
  creating: boolean,
  text: ReturnType<typeof useStaffText>,
) {
  if (error instanceof ApiError) {
    if (
      !creating &&
      error.message ===
        "the last active Root account cannot be disabled or demoted"
    )
      return text(
        ...translatedMessage(
          "the_last_active_root_cannot_be_disabled_or_de_4e1cb81091",
        ),
      );
    if (creating && error.message.includes("username may already exist"))
      return text(
        ...translatedMessage(
          "could_not_create_the_account_this_username_ma_9347240a18",
        ),
      );
    if (error.message === "fresh password and 2FA verification required")
      return text(
        ...translatedMessage(
          "check_your_password_and_current_2fa_code_4764793318",
        ),
      );
    if (error.status === 403)
      return text(
        ...translatedMessage(
          "this_action_requires_root_access_and_an_allow_f5b767594e",
        ),
      );
    if (error.status === 409)
      return text(
        ...translatedMessage(
          "the_account_or_session_has_changed_refresh_th_753b4a041f",
        ),
      );
  }
  return text(
    ...translatedMessage(
      "could_not_confirm_the_save_refresh_the_accoun_66ee6112e1",
    ),
  );
}

export function StaffAccounts() {
  const text = useStaffText(),
    identity = useSessionQueryIdentity(),
    cache = useQueryClient();
  const query = useQuery({
    queryKey: ["staff-accounts", ...identity],
    queryFn: () => apiGet<StaffAccount[]>("/api/admin/staff"),
  });
  const [editing, setEditing] = useState<StaffAccount | "create" | null>(null),
    [name, setName] = useState(""),
    [password, setPassword] = useState(""),
    [code, setCode] = useState(""),
    [role, setRole] = useState(10),
    [status, setStatus] = useState(1),
    [reason, setReason] = useState(""),
    [reset, setReset] = useState(false);
  const self = useSystemAccess((s) => s.session.accountId);
  const [search, setSearch] = useState("");
  const [fieldIssue, setFieldIssue] = useState("");
  const usernameRule = text(
    ...translatedMessage(
      "use_3_64_letters_a_z_digits_periods_underscor_c7a5b66145",
    ),
  );
  const [filterStatus, setFilterStatus] = useState("all");
  const visibleAccounts = query.data?.filter(
    (account) =>
      account.username.toLowerCase().includes(search.trim().toLowerCase()) &&
      (filterStatus === "all" || String(account.status) === filterStatus),
  );
  const mutation = useMutation({
    mutationFn: async () => {
      if (editing === "create")
        return apiPost("/api/admin/staff", {
          username: name,
          temporary_password: password,
          role,
          reason,
        });
      if (!editing) return;
      return apiPut(`/api/admin/staff/${editing.id}`, {
        role,
        status,
        expected_version: editing.version,
        reason,
        reset_2fa: reset,
        current_password: password,
        code,
      });
    },
    onSuccess: () => {
      setEditing(null);
      setPassword("");
      setCode("");
      void cache.invalidateQueries({ queryKey: ["staff-accounts"] });
      void cache.invalidateQueries({ queryKey: ["admin-audit"] });
    },
  });
  const open = (account: StaffAccount | "create") => {
    setFieldIssue("");
    setEditing(account);
    setName("");
    setPassword("");
    setCode("");
    setRole(account === "create" ? 10 : account.role);
    setStatus(account === "create" ? 1 : account.status);
    setReason("");
    setReset(false);
    mutation.reset();
  };
  return (
    <section className="staff-card staff-grid">
      <div className="staff-actions">
        <h1>{text(...translatedMessage("staff_accounts_0df2bf0fa6"))}</h1>
        <Button onClick={() => open("create")}>
          {text(...translatedMessage("add_staff_6d25b85759"))}
        </Button>
      </div>
      <p>
        {text(
          ...translatedMessage(
            "new_accounts_must_change_their_temporary_pass_2a45f450a0",
          ),
        )}
      </p>
      {query.isError && (
        <p role="alert">
          {text(...translatedMessage("unable_to_load_accounts_4b777d2415"))}
          <Button onClick={() => void query.refetch()}>
            {text(...translatedMessage("retry_c16455021c"))}
          </Button>
        </p>
      )}
      {query.isPending && (
        <p role="status">{text(...translatedMessage("loading_4c20385375"))}</p>
      )}
      <div className="staff-list-toolbar">
        <label className="staff-list-search">
          <Search size={16} aria-hidden="true" />
          <Input
            aria-label={text(
              ...translatedMessage("search_staff_accounts_77a3dab258"),
            )}
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder={text(
              ...translatedMessage("search_username_99e8a61c4c"),
            )}
          />
        </label>
        <label>
          <span className="staff-visually-hidden">
            {text(...translatedMessage("filter_staff_status_a6a32ad2f1"))}
          </span>
          <select
            value={filterStatus}
            onChange={(event) => setFilterStatus(event.target.value)}
          >
            <option value="all">
              {text(...translatedMessage("all_statuses_11324f8057"))}
            </option>
            <option value="1">
              {text(...translatedMessage("enabled_5b579584c4"))}
            </option>
            <option value="2">
              {text(...translatedMessage("disabled_2fdba07e0b"))}
            </option>
          </select>
        </label>
        <span className="staff-list-count">
          {text(...translatedMessage("total_710d94671b"))}{" "}
          {visibleAccounts?.length ?? 0}
        </span>
      </div>
      <div className="staff-table-wrap">
        <table className="staff-table">
          <thead>
            <tr>
              <th>{text(...translatedMessage("account_b00965aefc"))}</th>
              <th>{text(...translatedMessage("role_5b688c6fd8"))}</th>
              <th>{text(...translatedMessage("status_dda39dcd63"))}</th>
              <th>2FA</th>
              <th>{text(...translatedMessage("actions_d83f4ed14a"))}</th>
            </tr>
          </thead>
          <tbody>
            {visibleAccounts?.map((account) => (
              <tr key={account.id}>
                <td
                  data-label={text(...translatedMessage("account_b00965aefc"))}
                >
                  {account.username}
                </td>
                <td data-label={text(...translatedMessage("role_5b688c6fd8"))}>
                  {staffRoleName(account.role, text)}
                </td>
                <td
                  data-label={text(...translatedMessage("status_dda39dcd63"))}
                >
                  {account.status === 1
                    ? text(...translatedMessage("enabled_5b579584c4"))
                    : text(...translatedMessage("disabled_2fdba07e0b"))}
                </td>
                <td data-label="2FA">
                  {account.two_factor_bound
                    ? text(...translatedMessage("enrolled_788ece79e7"))
                    : text(...translatedMessage("setup_required_673dc5df52"))}
                </td>
                <td
                  data-label={text(...translatedMessage("actions_d83f4ed14a"))}
                >
                  <Button
                    size="small"
                    variant="secondary"
                    onClick={() => open(account)}
                  >
                    {text(...translatedMessage("manage_e3761d66d9"))}
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {query.isSuccess && visibleAccounts?.length === 0 && (
        <p role="status">
          {text(...translatedMessage("no_matching_staff_accounts_4f86bc0b12"))}
        </p>
      )}
      <Modal
        description={text(
          ...translatedMessage(
            "staff_and_customer_accounts_are_managed_separ_5ff15423f3",
          ),
        )}
        open={editing !== null}
        onOpenChange={(open) => {
          if (!open && !mutation.isPending) {
            setEditing(null);
            setPassword("");
            setCode("");
          }
        }}
        title={
          editing === "create"
            ? text(...translatedMessage("add_staff_account_5ed5199f94"))
            : text(...translatedMessage("manage_staff_account_150a5b8df1"))
        }
        preventOutsideClose
        footer={
          <>
            <Button
              variant="secondary"
              disabled={mutation.isPending}
              onClick={() => {
                setEditing(null);
                setPassword("");
                setCode("");
              }}
            >
              {text(...translatedMessage("cancel_0828e4ce08"))}
            </Button>
            <Button
              form="staff-account-form"
              type="submit"
              disabled={mutation.isPending}
            >
              {text(...translatedMessage("save_6774e725d1"))}
            </Button>
          </>
        }
      >
        <form
          id="staff-account-form"
          className="staff-form"
          noValidate
          onChange={() => {
            setFieldIssue("");
            if (mutation.isError) mutation.reset();
          }}
          onSubmit={(e) => {
            e.preventDefault();
            const form = e.currentTarget;
            if (editing === "create") {
              const username = form.elements.namedItem(
                "username",
              ) as HTMLInputElement;
              username.setCustomValidity(
                /^[A-Za-z0-9_.-]{3,64}$/.test(name) ? "" : usernameRule,
              );
              const temporary = form.elements.namedItem(
                "temporary_password",
              ) as HTMLInputElement;
              const size = new TextEncoder().encode(password).length;
              temporary.setCustomValidity(
                size >= 12 && size <= 72
                  ? ""
                  : text(
                      ...translatedMessage(
                        "the_temporary_password_must_be_12_72_utf_8_by_e7ff8bd8c3",
                      ),
                    ),
              );
            }
            const reasonField = form.elements.namedItem(
              "reason",
            ) as HTMLTextAreaElement;
            const validReason =
              reason.trim() &&
              !/[\r\n\x00]/.test(reason) &&
              new TextEncoder().encode(reason).length <= 500;
            reasonField.setCustomValidity(
              validReason
                ? ""
                : text(
                    ...translatedMessage(
                      "enter_a_single_line_reason_of_up_to_500_utf_8_71f8148b2d",
                    ),
                  ),
            );
            if (!form.reportValidity()) {
              const field = form.querySelector<
                HTMLInputElement | HTMLTextAreaElement
              >(":invalid");
              setFieldIssue(field?.validationMessage ?? "");
              return;
            }
            mutation.mutate();
          }}
        >
          {editing === "create" ? (
            <>
              <label>
                {text(...translatedMessage("username_3274482a04"))}
                <Input
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  name="username"
                  maxLength={64}
                  aria-label={text(...translatedMessage("username_3274482a04"))}
                  aria-describedby="staff-username-rule"
                  required
                  autoComplete="off"
                />
                <small id="staff-username-rule">{usernameRule}</small>
              </label>
              <label>
                {text(
                  ...translatedMessage(
                    "temporary_password_12_72_bytes_b220057ac3",
                  ),
                )}
                <Input
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  required
                  name="temporary_password"
                  autoComplete="new-password"
                />
              </label>
            </>
          ) : (
            <p>{typeof editing === "object" && editing?.username}</p>
          )}
          <label>
            {text(...translatedMessage("role_5b688c6fd8"))}
            <select
              aria-label={text(...translatedMessage("role_5b688c6fd8"))}
              value={role}
              onChange={(e) => setRole(Number(e.target.value))}
            >
              {[100, 10, 5, 20].map((value) => (
                <option key={value} value={value}>
                  {staffRoleName(value, text)}
                </option>
              ))}
            </select>
          </label>
          {editing !== "create" && (
            <>
              <label>
                {text(...translatedMessage("status_dda39dcd63"))}
                <select
                  aria-label={text(...translatedMessage("status_dda39dcd63"))}
                  value={status}
                  onChange={(e) => setStatus(Number(e.target.value))}
                >
                  <option value={1}>
                    {text(...translatedMessage("enabled_5b579584c4"))}
                  </option>
                  <option value={2}>
                    {text(
                      ...translatedMessage(
                        "disable_and_revoke_sessions_3e403d85a0",
                      ),
                    )}
                  </option>
                </select>
              </label>
              {editing?.id !== self && (
                <label className="staff-actions">
                  <input
                    style={{ width: "auto" }}
                    type="checkbox"
                    checked={reset}
                    onChange={(e) => setReset(e.target.checked)}
                  />
                  {text(
                    ...translatedMessage(
                      "reset_this_employee_s_2fa_after_identity_veri_529cdda33d",
                    ),
                  )}
                </label>
              )}
              {reset && (
                <>
                  <p>
                    {text(
                      ...translatedMessage(
                        "confirm_the_employee_s_identity_through_your__f37d451022",
                      ),
                    )}
                  </p>
                  <label>
                    {text(...translatedMessage("your_password_82167a2918"))}
                    <Input
                      type="password"
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      required
                      autoComplete="current-password"
                    />
                  </label>
                  <label>
                    {text(
                      ...translatedMessage(
                        "your_fresh_verification_code_2368ba8c66",
                      ),
                    )}
                    <Input
                      value={code}
                      onChange={(e) => setCode(e.target.value)}
                      pattern="[0-9]{6}"
                      required
                      inputMode="numeric"
                      autoComplete="one-time-code"
                    />
                  </label>
                </>
              )}
            </>
          )}
          <label>
            {text(...translatedMessage("reason_7dbf24f62e"))}
            <textarea
              name="reason"
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              maxLength={500}
              required
              rows={3}
            />
          </label>
          {(fieldIssue || mutation.isError) && (
            <p role="alert" className="staff-error">
              {fieldIssue ||
                staffSaveMessage(mutation.error, editing === "create", text)}
            </p>
          )}
        </form>
      </Modal>
    </section>
  );
}

interface CustomerKey {
  id: number;
  name: string;
  status: number;
  group: string;
}
export function StaffCustomers() {
  const text = useStaffText(),
    identity = useSessionQueryIdentity(),
    role = useSystemAccess((s) => s.role),
    cache = useQueryClient();
  const query = useQuery({
    queryKey: ["admin-users", ...identity],
    queryFn: () => apiGet<AdminUser[]>("/api/user"),
  });
  const [search, setSearch] = useState("");
  const [filterStatus, setFilterStatus] = useState("all");
  const visibleCustomers = query.data?.filter(
    (user) =>
      `${user.username} ${user.id} ${user.email}`
        .toLowerCase()
        .includes(search.trim().toLowerCase()) &&
      (filterStatus === "all" || String(user.status) === filterStatus),
  );
  const [selected, setSelected] = useState<AdminUser | null>(null),
    [action, setAction] = useState<"adjust" | "disable" | null>(null),
    [reason, setReason] = useState(""),
    [amount, setAmount] = useState(""),
    [kind, setKind] = useState("offline_payment"),
    [operation, setOperation] = useState("");
  const [keyTarget, setKeyTarget] = useState<CustomerKey | null>(null);
  const keys = useQuery({
    queryKey: ["staff-customer-keys", ...identity, selected?.id],
    queryFn: () =>
      apiGet<CustomerKey[]>(`/api/admin/customers/${selected!.id}/keys`),
    enabled: !!selected && isAdmin(role),
  });
  const mutation = useMutation({
    mutationFn: async () => {
      if (!selected) return;
      if (keyTarget)
        return apiPost(
          `/api/admin/customers/${selected.id}/keys/${keyTarget.id}/disable`,
          { reason },
        );
      return action === "adjust"
        ? apiPost(`/api/admin/customers/${selected.id}/adjustments`, {
            amount_usd: amount,
            kind,
            reason,
            operation_id: operation,
          })
        : apiPost(`/api/admin/customers/${selected.id}/disable`, { reason });
    },
    onSuccess: () => {
      setAction(null);
      setKeyTarget(null);
      setSelected(null);
      setReason("");
      void cache.invalidateQueries({ queryKey: ["admin-users"] });
      void keys.refetch();
    },
  });
  const begin = (user: AdminUser, next: "adjust" | "disable") => {
    setSelected(user);
    setAction(next);
    setReason("");
    setAmount("");
    setOperation(crypto.randomUUID());
    setKind("offline_payment");
    mutation.reset();
  };
  return (
    <section className="staff-card staff-grid">
      <h1>{text(...translatedMessage("customer_accounts_86a2111562"))}</h1>
      {query.isError && (
        <p role="alert">
          {text(...translatedMessage("customer_list_unavailable_88a8ceb67b"))}
          <Button onClick={() => void query.refetch()}>
            {text(...translatedMessage("retry_c16455021c"))}
          </Button>
        </p>
      )}
      {query.isPending && (
        <p role="status">{text(...translatedMessage("loading_4c20385375"))}</p>
      )}
      <div className="staff-list-toolbar">
        <label className="staff-list-search">
          <Search size={16} aria-hidden="true" />
          <Input
            aria-label={text(
              ...translatedMessage("search_customer_accounts_a059378d06"),
            )}
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder={text(
              ...translatedMessage("search_username_id_or_email_bc6183ef15"),
            )}
          />
        </label>
        <label>
          <span className="staff-visually-hidden">
            {text(...translatedMessage("filter_customer_status_7d14c63b9c"))}
          </span>
          <select
            value={filterStatus}
            onChange={(event) => setFilterStatus(event.target.value)}
          >
            <option value="all">
              {text(...translatedMessage("all_statuses_11324f8057"))}
            </option>
            <option value="1">
              {text(...translatedMessage("enabled_5b579584c4"))}
            </option>
            <option value="2">
              {text(...translatedMessage("disabled_2fdba07e0b"))}
            </option>
          </select>
        </label>
        <span className="staff-list-count">
          {text(...translatedMessage("total_710d94671b"))}{" "}
          {visibleCustomers?.length ?? 0}
        </span>
      </div>
      <div className="staff-table-wrap">
        <table className="staff-table">
          <thead>
            <tr>
              <th>{text(...translatedMessage("customer_b6fee18c46"))}</th>
              <th>{text(...translatedMessage("status_dda39dcd63"))}</th>
              <th>{text(...translatedMessage("balance_usd_01de08312c"))}</th>
              <th>{text(...translatedMessage("actions_d83f4ed14a"))}</th>
            </tr>
          </thead>
          <tbody>
            {visibleCustomers?.map((user) => (
              <tr key={user.id}>
                <td
                  data-label={text(...translatedMessage("customer_b6fee18c46"))}
                >
                  {user.username}
                  <br />
                  <small>{user.email}</small>
                </td>
                <td
                  data-label={text(...translatedMessage("status_dda39dcd63"))}
                >
                  {user.status === 1
                    ? text(...translatedMessage("enabled_5b579584c4"))
                    : text(...translatedMessage("disabled_2fdba07e0b"))}
                </td>
                <td
                  data-label={text(
                    ...translatedMessage("balance_usd_01de08312c"),
                  )}
                >
                  {typeof user.api_balance_nano === "string" &&
                  /^-?\d+$/.test(user.api_balance_nano)
                    ? formatMoney(user.api_balance_nano, "detail")
                    : "—"}
                </td>
                <td
                  data-label={text(...translatedMessage("actions_d83f4ed14a"))}
                >
                  <div className="staff-actions">
                    {isAdmin(role) && (
                      <>
                        <Button
                          size="small"
                          variant="secondary"
                          onClick={() => {
                            setSelected(user);
                            setAction(null);
                            setReason("");
                          }}
                        >
                          {text(...translatedMessage("view_keys_da3047157a"))}
                        </Button>
                        {user.status === 1 && (
                          <Button
                            size="small"
                            variant="danger"
                            onClick={() => begin(user, "disable")}
                          >
                            {text(
                              ...translatedMessage(
                                "disable_customer_ba364899b8",
                              ),
                            )}
                          </Button>
                        )}
                      </>
                    )}
                    {canReadFinance(role) && (
                      <Button
                        size="small"
                        onClick={() => begin(user, "adjust")}
                      >
                        {text(
                          ...translatedMessage("adjust_balance_b29cbcf25c"),
                        )}
                      </Button>
                    )}
                    {!isAdmin(role) &&
                      !canReadFinance(role) &&
                      text(...translatedMessage("read_only_a501d9edeb"))}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {query.isSuccess && visibleCustomers?.length === 0 && (
        <p>{text(...translatedMessage("no_customers_yet_644eb34b1d"))}</p>
      )}
      <Modal
        open={!!selected}
        onOpenChange={(open) => {
          if (!open && !mutation.isPending) {
            setSelected(null);
            setAction(null);
            setKeyTarget(null);
          }
        }}
        title={
          action === "adjust"
            ? text(...translatedMessage("adjust_balance_b29cbcf25c"))
            : action === "disable"
              ? text(...translatedMessage("disable_customer_ba364899b8"))
              : text(...translatedMessage("customer_keys_4d038aaf7a"))
        }
        description={selected?.username ?? ""}
        footer={
          <Button
            variant="secondary"
            onClick={() => {
              setSelected(null);
              setAction(null);
              setKeyTarget(null);
            }}
            disabled={mutation.isPending}
          >
            {text(...translatedMessage("close_6bf477426b"))}
          </Button>
        }
      >
        {action || keyTarget ? (
          <form
            className="staff-form"
            onSubmit={(event: FormEvent) => {
              event.preventDefault();
              mutation.mutate();
            }}
          >
            {action === "adjust" && (
              <>
                <label>
                  {text(...translatedMessage("type_b91a24e5ad"))}
                  <select
                    aria-label={text(...translatedMessage("type_b91a24e5ad"))}
                    value={kind}
                    onChange={(e) => setKind(e.target.value)}
                  >
                    {[
                      ["gift", text(...translatedMessage("gift_7e12bff87d"))],
                      [
                        "compensation",
                        text(...translatedMessage("compensation_f4a2152f83")),
                      ],
                      [
                        "offline_payment",
                        text(
                          ...translatedMessage("offline_payment_41b40d81a0"),
                        ),
                      ],
                      [
                        "deduction",
                        text(...translatedMessage("deduction_09d825b922")),
                      ],
                    ].map(([value, label]) => (
                      <option value={value} key={value}>
                        {label}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  {text(
                    ...translatedMessage(
                      "signed_amount_usd_up_to_9_decimal_places_aead24dbc8",
                    ),
                  )}
                  <Input
                    value={amount}
                    onChange={(e) => setAmount(e.target.value)}
                    inputMode="decimal"
                    pattern="-?(0|[1-9][0-9]*)(\.[0-9]{1,9})?"
                    required
                  />
                </label>
                <p>
                  {text(
                    ...translatedMessage(
                      "posted_immediately_enter_a_negative_amount_fo_79f1c1c349",
                    ),
                  )}
                </p>
              </>
            )}
            {keyTarget && (
              <p>
                {text(...translatedMessage("disable_key_85671cf1c7"))}:{" "}
                {keyTarget.name}
              </p>
            )}
            <label>
              {text(...translatedMessage("reason_required_49162ff107"))}
              <textarea
                value={reason}
                onChange={(e) => setReason(e.target.value)}
                required
                maxLength={500}
                rows={3}
              />
            </label>
            {mutation.isError && (
              <p role="alert" className="staff-error">
                {text(
                  ...translatedMessage(
                    "action_failed_check_the_amount_permissions_an_934ea9d9d4",
                  ),
                )}
              </p>
            )}
            <Button type="submit" disabled={mutation.isPending}>
              {text(...translatedMessage("confirm_f2ab6dbf01"))}
            </Button>
          </form>
        ) : (
          <div className="staff-grid">
            <p>
              {text(
                ...translatedMessage(
                  "key_names_and_status_only_full_customer_keys__4b40db5381",
                ),
              )}
            </p>
            {keys.isError && (
              <p role="alert">
                {text(...translatedMessage("unable_to_load_keys_f8cfb5ade2"))}
              </p>
            )}
            {keys.data?.map((key) => (
              <div key={key.id} className="staff-actions">
                <span>
                  {key.name} · {key.group} ·{" "}
                  {key.status === 1
                    ? text(...translatedMessage("enabled_5b579584c4"))
                    : text(...translatedMessage("disabled_2fdba07e0b"))}
                </span>
                {key.status === 1 && (
                  <Button
                    variant="danger"
                    size="small"
                    onClick={() => {
                      setKeyTarget(key);
                      setReason("");
                      mutation.reset();
                    }}
                  >
                    {text(...translatedMessage("disable_300dc72531"))}
                  </Button>
                )}
              </div>
            ))}
          </div>
        )}
      </Modal>
    </section>
  );
}

export { AdminResourceStatus as StaffSync } from "./admin-resource-status";

export function StaffCalls() {
  const text = useStaffText(),
    identity = useSessionQueryIdentity();
  const [searchParams, setSearchParams] = useSearchParams();
  const resourceId = searchParams.get("resource_id") || "";
  const [pagination, setPagination] = useState({ resourceId, page: 1 });
  const page = pagination.resourceId === resourceId ? pagination.page : 1;
  const setPage = (value: number) => setPagination({ resourceId, page: value });
  const [selectedCall, setSelectedCall] = useState<AdminCall | null>(null);
  const query = useQuery({
    queryKey: ["staff-calls", ...identity, resourceId, page],
    queryFn: () =>
      apiGet<{
        items: AdminCall[];
        total: number;
      }>(
        `/api/log?${new URLSearchParams({ p: String(page), page_size: "20", ...(resourceId ? { resource_id: resourceId } : {}) })}`,
      ),
  });
  return (
    <section className="staff-card staff-grid">
      <h1>{text(...translatedMessage("call_details_50e18ac31f"))}</h1>
      {resourceId && (
        <div className="staff-actions" data-testid="calls-resource-filter">
          <span>
            {text(...translatedMessage("resource_68017d94a7"))}:{" "}
            <bdi>{resourceId}</bdi>
          </span>
          <Button
            size="small"
            onClick={() => {
              const next = new URLSearchParams(searchParams);
              next.delete("resource_id");
              setSearchParams(next);
            }}
          >
            {text(...translatedMessage("clear_filter_b4e85f7420"))}
          </Button>
        </div>
      )}
      {query.isError && (
        <p role="alert">
          {text(...translatedMessage("calls_unavailable_a1a91d2fc4"))}
        </p>
      )}
      <div className="staff-table-wrap">
        <table className="staff-table">
          <thead>
            <tr>
              <th>{text(...translatedMessage("customer_b6fee18c46"))}</th>
              <th>{text(...translatedMessage("model_9c2e5b5065"))}</th>
              <th>Trace ID</th>
              <th>{text(...translatedMessage("status_dda39dcd63"))}</th>
            </tr>
          </thead>
          <tbody>
            {query.data?.items?.map((row) => (
              <tr key={row.id}>
                <td
                  data-label={text(...translatedMessage("customer_b6fee18c46"))}
                >
                  {row.user_id}
                </td>
                <td data-label={text(...translatedMessage("model_9c2e5b5065"))}>
                  {row.model}
                </td>
                <td data-label="Trace ID">
                  <button
                    className="adm-link"
                    onClick={() => setSelectedCall(row)}
                  >
                    {row.trace_id}
                  </button>
                </td>
                <td
                  data-label={text(...translatedMessage("status_dda39dcd63"))}
                >
                  {row.status === 1
                    ? text(...translatedMessage("succeeded_1a524ac49d"))
                    : row.status === 3
                      ? text(...translatedMessage("cancelled_544275e39b"))
                      : text(...translatedMessage("failed_5da76a883a"))}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {query.isPending && (
        <p role="status">{text(...translatedMessage("loading_4c20385375"))}</p>
      )}
      {query.isSuccess && !query.data.items.length && (
        <p role="status">
          {text(...translatedMessage("no_matching_calls_59bac795a9"))}
        </p>
      )}
      <AdminCallDrawer
        call={selectedCall}
        onClose={() => setSelectedCall(null)}
      />
      <div className="staff-actions">
        <Button
          variant="secondary"
          disabled={page === 1}
          onClick={() => setPage(page - 1)}
        >
          {text(...translatedMessage("previous_2672646f39"))}
        </Button>
        <span>{page}</span>
        <Button
          variant="secondary"
          disabled={!query.data || page * 20 >= query.data.total}
          onClick={() => setPage(page + 1)}
        >
          {text(...translatedMessage("next_c0fd39244f"))}
        </Button>
      </div>
    </section>
  );
}

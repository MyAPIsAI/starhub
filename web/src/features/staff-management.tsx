import { AdminCallDrawer } from "./admin-customers";
import type { AdminCall } from "./admin-shared";
import { useState, type FormEvent } from "react";
import { Search } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
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
        {text("退出后台", "Sign out", "تسجيل الخروج")}
      </Button>
      {error && (
        <span role="alert">
          {text(
            "退出失败，请重试。",
            "Sign out failed. Retry.",
            "فشل تسجيل الخروج. حاول مجددًا.",
          )}
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
        "最后一位启用的超级管理员不能被停用或降权。",
        "The last active Root cannot be disabled or demoted.",
        "لا يمكن تعطيل آخر مسؤول أعلى نشط أو تخفيض دوره.",
      );
    if (creating && error.message.includes("username may already exist"))
      return text(
        "未能创建账号。账号名可能已被使用，请检查账号列表或换一个名称。",
        "Could not create the account. This username may already be in use. Check the account list or choose another name.",
        "تعذّر إنشاء الحساب. قد يكون اسم المستخدم مستخدمًا. تحقق من قائمة الحسابات أو اختر اسمًا آخر.",
      );
    if (error.message === "fresh password and 2FA verification required")
      return text(
        "请重新核对你的密码和当前 2FA 验证码。",
        "Check your password and current 2FA code.",
        "تحقق من كلمة مرورك ورمز المصادقة الحالي.",
      );
    if (error.status === 403)
      return text(
        "此操作需要超级管理员权限和允许的网络。",
        "This action requires Root access and an allowed network.",
        "يتطلب هذا الإجراء صلاحية المسؤول الأعلى وشبكة مسموحًا بها.",
      );
    if (error.status === 409)
      return text(
        "账号资料或会话已变化，请刷新账号列表后再试。",
        "The account or session has changed. Refresh the account list and try again.",
        "تغير الحساب أو الجلسة. حدّث قائمة الحسابات وحاول مجددًا.",
      );
  }
  return text(
    "未能确认保存结果，请刷新账号列表后再试。",
    "Could not confirm the save. Refresh the account list before retrying.",
    "تعذّر تأكيد الحفظ. حدّث قائمة الحسابات قبل إعادة المحاولة.",
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
    "账号名须为 3–64 位，只能使用英文字母、数字、点、下划线或连字符。",
    "Use 3–64 letters (A–Z), digits, periods, underscores or hyphens.",
    "أدخل من 3 إلى 64 حرفًا لاتينيًا أو رقمًا أو نقطة أو شرطة أو شرطة سفلية.",
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
        <h1>{text("后台账号", "Staff accounts", "حسابات الموظفين")}</h1>
        <Button onClick={() => open("create")}>
          {text("新增员工", "Add staff", "إضافة موظف")}
        </Button>
      </div>
      <p>
        {text(
          "新账号首次登录须改密并绑定 2FA。停用或更改角色会立即注销已有会话；账号与审计身份保留。",
          "New accounts must change their temporary password and set up 2FA. Disabling or changing a role revokes existing sessions; account and audit identities are retained.",
          "يجب تغيير كلمة المرور المؤقتة وإعداد المصادقة الثنائية. يؤدي التعطيل أو تغيير الدور إلى إلغاء الجلسات مع الاحتفاظ بهوية الحساب والتدقيق.",
        )}
      </p>
      {query.isError && (
        <p role="alert">
          {text(
            "账号加载失败。",
            "Unable to load accounts.",
            "تعذر تحميل الحسابات.",
          )}
          <Button onClick={() => void query.refetch()}>
            {text("重试", "Retry", "إعادة المحاولة")}
          </Button>
        </p>
      )}
      {query.isPending && (
        <p role="status">{text("加载中…", "Loading…", "جارٍ التحميل…")}</p>
      )}
      <div className="staff-list-toolbar">
        <label className="staff-list-search">
          <Search size={16} aria-hidden="true" />
          <Input
            aria-label={text(
              "搜索后台账号",
              "Search staff accounts",
              "البحث في حسابات الموظفين",
            )}
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder={text(
              "搜索用户名",
              "Search username",
              "البحث باسم المستخدم",
            )}
          />
        </label>
        <label>
          <span className="staff-visually-hidden">
            {text(
              "按状态筛选后台账号",
              "Filter staff status",
              "تصفية حالة الموظفين",
            )}
          </span>
          <select
            value={filterStatus}
            onChange={(event) => setFilterStatus(event.target.value)}
          >
            <option value="all">
              {text("全部状态", "All statuses", "جميع الحالات")}
            </option>
            <option value="1">{text("启用", "Enabled", "مفعّل")}</option>
            <option value="2">{text("停用", "Disabled", "معطّل")}</option>
          </select>
        </label>
        <span className="staff-list-count">
          {text("共", "Total", "المجموع")} {visibleAccounts?.length ?? 0}
        </span>
      </div>
      <div className="staff-table-wrap">
        <table className="staff-table">
          <thead>
            <tr>
              <th>{text("账号", "Account", "الحساب")}</th>
              <th>{text("角色", "Role", "الدور")}</th>
              <th>{text("状态", "Status", "الحالة")}</th>
              <th>2FA</th>
              <th>{text("操作", "Actions", "الإجراءات")}</th>
            </tr>
          </thead>
          <tbody>
            {visibleAccounts?.map((account) => (
              <tr key={account.id}>
                <td data-label={text("账号", "Account", "الحساب")}>
                  {account.username}
                </td>
                <td data-label={text("角色", "Role", "الدور")}>
                  {staffRoleName(account.role, text)}
                </td>
                <td data-label={text("状态", "Status", "الحالة")}>
                  {account.status === 1
                    ? text("启用", "Enabled", "مفعّل")
                    : text("停用", "Disabled", "معطّل")}
                </td>
                <td data-label="2FA">
                  {account.two_factor_bound
                    ? text("已绑定", "Enrolled", "مُعدّة")
                    : text("待绑定", "Setup required", "يلزم الإعداد")}
                </td>
                <td data-label={text("操作", "Actions", "الإجراءات")}>
                  <Button
                    size="small"
                    variant="secondary"
                    onClick={() => open(account)}
                  >
                    {text("管理", "Manage", "إدارة")}
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {query.isSuccess && visibleAccounts?.length === 0 && (
        <p role="status">
          {text(
            "没有匹配的后台账号。",
            "No matching staff accounts.",
            "لا توجد حسابات موظفين مطابقة.",
          )}
        </p>
      )}
      <Modal
        description={text(
          "内部员工账号与客户账号分开管理。",
          "Staff and customer accounts are managed separately.",
          "تُدار حسابات الموظفين والعملاء بشكل منفصل.",
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
            ? text("新增内部账号", "Add staff account", "إضافة حساب موظف")
            : text("管理内部账号", "Manage staff account", "إدارة حساب موظف")
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
              {text("取消", "Cancel", "إلغاء")}
            </Button>
            <Button
              form="staff-account-form"
              type="submit"
              disabled={mutation.isPending}
            >
              {text("保存", "Save", "حفظ")}
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
                      "临时密码须为 12–72 个 UTF-8 字节。",
                      "The temporary password must be 12–72 UTF-8 bytes.",
                      "يجب أن تكون كلمة المرور المؤقتة بين 12 و72 بايت بترميز UTF-8.",
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
                    "请填写单行原因，最多 500 个 UTF-8 字节。",
                    "Enter a single-line reason of up to 500 UTF-8 bytes.",
                    "أدخل سببًا في سطر واحد لا يتجاوز 500 بايت بترميز UTF-8.",
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
                {text("账号名", "Username", "اسم المستخدم")}
                <Input
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  name="username"
                  maxLength={64}
                  aria-label={text("账号名", "Username", "اسم المستخدم")}
                  aria-describedby="staff-username-rule"
                  required
                  autoComplete="off"
                />
                <small id="staff-username-rule">{usernameRule}</small>
              </label>
              <label>
                {text(
                  "临时密码（12–72 字节）",
                  "Temporary password (12–72 bytes)",
                  "كلمة المرور المؤقتة (12–72 بايت)",
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
            {text("角色", "Role", "الدور")}
            <select
              aria-label={text("角色", "Role", "الدور")}
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
                {text("状态", "Status", "الحالة")}
                <select
                  aria-label={text("状态", "Status", "الحالة")}
                  value={status}
                  onChange={(e) => setStatus(Number(e.target.value))}
                >
                  <option value={1}>{text("启用", "Enabled", "مفعّل")}</option>
                  <option value={2}>
                    {text(
                      "停用并注销会话",
                      "Disable and revoke sessions",
                      "تعطيل وإلغاء الجلسات",
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
                    "身份核验后重置该员工 2FA",
                    "Reset this employee's 2FA after identity verification",
                    "إعادة ضبط المصادقة الثنائية بعد التحقق من الهوية",
                  )}
                </label>
              )}
              {reset && (
                <>
                  <p>
                    {text(
                      "确认已通过线下流程核验员工身份，并重新验证你的密码与新的 2FA 验证码。",
                      "Confirm the employee's identity through your offline process, then verify your own password and a fresh 2FA code.",
                      "أكّد هوية الموظف عبر الإجراء المعتمد ثم أدخل كلمة مرورك ورمز مصادقة جديدًا.",
                    )}
                  </p>
                  <label>
                    {text("你的密码", "Your password", "كلمة مرورك")}
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
                      "你的新验证码",
                      "Your fresh verification code",
                      "رمز التحقق الجديد الخاص بك",
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
            {text("原因", "Reason", "السبب")}
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
      <h1>{text("账号管理", "Customer accounts", "حسابات العملاء")}</h1>
      {query.isError && (
        <p role="alert">
          {text(
            "客户列表加载失败。",
            "Customer list unavailable.",
            "تعذّر تحميل العملاء.",
          )}
          <Button onClick={() => void query.refetch()}>
            {text("重试", "Retry", "إعادة المحاولة")}
          </Button>
        </p>
      )}
      {query.isPending && (
        <p role="status">{text("加载中…", "Loading…", "جارٍ التحميل…")}</p>
      )}
      <div className="staff-list-toolbar">
        <label className="staff-list-search">
          <Search size={16} aria-hidden="true" />
          <Input
            aria-label={text(
              "搜索客户账号",
              "Search customer accounts",
              "البحث في حسابات العملاء",
            )}
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder={text(
              "搜索用户名、标识或邮箱",
              "Search username, ID or email",
              "البحث بالاسم أو المعرّف أو البريد",
            )}
          />
        </label>
        <label>
          <span className="staff-visually-hidden">
            {text(
              "按状态筛选客户账号",
              "Filter customer status",
              "تصفية حالة العملاء",
            )}
          </span>
          <select
            value={filterStatus}
            onChange={(event) => setFilterStatus(event.target.value)}
          >
            <option value="all">
              {text("全部状态", "All statuses", "جميع الحالات")}
            </option>
            <option value="1">{text("启用", "Enabled", "مفعّل")}</option>
            <option value="2">{text("停用", "Disabled", "معطّل")}</option>
          </select>
        </label>
        <span className="staff-list-count">
          {text("共", "Total", "المجموع")} {visibleCustomers?.length ?? 0}
        </span>
      </div>
      <div className="staff-table-wrap">
        <table className="staff-table">
          <thead>
            <tr>
              <th>{text("客户", "Customer", "العميل")}</th>
              <th>{text("状态", "Status", "الحالة")}</th>
              <th>{text("余额（USD）", "Balance (USD)", "الرصيد (USD)")}</th>
              <th>{text("操作", "Actions", "الإجراءات")}</th>
            </tr>
          </thead>
          <tbody>
            {visibleCustomers?.map((user) => (
              <tr key={user.id}>
                <td data-label={text("客户", "Customer", "العميل")}>
                  {user.username}
                  <br />
                  <small>{user.email}</small>
                </td>
                <td data-label={text("状态", "Status", "الحالة")}>
                  {user.status === 1
                    ? text("启用", "Enabled", "مفعّل")
                    : text("停用", "Disabled", "معطّل")}
                </td>
                <td
                  data-label={text(
                    "余额（USD）",
                    "Balance (USD)",
                    "الرصيد (USD)",
                  )}
                >
                  {typeof user.api_balance_nano === "string" &&
                  /^-?\d+$/.test(user.api_balance_nano)
                    ? formatMoney(user.api_balance_nano, "detail")
                    : "—"}
                </td>
                <td data-label={text("操作", "Actions", "الإجراءات")}>
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
                          {text("查看 Key", "View keys", "عرض المفاتيح")}
                        </Button>
                        {user.status === 1 && (
                          <Button
                            size="small"
                            variant="danger"
                            onClick={() => begin(user, "disable")}
                          >
                            {text(
                              "停用客户",
                              "Disable customer",
                              "تعطيل العميل",
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
                        {text("调整余额", "Adjust balance", "تعديل الرصيد")}
                      </Button>
                    )}
                    {!isAdmin(role) &&
                      !canReadFinance(role) &&
                      text("只读", "Read only", "للقراءة فقط")}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {query.isSuccess && visibleCustomers?.length === 0 && (
        <p>{text("暂无客户。", "No customers yet.", "لا يوجد عملاء بعد.")}</p>
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
            ? text("调整余额", "Adjust balance", "تعديل الرصيد")
            : action === "disable"
              ? text("停用客户", "Disable customer", "تعطيل العميل")
              : text("客户 Key", "Customer keys", "مفاتيح العميل")
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
            {text("关闭", "Close", "إغلاق")}
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
                  {text("类型", "Type", "النوع")}
                  <select
                    aria-label={text("类型", "Type", "النوع")}
                    value={kind}
                    onChange={(e) => setKind(e.target.value)}
                  >
                    {[
                      ["gift", text("赠送", "Gift", "هدية")],
                      ["compensation", text("补偿", "Compensation", "تعويض")],
                      [
                        "offline_payment",
                        text("线下付款", "Offline payment", "دفع غير متصل"),
                      ],
                      ["deduction", text("扣减", "Deduction", "خصم")],
                    ].map(([value, label]) => (
                      <option value={value} key={value}>
                        {label}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  {text(
                    "有符号金额（USD，最多 9 位小数）",
                    "Signed amount (USD, up to 9 decimal places)",
                    "المبلغ بإشارته (USD، حتى 9 منازل عشرية)",
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
                    "立即记账；扣减填写负数，允许账户余额变为负数。",
                    "Posted immediately. Enter a negative amount for a deduction; the balance may become negative.",
                    "يُسجّل فورًا. أدخل مبلغًا سالبًا للخصم؛ يمكن أن يصبح الرصيد سالبًا.",
                  )}
                </p>
              </>
            )}
            {keyTarget && (
              <p>
                {text("停用 Key", "Disable key", "تعطيل المفتاح")}:{" "}
                {keyTarget.name}
              </p>
            )}
            <label>
              {text("原因（必填）", "Reason (required)", "السبب (مطلوب)")}
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
                  "操作失败，请检查金额、权限和原因后重试。",
                  "Action failed. Check the amount, permissions and reason, then retry.",
                  "فشل الإجراء. تحقّق من المبلغ والصلاحيات والسبب وأعد المحاولة.",
                )}
              </p>
            )}
            <Button type="submit" disabled={mutation.isPending}>
              {text("确认", "Confirm", "تأكيد")}
            </Button>
          </form>
        ) : (
          <div className="staff-grid">
            <p>
              {text(
                "仅显示 Key 名称与状态，不显示客户完整 Key。",
                "Key names and status only; full customer keys are not shown.",
                "تُعرض أسماء المفاتيح وحالتها فقط، دون المفاتيح الكاملة.",
              )}
            </p>
            {keys.isError && (
              <p role="alert">
                {text(
                  "加载失败。",
                  "Unable to load keys.",
                  "تعذّر تحميل المفاتيح.",
                )}
              </p>
            )}
            {keys.data?.map((key) => (
              <div key={key.id} className="staff-actions">
                <span>
                  {key.name} · {key.group} ·{" "}
                  {key.status === 1
                    ? text("启用", "Enabled", "مفعّل")
                    : text("停用", "Disabled", "معطّل")}
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
                    {text("停用", "Disable", "تعطيل")}
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

export function StaffSync() {
  const text = useStaffText(),
    identity = useSessionQueryIdentity();
  const status = useQuery({
    queryKey: ["staff-sync", ...identity],
    queryFn: () =>
      apiGet<{
        configured: boolean;
        revision: number;
        sequence: number;
        updated_at: number;
      }>("/api/admin/resource-sync"),
  });
  const system = useQuery({
    queryKey: ["staff-system", ...identity],
    queryFn: () =>
      apiGet<{ version: string; go_version: string; timezone: string }>(
        "/api/admin/system",
      ),
  });
  const mutation = useMutation({
    mutationFn: () =>
      apiPost<{ revision: number; sequence: number }>(
        "/api/admin/resource-sync",
      ),
    onSuccess: () => void status.refetch(),
  });
  return (
    <section className="staff-card staff-grid">
      <div className="staff-actions">
        <h1>
          {text(
            "同步与路由状态",
            "Sync and routing status",
            "حالة المزامنة والتوجيه",
          )}
        </h1>
        <Button
          disabled={!status.data?.configured || mutation.isPending}
          onClick={() => mutation.mutate()}
        >
          {mutation.isPending
            ? text("同步中…", "Syncing…", "جارٍ المزامنة…")
            : text("强制全量同步", "Force full sync", "فرض مزامنة كاملة")}
        </Button>
      </div>
      <p>
        {text(
          "查看上游快照同步情况。只有全量同步成功后才替换已发布快照。",
          "Inspect upstream snapshot synchronization. Published snapshots change only after a successful full sync.",
          "تابع مزامنة لقطات المصدر. لا تتغير اللقطة المنشورة إلا بعد نجاح المزامنة الكاملة.",
        )}
      </p>
      {status.isPending && (
        <p role="status">{text("加载中…", "Loading…", "جارٍ التحميل…")}</p>
      )}
      {status.data && (
        <div className="staff-sync-summary">
          <div>
            <span>
              {text("上游快照来源", "Upstream snapshot source", "مصدر اللقطات")}
            </span>
            <strong>
              {status.data.configured
                ? text("已配置", "Configured", "مُعدّ")
                : text("未配置", "Not configured", "غير مُعدّ")}
            </strong>
            <small>
              {status.data.configured
                ? text(
                    "可请求全量同步",
                    "Full sync available",
                    "المزامنة الكاملة متاحة",
                  )
                : text(
                    "需在部署端配置快照来源",
                    "Configure the source in deployment",
                    "يلزم إعداد المصدر في النشر",
                  )}
            </small>
          </div>
          <div>
            <span>
              {text(
                "已发布快照序号",
                "Published snapshot sequence",
                "تسلسل اللقطة المنشورة",
              )}
            </span>
            <strong>{status.data.sequence}</strong>
          </div>
          <div>
            <span>
              {text(
                "资源配置版本",
                "Resource configuration revision",
                "إصدار إعداد الموارد",
              )}
            </span>
            <strong>{status.data.revision}</strong>
          </div>
        </div>
      )}
      {mutation.isSuccess && (
        <p role="status">
          {text("同步完成。", "Sync completed.", "اكتملت المزامنة.")}
        </p>
      )}
      {(status.isError || system.isError || mutation.isError) && (
        <p role="alert" className="staff-error">
          {text(
            "加载或同步失败，已发布快照保持不变。",
            "Loading or sync failed. The published snapshot is unchanged.",
            "فشل التحميل أو المزامنة. لم تتغير اللقطة المنشورة.",
          )}
          {mutation.error instanceof ApiError &&
            mutation.error.status === 409 &&
            text(
              " 请检查上游快照序号与内容。",
              " Check the upstream sequence and content.",
              " تحقّق من تسلسل المصدر ومحتواه.",
            )}
        </p>
      )}
      {system.data && (
        <p>
          StarHub {system.data.version} · {system.data.go_version} ·{" "}
          {system.data.timezone}
        </p>
      )}
    </section>
  );
}

export function StaffCalls() {
  const text = useStaffText(),
    identity = useSessionQueryIdentity(),
    [page, setPage] = useState(1);
  const [selectedCall, setSelectedCall] = useState<AdminCall | null>(null);
  const query = useQuery({
    queryKey: ["staff-calls", ...identity, page],
    queryFn: () =>
      apiGet<{
        items: AdminCall[];
        total: number;
      }>(`/api/log?p=${page}&page_size=20`),
  });
  return (
    <section className="staff-card staff-grid">
      <h1>
        {text(
          "调用明细",
          "Call details",
          "تفاصيل الاستدعاءات",
        )}
      </h1>
      {query.isError && (
        <p role="alert">
          {text(
            "调用记录加载失败。",
            "Calls unavailable.",
            "تعذّر تحميل الاستدعاءات.",
          )}
        </p>
      )}
      <div className="staff-table-wrap">
        <table className="staff-table">
          <thead>
            <tr>
              <th>{text("客户", "Customer", "العميل")}</th>
              <th>{text("模型", "Model", "النموذج")}</th>
              <th>Trace ID</th>
              <th>{text("状态", "Status", "الحالة")}</th>
            </tr>
          </thead>
          <tbody>
            {query.data?.items?.map((row) => (
              <tr key={row.id}>
                <td data-label={text("客户", "Customer", "العميل")}>
                  {row.user_id}
                </td>
                <td data-label={text("模型", "Model", "النموذج")}>
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
                <td data-label={text("状态", "Status", "الحالة")}>
                  {row.status === 1
                    ? text("成功", "Succeeded", "نجح")
                    : row.status === 3
                      ? text("已取消", "Cancelled", "ملغى")
                      : text("失败", "Failed", "فشل")}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <AdminCallDrawer
        call={selectedCall}
        onClose={() => setSelectedCall(null)}
      />
      <div className="staff-actions">
        <Button
          variant="secondary"
          disabled={page === 1}
          onClick={() => setPage((p) => p - 1)}
        >
          {text("上一页", "Previous", "السابق")}
        </Button>
        <span>{page}</span>
        <Button
          variant="secondary"
          disabled={!query.data || page * 20 >= query.data.total}
          onClick={() => setPage((p) => p + 1)}
        >
          {text("下一页", "Next", "التالي")}
        </Button>
      </div>
    </section>
  );
}

import { staffRoleName } from "./api/staff";
import { formatFinanceAmount } from "./finance-format";

export type AuditText = (zh: string, en: string, ar: string) => string;
type Labels = Record<string, readonly [string, string, string]>;

const actions: Labels = {
  staff_password_authenticated: [
    "验证后台密码",
    "Verify staff password",
    "التحقق من كلمة مرور الموظف",
  ],
  staff_password_authenticated_rejected: [
    "后台密码验证失败",
    "Staff password verification failed",
    "فشل التحقق من كلمة مرور الموظف",
  ],
  staff_2fa_verified: [
    "登录后台",
    "Sign in to back office",
    "تسجيل الدخول إلى الإدارة",
  ],
  staff_2fa_verified_rejected: [
    "后台双重验证失败",
    "Staff 2FA failed",
    "فشلت المصادقة الثنائية للموظف",
  ],
  staff_password_changed: [
    "修改后台密码",
    "Change staff password",
    "تغيير كلمة مرور الموظف",
  ],
  staff_password_changed_rejected: [
    "修改后台密码失败",
    "Staff password change failed",
    "فشل تغيير كلمة مرور الموظف",
  ],
  staff_created: ["创建后台账号", "Create staff account", "إنشاء حساب موظف"],
  staff_updated: ["更新后台账号", "Update staff account", "تحديث حساب موظف"],
  staff_root_initialized: [
    "初始化超级管理员",
    "Initialize super administrator",
    "تهيئة المسؤول الأعلى",
  ],
  staff_2fa_reset: [
    "重置后台双重验证",
    "Reset staff 2FA",
    "إعادة تعيين المصادقة الثنائية للموظف",
  ],
  customer_enabled: [
    "启用账号",
    "Enable customer account",
    "تفعيل حساب العميل",
  ],
  customer_disabled: [
    "禁用账号",
    "Disable customer account",
    "تعطيل حساب العميل",
  ],
  customer_key_disabled: [
    "停用客户密钥",
    "Disable customer key",
    "تعطيل مفتاح العميل",
  ],
  customer_concurrency_updated: [
    "调整并发上限",
    "Change concurrency limit",
    "تغيير حد التزامن",
  ],
  customer_sessions_revoked: [
    "强制账号下线",
    "Revoke customer sessions",
    "إلغاء جلسات العميل",
  ],
  balance_gift: ["手动加款", "Add account credit", "إضافة رصيد للحساب"],
  balance_compensation: ["手动补偿", "Add compensation", "إضافة تعويض"],
  balance_offline_payment: [
    "登记线下付款",
    "Record offline payment",
    "تسجيل دفعة خارجية",
  ],
  balance_deduction: ["手动扣款", "Deduct account credit", "خصم رصيد الحساب"],
  pricing_changed: [
    "修改计价配置",
    "Change pricing configuration",
    "تغيير إعدادات التسعير",
  ],
  model_pricing_changed: [
    "价格变更",
    "Change model price",
    "تغيير سعر النموذج",
  ],
  model_reference_updated: [
    "更新官方参考价",
    "Update official reference prices",
    "تحديث الأسعار المرجعية الرسمية",
  ],
  service_group_multiplier_changed: [
    "修改服务分组倍率",
    "Change service group multiplier",
    "تغيير معامل مجموعة الخدمة",
  ],
  standard_model_saved: [
    "保存标准模型",
    "Save standard model",
    "حفظ النموذج القياسي",
  ],
  standard_model_deleted: [
    "删除标准模型",
    "Delete standard model",
    "حذف النموذج القياسي",
  ],
  deployment_profile_updated: [
    "更新地区档案",
    "Update region profile",
    "تحديث ملف المنطقة",
  ],
  legal_version_scheduled: [
    "设置法律文本生效版本",
    "Schedule legal document version",
    "جدولة إصدار الوثيقة القانونية",
  ],
  reconciliation_run: [
    "执行账务对账",
    "Run reconciliation",
    "تنفيذ المطابقة المالية",
  ],
};

const objects: Labels = {
  staff: ["后台账号", "Staff account", "حساب موظف"],
  customers: ["客户账号", "Customer account", "حساب عميل"],
  keys: ["访问密钥", "Access key", "مفتاح الوصول"],
  models: ["标准模型", "Standard model", "النموذج القياسي"],
  pricing: ["计价配置", "Pricing configuration", "إعدادات التسعير"],
  "service-group": ["服务分组", "Service group", "مجموعة الخدمة"],
  "region-profile": ["地区档案", "Region profile", "ملف المنطقة"],
  legal: ["法律文本", "Legal document", "وثيقة قانونية"],
  reconciliation: ["对账报告", "Reconciliation report", "تقرير المطابقة"],
};

const fields: Labels = {
  username: ["用户名", "Username", "اسم المستخدم"],
  platform_admin: ["平台管理员", "Platform administrator", "مسؤول المنصة"],
  role: ["角色", "Role", "الدور"],
  status: ["状态", "Status", "الحالة"],
  two_factor_bound: [
    "双重验证（2FA）",
    "Two-factor authentication",
    "المصادقة الثنائية",
  ],
  must_change_password: [
    "需修改密码",
    "Password change required",
    "يلزم تغيير كلمة المرور",
  ],
  concurrency_limit: ["并发上限", "Concurrency limit", "حد التزامن"],
  revoked: ["会话已撤销", "Sessions revoked", "أُلغيت الجلسات"],
  balance_nano: ["余额", "Balance", "الرصيد"],
  amount_nano: ["变动金额", "Adjustment amount", "مبلغ التعديل"],
  user_id: ["账号 ID", "Account ID", "معرّف الحساب"],
  reports: ["报告数量", "Report count", "عدد التقارير"],
  component: ["计价项", "Pricing component", "عنصر التسعير"],
  value: ["数值", "Value", "القيمة"],
  model: ["模型", "Model", "النموذج"],
  name: ["名称", "Name", "الاسم"],
  vendor: ["厂商", "Vendor", "المزوّد"],
  vendor_display: [
    "厂商显示名称",
    "Vendor display name",
    "اسم المزوّد المعروض",
  ],
  kind: ["类型", "Type", "النوع"],
  protocols: ["协议", "Protocols", "البروتوكولات"],
  capabilities: ["能力", "Capabilities", "الإمكانات"],
  tags: ["标签", "Tags", "الوسوم"],
  context_length: ["上下文长度", "Context length", "طول السياق"],
  max_output: [
    "最大输出词元",
    "Maximum output tokens",
    "الحد الأقصى لرموز الإخراج",
  ],
  description: ["说明", "Description", "الوصف"],
  listed: ["上架状态", "Listing status", "حالة الإدراج"],
  sort_weight: ["排序权重", "Sort weight", "وزن الترتيب"],
  revision: ["版本号", "Revision", "المراجعة"],
  version: ["版本", "Version", "الإصدار"],
  created_at: ["创建时间戳", "Creation timestamp", "الطابع الزمني للإنشاء"],
  updated_at: ["更新时间戳", "Update timestamp", "الطابع الزمني للتحديث"],
  effective_at: ["生效时间戳", "Effective timestamp", "الطابع الزمني للسريان"],
  reference: ["官方参考价", "Official reference", "المرجع الرسمي"],
  official_source_url: [
    "官方价格来源",
    "Official price source",
    "مصدر السعر الرسمي",
  ],
  official_verified_date: ["核验日期", "Verification date", "تاريخ التحقق"],
  official_conditions: [
    "参考价适用条件",
    "Reference price conditions",
    "شروط السعر المرجعي",
  ],
  official_historical: ["历史参考价", "Historical reference", "مرجع تاريخي"],
  official_input_per_m: [
    "官方输入价（美元/百万词元）",
    "Official input USD / million tokens",
    "سعر الإدخال الرسمي لكل مليون رمز بالدولار",
  ],
  official_output_per_m: [
    "官方输出价（美元/百万词元）",
    "Official output USD / million tokens",
    "سعر الإخراج الرسمي لكل مليون رمز بالدولار",
  ],
  official_cache_read_per_m: [
    "官方缓存读取价（美元/百万词元）",
    "Official cache read USD / million tokens",
    "سعر قراءة الذاكرة الرسمي لكل مليون رمز بالدولار",
  ],
  official_cache_write_per_m: [
    "官方缓存写入价（美元/百万词元）",
    "Official cache write USD / million tokens",
    "سعر كتابة الذاكرة الرسمي لكل مليون رمز بالدولار",
  ],
  official_per_request: [
    "官方单次价格（美元）",
    "Official price per request (USD)",
    "السعر الرسمي لكل طلب بالدولار",
  ],
  per_request_usd: [
    "单次价格（美元）",
    "Price per request (USD)",
    "السعر لكل طلب بالدولار",
  ],
  score: ["能力评分", "Capability score", "تقييم الإمكانات"],
  compatible_groups: ["兼容分组", "Compatible groups", "المجموعات المتوافقة"],
  region_id: ["地区 ID", "Region ID", "معرّف المنطقة"],
  brand: ["品牌", "Brand", "العلامة التجارية"],
  domain: ["域名", "Domain", "النطاق"],
  logo_url: ["标志地址", "Logo URL", "رابط الشعار"],
  default_language: ["默认语言", "Default language", "اللغة الافتراضية"],
  languages: ["支持语言", "Languages", "اللغات"],
  timezone: ["时区", "Timezone", "المنطقة الزمنية"],
  currency: ["币种", "Currency", "العملة"],
  allowed_vendors: ["允许的厂商", "Allowed vendors", "المزوّدون المسموحون"],
  login_methods: ["登录方式", "Login methods", "طرق تسجيل الدخول"],
  payment_channels: ["支付渠道", "Payment channels", "قنوات الدفع"],
  payment_config_refs: [
    "支付配置引用",
    "Payment configuration references",
    "مراجع إعدادات الدفع",
  ],
  cookie_consent: [
    "Cookie 同意提示",
    "Cookie consent",
    "الموافقة على ملفات الارتباط",
  ],
  features: ["功能开关", "Features", "الميزات"],
  store_request_content: [
    "保存请求内容",
    "Store request content",
    "حفظ محتوى الطلب",
  ],
  playground: ["调试台", "Playground", "ساحة التجربة"],
  models: ["模型广场", "Model catalog", "كتالوج النماذج"],
  smart_entry: ["智能入口", "Smart entry", "المدخل الذكي"],
  video: ["视频", "Video", "الفيديو"],
  arabic: ["阿拉伯语", "Arabic", "العربية"],
  model_ratio: ["输入计费系数", "Input billing ratio", "معامل فوترة الإدخال"],
  completion_ratio: [
    "输出计费系数",
    "Output billing ratio",
    "معامل فوترة الإخراج",
  ],
  group_ratio: ["售价倍率", "Sale multiplier", "معامل سعر البيع"],
  group_group_ratio: [
    "账号分组售价倍率",
    "Account group sale multiplier",
    "معامل البيع لمجموعة الحساب",
  ],
  model_price: [
    "按次计价（美元）",
    "Per-request price (USD)",
    "السعر لكل طلب بالدولار",
  ],
  model_price_expr: ["计价表达式", "Pricing expression", "صيغة التسعير"],
  cache_ratio: ["缓存计费系数", "Cache billing ratio", "معامل فوترة الذاكرة"],
  audio_ratio: ["音频计费系数", "Audio billing ratio", "معامل فوترة الصوت"],
  model_time_pricing: [
    "分时价格系数",
    "Time pricing factors",
    "معاملات التسعير الزمني",
  ],
  model_time_token_prices: [
    "分时词元价格",
    "Time token prices",
    "أسعار الرموز الزمنية",
  ],
};

function label(labels: Labels, key: string, text: AuditText): string {
  const value = labels[key];
  return Array.isArray(value) ? text(value[0], value[1], value[2]) : key;
}

export function auditActionLabel(action: string, text: AuditText): string {
  const translated = label(actions, action, text);
  return translated !== action
    ? translated
    : `${text("未识别操作", "Unrecognized action", "إجراء غير معروف")} (${action})`;
}

export function auditSearchActions(query: string, text: AuditText): string[] {
  const search = query.trim().toLocaleLowerCase();
  return search
    ? Object.keys(actions).filter((key) =>
        auditActionLabel(key, text).toLocaleLowerCase().includes(search),
      )
    : [];
}

export function auditObjectLabel(kind: string, text: AuditText): string {
  return label(objects, kind, text);
}

export function auditRoleLabel(role: number, text: AuditText): string {
  if (role === 0)
    return text("未记录角色", "Role not recorded", "الدور غير مسجل");
  if (role === 1) return text("客户", "Customer", "عميل");
  return [5, 10, 20, 100].includes(role)
    ? staffRoleName(role, text)
    : `${staffRoleName(role, text)} (${role})`;
}

function describe(value: unknown, text: AuditText, key = ""): string {
  if (value === null || value === undefined || value === "") return "—";
  if (Array.isArray(value))
    return value.length
      ? value.map((item) => describe(item, text, key)).join(" / ")
      : "—";
  if (typeof value === "object")
    return (
      Object.entries(value)
        .map(
          ([field, item]) =>
            `${label(fields, field, text)}: ${describe(item, text, field)}`,
        )
        .join("; ") || "—"
    );
  if (key === "role" && typeof value === "number")
    return auditRoleLabel(value, text);
  if (key === "status" && value === 1)
    return text("已启用", "Enabled", "مفعّل");
  if (key === "status" && value === 2)
    return text("已禁用", "Disabled", "معطّل");
  if (typeof value === "boolean") {
    if (key === "two_factor_bound")
      return value
        ? text("已绑定", "Bound", "مرتبط")
        : text("未绑定", "Not bound", "غير مرتبط");
    if (key === "listed")
      return value
        ? text("已上架", "Listed", "مدرج")
        : text("未上架", "Not listed", "غير مدرج");
    return value ? text("是", "Yes", "نعم") : text("否", "No", "لا");
  }
  if (
    (key === "balance_nano" || key === "amount_nano") &&
    /^-?\d+$/.test(String(value))
  )
    return isolateValue(formatFinanceAmount(String(value)), text);
  if (key === "component") return label(fields, String(value), text);
  return String(value);
}

function isolateValue(value: string, text: AuditText): string {
  return text("ltr", "ltr", "rtl") === "rtl" ? `\u2066${value}\u2069` : value;
}

function parse(value: string): unknown {
  try {
    return value ? JSON.parse(value) : undefined;
  } catch {
    return value;
  }
}

export function auditSummary(value: string, text: AuditText): string {
  return describe(parse(value), text);
}

type Entry = { path: string[]; value: unknown };
function flatten(value: unknown, path: string[] = []): Entry[] {
  if (
    value &&
    typeof value === "object" &&
    !Array.isArray(value) &&
    Object.keys(value).length
  ) {
    return Object.entries(value).flatMap(([key, item]) =>
      flatten(item, [...path, key]),
    );
  }
  return [{ path, value }];
}

export function auditChanges(
  before: string,
  after: string,
  text: AuditText,
): string[] {
  if (!before && !after) return [];
  const old = new Map(
    flatten(parse(before)).map((entry) => [JSON.stringify(entry.path), entry]),
  );
  const next = new Map(
    flatten(parse(after)).map((entry) => [JSON.stringify(entry.path), entry]),
  );
  const result: string[] = [];
  for (const path of new Set([...old.keys(), ...next.keys()])) {
    const previous = old.get(path),
      current = next.get(path);
    if (JSON.stringify(previous?.value) === JSON.stringify(current?.value))
      continue;
    const keys = (current ?? previous)!.path;
    const key = keys.at(-1) ?? "";
    let name = keys.map((part) => label(fields, part, text)).join(" · ");
    if (key === "value" && keys.length === 1) {
      const component =
        next.get('["component"]')?.value ?? old.get('["component"]')?.value;
      if (typeof component === "string") name = label(fields, component, text);
    }
    const value = !before
      ? describe(current?.value, text, key)
      : isolateValue(
          `${describe(previous?.value, text, key)} → ${describe(current?.value, text, key)}`,
          text,
        );
    result.push(name ? `${name}: ${value}` : value);
  }
  return result;
}

export function auditAuthResult(action: string, text: AuditText): string {
  switch (action) {
    case "staff_2fa_verified":
      return text(
        "双重验证（2FA）通过",
        "2FA verification passed",
        "نجحت المصادقة الثنائية",
      );
    case "staff_password_authenticated":
      return text(
        "密码验证通过，等待双重验证",
        "Password verified; awaiting 2FA",
        "تم التحقق من كلمة المرور؛ بانتظار المصادقة الثنائية",
      );
    case "staff_password_changed":
      return text("密码已修改", "Password changed", "تم تغيير كلمة المرور");
    case "staff_password_authenticated_rejected":
    case "staff_password_changed_rejected":
    case "staff_2fa_verified_rejected":
      return text("验证未通过", "Verification failed", "فشل التحقق");
    default:
      return "";
  }
}

export function auditHTTPAction(method: string, text: AuditText): string {
  const methods: Labels = {
    POST: ["提交操作", "Submit operation", "إرسال عملية"],
    PUT: ["修改配置", "Update configuration", "تحديث الإعدادات"],
    PATCH: ["修改配置", "Update configuration", "تحديث الإعدادات"],
    DELETE: ["删除操作", "Delete operation", "عملية حذف"],
    CHANGE: ["配置变更", "Configuration change", "تغيير الإعدادات"],
  };
  return label(methods, method, text);
}

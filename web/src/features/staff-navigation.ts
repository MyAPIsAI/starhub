import {
  BarChart3,
  Users,
  CircleDollarSign,
  Cpu,
  Layers3,
  Network,
  RefreshCw,
  Radio,
  Globe2,
  History,
  ShieldCheck,
  FileSearch,
  Puzzle,
  Ticket,
  Server,
} from "lucide-react";

export const staffNavigation = [
  {
    id: "analytics",
    group: "business",
    label: ["运营看板", "Operations dashboard", "لوحة العمليات"],
    Icon: BarChart3,
    roles: [100, 10, 5],
  },
  {
    id: "customers",
    group: "business",
    label: ["账号管理", "Customer accounts", "حسابات العملاء"],
    Icon: Users,
    roles: [100, 10, 5, 20],
  },
  {
    id: "finance",
    group: "business",
    label: ["账务对账", "Financial reconciliation", "المطابقة المالية"],
    Icon: CircleDollarSign,
    roles: [100, 5],
  },
  {
    id: "calls",
    group: "business",
    label: ["调用明细", "Call details", "تفاصيل الاستدعاءات"],
    Icon: FileSearch,
    roles: [100, 20],
  },
  {
    id: "models",
    group: "models",
    label: ["标准模型与价格", "Models and pricing", "النماذج والأسعار"],
    Icon: Cpu,
    roles: [100, 10],
  },
  {
    id: "groups",
    group: "models",
    label: ["服务分组", "Service groups", "مجموعات الخدمة"],
    Icon: Layers3,
    roles: [100, 10],
  },
  {
    id: "resources",
    group: "models",
    label: ["渠道与投放", "Channels and placement", "القنوات والتوزيع"],
    Icon: Network,
    roles: [100, 20],
  },
  {
    id: "sync",
    group: "operations",
    label: [
      "同步与路由状态",
      "Sync and routing status",
      "حالة المزامنة والتوجيه",
    ],
    Icon: RefreshCw,
    roles: [100, 20],
  },
  {
    id: "incidents",
    group: "operations",
    label: ["事件公告", "Incident notices", "إعلانات الحوادث"],
    Icon: Radio,
    roles: [100, 10],
  },
  {
    id: "regions",
    group: "system",
    label: ["地区档案", "Region profiles", "ملفات المناطق"],
    Icon: Globe2,
    roles: [100],
  },
  {
    id: "system",
    group: "system",
    label: ["系统信息", "System information", "معلومات النظام"],
    Icon: Server,
    roles: [100, 20],
  },
  {
    id: "staff",
    group: "system",
    label: ["后台账号", "Staff accounts", "حسابات الموظفين"],
    Icon: ShieldCheck,
    roles: [100],
  },
  {
    id: "audit",
    group: "system",
    label: ["审计日志", "Audit log", "سجل التدقيق"],
    Icon: History,
    roles: [100, 10, 5, 20],
  },
  {
    id: "plugins",
    group: "tools",
    label: ["视频插件", "Video plugins", "إضافات الفيديو"],
    Icon: Puzzle,
    roles: [100],
  },
  {
    id: "redemption",
    group: "tools",
    label: ["兑换码", "Redemption codes", "رموز الاسترداد"],
    Icon: Ticket,
    roles: [100],
  },
] as const;

export type StaffTab = (typeof staffNavigation)[number]["id"];
export const staffNavigationFor = (role: number) =>
  staffNavigation.filter((item) =>
    (item.roles as readonly number[]).includes(role),
  );
export const defaultStaffTab = (role: number): StaffTab =>
  role === 20 ? "sync" : role === 5 ? "finance" : "analytics";

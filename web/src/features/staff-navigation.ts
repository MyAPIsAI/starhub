import { message as translatedMessage } from "../i18n/messages";
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
  Server,
} from "lucide-react";

export const staffNavigation = [
  {
    id: "analytics",
    group: "business",
    label: translatedMessage("operations_dashboard_7d445d520b"),
    Icon: BarChart3,
    roles: [100, 10, 5],
  },
  {
    id: "customers",
    group: "business",
    label: translatedMessage("customer_accounts_86a2111562"),
    Icon: Users,
    roles: [100, 10, 5, 20],
  },
  {
    id: "finance",
    group: "business",
    label: translatedMessage("financial_reconciliation_a74dc189b2"),
    Icon: CircleDollarSign,
    roles: [100, 5],
  },
  {
    id: "calls",
    group: "business",
    label: translatedMessage("call_details_50e18ac31f"),
    Icon: FileSearch,
    roles: [100, 20],
  },
  {
    id: "models",
    group: "models",
    label: translatedMessage("models_and_pricing_fb0d21ec28"),
    Icon: Cpu,
    roles: [100, 10],
  },
  {
    id: "groups",
    group: "models",
    label: translatedMessage("service_groups_0b4d78d515"),
    Icon: Layers3,
    roles: [100, 10],
  },
  {
    id: "resources",
    group: "models",
    label: translatedMessage("channels_and_placement_1d2d89c7c1"),
    Icon: Network,
    roles: [100, 20],
  },
  {
    id: "sync",
    group: "operations",
    label: translatedMessage("sync_and_routing_status_b65492fd1f"),
    Icon: RefreshCw,
    roles: [100, 20],
  },
  {
    id: "observability",
    group: "operations",
    label: translatedMessage("nfr_title"),
    Icon: Radio,
    roles: [100, 20],
  },
  {
    id: "incidents",
    group: "operations",
    label: translatedMessage("incident_notices_04954219fa"),
    Icon: Radio,
    roles: [100, 10],
  },
  {
    id: "regions",
    group: "system",
    label: translatedMessage("region_profiles_344f3e08f8"),
    Icon: Globe2,
    roles: [100],
  },
  {
    id: "system",
    group: "system",
    label: translatedMessage("system_information_dc63accbb4"),
    Icon: Server,
    roles: [100, 20],
  },
  {
    id: "staff",
    group: "system",
    label: translatedMessage("staff_accounts_0df2bf0fa6"),
    Icon: ShieldCheck,
    roles: [100],
  },
  {
    id: "audit",
    group: "system",
    label: translatedMessage("audit_log_f73fcc381d"),
    Icon: History,
    roles: [100, 10, 5, 20],
  },
] as const;

export type StaffTab = (typeof staffNavigation)[number]["id"];
export const staffNavigationFor = (role: number) =>
  staffNavigation.filter((item) =>
    (item.roles as readonly number[]).includes(role),
  );
export const defaultStaffTab = (role: number): StaffTab =>
  role === 20 ? "sync" : role === 5 ? "finance" : "analytics";

import { useEffect, useRef, useState, type ReactNode } from "react";
import { Link } from "react-router-dom";
import * as Dialog from "@radix-ui/react-dialog";
import {
  Menu,
  X,
  ChevronDown,
  ChevronRight,
  SlidersHorizontal,
  BookOpen,
  Grid2X2,
  Activity,
} from "lucide-react";
import { ProductBrand, Preferences } from "./product-shell";
import { StaffLogout } from "../features/staff-management";
import { WorkspaceLinks } from "../features/workspace-navigation";
import {
  staffNavigationFor,
  defaultStaffTab,
  type StaffTab,
} from "../features/staff-navigation";
import { useStaffText, staffRoleName } from "../lib/api/staff";
import {
  useSystemAccess,
  sessionUsername,
  canManagePlatform,
} from "../lib/system-access";
import "./console-redesign.css";
import "./staff-workspace.css";

const groups = [
  { id: "business", label: ["经营", "Business", "الأعمال"] },
  {
    id: "models",
    label: ["模型与资源", "Models and resources", "النماذج والموارد"],
  },
  { id: "operations", label: ["运维", "Operations", "التشغيل"] },
  { id: "system", label: ["系统", "System", "النظام"] },
  {
    id: "tools",
    label: ["更多管理工具", "More management tools", "أدوات الإدارة الإضافية"],
  },
] as const;

export function StaffWorkspace({
  active,
  children,
}: {
  active: StaffTab;
  children: ReactNode;
}) {
  const text = useStaffText(),
    role = useSystemAccess((s) => s.role),
    session = useSystemAccess((s) => s.session);
  const username =
    sessionUsername(session) ?? text("内部员工", "Staff", "موظف");
  const platformAdmin = canManagePlatform({ role, session });
  const workspaceLabel = platformAdmin
    ? text("平台管理", "Platform management", "إدارة المنصة")
    : text("后台工作台", "Staff workspace", "مساحة الموظفين");
  const items = staffNavigationFor(role),
    current = items.find((item) => item.id === active);
  const [mobile, setMobile] = useState(
    () => window.matchMedia("(max-width: 900px)").matches,
  );
  const [open, setOpen] = useState(false);
  const navigation = useRef<HTMLElement>(null);
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({
    business: true,
    models: true,
    operations: true,
    system: true,
    tools: true,
  });
  useEffect(() => {
    const query = window.matchMedia("(max-width: 900px)");
    const change = () => {
      setMobile(query.matches);
      if (!query.matches) setOpen(false);
    };
    query.addEventListener("change", change);
    return () => query.removeEventListener("change", change);
  }, []);
  useEffect(() => {
    setOpen(false);
    const group = items.find((item) => item.id === active)?.group;
    if (group) setCollapsed((value) => ({ ...value, [group]: false }));
  }, [active, role]);
  useEffect(() => {
    const frame = requestAnimationFrame(() =>
      navigation.current
        ?.querySelector("[aria-current=page]")
        ?.scrollIntoView({ block: "nearest" }),
    );
    return () => cancelAnimationFrame(frame);
  }, [active, role, open, mobile]);
  const name = (label: readonly [string, string, string]) => text(...label);
  const sidebar = (drawer: boolean) => (
    <>
      <div className="staff-shell-brand">
        <ProductBrand compact />
        {drawer && (
          <Dialog.Close
            className="staff-icon-button"
            aria-label={text("关闭导航", "Close navigation", "إغلاق التنقل")}
          >
            <X size={18} />
          </Dialog.Close>
        )}
      </div>
      <nav
        ref={navigation}
        className="staff-shell-nav"
        aria-label={text(
          "平台管理导航",
          "Platform management navigation",
          "التنقل في إدارة المنصة",
        )}
      >
        {platformAdmin && (
          <details className="staff-shell-workspace-switch">
            <summary className="staff-shell-group-toggle">
              <span>{text("工作空间", "Workspace", "مساحة العمل")}</span>
              <ChevronDown size={14} />
            </summary>
            <WorkspaceLinks onNavigate={() => setOpen(false)} />
          </details>
        )}
        <div className="staff-shell-resources">
          <span className="staff-shell-label">
            {text("资源", "Resources", "الموارد")}
          </span>
          {[
            {
              to: "/models",
              label: ["模型广场", "Model plaza", "ساحة النماذج"] as const,
              Icon: Grid2X2,
            },
            {
              to: "/docs",
              label: ["开发文档", "Documentation", "الوثائق"] as const,
              Icon: BookOpen,
            },
            {
              to: "/status",
              label: ["服务状态", "Service status", "حالة الخدمة"] as const,
              Icon: Activity,
            },
          ].map(({ to, label, Icon }) => (
            <Link
              key={to}
              to={to}
              className="staff-shell-link"
              onClick={() => setOpen(false)}
            >
              <Icon size={17} />
              <span>{name(label)}</span>
            </Link>
          ))}
        </div>
        {platformAdmin && (
          <>
            <span className="staff-shell-label">{workspaceLabel}</span>
            <Link
              className="staff-shell-platform"
              data-platform-management
              to={`/admin?tab=${defaultStaffTab(role)}`}
              onClick={() => setOpen(false)}
            >
              <SlidersHorizontal size={17} />
              <span>{workspaceLabel}</span>
              <small>{text("管理员", "Administrator", "مسؤول")}</small>
            </Link>
          </>
        )}
        {groups.map((group) => {
          const entries = items.filter((item) => item.group === group.id);
          if (!entries.length) return null;
          const expanded = !collapsed[group.id];
          return (
            <section className="staff-shell-group" key={group.id}>
              <button
                className="staff-shell-group-toggle"
                aria-expanded={expanded}
                aria-controls={`staff-navigation-${group.id}`}
                onClick={() =>
                  setCollapsed((value) => ({ ...value, [group.id]: expanded }))
                }
              >
                <span>{name(group.label)}</span>
                <ChevronDown
                  size={14}
                  className={expanded ? "" : "is-collapsed"}
                />
              </button>
              <div id={`staff-navigation-${group.id}`} hidden={!expanded}>
                {entries.map(({ id, label, Icon }) => (
                  <Link
                    key={id}
                    to={`/admin?tab=${id}`}
                    data-staff-tab={id}
                    className={`staff-shell-link ${active === id ? "is-active" : ""}`}
                    aria-current={active === id ? "page" : undefined}
                    onClick={() => setOpen(false)}
                  >
                    <Icon size={17} />
                    <span>{name(label)}</span>
                  </Link>
                ))}
              </div>
            </section>
          );
        })}
      </nav>
      <div className="staff-shell-profile">
        <span className="staff-shell-avatar" aria-hidden="true">
          {username.slice(0, 2).toUpperCase()}
        </span>
        <div>
          <strong>{username}</strong>
          <small>
            {platformAdmin
              ? text("平台管理员", "Platform administrator", "مسؤول المنصة")
              : staffRoleName(role, text)}
          </small>
        </div>
      </div>
    </>
  );
  return (
    <Dialog.Root open={mobile && open} onOpenChange={setOpen}>
      <div className="xy-console staff-workspace">
        {!mobile && (
          <aside className="staff-shell-sidebar">{sidebar(false)}</aside>
        )}
        <div className="staff-shell-body">
          <header className="staff-shell-topbar">
            <div className="staff-shell-breadcrumb">
              {mobile && (
                <Dialog.Trigger asChild>
                  <button
                    className="staff-icon-button"
                    aria-label={text(
                      "打开导航",
                      "Open navigation",
                      "افتح التنقل",
                    )}
                  >
                    <Menu size={20} />
                  </button>
                </Dialog.Trigger>
              )}
              <span>{workspaceLabel}</span>
              <ChevronRight className="directional" size={14} />
              <strong>{current ? name(current.label) : ""}</strong>
            </div>
            <div className="staff-shell-toolbar">
              <span className="staff-role-badge">
                {staffRoleName(role, text)}
              </span>
              <Preferences />
              <StaffLogout />
            </div>
          </header>
          <main className="staff-shell-content" data-staff-page={active}>
            {children}
          </main>
        </div>
      </div>
      {mobile && (
        <Dialog.Portal>
          <Dialog.Overlay className="staff-navigation-shade" />
          <Dialog.Content className="xy-console staff-workspace staff-navigation-dialog">
            <Dialog.Title className="staff-visually-hidden">
              {text(
                "平台管理导航",
                "Platform management navigation",
                "التنقل في إدارة المنصة",
              )}
            </Dialog.Title>
            <Dialog.Description className="staff-visually-hidden">
              {text(
                "选择有权限访问的后台页面。",
                "Choose an authorized management page.",
                "اختر صفحة إدارة مصرحًا بها.",
              )}
            </Dialog.Description>
            {sidebar(true)}
          </Dialog.Content>
        </Dialog.Portal>
      )}
    </Dialog.Root>
  );
}

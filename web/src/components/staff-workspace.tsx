import { message as translatedMessage } from "../i18n/messages";

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
  { id: "business", label: translatedMessage("business_d01b2f330b") },
  {
    id: "models",
    label: translatedMessage("models_and_resources_b73ed9af15"),
  },
  { id: "operations", label: translatedMessage("operations_f9cd9ee8bf") },
  { id: "system", label: translatedMessage("system_b6cf77aa49") },
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
    sessionUsername(session) ?? text(...translatedMessage("staff_1777523a13"));
  const platformAdmin = canManagePlatform({ role, session });
  const workspaceLabel = platformAdmin
    ? text(...translatedMessage("platform_management_4b7a0c4faa"))
    : text(...translatedMessage("staff_workspace_1992abaa9a"));
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
            aria-label={text(
              ...translatedMessage("close_navigation_d80a778122"),
            )}
          >
            <X size={18} />
          </Dialog.Close>
        )}
      </div>
      <nav
        ref={navigation}
        className="staff-shell-nav"
        aria-label={text(
          ...translatedMessage("platform_management_navigation_146719958c"),
        )}
      >
        {platformAdmin && (
          <details className="staff-shell-workspace-switch">
            <summary className="staff-shell-group-toggle">
              <span>{text(...translatedMessage("workspace_05e85246b8"))}</span>
              <ChevronDown size={14} />
            </summary>
            <WorkspaceLinks onNavigate={() => setOpen(false)} />
          </details>
        )}
        <div className="staff-shell-resources">
          <span className="staff-shell-label">
            {text(...translatedMessage("resources_dd59219f36"))}
          </span>
          {[
            {
              to: "/models",
              label: translatedMessage("model_plaza_0921c9eaa2"),
              Icon: Grid2X2,
            },
            {
              to: "/docs",
              label: translatedMessage("documentation_fdf95da9be"),
              Icon: BookOpen,
            },
            {
              to: "/status",
              label: translatedMessage("service_status_b6209e14c8"),
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
              <small>
                {text(...translatedMessage("administrator_5ccdea960a"))}
              </small>
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
              ? text(...translatedMessage("platform_administrator_489c68ad11"))
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
                      ...translatedMessage("open_navigation_77b8073ea6"),
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
                ...translatedMessage(
                  "platform_management_navigation_146719958c",
                ),
              )}
            </Dialog.Title>
            <Dialog.Description className="staff-visually-hidden">
              {text(
                ...translatedMessage(
                  "choose_an_authorized_management_page_efefe90f09",
                ),
              )}
            </Dialog.Description>
            {sidebar(true)}
          </Dialog.Content>
        </Dialog.Portal>
      )}
    </Dialog.Root>
  );
}

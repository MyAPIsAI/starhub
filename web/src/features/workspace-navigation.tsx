import { message as translatedMessage } from "../i18n/messages";
import {
  Activity,
  BarChart3,
  BookOpen,
  Boxes,
  Download,
  KeyRound,
  LayoutDashboard,
  Settings,
  ShieldCheck,
  Terminal,
  Wallet,
} from "lucide-react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useStaffText } from "../lib/api/staff";

const groups = [
  {
    id: "console" as const,
    items: [
      { path: "", key: "pOverview", Icon: LayoutDashboard },
      { path: "models", key: "navModels", Icon: Boxes },
      { path: "keys", key: "pKeys", Icon: KeyRound },
      { path: "connect", key: "navConnect", Icon: BookOpen },
      { path: "playground", key: "pPlayground", Icon: Terminal },
    ],
  },
  {
    id: "quality" as const,
    items: [
      { path: "service-quality", key: "navServiceQuality", Icon: ShieldCheck },
      { path: "usage", key: "usageReportsTitle", Icon: BarChart3 },
      { path: "requests", key: "pUsage", Icon: Activity },
    ],
  },
  {
    id: "account" as const,
    items: [
      { path: "billing", key: "billingTitle", Icon: Wallet },
      { path: "exports", key: "exportsTitle", Icon: Download },
      { path: "settings", key: "settingsTitle", Icon: Settings },
    ],
  },
];

export const workspaceNavigation = groups.flatMap((group) => group.items);

function workspaceHref(path: string) {
  if (path === "models") return "/models";
  return path ? `/console/${path}` : "/console";
}

export function WorkspaceLinks({
  section,
  keysNearQuota = 0,
  onNavigate,
}: {
  section?: string;
  keysNearQuota?: number;
  onNavigate: () => void;
}) {
  const { t } = useTranslation();
  const text = useStaffText();
  const labels = {
    console: text(...translatedMessage("console_12470788db")),
    quality: t("navObservability"),
    account: text(...translatedMessage("account_7176d568ed")),
  };
  const inConsole = section !== undefined;
  return groups.map(({ id, items }) => (
    <div
      key={id}
      role="group"
      aria-label={labels[id]}
      className="workspace-nav-group"
    >
      {(inConsole || id !== "console") && (
        <span
          className={
            id === "console" ? "xy-nav-label" : "xy-nav-label xy-nav-divider"
          }
        >
          {labels[id]}
        </span>
      )}
      {items.map(({ path, key, Icon }) => {
        const active =
          section === path || (path === "" && section === "console");
        return (
          <Link
            key={path}
            to={workspaceHref(path)}
            onClick={onNavigate}
            className={
              inConsole
                ? `xy-nav-link${active ? " is-active" : ""}`
                : "staff-shell-link"
            }
            aria-current={active ? "page" : undefined}
          >
            <Icon size={inConsole ? 18 : 17} />
            <span>{t(key)}</span>
            {path === "keys" && keysNearQuota > 0 && (
              <span className="xy-nav-badge">{keysNearQuota}</span>
            )}
          </Link>
        );
      })}
    </div>
  ));
}

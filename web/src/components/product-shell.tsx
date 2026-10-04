import { useModelPricing } from "../lib/model-pricing";
import { modelCatalog } from "../lib/platform-catalog";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { Command } from "cmdk";
import * as Dialog from "@radix-ui/react-dialog";
import {
  ArrowRight,
  ArrowUpRight,
  Menu,
  Search,
  Moon,
  Sun,
  X,
} from "lucide-react";
import { Button } from "./ui/button";
import { Select } from "./ui/primitives";
import { Modal } from "./ui/overlay";
import { BrandMark } from "./brand";
import { usePreferences } from "../lib/preferences";
import { CodeExample } from "../features/code-example";
import { type Language } from "../config/placeholders";
import { useFocusReturn } from "./ui/focus-return";
import "./public-header.css";
import { useSystemAccess, isAdmin } from "../lib/system-access";

export function ProductBrand({ compact = false }: { compact?: boolean }) {
  const { t } = useTranslation();
  return (
    <Link to="/" className="product-brand" aria-label={t("pHome")}>
      <BrandMark />
      <span className="product-wordmark">{t("pBrand")}</span>
      {!compact && (
        <span className="product-brand-caption">{t("pPlatform")}</span>
      )}
    </Link>
  );
}
export function PublicHeader() {
  const { t } = useTranslation();
  const { pathname, hash } = useLocation();
  const language = usePreferences((s) => s.language);
  const [menuOpen, setMenuOpen] = useState(false);
  const menuId = useId();
  const headerRef = useRef<HTMLElement>(null);
  const navRef = useRef<HTMLElement>(null);
  const menuButton = useRef<HTMLButtonElement>(null);
  const home = pathname === "/" || pathname === "/preview/home";
  const text = (zh: string, en: string, ar: string) =>
    language === "zh" ? zh : language === "ar" ? ar : en;
  const links = [
    { to: "/", label: t("pHome"), active: home },
    {
      to: "/models",
      label: t("navModels"),
      active: pathname.startsWith("/models"),
    },
    { to: "/docs", label: t("navDocs"), active: pathname === "/docs" },
    { to: "/status", label: t("navStatus"), active: pathname === "/status" },
  ];
  useEffect(() => setMenuOpen(false), [pathname, hash]);
  useEffect(() => {
    if (!menuOpen) return;
    navRef.current?.querySelector<HTMLAnchorElement>("a")?.focus();
    const dismiss = (event: PointerEvent) => {
      if (
        event.target instanceof Node &&
        !headerRef.current?.contains(event.target)
      )
        setMenuOpen(false);
    };
    document.addEventListener("pointerdown", dismiss);
    return () => document.removeEventListener("pointerdown", dismiss);
  }, [menuOpen]);
  return (
    <header
      ref={headerRef}
      className="veyra-public-header"
      onKeyDown={(event) => {
        if (event.key === "Escape" && menuOpen) {
          setMenuOpen(false);
          menuButton.current?.focus();
        }
      }}
    >
      <div className="veyra-public-header-inner">
        <ProductBrand compact />
        <nav
          ref={navRef}
          id={menuId}
          className={`veyra-public-nav${menuOpen ? " is-open" : ""}`}
          aria-label={t("pNavigation")}
        >
          {links.map(({ to, label, active }) => (
            <Link
              key={to}
              to={to}
              aria-current={active ? "page" : undefined}
              onClick={() => setMenuOpen(false)}
            >
              {label}
            </Link>
          ))}
          <div className="veyra-public-mobile-actions">
            <Link to="/login" onClick={() => setMenuOpen(false)}>
              {t("navLogin")}
            </Link>
            <Link
              className="veyra-public-console"
              to="/console"
              onClick={() => setMenuOpen(false)}
            >
              {text("进入工作台", "Open console", "لوحة التحكم")}
              <ArrowUpRight size={15} className="directional" />
            </Link>
          </div>
        </nav>
        <div className="veyra-public-actions">
          <Preferences />
          <Link className="veyra-public-login" to="/login">
            {t("navLogin")}
          </Link>
          <Link
            className="veyra-public-console veyra-public-desktop-console"
            to="/console"
          >
            {text("进入工作台", "Open console", "لوحة التحكم")}
            <ArrowUpRight size={15} className="directional" />
          </Link>
          <button
            ref={menuButton}
            type="button"
            className="veyra-public-menu"
            aria-controls={menuId}
            aria-expanded={menuOpen}
            aria-label={
              menuOpen
                ? text("收起导航", "Close navigation", "إغلاق التنقل")
                : text("展开导航", "Open navigation", "فتح التنقل")
            }
            onClick={() => setMenuOpen((open) => !open)}
          >
            {menuOpen ? <X size={20} /> : <Menu size={20} />}
          </button>
        </div>
      </div>
    </header>
  );
}
export function Preferences() {
  const { t } = useTranslation();
  const { language, setLanguage, theme, setTheme } = usePreferences();
  return (
    <div className="product-preferences">
      <Select
        label={t("language")}
        value={language}
        onChange={(v) => setLanguage(v as Language)}
        options={[
          { value: "en", label: "English" },
          { value: "zh", label: "中文" },
          { value: "ar", label: "العربية" },
        ]}
      />
      <Button
        variant="ghost"
        size="icon"
        onClick={() => setTheme(theme === "light" ? "dark" : "light")}
        aria-label={t("theme")}
      >
        {theme === "light" ? <Moon /> : <Sun />}
      </Button>
    </div>
  );
}
export function PreviewNote({ controls }: { controls?: ReactNode }) {
  const { t } = useTranslation();
  return (
    <div className="preview-note">
      <span>
        <i />
        {t("pPreview")}
      </span>
      <div className="preview-actions">
        {controls}
        <Link to="/design-system">
          {t("pSystem")}
          <ArrowRight className="directional" size={13} />
        </Link>
      </div>
    </div>
  );
}
export function Quickstart({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <Modal
      size="wide"
      open={open}
      onOpenChange={onOpenChange}
      title={t("pQuickstartTitle")}
      description={t("pQuickstartBody")}
      footer={
        <Button variant="primary" asChild>
          <Link
            to="/preview/console/playground"
            onClick={() => onOpenChange(false)}
          >
            {t("pGoPlayground")}
            <ArrowRight className="directional" />
          </Link>
        </Button>
      }
    >
      <ol className="quickstart-steps">
        {["pGuideStep1", "pGuideStep2", "pGuideStep3"].map((k, i) => (
          <li key={k}>
            <span>{String(i + 1).padStart(2, "0")}</span>
            <b>{t(k)}</b>
          </li>
        ))}
      </ol>
      <CodeExample />
      <p className="product-fineprint">{t("pMockKey")}</p>
    </Modal>
  );
}
export function ProductSearch() {
  const { t } = useTranslation(),
    navigate = useNavigate();
  const modelIds = modelCatalog(useModelPricing((s) => s.config)).map(
    (m) => m.id,
  );
  const role = useSystemAccess((s) => s.role);
  const language = usePreferences((s) => s.language);
  const text = (zh: string, en: string, ar: string) =>
    language === "zh" ? zh : language === "ar" ? ar : en;
  const [open, setOpen] = useState(false);
  const focus = useFocusReturn();
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setOpen((v) => !v);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, []);
  const go = (path: string) => {
    setOpen(false);
    navigate(path);
  };
  return (
    <>
      <button
        className="product-search"
        aria-label={t("pSearch")}
        onClick={() => setOpen(true)}
      >
        <Search size={15} />
        <span>{t("pSearch")}</span>
        <kbd>⌘ K</kbd>
      </button>
      <Dialog.Root open={open} onOpenChange={setOpen}>
        <Dialog.Portal>
          <Dialog.Overlay className="overlay" />
          <Dialog.Content className="command-modal" {...focus}>
            <Dialog.Title className="sr-only">{t("pSearch")}</Dialog.Title>
            <Dialog.Description className="sr-only">
              {t("pSearch")}
            </Dialog.Description>
            <Command label={t("pSearch")}>
              <div className="command-input">
                <Search />
                <Command.Input
                  placeholder={t("pSearch")}
                  aria-label={t("pSearch")}
                />
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={t("close")}
                  onClick={() => setOpen(false)}
                >
                  <X />
                </Button>
              </div>
              <Command.List>
                <Command.Empty>{t("pSearchEmpty")}</Command.Empty>
                <Command.Group heading={t("pWorkspaceSection")}>
                  {[
                    ["pOverview", "/preview/console"],
                    ["navModels", "/models"],
                    ["navConnect", "/console/connect"],
                    ["navServiceQuality", "/console/service-quality"],

                    ["billingTitle", "/console/billing"],
                    ["pKeys", "/preview/console/keys"],
                    ["pPlayground", "/preview/console/playground"],
                    ["pUsage", "/preview/console/requests"],
                    ["pHome", "/preview/home"],
                  ].map(([k, path]) => (
                    <Command.Item key={k} onSelect={() => go(path)}>
                      {t(k)}
                      <ArrowRight className="directional" size={16} />
                    </Command.Item>
                  ))}
                </Command.Group>
                {isAdmin(role) && (
                  <Command.Group
                    heading={text(
                      "系统设置",
                      "System settings",
                      "إعدادات النظام",
                    )}
                  >
                    <Command.Item onSelect={() => go("/console/model-pricing")}>
                      {text("标准模型", "Standard models", "النماذج القياسية")}
                    </Command.Item>
                    <Command.Item onSelect={() => go("/console/channels")}>
                      {text("渠道管理", "Channels", "القنوات")}
                    </Command.Item>
                    <Command.Item
                      onSelect={() => go("/console/service-groups")}
                    >
                      {text("服务分组", "Service groups", "مجموعات الخدمة")}
                    </Command.Item>
                  </Command.Group>
                )}
                <Command.Group heading={t("pModels")}>
                  {modelIds.map((m) => (
                    <Command.Item
                      key={m}
                      onSelect={() => go(`/models/${encodeURIComponent(m)}`)}
                    >
                      {m}
                    </Command.Item>
                  ))}
                </Command.Group>
              </Command.List>
            </Command>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
    </>
  );
}

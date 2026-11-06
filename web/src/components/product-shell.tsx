import { message as translatedMessage } from "../i18n/messages";
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
import { useSessionRestore } from "../lib/session-restore";
import { CodeExample } from "../features/code-example";
import { type Language } from "../config/placeholders";
import { useFocusReturn } from "./ui/focus-return";
import "./public-header.css";
import {
  useSystemAccess,
  isAdmin,
  canReadFinance,
  sessionUsername,
} from "../lib/system-access";

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
  const { pathname, search, hash } = useLocation();
  const language = usePreferences((s) => s.language);
  const session = useSystemAccess((s) => s.session);
  const { status, retry } = useSessionRestore();
  const [menuOpen, setMenuOpen] = useState(false);
  const menuId = useId();
  const headerRef = useRef<HTMLElement>(null);
  const navRef = useRef<HTMLElement>(null);
  const menuButton = useRef<HTMLButtonElement>(null);
  const home = pathname === "/" || pathname === "/preview/home";
  const text = (zh: string, en: string, ar: string) =>
    language === "zh" ? zh : language === "ar" ? ar : en;
  const accountCenter = text(...translatedMessage("account_center_60e51179ff"));
  const authenticated = session.accountId !== null;
  const accountName = authenticated
    ? (sessionUsername(session) ?? accountCenter)
    : t("navLogin");
  const accountLabel = authenticated
    ? `${accountName} · ${accountCenter}`
    : accountName;
  const accountEntry = (mobile = false) => {
    const className = [
      !mobile && "veyra-public-login",
      status === "ready" && authenticated && "veyra-public-account",
    ]
      .filter(Boolean)
      .join(" ");
    if (status === "checking") {
      return (
        <span
          className={className}
          role="status"
          aria-label={text(
            ...translatedMessage("checking_your_session_893684fa5e"),
          )}
        >
          …
        </span>
      );
    }
    const unavailable = status === "unavailable";
    const label = unavailable
      ? text(
          ...translatedMessage("unable_to_check_your_session_retry_f8aba0ff9b"),
        )
      : accountLabel;
    return (
      <Link
        className={className}
        to={
          unavailable
            ? `${pathname}${search}${hash}`
            : authenticated
              ? "/console/settings"
              : "/login"
        }
        aria-label={label}
        title={label}
        onClick={(event) => {
          if (unavailable) {
            event.preventDefault();
            retry();
          } else if (mobile) setMenuOpen(false);
        }}
      >
        {unavailable
          ? text(...translatedMessage("retry_c16455021c"))
          : accountName}
      </Link>
    );
  };
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
            {accountEntry(true)}
            <Link
              className="veyra-public-console"
              to="/console"
              onClick={() => setMenuOpen(false)}
            >
              {text(...translatedMessage("open_console_9b01a2b8d8"))}
              <ArrowUpRight size={15} className="directional" />
            </Link>
          </div>
        </nav>
        <div className="veyra-public-actions">
          <Preferences />
          {accountEntry()}
          <Link
            className="veyra-public-console veyra-public-desktop-console"
            to="/console"
          >
            {text(...translatedMessage("open_console_9b01a2b8d8"))}
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
                ? text(...translatedMessage("close_navigation_f2254b385d"))
                : text(...translatedMessage("open_navigation_2da59c6a14"))
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
  const {
    language,
    languages,
    languageSaving,
    languageError,
    setLanguage,
    theme,
    setTheme,
  } = usePreferences();
  return (
    <div className="product-preferences">
      <Select
        label={t("language")}
        value={language}
        disabled={languageSaving}
        onChange={(v) => setLanguage(v as Language)}
        options={[
          { value: "en", label: "English" },
          { value: "zh", label: "中文" },
          { value: "ar", label: "العربية" },
        ].filter((option) =>
          languages.includes(option.value as "en" | "zh" | "ar"),
        )}
      />
      {languageError && <span role="alert">{t("languageSaveFailed")}</span>}
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
                      ...translatedMessage("system_settings_966a9e1755"),
                    )}
                  >
                    <Command.Item onSelect={() => go("/console/model-pricing")}>
                      {text(...translatedMessage("standard_models_d7f00fd7f0"))}
                    </Command.Item>
                    <Command.Item onSelect={() => go("/console/channels")}>
                      {text(...translatedMessage("channels_eee07fd951"))}
                    </Command.Item>
                    <Command.Item
                      onSelect={() => go("/console/service-groups")}
                    >
                      {text(...translatedMessage("service_groups_0b4d78d515"))}
                    </Command.Item>
                  </Command.Group>
                )}
                {canReadFinance(role) && (
                  <Command.Group
                    heading={text(...translatedMessage("finance_dfaa8d1e15"))}
                  >
                    <Command.Item onSelect={() => go("/finance")}>
                      {text(
                        ...translatedMessage("finance_overview_f47c7aaf02"),
                      )}
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

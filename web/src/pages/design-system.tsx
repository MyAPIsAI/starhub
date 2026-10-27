import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { Command } from "cmdk";
import * as DialogPrimitive from "@radix-ui/react-dialog";
import { motion } from "framer-motion";
import {
  ArrowUpRight,
  BookOpen,
  Braces,
  Check,
  ChevronRight,
  Command as CommandIcon,
  Copy,
  Layers3,
  Menu,
  Moon,
  PanelRightOpen,
  Search,
  SlidersHorizontal,
  Sun,
  Type,
  X,
  ShieldCheck,
  CircleHelp,
} from "lucide-react";
import { Button } from "../components/ui/button";
import { Badge, Segmented, Select, Status } from "../components/ui/primitives";
import { Drawer, ConfirmDialog } from "../components/ui/overlay";
import { useFocusReturn } from "../components/ui/focus-return";
import { BrandMark } from "../components/brand";
import { TierBadge } from "../components/tier-badge";
import { DataStateView } from "../components/ui/states";
import { FormSpecimens } from "../features/form-specimens";
import { CodeExample, copyText } from "../features/code-example";
import { usePreferences, type DataState } from "../lib/preferences";
import { formatMoney } from "../lib/money";
import { placeholders, type Language } from "../config/placeholders";
import { sampleRequests, type RequestRecord } from "../mocks/service";

const navigation = [
  { id: "foundations", label: "foundations", Icon: Layers3 },
  { id: "data", label: "dataDisplay", Icon: BookOpen },
  { id: "components", label: "components", Icon: SlidersHorizontal },
  { id: "developer", label: "codeTitle", Icon: Braces },
  { id: "patterns", label: "patterns", Icon: CircleHelp },
];

function SectionTitle({
  kicker,
  title,
  subtitle,
}: {
  kicker: string;
  title: string;
  subtitle: string;
}) {
  return (
    <div className="section-title">
      <p className="eyebrow">{kicker}</p>
      <h2>{title}</h2>
      <p>{subtitle}</p>
    </div>
  );
}

export default function DesignSystem() {
  const { t } = useTranslation();
  const { language, setLanguage, theme, setTheme, dataState, setDataState } =
    usePreferences();
  const [mobileOpen, setMobileOpen] = useState(false);
  const [active, setActive] = useState("foundations");
  const [selected, setSelected] = useState<RequestRecord | null>(null);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [commandOpen, setCommandOpen] = useState(false);
  const [demoOpen, setDemoOpen] = useState(false);
  const [pattern, setPattern] = useState("empty");
  const mobileFocus = useFocusReturn();
  const commandFocus = useFocusReturn();
  const chooseRecord = useCallback((record: RequestRecord) => {
    setSelected(record);
  }, []);
  const closeDrawer = (open: boolean) => {
    if (!open) {
      setSelected(null);
    }
  };
  const reset = () => {
    setDataState("ready");
    setConfirmOpen(false);
  };
  const navigate = (id: string) => {
    setActive(id);
    setMobileOpen(false);
    setCommandOpen(false);
    requestAnimationFrame(() =>
      document.getElementById(id)?.scrollIntoView({
        behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches
          ? "instant"
          : "smooth",
      }),
    );
  };
  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setCommandOpen((open) => !open);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, []);
  useEffect(() => {
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries)
          if (entry.isIntersecting) setActive(entry.target.id);
      },
      { rootMargin: "-15% 0px -60% 0px" },
    );
    navigation.forEach((item) => {
      const section = document.getElementById(item.id);
      if (section) observer.observe(section);
    });
    return () => observer.disconnect();
  }, []);

  const sideContent = (
    <>
      <a href="/design-system" className="brand">
        <BrandMark />
        <span>{t("brand")}</span>
        <span className="brand-divider" />
        <span className="brand-product">{t("workspace")}</span>
      </a>
      <div className="sidebar-section">
        <p className="nav-eyebrow">{t("onThisPage")}</p>
        <nav aria-label={t("navigationLabel")}>
          {navigation.map(({ id, label, Icon }) => (
            <a
              href={`#${id}`}
              key={id}
              className={`nav-item ${active === id ? "active" : ""}`}
              aria-current={active === id ? "location" : undefined}
              onClick={(e) => {
                e.preventDefault();
                navigate(id);
              }}
            >
              <Icon />
              <span>{t(label)}</span>
              {active === id && <span className="nav-indicator" />}
            </a>
          ))}
        </nav>
      </div>
      <div className="sidebar-bottom">
        <div className="sidebar-note">
          <span className="note-rule" />
          <p>{t("builtFor")}</p>
          <span>{t("typeBody")}</span>
        </div>
        <div className="sidebar-version">
          <span className="version-mark">
            <BrandMark />
          </span>
          <div>
            <b>{t("designSystem")}</b>
            <span>{t("version")}</span>
          </div>
        </div>
      </div>
    </>
  );
  return (
    <div className="app-shell">
      <a href="#main" className="skip-link">
        {t("skip")}
      </a>
      <aside className="sidebar">{sideContent}</aside>
      <DialogPrimitive.Root open={mobileOpen} onOpenChange={setMobileOpen}>
        <DialogPrimitive.Portal>
          <DialogPrimitive.Overlay className="overlay mobile-overlay" />
          <DialogPrimitive.Content className="mobile-sidebar" {...mobileFocus}>
            <DialogPrimitive.Title className="sr-only">
              {t("navigationLabel")}
            </DialogPrimitive.Title>
            <DialogPrimitive.Description className="sr-only">
              {t("sections")}
            </DialogPrimitive.Description>
            <DialogPrimitive.Close asChild>
              <Button
                variant="ghost"
                size="icon"
                className="mobile-close"
                aria-label={t("close")}
              >
                <X />
              </Button>
            </DialogPrimitive.Close>
            {sideContent}
          </DialogPrimitive.Content>
        </DialogPrimitive.Portal>
      </DialogPrimitive.Root>
      <div className="workspace">
        <header className="topbar">
          <div className="breadcrumbs">
            <Button
              variant="ghost"
              size="icon"
              className="mobile-menu"
              onClick={() => setMobileOpen(true)}
              aria-label={t("menu")}
            >
              <Menu />
            </Button>
            <span>{t("workspace")}</span>
            <ChevronRight className="directional" size={14} />
            <b>{t("overview")}</b>
          </div>
          <div className="header-actions">
            <button
              className="command-trigger"
              onClick={() => setCommandOpen(true)}
              aria-label={t("searchLabel")}
            >
              <Search size={15} />
              <span>{t("search")}</span>
              <kbd>⌘ K</kbd>
            </button>
            <Select
              value={language}
              onChange={(value) => setLanguage(value as Language)}
              label={t("language")}
              options={[
                { value: "en", label: t("languageEnglish") },
                { value: "zh", label: t("languageChinese") },
                { value: "ar", label: t("languageArabic") },
                { value: "ko", label: t("languageKorean") },
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
        </header>
        <main id="main" className="main-content">
          <div className="page-heading">
            <div>
              <div className="heading-meta">
                <Badge tone="info">{t("preview")}</Badge>
                <span>{t("version")}</span>
              </div>
              <h1>
                {t("designSystem")}
                <span className="title-dot">.</span>
              </h1>
              <p>{t("intro")}</p>
            </div>
            <Button asChild variant="primary">
              <Link to="/preview/home">
                {t("pReviewHome")}
                <ArrowUpRight />
              </Link>
            </Button>
            <Button onClick={() => chooseRecord(sampleRequests[0])}>
              <PanelRightOpen />
              {t("openDrawer")}
            </Button>
          </div>
          <section id="foundations" className="section foundation-section">
            <SectionTitle
              kicker={t("sectionIntro")}
              title={t("foundationTitle")}
              subtitle={t("foundationSubtitle")}
            />
            <div className="foundation-grid">
              <div className="palette-block">
                <div className="color-swatches">
                  {[
                    { id: "brand", name: "starBlue", use: "brandUse" },
                    { id: "success", name: "jade", use: "jadeUse" },
                    { id: "warning", name: "amber", use: "amberUse" },
                    { id: "danger", name: "red", use: "redUse" },
                    { id: "text", name: "slate", use: "slateUse" },
                  ].map((color) => (
                    <div className="color-swatch" key={color.id}>
                      <span
                        className="color-sample"
                        style={{ background: `var(--${color.id})` }}
                      />
                      <b>{t(color.name)}</b>
                      <small>{t(color.use)}</small>
                    </div>
                  ))}
                </div>
                <div className="neutral-row">
                  <div className="neutral-heading">
                    <span>{t("neutralScale")}</span>
                    <code>01—12</code>
                  </div>
                  <div className="neutral-scale" aria-label={t("neutralScale")}>
                    {Array.from({ length: 12 }, (_, i) => (
                      <span
                        key={i}
                        style={{ background: `var(--neutral-${i + 1})` }}
                        title={`--neutral-${i + 1}`}
                      />
                    ))}
                  </div>
                </div>
              </div>
              <div className="type-specimen">
                <div className="type-symbol" aria-hidden="true">
                  {language === "ar" ? "أب" : "Aa"}
                  <span>0123456789</span>
                </div>
                <h3>{t("typeBody")}</h3>
                <p>{t("typeNote")}</p>
                <div className="token-notes">
                  <span>{t("spacing")}</span>
                  <span>{t("radius")}</span>
                  <span>{t("motion")}</span>
                </div>
              </div>
            </div>
          </section>
          <section id="data" className="section">
            <SectionTitle
              kicker={t("sectionData")}
              title={t("activityTitle")}
              subtitle={t("activityNote")}
            />
            <p style={{ color: "var(--muted)", fontSize: "var(--text-sm)", padding: "var(--space-4) 0" }}>
              {t("activityNote")}
            </p>
          </section>
          <section id="components" className="section">
            <SectionTitle
              kicker={t("sectionActions")}
              title={t("actionsTitle")}
              subtitle={t("actionsSubtitle")}
            />
            <FormSpecimens onConfirm={() => setConfirmOpen(true)} />
          </section>
          <section id="developer" className="section">
            <SectionTitle
              kicker={t("sectionCode")}
              title={t("codeTitle")}
              subtitle={t("codeSubtitle")}
            />
            <div className="developer-grid">
              <CodeExample />
              <div className="precision-block">
                <h3>{t("moneyTitle")}</h3>
                <dl>
                  {[
                    { key: "balance", nano: "8880123456", mode: "balance" },
                    { key: "reportTotal", nano: "8880123456", mode: "total" },
                    { key: "callCost", nano: "8880123456", mode: "call" },
                    {
                      key: "fullPrecision",
                      nano: "8880123456",
                      mode: "detail",
                    },
                    { key: "smallAmount", nano: "296", mode: "call" },
                    { key: "zeroAmount", nano: "0", mode: "call" },
                  ].map((item) => (
                    <div key={item.key}>
                      <dt>{t(item.key)}</dt>
                      <dd className="mono numeric ltr">
                        {formatMoney(
                          item.nano,
                          item.mode as "balance" | "total" | "call" | "detail",
                        )}
                      </dd>
                    </div>
                  ))}
                </dl>
                <p>{t("moneyNote")}</p>
                <small>{t("callZeroHelp")}</small>
              </div>
            </div>
          </section>
          <section id="patterns" className="section">
            <SectionTitle
              kicker={t("sectionPatterns")}
              title={t("patternsTitle")}
              subtitle={t("patternsSubtitle")}
            />
            <div className="patterns-block">
              <div className="patterns-toolbar">
                <Segmented
                  value={pattern}
                  onChange={setPattern}
                  options={["loading", "empty", "error"].map((value) => ({
                    value,
                    label: t(`${value}Tab`),
                  }))}
                  label={t("stateLabel")}
                />
                <Button variant="ghost" onClick={() => setConfirmOpen(true)}>
                  {t("openModal")}
                  <ArrowUpRight className="directional" />
                </Button>
              </div>
              <DataStateView
                state={pattern as "loading" | "empty" | "error"}
                onReset={() => {
                  setPattern("loading");
                  setTimeout(() => setPattern("empty"), 650);
                }}
              />
            </div>
          </section>
          <footer className="page-footer">
            <span>{t("footer")}</span>
            <span>{t("footerNote")}</span>
          </footer>
        </main>
      </div>
      <div className="demo-controls">
        {demoOpen && (
          <motion.div
            className="demo-panel"
            initial={{ opacity: 0, y: 6 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.2 }}
          >
            <div className="demo-panel-title">
              <h3>{t("demoTitle")}</h3>
              <Button
                variant="ghost"
                size="icon"
                aria-label={t("close")}
                onClick={() => setDemoOpen(false)}
              >
                <X />
              </Button>
            </div>
            <p>{t("demoNote")}</p>
            <Select
              value={dataState}
              onChange={(value) => setDataState(value as DataState)}
              label={t("stateLabel")}
              options={["ready", "loading", "empty", "error"].map((value) => ({
                value,
                label: t(`${value}Tab`),
              }))}
            />
            <Button onClick={reset}>{t("reset")}</Button>
          </motion.div>
        )}
        <Button
          className="demo-trigger"
          size="small"
          onClick={() => setDemoOpen(!demoOpen)}
          aria-expanded={demoOpen}
        >
          <SlidersHorizontal />
          {t("demoTitle")}
        </Button>
      </div>
      <Drawer
        open={!!selected}
        onOpenChange={closeDrawer}
        title={t("drawerTitle")}
        description={t("drawerDescription")}
        footer={
          <>
            <p>{t("drawerHint")}</p>
            <Button onClick={() => closeDrawer(false)}>{t("close")}</Button>
          </>
        }
      >
        {selected && (
          <>
            <div className="drawer-callout">
              <p className="eyebrow">{t("metadataLabel")}</p>
              <h3 className="ltr">{selected.model}</h3>
              <div className="inline-controls">
                <Status
                  label={t(selected.status)}
                  tone={
                    selected.status === "success"
                      ? "success"
                      : selected.status === "failed"
                        ? "danger"
                        : "warning"
                  }
                />
                <TierBadge id={selected.tier} />
              </div>
            </div>
            <div className="trace-block">
              <span>{t("trace")}</span>
              <div>
                <code dir="ltr">{selected.trace_id}</code>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={t("copyTrace")}
                  onClick={() =>
                    void copyText(
                      selected.trace_id,
                      t("copied"),
                      t("copyFailed"),
                    )
                  }
                >
                  <Copy />
                </Button>
              </div>
            </div>
            <dl className="detail-list">
              {[
                ["vendor", selected.vendor],
                ["requestType", t("textRequest")],
                ["source", t("playground")],
                [
                  "time",
                  new Intl.DateTimeFormat(
                    language === "ar" ? "ar-AE-u-nu-latn" : "en-GB",
                    {
                      dateStyle: "medium",
                      timeStyle: "medium",
                      timeZone: placeholders.region.timeZone,
                    },
                  ).format(new Date(selected.time)) + " UTC+4",
                ],
                ["inputTokens", selected.input_tokens],
                ["outputTokens", selected.output_tokens],
                ["cacheTokens", selected.cache_tokens],
                ["ttft", `${selected.ttft_ms} ms`],
                ["duration", `${selected.duration_ms} ms`],
              ].map(([key, value]) => (
                <div key={key}>
                  <dt>{t(String(key))}</dt>
                  <dd>{value}</dd>
                </div>
              ))}
              <div>
                <dt>{t("fallback")}</dt>
                <dd>{selected.fallback ? <Check size={16} /> : "—"}</dd>
              </div>
              <div className="detail-cost">
                <dt>{t("billedCost")}</dt>
                <dd className="numeric ltr">
                  {formatMoney(selected.cost_nano, "detail")}
                </dd>
              </div>
            </dl>
            <h3 className="detail-subheading">{t("drawerPrice")}</h3>
            <dl className="detail-list">
              <div>
                <dt>{t("inputPrice")}</dt>
                <dd dir="ltr">
                  {formatMoney(selected.input_price_nano, "detail")}
                </dd>
              </div>
              <div>
                <dt>{t("outputPrice")}</dt>
                <dd dir="ltr">
                  {formatMoney(selected.output_price_nano, "detail")}
                </dd>
              </div>
            </dl>
            <div className="privacy-note">
              <ShieldCheck size={18} />
              <p>{t("privacy")}</p>
            </div>
          </>
        )}
      </Drawer>
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        onConfirm={reset}
      />
      <DialogPrimitive.Root open={commandOpen} onOpenChange={setCommandOpen}>
        <DialogPrimitive.Portal>
          <DialogPrimitive.Overlay className="overlay" />
          <DialogPrimitive.Content className="command-modal" {...commandFocus}>
            <DialogPrimitive.Title className="sr-only">
              {t("commandTitle")}
            </DialogPrimitive.Title>
            <DialogPrimitive.Description className="sr-only">
              {t("commandDescription")}
            </DialogPrimitive.Description>
            <Command label={t("commandTitle")}>
              <div className="command-input">
                <Search size={18} />
                <Command.Input placeholder={t("commandPlaceholder")} />
                <DialogPrimitive.Close asChild>
                  <Button variant="ghost" size="icon" aria-label={t("close")}>
                    <X />
                  </Button>
                </DialogPrimitive.Close>
              </div>
              <Command.List>
                <Command.Empty>{t("noCommands")}</Command.Empty>
                <Command.Group heading={t("sections")}>
                  {navigation.map(({ id, label, Icon }) => (
                    <Command.Item
                      key={id}
                      value={`${id} ${t(label)}`}
                      onSelect={() => navigate(id)}
                    >
                      <Icon size={17} />
                      <span>{t(label)}</span>
                      <ChevronRight className="directional" size={15} />
                    </Command.Item>
                  ))}
                </Command.Group>
                <Command.Group heading={t("matchingRequests")}>
                  {sampleRequests.slice(0, 5).map((record) => (
                    <Command.Item
                      key={record.trace_id}
                      value={`${record.model} ${record.trace_id}`}
                      onSelect={() => {
                        setCommandOpen(false);
                        setTimeout(() => chooseRecord(record), 250);
                      }}
                    >
                      <Type size={16} />
                      <span className="command-result">
                        <b dir="ltr">{record.model}</b>
                        <small dir="ltr">{record.trace_id}</small>
                      </span>
                    </Command.Item>
                  ))}
                </Command.Group>
              </Command.List>
              <div className="command-footer">
                <CommandIcon size={14} />
                <span>{t("commandDescription")}</span>
              </div>
            </Command>
          </DialogPrimitive.Content>
        </DialogPrimitive.Portal>
      </DialogPrimitive.Root>
    </div>
  );
}

import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { Link } from "react-router-dom";
import {
  ArrowDown,
  ArrowRight,
  ArrowUpRight,
  Code2,
  Pause,
  Play,
} from "lucide-react";
import type { Language } from "../config/placeholders";
import { writtenLanguage } from "../lib/localize";
import { BrandMark } from "./brand";
import "./home-portal.css";

const routes = [
  {
    name: "OpenAI",
    model: "gpt-4.1",
    protocol: "Chat Completions",
    symbol: "✳",
  },
  {
    name: "Claude",
    model: "claude-sonnet-4",
    protocol: "Anthropic Messages",
    symbol: "✺",
  },
  { name: "Gemini", model: "gemini-2.5-pro", protocol: "Gemini", symbol: "✦" },
];

const copy = {
  zh: {
    title: ["一把密钥，", "连接更多可能。"],
    intro:
      "从你的应用出发，让模型连接变得简单。\n统一接入、灵活选择，专注下一次创造。",
    start: "开始接入",
    demo: "体验演示",
    app: "你的应用",
    idea: "从一个想法开始",
    gateway: "统一模型网关",
    gateNote: "协议 · 授权 · 路由",
    choose: "选择模型协议",
    selected: "当前协议",
    note: "连接动效为示意，非实时请求",
    pause: "暂停动效",
    resume: "恢复动效",
    reduced: "已遵循减少动态效果偏好",
    scroll: "向下探索",
    content: "了解平台与模型",
    diagram: "应用经统一网关连接三种模型协议",
  },
  en: {
    title: ["One key.", "More possibilities."],
    intro:
      "From your application to your next great idea.\nOne gateway. Familiar protocols. Room to build.",
    start: "Start building",
    demo: "Try the demo",
    app: "Your application",
    idea: "It starts with an idea",
    gateway: "One model gateway",
    gateNote: "Protocols · Access · Routing",
    choose: "Choose a model protocol",
    selected: "Selected protocol",
    note: "Illustrative flow, not a live request",
    pause: "Pause motion",
    resume: "Resume motion",
    reduced: "Reduced motion enabled",
    scroll: "Scroll to explore",
    content: "Explore the platform and models",
    diagram:
      "An application connects to three model protocols through one gateway",
  },
  ar: {
    title: ["مفتاح واحد.", "إمكانيات أكثر."],
    intro:
      "من تطبيقك إلى فكرتك القادمة.\nبوابة واحدة وبروتوكولات مألوفة ومساحة للإبداع.",
    start: "ابدأ الاتصال",
    demo: "جرّب العرض",
    app: "تطبيقك",
    idea: "كل شيء يبدأ بفكرة",
    gateway: "بوابة نماذج موحدة",
    gateNote: "بروتوكولات · صلاحيات · توجيه",
    choose: "اختر بروتوكول النموذج",
    selected: "البروتوكول المختار",
    note: "رسم توضيحي وليس طلباً مباشراً",
    pause: "إيقاف الحركة",
    resume: "استئناف الحركة",
    reduced: "تقليل الحركة مفعّل",
    scroll: "مرر للاستكشاف",
    content: "استكشف المنصة والنماذج",
    diagram: "تطبيق يتصل بثلاثة بروتوكولات عبر بوابة واحدة",
  },
};

const clamp = (value: number) => Math.max(0, Math.min(1, value));
const smooth = (value: number) => {
  const x = clamp(value);
  return x * x * (3 - 2 * x);
};

function PortalPaths({
  selected,
  mobile = false,
}: {
  selected: number;
  mobile?: boolean;
}) {
  const id = useId().replace(/:/g, "");
  const input = mobile ? "M200 55V115" : "M270 180C365 180 420 180 510 180";
  const paths = mobile
    ? [70, 200, 330].map((x) => `M200 210C200 242 ${x} 234 ${x} 267`)
    : [75, 180, 285].map((y) => `M690 180C810 180 810 ${y} 905 ${y}`);
  return (
    <svg
      className={`portal-paths${mobile ? " portal-paths-mobile" : ""}`}
      viewBox={mobile ? "0 0 400 330" : "0 0 1200 360"}
      preserveAspectRatio="none"
      fill="none"
      aria-hidden="true"
    >
      <defs>
        <linearGradient
          id={id}
          gradientUnits="userSpaceOnUse"
          x1="0"
          y1="0"
          x2={mobile ? 400 : 1200}
          y2="0"
        >
          <stop stopColor="#327aff" stopOpacity=".2" />
          <stop offset=".55" stopColor="#69d7ff" />
          <stop offset="1" stopColor="#d3f5ff" />
        </linearGradient>
      </defs>
      <path d={input} className="portal-path-input" stroke={`url(#${id})`} />
      {paths.map((path, index) => (
        <g key={path} className={selected === index ? "is-selected" : ""}>
          <path d={path} className="portal-path-track" />
          <path d={path} className="portal-path-glow" stroke={`url(#${id})`} />
          <path d={path} className="portal-path-flow" pathLength="1" />
        </g>
      ))}
      <path
        d={input}
        className="portal-path-flow portal-input-flow"
        pathLength="1"
      />
    </svg>
  );
}

export function HomePortal({
  language,
  onDemo,
  children,
}: {
  language: Language;
  onDemo: () => void;
  children: ReactNode;
}) {
  const c = copy[writtenLanguage(language)];
  const root = useRef<HTMLDivElement>(null);
  const scene = useRef<HTMLElement>(null);
  const sticky = useRef<HTMLDivElement>(null);
  const visual = useRef<HTMLDivElement>(null);
  const content = useRef<HTMLDivElement>(null);
  const [selected, setSelected] = useState(0);
  const [paused, setPaused] = useState(false);
  const [reduced, setReduced] = useState(
    () => window.matchMedia("(prefers-reduced-motion: reduce)").matches,
  );
  const [visible, setVisible] = useState(false);
  const [pageVisible, setPageVisible] = useState(() => !document.hidden);
  const [collapsed, setCollapsed] = useState(false);
  const [hovered, setHovered] = useState(false);
  const [focused, setFocused] = useState(false);
  const [announcement, setAnnouncement] = useState("");
  const quiet = reduced || paused;
  const running = !quiet && visible && pageVisible && !collapsed;

  useEffect(() => {
    const media = window.matchMedia("(prefers-reduced-motion: reduce)");
    const onChange = () => setReduced(media.matches);
    const onVisibility = () => setPageVisible(!document.hidden);
    media.addEventListener("change", onChange);
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      media.removeEventListener("change", onChange);
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, []);

  useEffect(() => {
    if (!sticky.current) return;
    const observer = new IntersectionObserver(([entry]) =>
      setVisible(entry.isIntersecting),
    );
    observer.observe(sticky.current);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    if (!running || hovered || focused) return;
    const timeout = window.setTimeout(
      () => setSelected((value) => (value + 1) % routes.length),
      4800,
    );
    return () => window.clearTimeout(timeout);
  }, [selected, running, hovered, focused]);

  useEffect(() => {
    const host = root.current;
    const section = scene.current;
    const screen = sticky.current;
    const hero = visual.current;
    if (!host || !section || !screen || !hero) return;
    let frame = 0;
    const header = host
      .closest(".xy-home")
      ?.querySelector(".veyra-public-header");
    const update = () => {
      frame = 0;
      const height = screen.offsetHeight;
      const travel = Math.max(0, section.offsetHeight - height);
      const progress =
        quiet || travel < 1
          ? 0
          : clamp(-section.getBoundingClientRect().top / travel);
      const eased = smooth(progress);
      host.style.setProperty("--hero-scale", String(1 - eased * 0.12));
      host.style.setProperty("--hero-shift", `${-height * 0.065 * eased}px`);
      host.style.setProperty(
        "--hero-opacity",
        String(1 - smooth(progress / 0.8)),
      );
      host.style.setProperty("--hero-radius", `${32 * eased}px`);
      host.style.setProperty(
        "--content-shift",
        `${travel * (progress - eased)}px`,
      );
      host.style.setProperty("--content-radius", `${28 * (1 - eased)}px`);
      host.dataset.heroProgress = progress.toFixed(4);
      const hidden = !quiet && progress >= 0.8;
      if (hidden && hero.contains(document.activeElement))
        content.current?.focus({ preventScroll: true });
      hero.inert = hidden;
      hero.setAttribute("aria-hidden", String(hidden));
      host.dataset.heroHidden = String(hidden);
      setCollapsed(hidden);
    };
    const schedule = () => {
      if (!frame) frame = window.requestAnimationFrame(update);
    };
    const resize = new ResizeObserver(() => {
      host.style.setProperty(
        "--portal-header",
        `${header?.getBoundingClientRect().height ?? 0}px`,
      );
      schedule();
    });
    resize.observe(section);
    resize.observe(screen);
    if (header) resize.observe(header);
    window.addEventListener("scroll", schedule, { passive: true });
    window.addEventListener("resize", schedule, { passive: true });
    schedule();
    return () => {
      window.cancelAnimationFrame(frame);
      window.removeEventListener("scroll", schedule);
      window.removeEventListener("resize", schedule);
      resize.disconnect();
    };
  }, [quiet]);

  const explore = () => {
    if (!scene.current || !sticky.current || !content.current) return;
    const travel = Math.max(
      0,
      scene.current.offsetHeight - sticky.current.offsetHeight,
    );
    const top =
      travel > 0
        ? scene.current.getBoundingClientRect().top + window.scrollY + travel
        : content.current.getBoundingClientRect().top + window.scrollY;
    content.current.focus({ preventScroll: true });
    window.scrollTo({ top, behavior: quiet ? "instant" : "smooth" });
  };

  return (
    <div
      ref={root}
      className="home-portal"
      data-experience={quiet ? "quiet" : "full"}
      data-motion={running ? "running" : "paused"}
    >
      <section
        ref={scene}
        className="portal-scene"
        aria-labelledby="portal-title"
      >
        <div ref={sticky} className="portal-sticky">
          <div ref={visual} className="portal-visual">
            <div className="portal-hero">
              <div className="portal-atmosphere" aria-hidden="true" />
              <div className="portal-floor" aria-hidden="true" />
              <div className="portal-copy">
                <span className="portal-eyebrow" dir="ltr">
                  <i />
                  myapis.ai
                  <ArrowUpRight size={12} />
                </span>
                <h1 id="portal-title">
                  {c.title[0]}
                  <br />
                  <span>{c.title[1]}</span>
                </h1>
                <p>{c.intro}</p>
                <div className="portal-actions">
                  <Link className="xy-btn xy-btn-blue" to="/console/connect">
                    {c.start}
                    <ArrowUpRight size={17} />
                  </Link>
                  <button className="portal-demo" onClick={onDemo}>
                    {c.demo}
                    <ArrowRight size={16} />
                  </button>
                </div>
              </div>
              <div
                className="portal-stage"
                role="group"
                aria-label={c.diagram}
                dir="ltr"
              >
                <PortalPaths selected={selected} />
                <PortalPaths selected={selected} mobile />
                <div className="portal-request">
                  <Code2 size={24} />
                  <div dir="auto">
                    <strong>{c.app}</strong>
                    <small>{c.idea}</small>
                  </div>
                  <i />
                </div>
                <div className="portal-gate-wrap">
                  <div className="portal-gate-back" aria-hidden="true" />
                  <div className="portal-gate">
                    <div className="portal-gate-orbit" aria-hidden="true" />
                    <BrandMark />
                    <strong translate="no">MyAPIs</strong>
                    <small translate="no">by Shine AI</small>
                    <span dir="auto">{c.gateway}</span>
                    <p dir="auto">{c.gateNote}</p>
                  </div>
                </div>
                <div
                  className="portal-models"
                  role="group"
                  aria-label={c.choose}
                  onPointerEnter={(event) => {
                    if (event.pointerType !== "touch") setHovered(true);
                  }}
                  onPointerLeave={() => setHovered(false)}
                  onFocusCapture={() => setFocused(true)}
                  onBlurCapture={(event) => {
                    if (!event.currentTarget.contains(event.relatedTarget))
                      setFocused(false);
                  }}
                >
                  {routes.map((route, index) => (
                    <button
                      key={route.name}
                      className={`portal-model${selected === index ? " is-selected" : ""}`}
                      aria-pressed={selected === index}
                      aria-label={`${route.name} · ${route.protocol}`}
                      onClick={() => {
                        setSelected(index);
                        setAnnouncement(`${c.selected}: ${route.name}`);
                      }}
                    >
                      <span className="portal-vendor" aria-hidden="true">
                        {route.symbol}
                      </span>
                      <span>
                        <strong>{route.name}</strong>
                        <code>{route.model}</code>
                      </span>
                      <ArrowUpRight size={14} aria-hidden="true" />
                      <i aria-hidden="true" />
                    </button>
                  ))}
                </div>
              </div>
              <div className="portal-caption">
                <span>
                  {c.selected}
                  <b dir="ltr">{routes[selected].protocol}</b>
                </span>
                <span className="portal-demo-note">{c.note}</span>
                <span className="sr-only" aria-live="polite">
                  {announcement}
                </span>
                <button
                  className="portal-motion"
                  aria-pressed={paused}
                  disabled={reduced}
                  onClick={() => setPaused((value) => !value)}
                >
                  {paused || reduced ? <Play size={12} /> : <Pause size={12} />}
                  {reduced ? c.reduced : paused ? c.resume : c.pause}
                </button>
              </div>
              <button className="portal-scroll-cue" onClick={explore}>
                {c.scroll}
                <span>
                  <ArrowDown size={13} />
                </span>
              </button>
            </div>
          </div>
        </div>
      </section>
      <div
        ref={content}
        className="home-portal-content"
        tabIndex={-1}
        aria-label={c.content}
      >
        {children}
      </div>
    </div>
  );
}

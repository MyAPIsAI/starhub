import { useEffect, useId, useLayoutEffect, useRef, useState } from "react";
import { Check, Info, Layers3, Pin, X } from "lucide-react";
import {
  getModelsForTier,
  getPricingSnapshot,
  useModelPricing,
} from "../lib/model-pricing";
import { type TierId } from "../config/placeholders";
import { usePreferences } from "../lib/preferences";
import { writtenLanguage } from "../lib/localize";
import { serviceGroupLabel } from "../lib/service-group-label";
import "./tier-model-picker.css";
import { useProductPreview } from "../lib/product-preview";
import { visibleGroups } from "../lib/platform-catalog";

export const getTierModels = (tier: TierId, config = getPricingSnapshot()) =>
  getModelsForTier(tier, config);

const copy = {
  zh: {
    title: "服务分组",
    hint: "悬停或聚焦分组查看模型；点击信息按钮可固定预览。",
    models: (count: number) => `${count} 个可用模型`,
    multiplier: "价格系数",
    selected: "已选择",
    preview: "仅预览 · 未选择",
    pinned: "预览已固定",
    close: "关闭模型预览",
    pin: (name: string) => `固定查看 ${name} 的模型`,
    unpin: (name: string) => `取消固定 ${name} 的模型预览`,
    available: "分组内模型",
    empty: "此分组暂无可用模型，请选择其他分组。",
    adjusted: (from: string, to: string) =>
      `${from} 不在此分组内，指定模型已自动改为 ${to}。`,
  },
  en: {
    title: "Service group",
    hint: "Hover or focus a group to explore models. Use the info button to pin its preview.",
    models: (count: number) => `${count} available models`,
    multiplier: "Price multiplier",
    selected: "Selected",
    preview: "Preview only · not selected",
    pinned: "Preview pinned",
    close: "Close model preview",
    pin: (name: string) => `Pin models in ${name}`,
    unpin: (name: string) => `Unpin model preview for ${name}`,
    available: "Models in this group",
    empty: "No models are available in this group. Choose another group.",
    adjusted: (from: string, to: string) =>
      `${from} is not in this group. The fixed model has been changed to ${to}.`,
  },
  ar: {
    title: "مجموعة الخدمة",
    hint: "مرر المؤشر أو ركّز على مجموعة لعرض نماذجها. استخدم زر المعلومات لتثبيت المعاينة.",
    models: (count: number) => `${count} نماذج متاحة`,
    multiplier: "معامل السعر",
    selected: "محددة",
    preview: "معاينة فقط · غير محددة",
    pinned: "المعاينة مثبتة",
    close: "إغلاق معاينة النماذج",
    pin: (name: string) => `تثبيت نماذج ${name}`,
    unpin: (name: string) => `إلغاء تثبيت معاينة نماذج ${name}`,
    available: "نماذج هذه المجموعة",
    empty: "لا توجد نماذج متاحة في هذه المجموعة. اختر مجموعة أخرى.",
    adjusted: (from: string, to: string) =>
      `${from} غير متاح في هذه المجموعة. تم تغيير النموذج المحدد إلى ${to}.`,
  },
};

export function TierModelPicker({
  value,
  onChange,
  adjustment,
}: {
  value: TierId;
  onChange: (tier: TierId) => void;
  adjustment?: { from: string; to: string } | null;
}) {
  const language = usePreferences((state) => state.language);
  const pricingConfig = useModelPricing((state) => state.config);
  const accountId = useProductPreview((s) => s.user.id);
  const groups = visibleGroups(pricingConfig, accountId);
  const text = copy[writtenLanguage(language)];
  const tierName = (tier: (typeof groups)[number]) =>
    serviceGroupLabel(tier.id, language, pricingConfig);
  const id = useId();
  const pickerElement = useRef<HTMLFieldSetElement>(null);
  const [previewLayout, setPreviewLayout] = useState({
    above: false,
    maxHeight: 280,
  });
  const [hovered, setHovered] = useState<TierId | null>(null);
  const [focused, setFocused] = useState<TierId | null>(null);
  const [pinned, setPinned] = useState<TierId | null>(null);
  const preview = pinned ?? hovered ?? focused;
  const previewTier = groups.find((tier) => tier.id === preview);
  const models = preview ? getTierModels(preview, pricingConfig) : [];
  const leaveTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const previewElement = useRef<HTMLElement>(null);
  const triggerElement = useRef<HTMLElement | null>(null);
  const skipRestoredFocus = useRef(false);

  const keepPreview = () => {
    if (leaveTimer.current) clearTimeout(leaveTimer.current);
    leaveTimer.current = null;
  };
  const dismissPreview = () => {
    keepPreview();
    const restoreFocus = previewElement.current?.contains(
      document.activeElement,
    );
    setPinned(null);
    setHovered(null);
    setFocused(null);
    if (restoreFocus && triggerElement.current?.isConnected) {
      skipRestoredFocus.current = true;
      triggerElement.current.focus({ preventScroll: true });
    }
  };
  useEffect(
    () => () => {
      if (leaveTimer.current) clearTimeout(leaveTimer.current);
    },
    [],
  );
  useEffect(() => {
    if (!preview) return;
    // Window capture runs before the parent Radix modal's document handler.
    // The first Escape closes this non-modal preview, keeping the task open.
    const onEscape = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      event.stopPropagation();
      dismissPreview();
    };
    window.addEventListener("keydown", onEscape, true);
    return () => window.removeEventListener("keydown", onEscape, true);
  }, [preview]);

  useLayoutEffect(() => {
    if (!preview) return;
    const picker = pickerElement.current;
    const body = picker?.closest<HTMLElement>("[data-overlay-body]");
    if (!picker) return;
    const position = () => {
      const anchor = picker.getBoundingClientRect();
      const bounds = body?.getBoundingClientRect();
      const start = Math.max(8, (bounds?.top ?? 0) + 8);
      const end = Math.min(
        window.innerHeight - 8,
        (bounds?.bottom ?? window.innerHeight) - 8,
      );
      const below = Math.max(0, end - anchor.bottom - 8);
      const above = Math.max(0, anchor.top - start - 8);
      const placeAbove = below < 170 && above > below;
      setPreviewLayout({
        above: placeAbove,
        maxHeight: Math.max(0, Math.min(280, placeAbove ? above : below)),
      });
    };
    position();
    const resize = new ResizeObserver(position);
    resize.observe(picker);
    if (body) resize.observe(body);
    body?.addEventListener("scroll", position, { passive: true });
    window.addEventListener("resize", position);
    return () => {
      resize.disconnect();
      body?.removeEventListener("scroll", position);
      window.removeEventListener("resize", position);
    };
  }, [preview]);

  return (
    <fieldset
      ref={pickerElement}
      className="tier-model-picker"
      data-testid="tier-model-picker"
      onPointerEnter={keepPreview}
      onPointerLeave={() => {
        keepPreview();
        leaveTimer.current = setTimeout(() => setHovered(null), 160);
      }}
      onBlurCapture={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget as Node | null))
          setFocused(null);
      }}
    >
      <legend>{text.title}</legend>
      <p className="tier-picker-hint" id={`${id}-hint`}>
        {text.hint}
      </p>
      <div className="tier-picker-grid">
        {groups.map((tier) => {
          const count = getTierModels(tier.id, pricingConfig).length;
          const selected = value === tier.id;
          const isPinned = pinned === tier.id;
          return (
            <div
              className={`tier-picker-card${selected ? " is-selected" : ""}${preview === tier.id ? " is-previewed" : ""}`}
              key={tier.id}
              data-tier={tier.id}
              onPointerEnter={(event) => {
                if (event.pointerType !== "touch") setHovered(tier.id);
              }}
              onFocusCapture={(event) => {
                triggerElement.current = event.target as HTMLElement;
                if (skipRestoredFocus.current) {
                  skipRestoredFocus.current = false;
                  return;
                }
                setHovered(null);
                setFocused(tier.id);
              }}
            >
              <label className="tier-picker-choice">
                <input
                  type="radio"
                  name="tier"
                  value={tier.id}
                  checked={selected}
                  disabled={
                    !count || pricingConfig.groups?.[tier.id]?.enabled === false
                  }
                  aria-label={tierName(tier)}
                  aria-describedby={`${id}-hint`}
                  onClick={() => setPinned(null)}
                  onChange={() => {
                    setPinned(null);
                    onChange(tier.id);
                  }}
                />
                <span className="tier-picker-name">{tierName(tier)}</span>
                <span className="tier-picker-count">{text.models(count)}</span>
                <span className="tier-picker-price">
                  {text.multiplier}{" "}
                  <bdi>
                    {Number(pricingConfig.multipliers[tier.id]).toString()}×
                  </bdi>
                </span>
              </label>
              <button
                type="button"
                className="tier-picker-info"
                aria-label={
                  isPinned
                    ? text.unpin(tierName(tier))
                    : text.pin(tierName(tier))
                }
                aria-pressed={isPinned}
                aria-controls={
                  preview === tier.id ? `${id}-preview` : undefined
                }
                aria-expanded={preview === tier.id}
                onClick={(event) => {
                  triggerElement.current = event.currentTarget;
                  if (isPinned) dismissPreview();
                  else setPinned(tier.id);
                }}
              >
                {isPinned ? <Pin size={14} /> : <Info size={15} />}
              </button>
            </div>
          );
        })}
      </div>

      {/* Bound the full-width preview to the modal body; only its model list scrolls. */}
      {preview && previewTier && (
        <section
          ref={previewElement}
          tabIndex={0}
          onPointerEnter={keepPreview}
          className="tier-picker-preview"
          style={{
            maxBlockSize: previewLayout.maxHeight,
            insetBlockStart: previewLayout.above ? "auto" : "calc(100% + 8px)",
            insetBlockEnd: previewLayout.above ? "calc(100% + 8px)" : "auto",
            visibility: previewLayout.maxHeight < 96 ? "hidden" : "visible",
          }}
          id={`${id}-preview`}
          aria-labelledby={`${id}-preview-title`}
          data-testid="tier-model-preview"
          data-preview-tier={preview}
          data-pinned={pinned !== null}
        >
          <div className="tier-preview-heading">
            <span className="tier-preview-symbol">
              <Layers3 size={17} />
            </span>
            <div>
              <h4 id={`${id}-preview-title`}>{tierName(previewTier)}</h4>
              <span>{text.models(models.length)}</span>
            </div>
            <span
              className={`tier-preview-status${preview === value ? " is-selected" : ""}`}
            >
              {pinned && <Pin size={12} aria-label={text.pinned} />}
              {preview === value && <Check size={12} />}
              {preview === value ? text.selected : text.preview}
            </span>
            <button
              type="button"
              className="tier-preview-close"
              onClick={dismissPreview}
              aria-label={text.close}
            >
              <X size={15} />
            </button>
          </div>
          <ul className="tier-preview-models" aria-label={text.available}>
            {models.map((model) => (
              <li key={model.id} data-model-id={model.id}>
                <span className="tier-model-vendor-mark" aria-hidden="true">
                  {model.vendor.slice(0, 1)}
                </span>
                <span className="tier-model-name">
                  <bdi>{model.name}</bdi>
                </span>
                <span className="tier-model-vendor">
                  <bdi>{model.vendor}</bdi>
                </span>
              </li>
            ))}
            {!models.length && (
              <li className="tier-model-empty">{text.empty}</li>
            )}
          </ul>
        </section>
      )}
      {adjustment && (
        <p className="tier-model-adjustment" role="status">
          <Info size={15} aria-hidden="true" />
          <span>{text.adjusted(adjustment.from, adjustment.to)}</span>
        </p>
      )}
    </fieldset>
  );
}
